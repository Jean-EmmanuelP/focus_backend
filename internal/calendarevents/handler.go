package calendarevents

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"firelevel-backend/internal/auth"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// =============================================
// CALENDAR EVENTS — Cached External Events
// =============================================
// Endpoints for the AI coach + iOS to read
// cached calendar events and manage blocking:
//   GET   /calendar/events?date=YYYY-MM-DD
//   POST  /calendar/sync-events
//   GET   /calendar/providers
//   GET   /calendar/blocking-schedule?date=YYYY-MM-DD
//   PATCH /calendar/events/{id}/blocking
// =============================================

// CalendarEventResponse for API responses
type CalendarEventResponse struct {
	ID           string  `json:"id"`
	Title        string  `json:"title"`
	Description  *string `json:"description,omitempty"`
	Location     *string `json:"location,omitempty"`
	StartAt      string  `json:"start_at"`
	EndAt        string  `json:"end_at"`
	IsAllDay     bool    `json:"is_all_day"`
	EventType    string  `json:"event_type"`
	EventStatus  string  `json:"event_status"`
	BlockApps    bool    `json:"block_apps"`
	IsBusy       bool    `json:"is_busy"`
	ProviderType string  `json:"provider_type"`
	ProviderEmail *string `json:"provider_email,omitempty"`
}

// ProviderResponse for /calendar/providers
type ProviderResponse struct {
	ID             string  `json:"id"`
	ProviderType   string  `json:"provider_type"`
	ProviderEmail  *string `json:"provider_email,omitempty"`
	IsConnected    bool    `json:"is_connected"`
	IsActive       bool    `json:"is_active"`
	SyncDirection  string  `json:"sync_direction"`
	LastSyncAt     *string `json:"last_sync_at,omitempty"`
	LastSyncStatus string  `json:"last_sync_status"`
}

// BlockingWindow for /calendar/blocking-schedule
type BlockingWindow struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Source    string `json:"source"` // "task" or "calendar_event"
	StartAt   string `json:"start_at"`
	EndAt     string `json:"end_at"`
	BlockApps bool   `json:"block_apps"`
}

// UpdateBlockingRequest for PATCH /calendar/events/{id}/blocking
type UpdateBlockingRequest struct {
	BlockApps bool   `json:"block_apps"`
	Source    string `json:"source,omitempty"` // "manual", "ai", "auto"
}

// Handler holds dependencies
type Handler struct {
	db *pgxpool.Pool
}

// NewHandler creates a new calendar events handler
func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
}

// ListEvents returns cached calendar events for a date
// GET /calendar/events?date=YYYY-MM-DD
func (h *Handler) ListEvents(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	dateStr := r.URL.Query().Get("date")

	// Default to today
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}

	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		http.Error(w, "Invalid date format (use YYYY-MM-DD)", http.StatusBadRequest)
		return
	}

	startOfDay := date
	endOfDay := date.AddDate(0, 0, 1)

	// Check if cache is stale (> 15 min since last sync)
	var lastSyncAt *time.Time
	h.db.QueryRow(r.Context(), `
		SELECT MIN(last_sync_at) FROM public.calendar_providers
		WHERE user_id = $1 AND is_connected = true AND is_active = true
	`, userID).Scan(&lastSyncAt)

	cacheStale := lastSyncAt == nil || time.Since(*lastSyncAt) > 15*time.Minute

	// Fetch events
	query := `
		SELECT ce.id, ce.title, ce.description, ce.location,
			   ce.start_at, ce.end_at, ce.is_all_day,
			   ce.event_type, ce.event_status, ce.block_apps, ce.is_busy,
			   cp.provider_type, cp.provider_email
		FROM public.calendar_events ce
		JOIN public.calendar_providers cp ON cp.id = ce.provider_id
		WHERE ce.user_id = $1
		  AND ce.event_status != 'cancelled'
		  AND (
			  (ce.start_at >= $2 AND ce.start_at < $3)
			  OR (ce.start_at < $2 AND ce.end_at > $2)
		  )
		ORDER BY ce.start_at ASC
	`

	rows, err := h.db.Query(r.Context(), query, userID, startOfDay, endOfDay)
	if err != nil {
		fmt.Printf("❌ Calendar events query error: %v\n", err)
		http.Error(w, "Failed to fetch events", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	events := []CalendarEventResponse{}
	for rows.Next() {
		var ev CalendarEventResponse
		var startAt, endAt time.Time
		err := rows.Scan(
			&ev.ID, &ev.Title, &ev.Description, &ev.Location,
			&startAt, &endAt, &ev.IsAllDay,
			&ev.EventType, &ev.EventStatus, &ev.BlockApps, &ev.IsBusy,
			&ev.ProviderType, &ev.ProviderEmail,
		)
		if err != nil {
			continue
		}
		ev.StartAt = startAt.Format(time.RFC3339)
		ev.EndAt = endAt.Format(time.RFC3339)
		events = append(events, ev)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"events":      events,
		"count":       len(events),
		"date":        dateStr,
		"cache_stale": cacheStale,
	})
}

// ListProviders returns connected calendar providers
// GET /calendar/providers
func (h *Handler) ListProviders(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	query := `
		SELECT id, provider_type, provider_email, is_connected,
			   is_active, COALESCE(sync_direction, 'from_provider'),
			   last_sync_at, COALESCE(last_sync_status, 'never')
		FROM public.calendar_providers
		WHERE user_id = $1
		ORDER BY created_at ASC
	`

	rows, err := h.db.Query(r.Context(), query, userID)
	if err != nil {
		http.Error(w, "Failed to fetch providers", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	providers := []ProviderResponse{}
	for rows.Next() {
		var p ProviderResponse
		var lastSyncAt *time.Time
		err := rows.Scan(
			&p.ID, &p.ProviderType, &p.ProviderEmail, &p.IsConnected,
			&p.IsActive, &p.SyncDirection, &lastSyncAt, &p.LastSyncStatus,
		)
		if err != nil {
			continue
		}
		if lastSyncAt != nil {
			s := lastSyncAt.Format(time.RFC3339)
			p.LastSyncAt = &s
		}
		providers = append(providers, p)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"providers": providers,
		"count":     len(providers),
	})
}

// GetBlockingSchedule returns merged blocking windows (calendar events + tasks with blockApps)
// GET /calendar/blocking-schedule?date=YYYY-MM-DD
func (h *Handler) GetBlockingSchedule(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	dateStr := r.URL.Query().Get("date")

	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}

	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		http.Error(w, "Invalid date format", http.StatusBadRequest)
		return
	}

	startOfDay := date
	endOfDay := date.AddDate(0, 0, 1)

	windows := []BlockingWindow{}

	// 1. Calendar events with block_apps = true
	eventQuery := `
		SELECT ce.id, ce.title, ce.start_at, ce.end_at, ce.block_apps
		FROM public.calendar_events ce
		WHERE ce.user_id = $1
		  AND ce.block_apps = true
		  AND ce.event_status = 'confirmed'
		  AND ce.start_at >= $2 AND ce.start_at < $3
		ORDER BY ce.start_at ASC
	`
	rows, err := h.db.Query(r.Context(), eventQuery, userID, startOfDay, endOfDay)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id, title string
			var startAt, endAt time.Time
			var blockApps bool
			if err := rows.Scan(&id, &title, &startAt, &endAt, &blockApps); err == nil {
				windows = append(windows, BlockingWindow{
					ID:        id,
					Title:     title,
					Source:    "calendar_event",
					StartAt:   startAt.Format(time.RFC3339),
					EndAt:     endAt.Format(time.RFC3339),
					BlockApps: blockApps,
				})
			}
		}
	}

	// 2. Tasks with block_apps = true
	taskQuery := `
		SELECT id, title, scheduled_start, scheduled_end, COALESCE(block_apps, false)
		FROM public.calendar_tasks
		WHERE user_id = $1
		  AND COALESCE(block_apps, false) = true
		  AND status != 'completed'
		  AND date = $2
		  AND scheduled_start IS NOT NULL
		  AND scheduled_end IS NOT NULL
		ORDER BY scheduled_start ASC
	`
	taskRows, err := h.db.Query(r.Context(), taskQuery, userID, dateStr)
	if err == nil {
		defer taskRows.Close()
		for taskRows.Next() {
			var id, title string
			var startAt, endAt time.Time
			var blockApps bool
			if err := taskRows.Scan(&id, &title, &startAt, &endAt, &blockApps); err == nil {
				windows = append(windows, BlockingWindow{
					ID:        id,
					Title:     title,
					Source:    "task",
					StartAt:   startAt.Format(time.RFC3339),
					EndAt:     endAt.Format(time.RFC3339),
					BlockApps: blockApps,
				})
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"windows": windows,
		"count":   len(windows),
		"date":    dateStr,
	})
}

// UpdateBlocking toggles block_apps on a calendar event
// PATCH /calendar/events/{id}/blocking
func (h *Handler) UpdateBlocking(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	eventID := chi.URLParam(r, "id")

	var req UpdateBlockingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	source := req.Source
	if source == "" {
		source = "manual"
	}

	query := `
		UPDATE public.calendar_events
		SET block_apps = $1,
			block_apps_source = $2,
			updated_at = NOW()
		WHERE id = $3 AND user_id = $4
	`
	result, err := h.db.Exec(r.Context(), query, req.BlockApps, source, eventID, userID)
	if err != nil {
		http.Error(w, "Failed to update blocking", http.StatusInternalServerError)
		return
	}

	if result.RowsAffected() == 0 {
		http.Error(w, "Event not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"updated":   true,
		"event_id":  eventID,
		"block_apps": req.BlockApps,
	})
}
