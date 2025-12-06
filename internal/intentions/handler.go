package intentions

import (
	"encoding/json"
	"fmt"
	"net/http"

	"firelevel-backend/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DailyIntention struct {
	ID           string  `json:"id"`
	Date         string  `json:"date"`
	Mood         string  `json:"mood"`
	SleepQuality string  `json:"sleep_quality"`
	Intention1   string  `json:"intention_1"`
	Intention2   *string `json:"intention_2"`
	Intention3   *string `json:"intention_3"`
}

type UpsertIntentionRequest struct {
	Mood         string  `json:"mood"`
	SleepQuality string  `json:"sleep_quality"`
	Intention1   string  `json:"intention_1"`
	Intention2   *string `json:"intention_2"`
	Intention3   *string `json:"intention_3"`
}

type Handler struct {
	db *pgxpool.Pool
}

func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
}

// GetByDate - GET /intentions/{date}
// Get intention for a specific date (YYYY-MM-DD)
func (h *Handler) GetByDate(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	date := chi.URLParam(r, "date")

	query := `
		SELECT id, date, mood, sleep_quality, intention_1, intention_2, intention_3
		FROM public.daily_intentions
		WHERE user_id = $1 AND date = $2
	`

	var i DailyIntention
	err := h.db.QueryRow(r.Context(), query, userID, date).Scan(
		&i.ID, &i.Date, &i.Mood, &i.SleepQuality, &i.Intention1, &i.Intention2, &i.Intention3,
	)

	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(i)
}

// GetToday - GET /intentions/today
// Shortcut to get today's intention
func (h *Handler) GetToday(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	query := `
		SELECT id, date, mood, sleep_quality, intention_1, intention_2, intention_3
		FROM public.daily_intentions
		WHERE user_id = $1 AND date = CURRENT_DATE
	`

	var i DailyIntention
	err := h.db.QueryRow(r.Context(), query, userID).Scan(
		&i.ID, &i.Date, &i.Mood, &i.SleepQuality, &i.Intention1, &i.Intention2, &i.Intention3,
	)

	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(i)
}

// Upsert - PUT /intentions/{date}
// Create or update intention for a specific date
func (h *Handler) Upsert(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	date := chi.URLParam(r, "date")

	var req UpsertIntentionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	if req.Mood == "" || req.SleepQuality == "" || req.Intention1 == "" {
		http.Error(w, "mood, sleep_quality and intention_1 are required", http.StatusBadRequest)
		return
	}

	query := `
		INSERT INTO public.daily_intentions (user_id, date, mood, sleep_quality, intention_1, intention_2, intention_3)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (user_id, date) DO UPDATE SET
			mood = EXCLUDED.mood,
			sleep_quality = EXCLUDED.sleep_quality,
			intention_1 = EXCLUDED.intention_1,
			intention_2 = EXCLUDED.intention_2,
			intention_3 = EXCLUDED.intention_3
		RETURNING id, date, mood, sleep_quality, intention_1, intention_2, intention_3
	`

	var i DailyIntention
	err := h.db.QueryRow(r.Context(), query, userID, date, req.Mood, req.SleepQuality, req.Intention1, req.Intention2, req.Intention3).Scan(
		&i.ID, &i.Date, &i.Mood, &i.SleepQuality, &i.Intention1, &i.Intention2, &i.Intention3,
	)

	if err != nil {
		fmt.Println("Upsert intention error:", err)
		http.Error(w, "Failed to save intention", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(i)
}

// List - GET /intentions
// Get recent intentions (last 7 days by default)
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	query := `
		SELECT id, date, mood, sleep_quality, intention_1, intention_2, intention_3
		FROM public.daily_intentions
		WHERE user_id = $1
		ORDER BY date DESC
		LIMIT 7
	`

	rows, err := h.db.Query(r.Context(), query, userID)
	if err != nil {
		http.Error(w, "Failed to list intentions", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	intentions := []DailyIntention{}
	for rows.Next() {
		var i DailyIntention
		if err := rows.Scan(&i.ID, &i.Date, &i.Mood, &i.SleepQuality, &i.Intention1, &i.Intention2, &i.Intention3); err != nil {
			continue
		}
		intentions = append(intentions, i)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(intentions)
}
