package focus

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"firelevel-backend/internal/auth"
	ws "firelevel-backend/internal/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type FocusSession struct {
	ID              string     `json:"id"`
	QuestID         *string    `json:"quest_id"`
	Description     *string    `json:"description"`
	DurationMinutes int        `json:"duration_minutes"`
	Status          string     `json:"status"`
	StartedAt       time.Time  `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at"`
}

type StartSessionRequest struct {
	QuestID         *string `json:"quest_id"`
	Description     *string `json:"description"`
	DurationMinutes int     `json:"duration_minutes"`
	Status          *string `json:"status"` // Optional: "active" or "completed", defaults to "active"
}

type UpdateSessionRequest struct {
	Status      *string `json:"status"`
	Description *string `json:"description"`
}

type Handler struct {
	db *pgxpool.Pool
}

func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
}

// Start - POST /focus-sessions
func (h *Handler) Start(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	var req StartSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	if req.DurationMinutes <= 0 {
		http.Error(w, "Duration must be positive", http.StatusBadRequest)
		return
	}

	// Default status to "active" if not provided
	status := "active"
	if req.Status != nil && (*req.Status == "active" || *req.Status == "completed") {
		status = *req.Status
	}

	// If status is completed, also set completed_at
	var query string
	var s FocusSession
	var err error

	if status == "completed" {
		query = `
			INSERT INTO public.focus_sessions (user_id, quest_id, description, duration_minutes, status, started_at, completed_at)
			VALUES ($1, $2, $3, $4, $5, now(), now())
			RETURNING id, quest_id, description, duration_minutes, status, started_at, completed_at
		`
		err = h.db.QueryRow(r.Context(), query, userID, req.QuestID, req.Description, req.DurationMinutes, status).Scan(
			&s.ID, &s.QuestID, &s.Description, &s.DurationMinutes, &s.Status, &s.StartedAt, &s.CompletedAt,
		)
	} else {
		query = `
			INSERT INTO public.focus_sessions (user_id, quest_id, description, duration_minutes, status, started_at)
			VALUES ($1, $2, $3, $4, $5, now())
			RETURNING id, quest_id, description, duration_minutes, status, started_at, completed_at
		`
		err = h.db.QueryRow(r.Context(), query, userID, req.QuestID, req.Description, req.DurationMinutes, status).Scan(
			&s.ID, &s.QuestID, &s.Description, &s.DurationMinutes, &s.Status, &s.StartedAt, &s.CompletedAt,
		)
	}

	if err != nil {
		fmt.Println("Start session error:", err)
		http.Error(w, "Failed to start session", http.StatusInternalServerError)
		return
	}

	// Broadcast focus started to WebSocket clients (only for active sessions)
	if status == "active" {
		go h.broadcastFocusUpdate(r.Context(), userID, true, &s.StartedAt, &req.DurationMinutes)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s)
}

// Update - PATCH /focus-sessions/{id}
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	sessionID := chi.URLParam(r, "id")

	var req UpdateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	setParts := []string{}
	args := []interface{}{}
	argId := 1

	if req.Description != nil {
		setParts = append(setParts, fmt.Sprintf("description = $%d", argId))
		args = append(args, *req.Description)
		argId++
	}

	if req.Status != nil {
		status := *req.Status
		setParts = append(setParts, fmt.Sprintf("status = $%d", argId))
		args = append(args, status)
		argId++

		// Automatically set completed_at if status is 'completed'
		if status == "completed" {
			setParts = append(setParts, "completed_at = now()")
		}
	}

	if len(setParts) == 0 {
		http.Error(w, "No fields to update", http.StatusBadRequest)
		return
	}

	args = append(args, userID, sessionID)
	query := fmt.Sprintf(
		"UPDATE public.focus_sessions SET %s WHERE user_id = $%d AND id = $%d RETURNING id, quest_id, description, duration_minutes, status, started_at, completed_at",
		strings.Join(setParts, ", "),
		argId,
		argId+1,
	)

	var s FocusSession
	err := h.db.QueryRow(r.Context(), query, args...).Scan(
		&s.ID, &s.QuestID, &s.Description, &s.DurationMinutes, &s.Status, &s.StartedAt, &s.CompletedAt,
	)

	if err != nil {
		http.Error(w, "Failed to update session", http.StatusInternalServerError)
		return
	}

	// Broadcast focus stopped if session was completed or cancelled
	if req.Status != nil && (*req.Status == "completed" || *req.Status == "cancelled") {
		go h.broadcastFocusUpdate(r.Context(), userID, false, nil, nil)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s)
}

// List - GET /focus-sessions
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	questID := r.URL.Query().Get("quest_id")
	status := r.URL.Query().Get("status")

	query := `SELECT id, quest_id, description, duration_minutes, status, started_at, completed_at FROM public.focus_sessions WHERE user_id = $1`
	args := []interface{}{userID}
	argId := 2

	if questID != "" {
		query += fmt.Sprintf(" AND quest_id = $%d", argId)
		args = append(args, questID)
		argId++
	}

	if status != "" {
		query += fmt.Sprintf(" AND status = $%d", argId)
		args = append(args, status)
		argId++
	}

	query += " ORDER BY started_at DESC LIMIT 20"

	rows, err := h.db.Query(r.Context(), query, args...)
	if err != nil {
		fmt.Println("List sessions error:", err)
		http.Error(w, "Failed to list sessions", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	sessions := []FocusSession{}
	for rows.Next() {
		var s FocusSession
		if err := rows.Scan(&s.ID, &s.QuestID, &s.Description, &s.DurationMinutes, &s.Status, &s.StartedAt, &s.CompletedAt); err != nil {
			fmt.Println("Scan session error:", err)
			continue
		}
		sessions = append(sessions, s)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessions)
}

// Delete - DELETE /focus-sessions/{id}
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	sessionID := chi.URLParam(r, "id")

	query := `DELETE FROM public.focus_sessions WHERE user_id = $1 AND id = $2`
	if _, err := h.db.Exec(r.Context(), query, userID, sessionID); err != nil {
		http.Error(w, "Failed to delete session", http.StatusInternalServerError)
		return
	}

	// Broadcast focus stopped when session is deleted
	go h.broadcastFocusUpdate(r.Context(), userID, false, nil, nil)

	w.WriteHeader(http.StatusOK)
}

// broadcastFocusUpdate sends a WebSocket message to all clients about a focus status change
func (h *Handler) broadcastFocusUpdate(ctx context.Context, userID string, isLive bool, startedAt *time.Time, durationMins *int) {
	if ws.GlobalHub == nil {
		return
	}

	// Fetch user info for the broadcast
	var pseudo *string
	var avatarURL *string
	query := `SELECT pseudo, avatar_url FROM users WHERE id = $1`
	_ = h.db.QueryRow(ctx, query, userID).Scan(&pseudo, &avatarURL)

	pseudoStr := ""
	if pseudo != nil {
		pseudoStr = *pseudo
	}

	update := ws.FocusUpdate{
		UserID:    userID,
		Pseudo:    pseudoStr,
		AvatarURL: avatarURL,
		IsLive:    isLive,
	}

	if startedAt != nil {
		startedStr := startedAt.Format(time.RFC3339)
		update.StartedAt = &startedStr
	}
	if durationMins != nil {
		update.DurationMins = durationMins
	}

	ws.GlobalHub.BroadcastFocusUpdate(update)
}
