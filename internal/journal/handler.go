package journal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"firelevel-backend/internal/auth"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ===========================================
// JOURNAL - Daily Reflections & Entries
// ===========================================

type Handler struct {
	db *pgxpool.Pool
}

func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
}

// ============================================
// Models
// ============================================

// DailyReflection is the end-of-day journaling
type DailyReflection struct {
	ID              string    `json:"id"`
	UserID          string    `json:"user_id"`
	Date            string    `json:"date"`
	BiggestWin      *string   `json:"biggest_win"`
	Challenges      *string   `json:"challenges"`
	BestMoment      *string   `json:"best_moment"`
	GoalForTomorrow *string   `json:"goal_for_tomorrow"`
	CreatedAt       time.Time `json:"created_at"`
}

// JournalEntry is an audio/video progress journal entry
type JournalEntry struct {
	ID              string    `json:"id"`
	UserID          string    `json:"user_id"`
	MediaType       string    `json:"media_type"` // audio, video
	MediaURL        string    `json:"media_url"`
	DurationSeconds int       `json:"duration_seconds"`
	Transcript      *string   `json:"transcript"`
	Summary         *string   `json:"summary"`
	Title           *string   `json:"title"`
	Mood            *string   `json:"mood"`       // great, good, neutral, low, bad
	MoodScore       *int      `json:"mood_score"` // 1-10
	Tags            []string  `json:"tags"`
	EntryDate       string    `json:"entry_date"`
	CreatedAt       time.Time `json:"created_at"`
}

// ============================================
// Daily Reflections CRUD
// ============================================

// ListReflections returns recent daily reflections
// GET /journal/reflections?from=2024-01-01&to=2024-01-31
func (h *Handler) ListReflections(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	query := `
		SELECT id, date, biggest_win, challenges, best_moment, goal_for_tomorrow, created_at
		FROM public.daily_reflections
		WHERE user_id = $1
	`
	args := []interface{}{userID}
	argIdx := 2

	if from != "" {
		query += fmt.Sprintf(` AND date >= $%d`, argIdx)
		args = append(args, from)
		argIdx++
	}
	if to != "" {
		query += fmt.Sprintf(` AND date <= $%d`, argIdx)
		args = append(args, to)
		argIdx++
	}
	query += ` ORDER BY date DESC LIMIT 30`

	rows, err := h.db.Query(r.Context(), query, args...)
	if err != nil {
		http.Error(w, "Failed to list reflections", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	reflections := []DailyReflection{}
	for rows.Next() {
		var ref DailyReflection
		rows.Scan(&ref.ID, &ref.Date, &ref.BiggestWin, &ref.Challenges,
			&ref.BestMoment, &ref.GoalForTomorrow, &ref.CreatedAt)
		ref.UserID = userID
		reflections = append(reflections, ref)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reflections)
}

// GetReflection returns a specific reflection by date
// GET /journal/reflections/{date}
func (h *Handler) GetReflection(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	date := chi.URLParam(r, "date")

	query := `
		SELECT id, date, biggest_win, challenges, best_moment, goal_for_tomorrow, created_at
		FROM public.daily_reflections
		WHERE user_id = $1 AND date = $2
	`

	var ref DailyReflection
	err := h.db.QueryRow(r.Context(), query, userID, date).Scan(
		&ref.ID, &ref.Date, &ref.BiggestWin, &ref.Challenges,
		&ref.BestMoment, &ref.GoalForTomorrow, &ref.CreatedAt)
	if err != nil {
		http.Error(w, "Reflection not found", http.StatusNotFound)
		return
	}
	ref.UserID = userID

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ref)
}

// UpsertReflection creates or updates a daily reflection
// POST /journal/reflections
type UpsertReflectionRequest struct {
	Date            string  `json:"date"` // YYYY-MM-DD, defaults to today
	BiggestWin      *string `json:"biggest_win"`
	Challenges      *string `json:"challenges"`
	BestMoment      *string `json:"best_moment"`
	GoalForTomorrow *string `json:"goal_for_tomorrow"`
}

func (h *Handler) UpsertReflection(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	var req UpsertReflectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Date == "" {
		req.Date = time.Now().Format("2006-01-02")
	}

	query := `
		INSERT INTO public.daily_reflections (user_id, date, biggest_win, challenges, best_moment, goal_for_tomorrow)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_id, date)
		DO UPDATE SET
			biggest_win = COALESCE(EXCLUDED.biggest_win, daily_reflections.biggest_win),
			challenges = COALESCE(EXCLUDED.challenges, daily_reflections.challenges),
			best_moment = COALESCE(EXCLUDED.best_moment, daily_reflections.best_moment),
			goal_for_tomorrow = COALESCE(EXCLUDED.goal_for_tomorrow, daily_reflections.goal_for_tomorrow)
		RETURNING id, date, biggest_win, challenges, best_moment, goal_for_tomorrow, created_at
	`

	var ref DailyReflection
	err := h.db.QueryRow(r.Context(), query,
		userID, req.Date, req.BiggestWin, req.Challenges, req.BestMoment, req.GoalForTomorrow,
	).Scan(&ref.ID, &ref.Date, &ref.BiggestWin, &ref.Challenges,
		&ref.BestMoment, &ref.GoalForTomorrow, &ref.CreatedAt)

	if err != nil {
		fmt.Printf("❌ Reflection upsert error: %v\n", err)
		http.Error(w, "Failed to save reflection", http.StatusInternalServerError)
		return
	}
	ref.UserID = userID

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ref)
}

// DeleteReflection deletes a daily reflection
// DELETE /journal/reflections/{date}
func (h *Handler) DeleteReflection(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	date := chi.URLParam(r, "date")

	h.db.Exec(r.Context(),
		`DELETE FROM public.daily_reflections WHERE user_id = $1 AND date = $2`,
		userID, date)

	w.WriteHeader(http.StatusNoContent)
}

// ============================================
// Journal Entries CRUD (Audio/Video)
// ============================================

// ListEntries returns recent journal entries
// GET /journal/entries?from=2024-01-01&to=2024-01-31
func (h *Handler) ListEntries(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	query := `
		SELECT id, media_type, media_url, duration_seconds, transcript, summary, title,
		       mood, mood_score, tags, entry_date, created_at
		FROM public.journal_entries
		WHERE user_id = $1
	`
	args := []interface{}{userID}
	argIdx := 2

	if from != "" {
		query += fmt.Sprintf(` AND entry_date >= $%d`, argIdx)
		args = append(args, from)
		argIdx++
	}
	if to != "" {
		query += fmt.Sprintf(` AND entry_date <= $%d`, argIdx)
		args = append(args, to)
		argIdx++
	}
	query += ` ORDER BY entry_date DESC LIMIT 30`

	rows, err := h.db.Query(r.Context(), query, args...)
	if err != nil {
		http.Error(w, "Failed to list entries", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	entries := []JournalEntry{}
	for rows.Next() {
		var e JournalEntry
		rows.Scan(&e.ID, &e.MediaType, &e.MediaURL, &e.DurationSeconds,
			&e.Transcript, &e.Summary, &e.Title,
			&e.Mood, &e.MoodScore, &e.Tags, &e.EntryDate, &e.CreatedAt)
		e.UserID = userID
		if e.Tags == nil {
			e.Tags = []string{}
		}
		entries = append(entries, e)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

// GetEntry returns a specific journal entry
// GET /journal/entries/{id}
func (h *Handler) GetEntry(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	id := chi.URLParam(r, "id")

	query := `
		SELECT id, media_type, media_url, duration_seconds, transcript, summary, title,
		       mood, mood_score, tags, entry_date, created_at
		FROM public.journal_entries
		WHERE id = $1 AND user_id = $2
	`

	var e JournalEntry
	err := h.db.QueryRow(r.Context(), query, id, userID).Scan(
		&e.ID, &e.MediaType, &e.MediaURL, &e.DurationSeconds,
		&e.Transcript, &e.Summary, &e.Title,
		&e.Mood, &e.MoodScore, &e.Tags, &e.EntryDate, &e.CreatedAt)
	if err != nil {
		http.Error(w, "Entry not found", http.StatusNotFound)
		return
	}
	e.UserID = userID
	if e.Tags == nil {
		e.Tags = []string{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(e)
}

// CreateEntry creates a new journal entry
// POST /journal/entries
type CreateEntryRequest struct {
	MediaType       string   `json:"media_type"`       // audio, video
	MediaURL        string   `json:"media_url"`
	DurationSeconds int      `json:"duration_seconds"`
	Transcript      *string  `json:"transcript,omitempty"`
	Summary         *string  `json:"summary,omitempty"`
	Title           *string  `json:"title,omitempty"`
	Mood            *string  `json:"mood,omitempty"`
	MoodScore       *int     `json:"mood_score,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	EntryDate       string   `json:"entry_date,omitempty"` // defaults to today
}

func (h *Handler) CreateEntry(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	var req CreateEntryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.MediaType == "" || req.MediaURL == "" {
		http.Error(w, "media_type and media_url are required", http.StatusBadRequest)
		return
	}

	if req.EntryDate == "" {
		req.EntryDate = time.Now().Format("2006-01-02")
	}

	if req.Tags == nil {
		req.Tags = []string{}
	}

	query := `
		INSERT INTO public.journal_entries (
			user_id, media_type, media_url, duration_seconds,
			transcript, summary, title, mood, mood_score, tags, entry_date
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT ON CONSTRAINT unique_daily_entry
		DO UPDATE SET
			media_type = EXCLUDED.media_type,
			media_url = EXCLUDED.media_url,
			duration_seconds = EXCLUDED.duration_seconds,
			transcript = COALESCE(EXCLUDED.transcript, journal_entries.transcript),
			summary = COALESCE(EXCLUDED.summary, journal_entries.summary),
			title = COALESCE(EXCLUDED.title, journal_entries.title),
			mood = COALESCE(EXCLUDED.mood, journal_entries.mood),
			mood_score = COALESCE(EXCLUDED.mood_score, journal_entries.mood_score),
			tags = COALESCE(EXCLUDED.tags, journal_entries.tags),
			updated_at = NOW()
		RETURNING id, media_type, media_url, duration_seconds, transcript, summary, title,
		          mood, mood_score, tags, entry_date, created_at
	`

	var e JournalEntry
	err := h.db.QueryRow(r.Context(), query,
		userID, req.MediaType, req.MediaURL, req.DurationSeconds,
		req.Transcript, req.Summary, req.Title, req.Mood, req.MoodScore, req.Tags, req.EntryDate,
	).Scan(&e.ID, &e.MediaType, &e.MediaURL, &e.DurationSeconds,
		&e.Transcript, &e.Summary, &e.Title,
		&e.Mood, &e.MoodScore, &e.Tags, &e.EntryDate, &e.CreatedAt)

	if err != nil {
		fmt.Printf("❌ Journal entry create error: %v\n", err)
		http.Error(w, "Failed to create journal entry", http.StatusInternalServerError)
		return
	}
	e.UserID = userID
	if e.Tags == nil {
		e.Tags = []string{}
	}

	w.WriteHeader(http.StatusCreated)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(e)
}

// DeleteEntry deletes a journal entry
// DELETE /journal/entries/{id}
func (h *Handler) DeleteEntry(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	id := chi.URLParam(r, "id")

	h.db.Exec(r.Context(),
		`DELETE FROM public.journal_entries WHERE id = $1 AND user_id = $2`,
		id, userID)

	w.WriteHeader(http.StatusNoContent)
}

// ============================================
// Mood Stats
// ============================================

// GetMoodStats returns mood trends for a period
// GET /journal/mood-stats?days=30
func (h *Handler) GetMoodStats(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	days := r.URL.Query().Get("days")
	if days == "" {
		days = "30"
	}

	query := `
		SELECT
			entry_date,
			mood,
			mood_score
		FROM public.journal_entries
		WHERE user_id = $1
			AND entry_date >= CURRENT_DATE - ($2 || ' days')::interval
			AND mood_score IS NOT NULL
		ORDER BY entry_date ASC
	`

	rows, err := h.db.Query(r.Context(), query, userID, days)
	if err != nil {
		http.Error(w, "Failed to get mood stats", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type MoodPoint struct {
		Date      string `json:"date"`
		Mood      string `json:"mood"`
		MoodScore int    `json:"mood_score"`
	}

	points := []MoodPoint{}
	for rows.Next() {
		var p MoodPoint
		rows.Scan(&p.Date, &p.Mood, &p.MoodScore)
		points = append(points, p)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"period":      days + " days",
		"data_points": points,
		"count":       len(points),
	})
}
