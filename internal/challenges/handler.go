package challenges

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"time"

	"firelevel-backend/internal/auth"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	db *pgxpool.Pool
}

func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
}

func (h *Handler) Routes(r chi.Router) {
	r.Post("/challenges/wakeup", h.CreateChallenge)
	r.Get("/challenges/wakeup", h.ListChallenges)
	r.Post("/challenges/wakeup/join-by-code", h.JoinByInviteCode)
	r.Get("/challenges/wakeup/{id}", h.GetChallenge)
	r.Post("/challenges/wakeup/{id}/join", h.JoinChallenge)
	r.Post("/challenges/wakeup/{id}/checkin", h.CheckIn)
	r.Post("/challenges/wakeup/{id}/taunt", h.SendTaunt)
	r.Get("/challenges/wakeup/{id}/taunts", h.GetTaunts)
}

// generateInviteCode generates a random 8-char lowercase alphanumeric code
func generateInviteCode() (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	code := make([]byte, 8)
	for i := range code {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		code[i] = charset[n.Int64()]
	}
	return string(code), nil
}

// CreateChallenge creates a new wake-up challenge
func (h *Handler) CreateChallenge(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		AlarmTime    string `json:"alarm_time"`
		DurationDays int    `json:"duration_days"`
		OpponentID   string `json:"opponent_id,omitempty"`
		Title        string `json:"title,omitempty"`
		Mantra       string `json:"mantra,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.AlarmTime == "" {
		req.AlarmTime = "07:00"
	}
	if req.DurationDays <= 0 {
		req.DurationDays = 30
	}

	inviteCode, err := generateInviteCode()
	if err != nil {
		log.Printf("Generate invite code error: %v", err)
		http.Error(w, "Failed to create challenge", http.StatusInternalServerError)
		return
	}

	var challengeID string

	// Solo or with opponent: always start immediately
	now := time.Now()
	end := now.AddDate(0, 0, req.DurationDays)
	startDate := &now
	endDate := &end

	isSolo := req.OpponentID == ""

	err = h.db.QueryRow(r.Context(), `
		INSERT INTO public.wake_up_challenges (creator_id, opponent_id, alarm_time, duration_days, status, start_date, end_date, invite_code, title, mantra)
		VALUES ($1::uuid, NULLIF($2, '')::uuid, $3, $4, 'active', $5, $6, $7, $8, $9)
		RETURNING id
	`, userID, req.OpponentID, req.AlarmTime, req.DurationDays, startDate, endDate, inviteCode, req.Title, req.Mantra).Scan(&challengeID)

	if err != nil {
		log.Printf("Create challenge error: %v", err)
		http.Error(w, "Failed to create challenge", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":            challengeID,
		"alarm_time":    req.AlarmTime,
		"duration_days": req.DurationDays,
		"status":        "active",
		"invite_code":   inviteCode,
		"title":         req.Title,
		"mantra":        req.Mantra,
		"solo":          isSolo,
	})
}

// JoinChallenge allows a user to join a pending challenge
func (h *Handler) JoinChallenge(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	challengeID := chi.URLParam(r, "id")
	now := time.Now()

	// Atomically claim the challenge: UPDATE only if still pending with no opponent
	var durationDays int
	err := h.db.QueryRow(r.Context(), `
		UPDATE public.wake_up_challenges
		SET opponent_id = $1::uuid, status = 'active',
		    start_date = $2, end_date = $2 + (duration_days * interval '1 day'),
		    updated_at = now()
		WHERE id = $3 AND status = 'pending' AND opponent_id IS NULL AND creator_id != $1::uuid
		RETURNING duration_days
	`, userID, now, challengeID).Scan(&durationDays)
	if err != nil {
		http.Error(w, "Challenge not found or already taken", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "joined", "challenge_id": challengeID})
}

// JoinByInviteCode allows a user to join a challenge using an invite code
func (h *Handler) JoinByInviteCode(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		InviteCode string `json:"invite_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.InviteCode == "" {
		http.Error(w, "invite_code is required", http.StatusBadRequest)
		return
	}

	now := time.Now()

	// Atomically claim the challenge and return all needed fields in one query
	var challengeID string
	var durationDays int
	var alarmTime, title, mantra string
	err := h.db.QueryRow(r.Context(), `
		UPDATE public.wake_up_challenges
		SET opponent_id = $1::uuid, status = 'active',
		    start_date = $2, end_date = $2 + (duration_days * interval '1 day'),
		    updated_at = now()
		WHERE invite_code = $3 AND status = 'pending' AND opponent_id IS NULL AND creator_id != $1::uuid
		RETURNING id, duration_days, alarm_time, COALESCE(title, ''), COALESCE(mantra, '')
	`, userID, now, req.InviteCode).Scan(&challengeID, &durationDays, &alarmTime, &title, &mantra)
	if err != nil {
		http.Error(w, "Challenge not found, already taken, or you are the creator", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":        "joined",
		"challenge_id":  challengeID,
		"alarm_time":    alarmTime,
		"duration_days": durationDays,
		"title":         title,
		"mantra":        mantra,
	})
}

// CheckIn records a wake-up entry with optional photo
func (h *Handler) CheckIn(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	challengeID := chi.URLParam(r, "id")

	var req struct {
		WakeUpTime      string `json:"wake_up_time"` // HH:mm
		PhotoURL        string `json:"photo_url"`
		MantraValidated *bool  `json:"mantra_validated,omitempty"`
		ExercisesDone   *bool  `json:"exercises_done,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Get challenge details
	var alarmTime string
	var startDate *time.Time
	var creatorID, opponentID string
	err := h.db.QueryRow(r.Context(), `
		SELECT alarm_time, start_date, creator_id, COALESCE(opponent_id::text, '')
		FROM public.wake_up_challenges
		WHERE id = $1 AND status = 'active' AND (creator_id = $2 OR opponent_id = $2::uuid)
	`, challengeID, userID).Scan(&alarmTime, &startDate, &creatorID, &opponentID)

	if err != nil {
		http.Error(w, "Challenge not found or not active", http.StatusNotFound)
		return
	}

	// Calculate day number
	if startDate == nil {
		http.Error(w, "Challenge not started", http.StatusBadRequest)
		return
	}
	daysSinceStart := int(time.Since(*startDate).Hours()/24) + 1

	// Determine if on time (within 15 min grace)
	isOnTime := false
	if req.WakeUpTime != "" {
		// Parse times to compare
		var aH, aM, wH, wM int
		fmt.Sscanf(alarmTime, "%d:%d", &aH, &aM)
		fmt.Sscanf(req.WakeUpTime, "%d:%d", &wH, &wM)
		diff := (wH*60 + wM) - (aH*60 + aM)
		isOnTime = diff >= -5 && diff <= 15
	}

	// Default booleans
	mantraValidated := false
	if req.MantraValidated != nil {
		mantraValidated = *req.MantraValidated
	}
	exercisesDone := false
	if req.ExercisesDone != nil {
		exercisesDone = *req.ExercisesDone
	}

	// Insert entry and update scores in a transaction
	tx, err := h.db.Begin(r.Context())
	if err != nil {
		log.Printf("Check-in begin tx error: %v", err)
		http.Error(w, "Failed to check in", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	_, err = tx.Exec(r.Context(), `
		INSERT INTO public.wake_up_entries (challenge_id, user_id, day_number, wake_up_time, photo_url, is_on_time, mantra_validated, exercises_done)
		VALUES ($1, $2::uuid, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (challenge_id, user_id, day_number) DO UPDATE
		SET wake_up_time = $4, photo_url = $5, is_on_time = $6, mantra_validated = $7, exercises_done = $8
	`, challengeID, userID, daysSinceStart, req.WakeUpTime, req.PhotoURL, isOnTime, mantraValidated, exercisesDone)

	if err != nil {
		log.Printf("Check-in error: %v", err)
		http.Error(w, "Failed to check in", http.StatusInternalServerError)
		return
	}

	// Update scores
	scoreField := "creator_score"
	streakField := "creator_streak"
	if userID == opponentID {
		scoreField = "opponent_score"
		streakField = "opponent_streak"
	}

	if isOnTime {
		_, err = tx.Exec(r.Context(), fmt.Sprintf(`
			UPDATE public.wake_up_challenges
			SET %s = %s + 1, %s = %s + 1, updated_at = now()
			WHERE id = $1
		`, scoreField, scoreField, streakField, streakField), challengeID)
	} else {
		// Reset streak on late/miss
		_, err = tx.Exec(r.Context(), fmt.Sprintf(`
			UPDATE public.wake_up_challenges SET %s = 0, updated_at = now() WHERE id = $1
		`, streakField), challengeID)
	}

	if err != nil {
		log.Printf("Check-in score update error: %v", err)
		http.Error(w, "Failed to check in", http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		log.Printf("Check-in commit error: %v", err)
		http.Error(w, "Failed to check in", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"day":               daysSinceStart,
		"on_time":           isOnTime,
		"photo_url":         req.PhotoURL,
		"mantra_validated":  mantraValidated,
		"exercises_done":    exercisesDone,
	})
}

// ListChallenges returns all challenges for the current user
func (h *Handler) ListChallenges(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	rows, err := h.db.Query(r.Context(), `
		SELECT c.id, c.alarm_time, c.duration_days, c.status,
			   c.creator_score, c.opponent_score, c.creator_streak, c.opponent_streak,
			   c.start_date, c.creator_id, COALESCE(c.opponent_id::text, ''),
			   COALESCE(u1.pseudo, u1.first_name, 'Joueur 1') as creator_name,
			   COALESCE(u2.pseudo, u2.first_name, 'En attente') as opponent_name,
			   COALESCE(c.invite_code, ''), COALESCE(c.title, ''), COALESCE(c.mantra, '')
		FROM public.wake_up_challenges c
		LEFT JOIN public.users u1 ON u1.id::text = c.creator_id::text
		LEFT JOIN public.users u2 ON u2.id::text = c.opponent_id::text
		WHERE c.creator_id = $1 OR c.opponent_id = $1::uuid
		ORDER BY c.created_at DESC
		LIMIT 10
	`, userID)
	if err != nil {
		http.Error(w, "Failed to fetch challenges", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type ChallengeResponse struct {
		ID             string  `json:"id"`
		AlarmTime      string  `json:"alarm_time"`
		DurationDays   int     `json:"duration_days"`
		Status         string  `json:"status"`
		CreatorScore   int     `json:"creator_score"`
		OpponentScore  int     `json:"opponent_score"`
		CreatorStreak  int     `json:"creator_streak"`
		OpponentStreak int     `json:"opponent_streak"`
		StartDate      *string `json:"start_date"`
		CreatorID      string  `json:"creator_id"`
		OpponentID     string  `json:"opponent_id"`
		CreatorName    string  `json:"creator_name"`
		OpponentName   string  `json:"opponent_name"`
		InviteCode     string  `json:"invite_code"`
		Title          string  `json:"title"`
		Mantra         string  `json:"mantra"`
	}

	var challenges []ChallengeResponse
	for rows.Next() {
		var c ChallengeResponse
		var startDate *time.Time
		if err := rows.Scan(&c.ID, &c.AlarmTime, &c.DurationDays, &c.Status,
			&c.CreatorScore, &c.OpponentScore, &c.CreatorStreak, &c.OpponentStreak,
			&startDate, &c.CreatorID, &c.OpponentID, &c.CreatorName, &c.OpponentName,
			&c.InviteCode, &c.Title, &c.Mantra); err != nil {
			continue
		}
		if startDate != nil {
			s := startDate.Format("2006-01-02")
			c.StartDate = &s
		}
		challenges = append(challenges, c)
	}

	if challenges == nil {
		challenges = []ChallengeResponse{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(challenges)
}

// GetChallenge returns a single challenge with entries
func (h *Handler) GetChallenge(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	challengeID := chi.URLParam(r, "id")

	// Get challenge
	var alarmTime, status, creatorID, opponentID, creatorName, opponentName string
	var inviteCode, title, mantra string
	var durationDays, creatorScore, opponentScore int
	var startDate *time.Time
	err := h.db.QueryRow(r.Context(), `
		SELECT c.alarm_time, c.status, c.duration_days,
			   c.creator_score, c.opponent_score,
			   c.start_date, c.creator_id, COALESCE(c.opponent_id::text, ''),
			   COALESCE(u1.pseudo, u1.first_name, '') as creator_name,
			   COALESCE(u2.pseudo, u2.first_name, '') as opponent_name,
			   COALESCE(c.invite_code, ''), COALESCE(c.title, ''), COALESCE(c.mantra, '')
		FROM public.wake_up_challenges c
		LEFT JOIN public.users u1 ON u1.id::text = c.creator_id::text
		LEFT JOIN public.users u2 ON u2.id::text = c.opponent_id::text
		WHERE c.id = $1 AND (c.creator_id = $2 OR c.opponent_id = $2::uuid)
	`, challengeID, userID).Scan(&alarmTime, &status, &durationDays,
		&creatorScore, &opponentScore, &startDate, &creatorID, &opponentID,
		&creatorName, &opponentName, &inviteCode, &title, &mantra)

	if err != nil {
		http.Error(w, "Challenge not found", http.StatusNotFound)
		return
	}

	// Get entries with new fields
	type Entry struct {
		UserID          string `json:"user_id"`
		DayNumber       int    `json:"day_number"`
		WakeUpTime      string `json:"wake_up_time"`
		PhotoURL        string `json:"photo_url"`
		IsOnTime        bool   `json:"is_on_time"`
		MantraValidated bool   `json:"mantra_validated"`
		ExercisesDone   bool   `json:"exercises_done"`
	}
	var entries []Entry

	entryRows, entryErr := h.db.Query(r.Context(), `
		SELECT user_id, day_number, wake_up_time, COALESCE(photo_url, ''), is_on_time,
			   COALESCE(mantra_validated, false), COALESCE(exercises_done, false)
		FROM public.wake_up_entries
		WHERE challenge_id = $1
		ORDER BY day_number, user_id
	`, challengeID)
	if entryErr != nil {
		log.Printf("GetChallenge entries query error: %v", entryErr)
	} else {
		defer entryRows.Close()
		for entryRows.Next() {
			var e Entry
			if err := entryRows.Scan(&e.UserID, &e.DayNumber, &e.WakeUpTime, &e.PhotoURL, &e.IsOnTime,
				&e.MantraValidated, &e.ExercisesDone); err != nil {
				continue
			}
			entries = append(entries, e)
		}
	}

	var startStr *string
	if startDate != nil {
		s := startDate.Format("2006-01-02")
		startStr = &s
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":             challengeID,
		"alarm_time":     alarmTime,
		"status":         status,
		"duration_days":  durationDays,
		"creator_id":     creatorID,
		"opponent_id":    opponentID,
		"creator_name":   creatorName,
		"opponent_name":  opponentName,
		"creator_score":  creatorScore,
		"opponent_score": opponentScore,
		"start_date":     startStr,
		"invite_code":    inviteCode,
		"title":          title,
		"mantra":         mantra,
		"entries":        entries,
	})
}

// SendTaunt sends a taunt message to the challenge partner
func (h *Handler) SendTaunt(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	challengeID := chi.URLParam(r, "id")

	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.Message == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}

	// Verify user is part of the challenge
	var exists bool
	err := h.db.QueryRow(r.Context(), `
		SELECT EXISTS(
			SELECT 1 FROM public.wake_up_challenges
			WHERE id = $1 AND (creator_id = $2::uuid OR opponent_id = $2::uuid)
		)
	`, challengeID, userID).Scan(&exists)
	if err != nil || !exists {
		http.Error(w, "Challenge not found or you are not a participant", http.StatusNotFound)
		return
	}

	// Store the taunt
	var tauntID string
	err = h.db.QueryRow(r.Context(), `
		INSERT INTO public.challenge_taunts (challenge_id, sender_id, message)
		VALUES ($1::uuid, $2::uuid, $3)
		RETURNING id
	`, challengeID, userID, req.Message).Scan(&tauntID)

	if err != nil {
		log.Printf("Send taunt error: %v", err)
		http.Error(w, "Failed to send taunt", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":           tauntID,
		"challenge_id": challengeID,
		"message":      req.Message,
		"status":       "sent",
	})
}

// GetTaunts returns taunts for a challenge
func (h *Handler) GetTaunts(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	challengeID := chi.URLParam(r, "id")

	// Verify user is part of the challenge
	var exists bool
	err := h.db.QueryRow(r.Context(), `
		SELECT EXISTS(
			SELECT 1 FROM public.wake_up_challenges
			WHERE id = $1 AND (creator_id = $2::uuid OR opponent_id = $2::uuid)
		)
	`, challengeID, userID).Scan(&exists)
	if err != nil || !exists {
		http.Error(w, "Challenge not found or you are not a participant", http.StatusNotFound)
		return
	}

	rows, err := h.db.Query(r.Context(), `
		SELECT t.id, t.sender_id, t.message, t.created_at,
			   COALESCE(u.pseudo, u.first_name, '') as sender_name
		FROM public.challenge_taunts t
		LEFT JOIN public.users u ON u.id = t.sender_id::text
		WHERE t.challenge_id = $1::uuid
		ORDER BY t.created_at DESC
		LIMIT 50
	`, challengeID)
	if err != nil {
		http.Error(w, "Failed to fetch taunts", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type TauntResponse struct {
		ID         string `json:"id"`
		SenderID   string `json:"sender_id"`
		SenderName string `json:"sender_name"`
		Message    string `json:"message"`
		CreatedAt  string `json:"created_at"`
	}

	var taunts []TauntResponse
	for rows.Next() {
		var t TauntResponse
		var createdAt time.Time
		if err := rows.Scan(&t.ID, &t.SenderID, &t.Message, &createdAt, &t.SenderName); err != nil {
			continue
		}
		t.CreatedAt = createdAt.Format(time.RFC3339)
		taunts = append(taunts, t)
	}

	if taunts == nil {
		taunts = []TauntResponse{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(taunts)
}
