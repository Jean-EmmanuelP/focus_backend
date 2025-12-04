package reflections

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"firelevel-backend/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Reflection struct {
	ID              string `json:"id"`
	Date            string `json:"date"` // YYYY-MM-DD
	BiggestWin      string `json:"biggest_win"`
	Challenges      string `json:"challenges"`
	BestMoment      string `json:"best_moment"`
	GoalForTomorrow string `json:"goal_for_tomorrow"`
}

type UpsertReflectionRequest struct {
	BiggestWin      string `json:"biggest_win"`
	Challenges      string `json:"challenges"`
	BestMoment      string `json:"best_moment"`
	GoalForTomorrow string `json:"goal_for_tomorrow"`
}

type Handler struct {
	db *pgxpool.Pool
}

func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
}

// GetByDate - GET /reflections/{date}
func (h *Handler) GetByDate(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	dateStr := chi.URLParam(r, "date")

	// Basic validation for YYYY-MM-DD
	if _, err := time.Parse("2006-01-02", dateStr); err != nil {
		http.Error(w, "Invalid date format (YYYY-MM-DD)", http.StatusBadRequest)
		return
	}

	query := `
		SELECT id, date, biggest_win, challenges, best_moment, goal_for_tomorrow
		FROM public.daily_reflections
		WHERE user_id = $1 AND date = $2
	`

	var ref Reflection
	var date time.Time // PG returns date as time.Time
	var bw, ch, bm, gft *string // Nullable fields

	err := h.db.QueryRow(r.Context(), query, userID, dateStr).Scan(
		&ref.ID, &date, &bw, &ch, &bm, &gft,
	)

	if err != nil {
		http.Error(w, "Reflection not found", http.StatusNotFound)
		return
	}

	ref.Date = date.Format("2006-01-02")
	if bw != nil { ref.BiggestWin = *bw }
	if ch != nil { ref.Challenges = *ch }
	if bm != nil { ref.BestMoment = *bm }
	if gft != nil { ref.GoalForTomorrow = *gft }

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ref)
}

// Upsert - PUT /reflections/{date}
func (h *Handler) Upsert(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	dateStr := chi.URLParam(r, "date")

	if _, err := time.Parse("2006-01-02", dateStr); err != nil {
		http.Error(w, "Invalid date format (YYYY-MM-DD)", http.StatusBadRequest)
		return
	}

	var req UpsertReflectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	// SQL Upsert (Insert or Update on Conflict)
	query := `
		INSERT INTO public.daily_reflections (user_id, date, biggest_win, challenges, best_moment, goal_for_tomorrow)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_id, date) DO UPDATE SET
			biggest_win = EXCLUDED.biggest_win,
			challenges = EXCLUDED.challenges,
			best_moment = EXCLUDED.best_moment,
			goal_for_tomorrow = EXCLUDED.goal_for_tomorrow
		RETURNING id, date, biggest_win, challenges, best_moment, goal_for_tomorrow
	`

	var ref Reflection
	var date time.Time
	var bw, ch, bm, gft *string

	err := h.db.QueryRow(r.Context(), query, userID, dateStr, req.BiggestWin, req.Challenges, req.BestMoment, req.GoalForTomorrow).Scan(
		&ref.ID, &date, &bw, &ch, &bm, &gft,
	)

	if err != nil {
		fmt.Println("Upsert error:", err)
		http.Error(w, "Failed to save reflection", http.StatusInternalServerError)
		return
	}

	ref.Date = date.Format("2006-01-02")
	if bw != nil { ref.BiggestWin = *bw }
	if ch != nil { ref.Challenges = *ch }
	if bm != nil { ref.BestMoment = *bm }
	if gft != nil { ref.GoalForTomorrow = *gft }

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ref)
}

// List - GET /reflections?from=...&to=...
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	
	query := `
		SELECT id, date, biggest_win, challenges, best_moment, goal_for_tomorrow
		FROM public.daily_reflections
		WHERE user_id = $1
		ORDER BY date DESC
		LIMIT 30
	`

	rows, err := h.db.Query(r.Context(), query, userID)
	if err != nil {
		http.Error(w, "Failed to list reflections", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	reflections := []Reflection{}
	for rows.Next() {
		var ref Reflection
		var date time.Time
		var bw, ch, bm, gft *string

		if err := rows.Scan(&ref.ID, &date, &bw, &ch, &bm, &gft); err != nil {
			continue
		}
		ref.Date = date.Format("2006-01-02")
		if bw != nil { ref.BiggestWin = *bw }
		if ch != nil { ref.Challenges = *ch }
		if bm != nil { ref.BestMoment = *bm }
		if gft != nil { ref.GoalForTomorrow = *gft }
		
		reflections = append(reflections, ref)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reflections)
}
