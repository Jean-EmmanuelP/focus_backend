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
	ID           string      `json:"id"`
	Date         string      `json:"date"`
	MoodRating   int         `json:"mood_rating"`
	MoodEmoji    string      `json:"mood_emoji"`
	SleepRating  int         `json:"sleep_rating"`
	SleepEmoji   string      `json:"sleep_emoji"`
	Intentions   []Intention `json:"intentions"`
}

type Intention struct {
	ID       string  `json:"id"`
	AreaID   *string `json:"area_id"`
	Content  string  `json:"content"`
	Position int     `json:"position"`
}

type IntentionInput struct {
	AreaID  *string `json:"area_id"`
	Content string  `json:"content"`
}

type UpsertIntentionRequest struct {
	MoodRating  int              `json:"mood_rating"`
	MoodEmoji   string           `json:"mood_emoji"`
	SleepRating int              `json:"sleep_rating"`
	SleepEmoji  string           `json:"sleep_emoji"`
	Intentions  []IntentionInput `json:"intentions"`
}

type Handler struct {
	db *pgxpool.Pool
}

func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
}

// GetByDate - GET /intentions/{date}
func (h *Handler) GetByDate(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	date := chi.URLParam(r, "date")

	// Get the daily intention
	query := `
		SELECT id, date, mood_rating, mood_emoji, sleep_rating, sleep_emoji
		FROM public.daily_intentions
		WHERE user_id = $1 AND date = $2
	`

	var di DailyIntention
	err := h.db.QueryRow(r.Context(), query, userID, date).Scan(
		&di.ID, &di.Date, &di.MoodRating, &di.MoodEmoji, &di.SleepRating, &di.SleepEmoji,
	)

	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// Get the intentions for this day
	intentionsQuery := `
		SELECT id, area_id, content, position
		FROM public.intention_items
		WHERE daily_intention_id = $1
		ORDER BY position ASC
	`

	rows, err := h.db.Query(r.Context(), intentionsQuery, di.ID)
	if err != nil {
		http.Error(w, "Failed to load intentions", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	di.Intentions = []Intention{}
	for rows.Next() {
		var i Intention
		if err := rows.Scan(&i.ID, &i.AreaID, &i.Content, &i.Position); err != nil {
			continue
		}
		di.Intentions = append(di.Intentions, i)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(di)
}

// GetToday - GET /intentions/today
func (h *Handler) GetToday(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	query := `
		SELECT id, date, mood_rating, mood_emoji, sleep_rating, sleep_emoji
		FROM public.daily_intentions
		WHERE user_id = $1 AND date = CURRENT_DATE
	`

	var di DailyIntention
	err := h.db.QueryRow(r.Context(), query, userID).Scan(
		&di.ID, &di.Date, &di.MoodRating, &di.MoodEmoji, &di.SleepRating, &di.SleepEmoji,
	)

	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// Get the intentions
	intentionsQuery := `
		SELECT id, area_id, content, position
		FROM public.intention_items
		WHERE daily_intention_id = $1
		ORDER BY position ASC
	`

	rows, err := h.db.Query(r.Context(), intentionsQuery, di.ID)
	if err != nil {
		http.Error(w, "Failed to load intentions", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	di.Intentions = []Intention{}
	for rows.Next() {
		var i Intention
		if err := rows.Scan(&i.ID, &i.AreaID, &i.Content, &i.Position); err != nil {
			continue
		}
		di.Intentions = append(di.Intentions, i)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(di)
}

// Upsert - PUT /intentions/{date}
func (h *Handler) Upsert(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	date := chi.URLParam(r, "date")

	var req UpsertIntentionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	if req.MoodRating < 1 || req.MoodRating > 5 {
		http.Error(w, "mood_rating must be between 1 and 5", http.StatusBadRequest)
		return
	}

	if req.SleepRating < 1 || req.SleepRating > 5 {
		http.Error(w, "sleep_rating must be between 1 and 5", http.StatusBadRequest)
		return
	}

	if len(req.Intentions) == 0 {
		http.Error(w, "at least one intention is required", http.StatusBadRequest)
		return
	}

	// Start transaction
	tx, err := h.db.Begin(r.Context())
	if err != nil {
		http.Error(w, "Failed to start transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	// Upsert daily_intentions
	upsertQuery := `
		INSERT INTO public.daily_intentions (user_id, date, mood_rating, mood_emoji, sleep_rating, sleep_emoji)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_id, date) DO UPDATE SET
			mood_rating = EXCLUDED.mood_rating,
			mood_emoji = EXCLUDED.mood_emoji,
			sleep_rating = EXCLUDED.sleep_rating,
			sleep_emoji = EXCLUDED.sleep_emoji
		RETURNING id, date, mood_rating, mood_emoji, sleep_rating, sleep_emoji
	`

	var di DailyIntention
	err = tx.QueryRow(r.Context(), upsertQuery, userID, date, req.MoodRating, req.MoodEmoji, req.SleepRating, req.SleepEmoji).Scan(
		&di.ID, &di.Date, &di.MoodRating, &di.MoodEmoji, &di.SleepRating, &di.SleepEmoji,
	)

	if err != nil {
		fmt.Println("Upsert daily_intentions error:", err)
		http.Error(w, "Failed to save intention", http.StatusInternalServerError)
		return
	}

	// Delete existing intention items for this day
	_, err = tx.Exec(r.Context(), "DELETE FROM public.intention_items WHERE daily_intention_id = $1", di.ID)
	if err != nil {
		http.Error(w, "Failed to update intentions", http.StatusInternalServerError)
		return
	}

	// Insert new intention items
	di.Intentions = []Intention{}
	for pos, intent := range req.Intentions {
		if intent.Content == "" {
			continue
		}

		insertQuery := `
			INSERT INTO public.intention_items (daily_intention_id, area_id, content, position)
			VALUES ($1, $2, $3, $4)
			RETURNING id, area_id, content, position
		`

		var i Intention
		err = tx.QueryRow(r.Context(), insertQuery, di.ID, intent.AreaID, intent.Content, pos+1).Scan(
			&i.ID, &i.AreaID, &i.Content, &i.Position,
		)

		if err != nil {
			fmt.Println("Insert intention_item error:", err)
			http.Error(w, "Failed to save intention item", http.StatusInternalServerError)
			return
		}

		di.Intentions = append(di.Intentions, i)
	}

	// Commit transaction
	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "Failed to commit transaction", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(di)
}

// List - GET /intentions
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	query := `
		SELECT id, date, mood_rating, mood_emoji, sleep_rating, sleep_emoji
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
		var di DailyIntention
		if err := rows.Scan(&di.ID, &di.Date, &di.MoodRating, &di.MoodEmoji, &di.SleepRating, &di.SleepEmoji); err != nil {
			continue
		}

		// Get intention items for each day
		itemsQuery := `
			SELECT id, area_id, content, position
			FROM public.intention_items
			WHERE daily_intention_id = $1
			ORDER BY position ASC
		`
		itemRows, err := h.db.Query(r.Context(), itemsQuery, di.ID)
		if err != nil {
			di.Intentions = []Intention{}
		} else {
			di.Intentions = []Intention{}
			for itemRows.Next() {
				var i Intention
				if err := itemRows.Scan(&i.ID, &i.AreaID, &i.Content, &i.Position); err != nil {
					continue
				}
				di.Intentions = append(di.Intentions, i)
			}
			itemRows.Close()
		}

		intentions = append(intentions, di)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(intentions)
}
