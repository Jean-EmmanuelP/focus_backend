package journal

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"firelevel-backend/internal/auth"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ==========================================
// TYPES
// ==========================================

type Handler struct {
	db        *pgxpool.Pool
	aiService *AIService
}

func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{
		db:        db,
		aiService: NewAIService(),
	}
}

// JournalEntry represents a daily journal entry
type JournalEntry struct {
	ID              string    `json:"id"`
	UserID          string    `json:"userId"`
	MediaType       string    `json:"mediaType"`       // audio or video
	MediaURL        string    `json:"mediaUrl"`
	DurationSeconds int       `json:"durationSeconds"`
	Transcript      *string   `json:"transcript,omitempty"`
	Summary         *string   `json:"summary,omitempty"`
	Title           *string   `json:"title,omitempty"`
	Mood            *string   `json:"mood,omitempty"`
	MoodScore       *int      `json:"moodScore,omitempty"`
	Tags            []string  `json:"tags,omitempty"`
	EntryDate       string    `json:"entryDate"` // YYYY-MM-DD
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

// JournalBilan represents weekly/monthly summaries
type JournalBilan struct {
	ID             string    `json:"id"`
	UserID         string    `json:"userId"`
	BilanType      string    `json:"bilanType"` // weekly or monthly
	PeriodStart    string    `json:"periodStart"`
	PeriodEnd      string    `json:"periodEnd"`
	Summary        string    `json:"summary"`
	Wins           []string  `json:"wins,omitempty"`
	Improvements   []string  `json:"improvements,omitempty"`
	MoodTrend      *string   `json:"moodTrend,omitempty"`
	AvgMoodScore   *float64  `json:"avgMoodScore,omitempty"`
	SuggestedGoals []string  `json:"suggestedGoals,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
}

// CreateEntryRequest for creating a new journal entry
type CreateEntryRequest struct {
	MediaBase64     string `json:"media_base64"`     // Base64 encoded audio/video
	MediaType       string `json:"media_type"`       // audio or video
	ContentType     string `json:"content_type"`     // audio/m4a, video/mp4, etc.
	DurationSeconds int    `json:"duration_seconds"`
	EntryDate       string `json:"entry_date,omitempty"` // Optional, defaults to today
}

// EntryListResponse for paginated list
type EntryListResponse struct {
	Entries    []JournalEntry `json:"entries"`
	HasMore    bool           `json:"hasMore"`
	NextOffset int            `json:"nextOffset"`
}

// MoodStats for the mood graph
type MoodStats struct {
	Date      string `json:"date"`
	MoodScore *int   `json:"moodScore"`
	Mood      *string `json:"mood"`
}

type StatsResponse struct {
	Stats         []MoodStats `json:"stats"`
	CurrentStreak int         `json:"currentStreak"`
	TotalEntries  int         `json:"totalEntries"`
}

// ==========================================
// HANDLERS
// ==========================================

// CreateEntry creates a new journal entry with AI analysis
// POST /journal/entries
func (h *Handler) CreateEntry(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	var req CreateEntryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[CreateEntry] Failed to decode request: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.MediaBase64 == "" {
		http.Error(w, "Media is required", http.StatusBadRequest)
		return
	}
	if req.MediaType != "audio" && req.MediaType != "video" {
		http.Error(w, "Media type must be 'audio' or 'video'", http.StatusBadRequest)
		return
	}
	if req.DurationSeconds <= 0 || req.DurationSeconds > 180 {
		http.Error(w, "Duration must be between 1 and 180 seconds", http.StatusBadRequest)
		return
	}

	// Default to today
	entryDate := req.EntryDate
	if entryDate == "" {
		entryDate = time.Now().Format("2006-01-02")
	}

	// Check if entry already exists for this date
	var existingID string
	err := h.db.QueryRow(r.Context(), `
		SELECT id FROM journal_entries WHERE user_id = $1 AND entry_date = $2
	`, userID, entryDate).Scan(&existingID)
	if err == nil {
		http.Error(w, "An entry already exists for this date", http.StatusConflict)
		return
	}

	// Decode base64 media
	mediaData, err := base64.StdEncoding.DecodeString(req.MediaBase64)
	if err != nil {
		http.Error(w, "Invalid base64 media", http.StatusBadRequest)
		return
	}

	// Upload to Supabase Storage
	mediaURL, err := h.uploadMediaToStorage(userID, entryDate, mediaData, req.ContentType, req.MediaType)
	if err != nil {
		log.Printf("[CreateEntry] Failed to upload media: %v", err)
		http.Error(w, "Failed to upload media", http.StatusInternalServerError)
		return
	}

	// Run STT (transcription ONLY - no analysis)
	// Analysis (summary, mood, tags) is done monthly via batch job
	var transcript *string
	sttResult, err := h.aiService.TranscribeAudio(mediaData, req.MediaType, req.ContentType)
	if err != nil {
		log.Printf("[CreateEntry] STT failed: %v", err)
		// Continue without transcript - we'll still save the entry with just the audio
	} else {
		transcript = &sttResult.Transcript
	}

	// Insert entry into database (NO analysis fields - those are populated monthly)
	var entry JournalEntry
	err = h.db.QueryRow(r.Context(), `
		INSERT INTO journal_entries (
			user_id, media_type, media_url, duration_seconds,
			transcript, entry_date
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, user_id, media_type, media_url, duration_seconds,
			transcript, summary, title, mood, mood_score, tags, entry_date, created_at, updated_at
	`, userID, req.MediaType, mediaURL, req.DurationSeconds, transcript, entryDate,
	).Scan(
		&entry.ID, &entry.UserID, &entry.MediaType, &entry.MediaURL, &entry.DurationSeconds,
		&entry.Transcript, &entry.Summary, &entry.Title, &entry.Mood, &entry.MoodScore, &entry.Tags, &entry.EntryDate, &entry.CreatedAt, &entry.UpdatedAt,
	)

	if err != nil {
		log.Printf("[CreateEntry] Failed to insert entry: %v", err)
		http.Error(w, "Failed to create entry", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(entry)
}

// ListEntries returns paginated journal entries
// GET /journal/entries?offset=0&limit=20&date_from=2024-01-01&date_to=2024-01-31
func (h *Handler) ListEntries(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	// Parse pagination
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	dateFrom := r.URL.Query().Get("date_from")
	dateTo := r.URL.Query().Get("date_to")

	// Build query
	query := `
		SELECT id, user_id, media_type, media_url, duration_seconds,
			transcript, summary, title, mood, mood_score, tags, entry_date, created_at, updated_at
		FROM journal_entries
		WHERE user_id = $1
	`
	args := []interface{}{userID}
	argIndex := 2

	if dateFrom != "" {
		query += fmt.Sprintf(" AND entry_date >= $%d", argIndex)
		args = append(args, dateFrom)
		argIndex++
	}
	if dateTo != "" {
		query += fmt.Sprintf(" AND entry_date <= $%d", argIndex)
		args = append(args, dateTo)
		argIndex++
	}

	query += fmt.Sprintf(" ORDER BY entry_date DESC LIMIT $%d OFFSET $%d", argIndex, argIndex+1)
	args = append(args, limit+1, offset)

	rows, err := h.db.Query(r.Context(), query, args...)
	if err != nil {
		log.Printf("[ListEntries] Query error: %v", err)
		http.Error(w, "Failed to fetch entries", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	entries := []JournalEntry{}
	for rows.Next() {
		var entry JournalEntry
		err := rows.Scan(
			&entry.ID, &entry.UserID, &entry.MediaType, &entry.MediaURL, &entry.DurationSeconds,
			&entry.Transcript, &entry.Summary, &entry.Title, &entry.Mood, &entry.MoodScore, &entry.Tags, &entry.EntryDate, &entry.CreatedAt, &entry.UpdatedAt,
		)
		if err != nil {
			log.Printf("[ListEntries] Scan error: %v", err)
			continue
		}
		entries = append(entries, entry)
	}

	hasMore := len(entries) > limit
	if hasMore {
		entries = entries[:limit]
	}

	response := EntryListResponse{
		Entries:    entries,
		HasMore:    hasMore,
		NextOffset: offset + limit,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetEntry returns a single journal entry
// GET /journal/entries/{id}
func (h *Handler) GetEntry(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	entryID := chi.URLParam(r, "id")

	var entry JournalEntry
	err := h.db.QueryRow(r.Context(), `
		SELECT id, user_id, media_type, media_url, duration_seconds,
			transcript, summary, title, mood, mood_score, tags, entry_date, created_at, updated_at
		FROM journal_entries
		WHERE id = $1 AND user_id = $2
	`, entryID, userID).Scan(
		&entry.ID, &entry.UserID, &entry.MediaType, &entry.MediaURL, &entry.DurationSeconds,
		&entry.Transcript, &entry.Summary, &entry.Title, &entry.Mood, &entry.MoodScore, &entry.Tags, &entry.EntryDate, &entry.CreatedAt, &entry.UpdatedAt,
	)

	if err != nil {
		http.Error(w, "Entry not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entry)
}

// GetTodayEntry returns today's journal entry if it exists
// GET /journal/entries/today
func (h *Handler) GetTodayEntry(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	today := time.Now().Format("2006-01-02")

	var entry JournalEntry
	err := h.db.QueryRow(r.Context(), `
		SELECT id, user_id, media_type, media_url, duration_seconds,
			transcript, summary, title, mood, mood_score, tags, entry_date, created_at, updated_at
		FROM journal_entries
		WHERE user_id = $1 AND entry_date = $2
	`, userID, today).Scan(
		&entry.ID, &entry.UserID, &entry.MediaType, &entry.MediaURL, &entry.DurationSeconds,
		&entry.Transcript, &entry.Summary, &entry.Title, &entry.Mood, &entry.MoodScore, &entry.Tags, &entry.EntryDate, &entry.CreatedAt, &entry.UpdatedAt,
	)

	if err != nil {
		http.Error(w, "No entry for today", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entry)
}

// DeleteEntry deletes a journal entry
// DELETE /journal/entries/{id}
func (h *Handler) DeleteEntry(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	entryID := chi.URLParam(r, "id")

	result, err := h.db.Exec(r.Context(), `
		DELETE FROM journal_entries WHERE id = $1 AND user_id = $2
	`, entryID, userID)

	if err != nil || result.RowsAffected() == 0 {
		http.Error(w, "Entry not found or not yours", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetStreak returns the current journaling streak
// GET /journal/entries/streak
func (h *Handler) GetStreak(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	// Get all entry dates ordered by most recent
	rows, err := h.db.Query(r.Context(), `
		SELECT entry_date FROM journal_entries
		WHERE user_id = $1
		ORDER BY entry_date DESC
	`, userID)
	if err != nil {
		http.Error(w, "Failed to calculate streak", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var dates []time.Time
	for rows.Next() {
		var d time.Time
		if err := rows.Scan(&d); err == nil {
			dates = append(dates, d)
		}
	}

	streak := 0
	today := time.Now().Truncate(24 * time.Hour)

	for i, d := range dates {
		expectedDate := today.AddDate(0, 0, -i)
		if d.Truncate(24*time.Hour).Equal(expectedDate) {
			streak++
		} else if d.Truncate(24*time.Hour).Equal(expectedDate.AddDate(0, 0, -1)) && i == 0 {
			// If today is missing but yesterday is present, check from yesterday
			continue
		} else {
			break
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"streak": streak})
}

// GetStats returns mood statistics for graphing
// GET /journal/stats?days=7
func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 || days > 90 {
		days = 7
	}

	rows, err := h.db.Query(r.Context(), `
		SELECT entry_date, mood, mood_score
		FROM journal_entries
		WHERE user_id = $1 AND entry_date >= CURRENT_DATE - $2::integer
		ORDER BY entry_date ASC
	`, userID, days)
	if err != nil {
		http.Error(w, "Failed to fetch stats", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	stats := []MoodStats{}
	for rows.Next() {
		var s MoodStats
		if err := rows.Scan(&s.Date, &s.Mood, &s.MoodScore); err == nil {
			stats = append(stats, s)
		}
	}

	// Get streak and total entries
	var streak, total int
	h.db.QueryRow(r.Context(), `
		SELECT COUNT(*) FROM journal_entries WHERE user_id = $1
	`, userID).Scan(&total)

	// Simple streak calculation
	var consecutiveDays int
	h.db.QueryRow(r.Context(), `
		WITH dates AS (
			SELECT entry_date, entry_date - (ROW_NUMBER() OVER (ORDER BY entry_date DESC))::integer AS grp
			FROM journal_entries
			WHERE user_id = $1
		)
		SELECT COUNT(*) FROM dates
		WHERE grp = (SELECT grp FROM dates WHERE entry_date = CURRENT_DATE OR entry_date = CURRENT_DATE - 1 LIMIT 1)
	`, userID).Scan(&consecutiveDays)
	streak = consecutiveDays

	response := StatsResponse{
		Stats:         stats,
		CurrentStreak: streak,
		TotalEntries:  total,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GenerateWeeklyBilan generates or retrieves weekly summary
// POST /journal/bilans/weekly
func (h *Handler) GenerateWeeklyBilan(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	// Calculate week period (Monday to Sunday)
	now := time.Now()
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	periodStart := now.AddDate(0, 0, -(weekday - 1)).Format("2006-01-02")
	periodEnd := now.AddDate(0, 0, 7-weekday).Format("2006-01-02")

	// Check if bilan already exists
	var existingBilan JournalBilan
	err := h.db.QueryRow(r.Context(), `
		SELECT id, user_id, bilan_type, period_start, period_end, summary, wins, improvements, mood_trend, avg_mood_score, suggested_goals, created_at
		FROM journal_bilans
		WHERE user_id = $1 AND bilan_type = 'weekly' AND period_start = $2
	`, userID, periodStart).Scan(
		&existingBilan.ID, &existingBilan.UserID, &existingBilan.BilanType, &existingBilan.PeriodStart, &existingBilan.PeriodEnd,
		&existingBilan.Summary, &existingBilan.Wins, &existingBilan.Improvements, &existingBilan.MoodTrend, &existingBilan.AvgMoodScore, &existingBilan.SuggestedGoals, &existingBilan.CreatedAt,
	)
	if err == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(existingBilan)
		return
	}

	// Fetch entries for the week
	rows, err := h.db.Query(r.Context(), `
		SELECT transcript, summary, mood, mood_score, entry_date
		FROM journal_entries
		WHERE user_id = $1 AND entry_date >= $2 AND entry_date <= $3
		ORDER BY entry_date
	`, userID, periodStart, periodEnd)
	if err != nil {
		http.Error(w, "Failed to fetch entries", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var entries []BilanEntryData
	for rows.Next() {
		var e BilanEntryData
		rows.Scan(&e.Transcript, &e.Summary, &e.Mood, &e.MoodScore, &e.Date)
		entries = append(entries, e)
	}

	if len(entries) == 0 {
		http.Error(w, "No entries found for this week", http.StatusNotFound)
		return
	}

	// Generate bilan using AI
	bilanData, err := h.aiService.GenerateBilan(entries, "weekly")
	if err != nil {
		log.Printf("[GenerateWeeklyBilan] AI error: %v", err)
		http.Error(w, "Failed to generate bilan", http.StatusInternalServerError)
		return
	}

	// Save bilan
	var bilan JournalBilan
	err = h.db.QueryRow(r.Context(), `
		INSERT INTO journal_bilans (user_id, bilan_type, period_start, period_end, summary, wins, improvements, mood_trend, avg_mood_score)
		VALUES ($1, 'weekly', $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, user_id, bilan_type, period_start, period_end, summary, wins, improvements, mood_trend, avg_mood_score, suggested_goals, created_at
	`, userID, periodStart, periodEnd, bilanData.Summary, bilanData.Wins, bilanData.Improvements, bilanData.MoodTrend, bilanData.AvgMoodScore,
	).Scan(
		&bilan.ID, &bilan.UserID, &bilan.BilanType, &bilan.PeriodStart, &bilan.PeriodEnd,
		&bilan.Summary, &bilan.Wins, &bilan.Improvements, &bilan.MoodTrend, &bilan.AvgMoodScore, &bilan.SuggestedGoals, &bilan.CreatedAt,
	)

	if err != nil {
		log.Printf("[GenerateWeeklyBilan] Insert error: %v", err)
		http.Error(w, "Failed to save bilan", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(bilan)
}

// GenerateMonthlyBilan generates or retrieves monthly summary
// POST /journal/bilans/monthly
func (h *Handler) GenerateMonthlyBilan(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	// Calculate month period
	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
	periodEnd := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, now.Location()).Format("2006-01-02")

	// Check if bilan already exists
	var existingBilan JournalBilan
	err := h.db.QueryRow(r.Context(), `
		SELECT id, user_id, bilan_type, period_start, period_end, summary, wins, improvements, mood_trend, avg_mood_score, suggested_goals, created_at
		FROM journal_bilans
		WHERE user_id = $1 AND bilan_type = 'monthly' AND period_start = $2
	`, userID, periodStart).Scan(
		&existingBilan.ID, &existingBilan.UserID, &existingBilan.BilanType, &existingBilan.PeriodStart, &existingBilan.PeriodEnd,
		&existingBilan.Summary, &existingBilan.Wins, &existingBilan.Improvements, &existingBilan.MoodTrend, &existingBilan.AvgMoodScore, &existingBilan.SuggestedGoals, &existingBilan.CreatedAt,
	)
	if err == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(existingBilan)
		return
	}

	// Fetch entries for the month
	rows, err := h.db.Query(r.Context(), `
		SELECT transcript, summary, mood, mood_score, entry_date
		FROM journal_entries
		WHERE user_id = $1 AND entry_date >= $2 AND entry_date <= $3
		ORDER BY entry_date
	`, userID, periodStart, periodEnd)
	if err != nil {
		http.Error(w, "Failed to fetch entries", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var entries []BilanEntryData
	for rows.Next() {
		var e BilanEntryData
		rows.Scan(&e.Transcript, &e.Summary, &e.Mood, &e.MoodScore, &e.Date)
		entries = append(entries, e)
	}

	if len(entries) == 0 {
		http.Error(w, "No entries found for this month", http.StatusNotFound)
		return
	}

	// Generate bilan using AI
	bilanData, err := h.aiService.GenerateBilan(entries, "monthly")
	if err != nil {
		log.Printf("[GenerateMonthlyBilan] AI error: %v", err)
		http.Error(w, "Failed to generate bilan", http.StatusInternalServerError)
		return
	}

	// Save bilan
	var bilan JournalBilan
	err = h.db.QueryRow(r.Context(), `
		INSERT INTO journal_bilans (user_id, bilan_type, period_start, period_end, summary, wins, improvements, mood_trend, avg_mood_score, suggested_goals)
		VALUES ($1, 'monthly', $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, user_id, bilan_type, period_start, period_end, summary, wins, improvements, mood_trend, avg_mood_score, suggested_goals, created_at
	`, userID, periodStart, periodEnd, bilanData.Summary, bilanData.Wins, bilanData.Improvements, bilanData.MoodTrend, bilanData.AvgMoodScore, bilanData.SuggestedGoals,
	).Scan(
		&bilan.ID, &bilan.UserID, &bilan.BilanType, &bilan.PeriodStart, &bilan.PeriodEnd,
		&bilan.Summary, &bilan.Wins, &bilan.Improvements, &bilan.MoodTrend, &bilan.AvgMoodScore, &bilan.SuggestedGoals, &bilan.CreatedAt,
	)

	if err != nil {
		log.Printf("[GenerateMonthlyBilan] Insert error: %v", err)
		http.Error(w, "Failed to save bilan", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(bilan)
}

// ListBilans returns all bilans for the user
// GET /journal/bilans
func (h *Handler) ListBilans(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	rows, err := h.db.Query(r.Context(), `
		SELECT id, user_id, bilan_type, period_start, period_end, summary, wins, improvements, mood_trend, avg_mood_score, suggested_goals, created_at
		FROM journal_bilans
		WHERE user_id = $1
		ORDER BY period_start DESC
	`, userID)
	if err != nil {
		http.Error(w, "Failed to fetch bilans", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	bilans := []JournalBilan{}
	for rows.Next() {
		var b JournalBilan
		err := rows.Scan(
			&b.ID, &b.UserID, &b.BilanType, &b.PeriodStart, &b.PeriodEnd,
			&b.Summary, &b.Wins, &b.Improvements, &b.MoodTrend, &b.AvgMoodScore, &b.SuggestedGoals, &b.CreatedAt,
		)
		if err != nil {
			continue
		}
		bilans = append(bilans, b)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(bilans)
}

// ==========================================
// MONTHLY ANALYSIS JOB (runs on the 1st of each month)
// ==========================================

// RunMonthlyAnalysis analyzes all entries from the previous month
// POST /jobs/journal/monthly-analysis?key=firelevel-journal-cron-2024
// This should be called by a cron job on the 1st of each month
func (h *Handler) RunMonthlyAnalysis(w http.ResponseWriter, r *http.Request) {
	// Verify cron key (hardcoded for simplicity)
	cronKey := r.URL.Query().Get("key")
	if cronKey != "firelevel-journal-cron-2024" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Calculate previous month period
	now := time.Now()
	firstOfThisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	lastOfPrevMonth := firstOfThisMonth.Add(-24 * time.Hour)
	firstOfPrevMonth := time.Date(lastOfPrevMonth.Year(), lastOfPrevMonth.Month(), 1, 0, 0, 0, 0, now.Location())

	periodStart := firstOfPrevMonth.Format("2006-01-02")
	periodEnd := lastOfPrevMonth.Format("2006-01-02")

	log.Printf("[MonthlyAnalysis] Starting analysis for period %s to %s", periodStart, periodEnd)

	// Get all entries that have transcript but NO analysis yet
	rows, err := h.db.Query(r.Context(), `
		SELECT id, user_id, transcript, entry_date
		FROM journal_entries
		WHERE entry_date >= $1 AND entry_date <= $2
		  AND transcript IS NOT NULL
		  AND (summary IS NULL OR summary = '')
		ORDER BY entry_date
	`, periodStart, periodEnd)
	if err != nil {
		log.Printf("[MonthlyAnalysis] Query error: %v", err)
		http.Error(w, "Failed to fetch entries", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type entryToAnalyze struct {
		ID         string
		UserID     string
		Transcript string
		EntryDate  string
	}

	var entries []entryToAnalyze
	for rows.Next() {
		var e entryToAnalyze
		if err := rows.Scan(&e.ID, &e.UserID, &e.Transcript, &e.EntryDate); err != nil {
			continue
		}
		entries = append(entries, e)
	}

	log.Printf("[MonthlyAnalysis] Found %d entries to analyze", len(entries))

	// Analyze each entry
	analyzed := 0
	failed := 0
	for _, e := range entries {
		analysis, err := h.aiService.AnalyzeEntry(e.Transcript)
		if err != nil {
			log.Printf("[MonthlyAnalysis] Failed to analyze entry %s: %v", e.ID, err)
			failed++
			continue
		}

		// Update entry with analysis
		_, err = h.db.Exec(r.Context(), `
			UPDATE journal_entries
			SET summary = $2, title = $3, mood = $4, mood_score = $5, tags = $6, updated_at = NOW()
			WHERE id = $1
		`, e.ID, analysis.Summary, analysis.Title, analysis.Mood, analysis.MoodScore, analysis.Tags)

		if err != nil {
			log.Printf("[MonthlyAnalysis] Failed to update entry %s: %v", e.ID, err)
			failed++
			continue
		}

		analyzed++
		log.Printf("[MonthlyAnalysis] Analyzed entry %s (user: %s, date: %s)", e.ID, e.UserID, e.EntryDate)
	}

	// Now generate bilans for users who had AT LEAST 20 entries this month
	// (minimum threshold to make the analysis meaningful)
	const minEntriesForBilan = 20

	usersRows, err := h.db.Query(r.Context(), `
		SELECT user_id, COUNT(*) as entry_count
		FROM journal_entries
		WHERE entry_date >= $1 AND entry_date <= $2
		GROUP BY user_id
		HAVING COUNT(*) >= $3
	`, periodStart, periodEnd, minEntriesForBilan)
	if err == nil {
		defer usersRows.Close()
		bilansGenerated := 0
		usersSkipped := 0

		for usersRows.Next() {
			var userID string
			var entryCount int
			if err := usersRows.Scan(&userID, &entryCount); err != nil {
				continue
			}

			log.Printf("[MonthlyAnalysis] User %s has %d entries (>= %d required)", userID, entryCount, minEntriesForBilan)

			// Get entries for bilan
			bilanEntries := []BilanEntryData{}
			entryRows, err := h.db.Query(r.Context(), `
				SELECT transcript, summary, mood, mood_score, entry_date
				FROM journal_entries
				WHERE user_id = $1 AND entry_date >= $2 AND entry_date <= $3
				ORDER BY entry_date
			`, userID, periodStart, periodEnd)
			if err != nil {
				continue
			}

			for entryRows.Next() {
				var e BilanEntryData
				entryRows.Scan(&e.Transcript, &e.Summary, &e.Mood, &e.MoodScore, &e.Date)
				bilanEntries = append(bilanEntries, e)
			}
			entryRows.Close()

			if len(bilanEntries) == 0 {
				continue
			}

			// Generate and save monthly bilan
			bilanData, err := h.aiService.GenerateBilan(bilanEntries, "monthly")
			if err != nil {
				log.Printf("[MonthlyAnalysis] Failed to generate bilan for user %s: %v", userID, err)
				continue
			}

			_, err = h.db.Exec(r.Context(), `
				INSERT INTO journal_bilans (user_id, bilan_type, period_start, period_end, summary, wins, improvements, mood_trend, avg_mood_score, suggested_goals)
				VALUES ($1, 'monthly', $2, $3, $4, $5, $6, $7, $8, $9)
				ON CONFLICT (user_id, bilan_type, period_start) DO UPDATE
				SET summary = EXCLUDED.summary, wins = EXCLUDED.wins, improvements = EXCLUDED.improvements,
				    mood_trend = EXCLUDED.mood_trend, avg_mood_score = EXCLUDED.avg_mood_score, suggested_goals = EXCLUDED.suggested_goals
			`, userID, periodStart, periodEnd, bilanData.Summary, bilanData.Wins, bilanData.Improvements, bilanData.MoodTrend, bilanData.AvgMoodScore, bilanData.SuggestedGoals)

			if err == nil {
				bilansGenerated++
				log.Printf("[MonthlyAnalysis] Generated bilan for user %s", userID)
			}
		}

		// Count users who didn't meet the threshold
		var skippedCount int
		h.db.QueryRow(r.Context(), `
			SELECT COUNT(DISTINCT user_id)
			FROM journal_entries
			WHERE entry_date >= $1 AND entry_date <= $2
			AND user_id NOT IN (
				SELECT user_id FROM journal_entries
				WHERE entry_date >= $1 AND entry_date <= $2
				GROUP BY user_id
				HAVING COUNT(*) >= $3
			)
		`, periodStart, periodEnd, minEntriesForBilan).Scan(&skippedCount)
		usersSkipped = skippedCount

		log.Printf("[MonthlyAnalysis] Generated %d monthly bilans, skipped %d users (< %d entries)", bilansGenerated, usersSkipped, minEntriesForBilan)
	}

	// Return summary
	response := map[string]interface{}{
		"period_start":     periodStart,
		"period_end":       periodEnd,
		"entries_found":    len(entries),
		"entries_analyzed": analyzed,
		"entries_failed":   failed,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ==========================================
// HELPER METHODS
// ==========================================

func (h *Handler) uploadMediaToStorage(userID, date string, mediaData []byte, contentType, mediaType string) (string, error) {
	supabaseURL := os.Getenv("SUPABASE_URL")
	serviceRoleKey := os.Getenv("SUPABASE_SERVICE_ROLE_KEY")
	if serviceRoleKey == "" {
		serviceRoleKey = os.Getenv("SUPABASE_KEY")
	}

	if supabaseURL == "" || serviceRoleKey == "" {
		return "", fmt.Errorf("missing Supabase configuration")
	}

	// Determine file extension
	ext := "m4a" // default for audio
	switch contentType {
	case "audio/mp4", "audio/m4a", "audio/x-m4a":
		ext = "m4a"
	case "audio/mpeg":
		ext = "mp3"
	case "audio/wav":
		ext = "wav"
	case "video/mp4":
		ext = "mp4"
	case "video/quicktime":
		ext = "mov"
	}

	// Generate filename: journal/{userId}/{date}_{uuid}.{ext}
	filename := fmt.Sprintf("%s/%s_%s.%s", userID, date, uuid.New().String()[:8], ext)

	// Upload to Supabase Storage (journal bucket)
	url := fmt.Sprintf("%s/storage/v1/object/journal/%s", supabaseURL, filename)

	req, err := http.NewRequest("POST", url, bytes.NewReader(mediaData))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+serviceRoleKey)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("x-upsert", "true")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("storage error %d: %s", resp.StatusCode, string(body))
	}

	// Return public URL
	publicURL := fmt.Sprintf("%s/storage/v1/object/public/journal/%s", supabaseURL, filename)
	return publicURL, nil
}
