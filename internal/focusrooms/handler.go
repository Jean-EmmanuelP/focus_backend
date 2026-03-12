package focusrooms

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"firelevel-backend/internal/auth"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	lkauth "github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
)

// ===========================================
// FOCUS ROOMS — Group audio/video sessions
// Matchmaking by category, max 6 participants,
// LiveKit room creation + token generation.
// ===========================================

type Handler struct {
	db *pgxpool.Pool
}

func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
}

// --- Models ---

type FocusRoom struct {
	ID              string            `json:"id"`
	Category        string            `json:"category"`
	LivekitRoomName string            `json:"livekit_room_name"`
	MaxParticipants int               `json:"max_participants"`
	CreatedAt       time.Time         `json:"created_at"`
	Participants    []RoomParticipant `json:"participants"`
}

type RoomParticipant struct {
	ID        string    `json:"id"`
	Pseudo    *string   `json:"pseudo"`
	FirstName *string   `json:"first_name"`
	AvatarURL *string   `json:"avatar_url"`
	JoinedAt  time.Time `json:"joined_at"`
}

// --- Requests ---

type joinRequest struct {
	Category string `json:"category"`
}

type joinResponse struct {
	Room  FocusRoom `json:"room"`
	Token string    `json:"token"`
	URL   string    `json:"url"`
}

// Join — POST /focus-rooms/join
// Matchmaking: find an active room with < 6 participants in the given category,
// or create a new one. Returns the room + LiveKit token.
func (h *Handler) Join(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	var req joinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}

	validCategories := map[string]bool{
		"sport": true, "travail": true, "etudes": true,
		"creativite": true, "lecture": true, "meditation": true,
	}
	if !validCategories[req.Category] {
		http.Error(w, `{"error":"invalid category"}`, http.StatusBadRequest)
		return
	}

	lkAPIKey := os.Getenv("LIVEKIT_API_KEY")
	lkAPISecret := os.Getenv("LIVEKIT_API_SECRET")
	lkURL := os.Getenv("LIVEKIT_URL")
	if lkAPIKey == "" || lkAPISecret == "" || lkURL == "" {
		http.Error(w, `{"error":"LiveKit not configured"}`, http.StatusInternalServerError)
		return
	}

	ctx := r.Context()

	// 1. Try to find an existing active room with < max participants
	var roomID, roomName string
	var maxParticipants int
	var createdAt time.Time

	err := h.db.QueryRow(ctx, `
		SELECT r.id, r.livekit_room_name, r.max_participants, r.created_at
		FROM public.focus_rooms r
		WHERE r.category = $1
		  AND r.status = 'active'
		  AND (SELECT count(*) FROM public.focus_room_participants p WHERE p.room_id = r.id AND p.left_at IS NULL) < r.max_participants
		ORDER BY r.created_at DESC
		LIMIT 1
	`, req.Category).Scan(&roomID, &roomName, &maxParticipants, &createdAt)

	if err != nil {
		// No room found — create a new one
		roomName = fmt.Sprintf("focus-room-%s-%s", req.Category, uuid.New().String()[:8])
		maxParticipants = 6

		// Create LiveKit room
		roomClient := lksdk.NewRoomServiceClient(lkURL, lkAPIKey, lkAPISecret)
		_, err := roomClient.CreateRoom(context.Background(), &livekit.CreateRoomRequest{
			Name:            roomName,
			EmptyTimeout:    300, // 5 min empty timeout
			MaxParticipants: uint32(maxParticipants),
		})
		if err != nil {
			log.Printf("Failed to create LiveKit room: %v", err)
			http.Error(w, `{"error":"failed to create room"}`, http.StatusInternalServerError)
			return
		}

		// Insert into DB
		err = h.db.QueryRow(ctx, `
			INSERT INTO public.focus_rooms (category, livekit_room_name, max_participants)
			VALUES ($1, $2, $3)
			RETURNING id, created_at
		`, req.Category, roomName, maxParticipants).Scan(&roomID, &createdAt)
		if err != nil {
			log.Printf("Failed to insert focus room: %v", err)
			http.Error(w, `{"error":"failed to create room"}`, http.StatusInternalServerError)
			return
		}
	}

	// 2. Add participant (upsert in case they rejoin)
	_, err = h.db.Exec(ctx, `
		INSERT INTO public.focus_room_participants (room_id, user_id)
		VALUES ($1, $2)
		ON CONFLICT (room_id, user_id) DO UPDATE SET left_at = NULL, joined_at = now()
	`, roomID, userID)
	if err != nil {
		log.Printf("Failed to add participant: %v", err)
		http.Error(w, `{"error":"failed to join room"}`, http.StatusInternalServerError)
		return
	}

	// 3. Generate LiveKit token for this user
	at := lkauth.NewAccessToken(lkAPIKey, lkAPISecret)
	grant := &lkauth.VideoGrant{
		RoomJoin: true,
		Room:     roomName,
	}
	at.AddGrant(grant).
		SetIdentity(userID).
		SetName(userID).
		SetValidFor(1 * time.Hour)

	token, err := at.ToJWT()
	if err != nil {
		http.Error(w, `{"error":"failed to generate token"}`, http.StatusInternalServerError)
		return
	}

	// 4. Fetch current participants for response
	participants := h.getParticipants(ctx, roomID)

	room := FocusRoom{
		ID:              roomID,
		Category:        req.Category,
		LivekitRoomName: roomName,
		MaxParticipants: maxParticipants,
		CreatedAt:       createdAt,
		Participants:    participants,
	}

	resp := joinResponse{
		Room:  room,
		Token: token,
		URL:   lkURL,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// Leave — POST /focus-rooms/{id}/leave
func (h *Handler) Leave(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	roomID := chi.URLParam(r, "id")

	ctx := r.Context()

	// Mark participant as left
	_, err := h.db.Exec(ctx, `
		UPDATE public.focus_room_participants
		SET left_at = now()
		WHERE room_id = $1 AND user_id = $2 AND left_at IS NULL
	`, roomID, userID)
	if err != nil {
		log.Printf("Failed to leave room: %v", err)
		http.Error(w, `{"error":"failed to leave room"}`, http.StatusInternalServerError)
		return
	}

	// Check if room is now empty — close it
	var activeCount int
	err = h.db.QueryRow(ctx, `
		SELECT count(*) FROM public.focus_room_participants
		WHERE room_id = $1 AND left_at IS NULL
	`, roomID).Scan(&activeCount)

	if err == nil && activeCount == 0 {
		_, _ = h.db.Exec(ctx, `
			UPDATE public.focus_rooms SET status = 'closed', closed_at = now()
			WHERE id = $1
		`, roomID)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}

// List — GET /focus-rooms?category=X
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	ctx := r.Context()

	query := `
		SELECT r.id, r.category, r.livekit_room_name, r.max_participants, r.created_at
		FROM public.focus_rooms r
		WHERE r.status = 'active'
	`
	args := []interface{}{}
	argIdx := 1

	if category != "" {
		query += fmt.Sprintf(" AND r.category = $%d", argIdx)
		args = append(args, category)
		argIdx++
	}

	query += " ORDER BY r.created_at DESC LIMIT 50"

	rows, err := h.db.Query(ctx, query, args...)
	if err != nil {
		log.Printf("List rooms error: %v", err)
		http.Error(w, `{"error":"failed to list rooms"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	rooms := []FocusRoom{}
	for rows.Next() {
		var room FocusRoom
		if err := rows.Scan(&room.ID, &room.Category, &room.LivekitRoomName, &room.MaxParticipants, &room.CreatedAt); err != nil {
			continue
		}
		room.Participants = h.getParticipants(ctx, room.ID)
		rooms = append(rooms, room)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rooms)
}

// Get — GET /focus-rooms/{id}
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	roomID := chi.URLParam(r, "id")
	ctx := r.Context()

	var room FocusRoom
	err := h.db.QueryRow(ctx, `
		SELECT id, category, livekit_room_name, max_participants, created_at
		FROM public.focus_rooms WHERE id = $1
	`, roomID).Scan(&room.ID, &room.Category, &room.LivekitRoomName, &room.MaxParticipants, &room.CreatedAt)

	if err != nil {
		http.Error(w, `{"error":"room not found"}`, http.StatusNotFound)
		return
	}

	room.Participants = h.getParticipants(ctx, room.ID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(room)
}

// --- Helpers ---

func (h *Handler) getParticipants(ctx context.Context, roomID string) []RoomParticipant {
	rows, err := h.db.Query(ctx, `
		SELECT u.id, u.pseudo, u.first_name, u.avatar_url, p.joined_at
		FROM public.focus_room_participants p
		JOIN public.users u ON u.id = p.user_id
		WHERE p.room_id = $1 AND p.left_at IS NULL
		ORDER BY p.joined_at ASC
	`, roomID)
	if err != nil {
		return []RoomParticipant{}
	}
	defer rows.Close()

	participants := []RoomParticipant{}
	for rows.Next() {
		var p RoomParticipant
		if err := rows.Scan(&p.ID, &p.Pseudo, &p.FirstName, &p.AvatarURL, &p.JoinedAt); err != nil {
			continue
		}
		participants = append(participants, p)
	}
	return participants
}
