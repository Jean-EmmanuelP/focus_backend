package challenges

import (
	"encoding/json"
	"fmt"
	"log"
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
	r.Get("/challenges/wakeup/{id}", h.GetChallenge)
	r.Post("/challenges/wakeup/{id}/join", h.JoinChallenge)
	r.Post("/challenges/wakeup/{id}/checkin", h.CheckIn)
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

	var challengeID string
	var startDate, endDate *time.Time

	// If opponent already specified, start immediately
	if req.OpponentID != "" {
		now := time.Now()
		end := now.AddDate(0, 0, req.DurationDays)
		startDate = &now
		endDate = &end
	}

	err := h.db.QueryRow(r.Context(), `
		INSERT INTO public.wake_up_challenges (creator_id, opponent_id, alarm_time, duration_days, status, start_date, end_date)
		VALUES ($1::uuid, NULLIF($2, '')::uuid, $3, $4, CASE WHEN $2 != '' THEN 'active' ELSE 'pending' END, $5, $6)
		RETURNING id
	`, userID, req.OpponentID, req.AlarmTime, req.DurationDays, startDate, endDate).Scan(&challengeID)

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
		"status":        "pending",
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

	// Get challenge duration to calculate end date
	var durationDays int
	err := h.db.QueryRow(r.Context(), `
		SELECT duration_days FROM public.wake_up_challenges
		WHERE id = $1 AND status = 'pending' AND opponent_id IS NULL AND creator_id != $2::uuid
	`, challengeID, userID).Scan(&durationDays)
	if err != nil {
		http.Error(w, "Challenge not found or already taken", http.StatusNotFound)
		return
	}

	endDate := now.AddDate(0, 0, durationDays)

	_, err = h.db.Exec(r.Context(), `
		UPDATE public.wake_up_challenges
		SET opponent_id = $1::uuid, status = 'active', start_date = $2, end_date = $3, updated_at = now()
		WHERE id = $4 AND status = 'pending' AND opponent_id IS NULL
	`, userID, now, endDate, challengeID)

	if err != nil {
		log.Printf("Join challenge error: %v", err)
		http.Error(w, "Failed to join", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "joined", "challenge_id": challengeID})
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
		WakeUpTime string `json:"wake_up_time"` // HH:mm
		PhotoURL   string `json:"photo_url"`
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

	// Insert entry
	_, err = h.db.Exec(r.Context(), `
		INSERT INTO public.wake_up_entries (challenge_id, user_id, day_number, wake_up_time, photo_url, is_on_time)
		VALUES ($1, $2::uuid, $3, $4, $5, $6)
		ON CONFLICT (challenge_id, user_id, day_number) DO UPDATE
		SET wake_up_time = $4, photo_url = $5, is_on_time = $6
	`, challengeID, userID, daysSinceStart, req.WakeUpTime, req.PhotoURL, isOnTime)

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
		h.db.Exec(r.Context(), fmt.Sprintf(`
			UPDATE public.wake_up_challenges
			SET %s = %s + 1, %s = %s + 1, updated_at = now()
			WHERE id = $1
		`, scoreField, scoreField, streakField, streakField), challengeID)
	} else {
		// Reset streak on late/miss
		h.db.Exec(r.Context(), fmt.Sprintf(`
			UPDATE public.wake_up_challenges SET %s = 0, updated_at = now() WHERE id = $1
		`, streakField), challengeID)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"day":       daysSinceStart,
		"on_time":   isOnTime,
		"photo_url": req.PhotoURL,
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
			   COALESCE(u2.pseudo, u2.first_name, 'En attente') as opponent_name
		FROM public.wake_up_challenges c
		LEFT JOIN public.users u1 ON u1.id = c.creator_id::text
		LEFT JOIN public.users u2 ON u2.id = c.opponent_id::text
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
	}

	var challenges []ChallengeResponse
	for rows.Next() {
		var c ChallengeResponse
		var startDate *time.Time
		if err := rows.Scan(&c.ID, &c.AlarmTime, &c.DurationDays, &c.Status,
			&c.CreatorScore, &c.OpponentScore, &c.CreatorStreak, &c.OpponentStreak,
			&startDate, &c.CreatorID, &c.OpponentID, &c.CreatorName, &c.OpponentName); err != nil {
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
	var durationDays, creatorScore, opponentScore int
	var startDate *time.Time
	err := h.db.QueryRow(r.Context(), `
		SELECT c.alarm_time, c.status, c.duration_days,
			   c.creator_score, c.opponent_score,
			   c.start_date, c.creator_id, COALESCE(c.opponent_id::text, ''),
			   COALESCE(u1.pseudo, u1.first_name, '') as creator_name,
			   COALESCE(u2.pseudo, u2.first_name, '') as opponent_name
		FROM public.wake_up_challenges c
		LEFT JOIN public.users u1 ON u1.id = c.creator_id::text
		LEFT JOIN public.users u2 ON u2.id = c.opponent_id::text
		WHERE c.id = $1 AND (c.creator_id = $2 OR c.opponent_id = $2::uuid)
	`, challengeID, userID).Scan(&alarmTime, &status, &durationDays,
		&creatorScore, &opponentScore, &startDate, &creatorID, &opponentID,
		&creatorName, &opponentName)

	if err != nil {
		http.Error(w, "Challenge not found", http.StatusNotFound)
		return
	}

	// Get entries
	entryRows, _ := h.db.Query(r.Context(), `
		SELECT user_id, day_number, wake_up_time, COALESCE(photo_url, ''), is_on_time
		FROM public.wake_up_entries
		WHERE challenge_id = $1
		ORDER BY day_number, user_id
	`, challengeID)
	defer entryRows.Close()

	type Entry struct {
		UserID     string `json:"user_id"`
		DayNumber  int    `json:"day_number"`
		WakeUpTime string `json:"wake_up_time"`
		PhotoURL   string `json:"photo_url"`
		IsOnTime   bool   `json:"is_on_time"`
	}
	var entries []Entry
	for entryRows.Next() {
		var e Entry
		if err := entryRows.Scan(&e.UserID, &e.DayNumber, &e.WakeUpTime, &e.PhotoURL, &e.IsOnTime); err != nil {
			continue
		}
		entries = append(entries, e)
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
		"entries":        entries,
	})
}
