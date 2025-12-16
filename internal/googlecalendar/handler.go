package googlecalendar

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
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

// ==========================================
// TYPES
// ==========================================

type GoogleCalendarConfig struct {
	ID            string     `json:"id"`
	UserID        string     `json:"userId"`
	AccessToken   string     `json:"-"` // Never exposed to client
	RefreshToken  string     `json:"-"` // Never exposed to client
	TokenExpiry   time.Time  `json:"-"`
	IsEnabled     bool       `json:"isEnabled"`
	SyncDirection string     `json:"syncDirection"` // bidirectional, to_google, from_google
	CalendarID    string     `json:"calendarId"`
	GoogleEmail   *string    `json:"googleEmail,omitempty"`
	LastSyncAt    *time.Time `json:"lastSyncAt,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

type GoogleCalendarConfigResponse struct {
	IsConnected   bool       `json:"isConnected"`
	IsEnabled     bool       `json:"isEnabled"`
	SyncDirection string     `json:"syncDirection"`
	CalendarID    string     `json:"calendarId"`
	GoogleEmail   *string    `json:"googleEmail,omitempty"`
	LastSyncAt    *time.Time `json:"lastSyncAt,omitempty"`
}

type SaveTokensRequest struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"` // seconds until expiry
	GoogleEmail  string `json:"google_email"`
}

type UpdateConfigRequest struct {
	IsEnabled     *bool   `json:"isEnabled,omitempty"`
	SyncDirection *string `json:"syncDirection,omitempty"`
	CalendarID    *string `json:"calendarId,omitempty"`
}

type GoogleCalendarEvent struct {
	ID          string  `json:"id"`
	Summary     string  `json:"summary"`
	Description *string `json:"description,omitempty"`
	Start       string  `json:"start"` // ISO8601
	End         string  `json:"end"`   // ISO8601
	Status      string  `json:"status"`
}

type SyncResult struct {
	TasksSynced     int      `json:"tasksSynced"`
	EventsImported  int      `json:"eventsImported"`
	Errors          []string `json:"errors,omitempty"`
	LastSyncAt      string   `json:"lastSyncAt"`
}

// ==========================================
// HANDLERS
// ==========================================

// GetConfig returns the current Google Calendar configuration
// GET /google-calendar/config
func (h *Handler) GetConfig(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	var config GoogleCalendarConfig
	err := h.db.QueryRow(r.Context(), `
		SELECT id, user_id, is_enabled, sync_direction, calendar_id, google_email, last_sync_at, created_at, updated_at
		FROM google_calendar_config
		WHERE user_id = $1
	`, userID).Scan(
		&config.ID, &config.UserID, &config.IsEnabled, &config.SyncDirection,
		&config.CalendarID, &config.GoogleEmail, &config.LastSyncAt,
		&config.CreatedAt, &config.UpdatedAt,
	)

	if err != nil {
		// No config found - not connected
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(GoogleCalendarConfigResponse{
			IsConnected:   false,
			IsEnabled:     false,
			SyncDirection: "bidirectional",
			CalendarID:    "primary",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(GoogleCalendarConfigResponse{
		IsConnected:   true,
		IsEnabled:     config.IsEnabled,
		SyncDirection: config.SyncDirection,
		CalendarID:    config.CalendarID,
		GoogleEmail:   config.GoogleEmail,
		LastSyncAt:    config.LastSyncAt,
	})
}

// SaveTokens saves OAuth tokens from the iOS app after Google Sign-In
// POST /google-calendar/tokens
func (h *Handler) SaveTokens(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	var req SaveTokensRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.AccessToken == "" || req.RefreshToken == "" {
		http.Error(w, "Access token and refresh token are required", http.StatusBadRequest)
		return
	}

	// Calculate token expiry
	tokenExpiry := time.Now().Add(time.Duration(req.ExpiresIn) * time.Second)

	var config GoogleCalendarConfig
	err := h.db.QueryRow(r.Context(), `
		INSERT INTO google_calendar_config (user_id, access_token, refresh_token, token_expiry, google_email, is_enabled)
		VALUES ($1, $2, $3, $4, $5, true)
		ON CONFLICT (user_id)
		DO UPDATE SET
			access_token = EXCLUDED.access_token,
			refresh_token = EXCLUDED.refresh_token,
			token_expiry = EXCLUDED.token_expiry,
			google_email = EXCLUDED.google_email,
			is_enabled = true,
			updated_at = now()
		RETURNING id, user_id, is_enabled, sync_direction, calendar_id, google_email, last_sync_at, created_at, updated_at
	`, userID, req.AccessToken, req.RefreshToken, tokenExpiry, req.GoogleEmail).Scan(
		&config.ID, &config.UserID, &config.IsEnabled, &config.SyncDirection,
		&config.CalendarID, &config.GoogleEmail, &config.LastSyncAt,
		&config.CreatedAt, &config.UpdatedAt,
	)

	if err != nil {
		log.Printf("[SaveTokens] ERROR: %v", err)
		http.Error(w, "Failed to save tokens", http.StatusInternalServerError)
		return
	}

	// Retroactive sync: sync all existing tasks and routines to Google Calendar
	go func() {
		ctx := context.Background()
		tasksSynced, _ := h.SyncAllTasksToGoogleCalendar(ctx, userID)
		routinesSynced, _ := h.SyncAllRoutinesToGoogleCalendar(ctx, userID)
		log.Printf("[SaveTokens] Retroactive sync completed for user %s: %d tasks, %d routines", userID, tasksSynced, routinesSynced)
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(GoogleCalendarConfigResponse{
		IsConnected:   true,
		IsEnabled:     config.IsEnabled,
		SyncDirection: config.SyncDirection,
		CalendarID:    config.CalendarID,
		GoogleEmail:   config.GoogleEmail,
		LastSyncAt:    config.LastSyncAt,
	})
}

// UpdateConfig updates sync preferences
// PATCH /google-calendar/config
func (h *Handler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	var req UpdateConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var config GoogleCalendarConfig
	err := h.db.QueryRow(r.Context(), `
		UPDATE google_calendar_config
		SET
			is_enabled = COALESCE($2, is_enabled),
			sync_direction = COALESCE($3, sync_direction),
			calendar_id = COALESCE($4, calendar_id),
			updated_at = now()
		WHERE user_id = $1
		RETURNING id, user_id, is_enabled, sync_direction, calendar_id, google_email, last_sync_at, created_at, updated_at
	`, userID, req.IsEnabled, req.SyncDirection, req.CalendarID).Scan(
		&config.ID, &config.UserID, &config.IsEnabled, &config.SyncDirection,
		&config.CalendarID, &config.GoogleEmail, &config.LastSyncAt,
		&config.CreatedAt, &config.UpdatedAt,
	)

	if err != nil {
		http.Error(w, "Config not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(GoogleCalendarConfigResponse{
		IsConnected:   true,
		IsEnabled:     config.IsEnabled,
		SyncDirection: config.SyncDirection,
		CalendarID:    config.CalendarID,
		GoogleEmail:   config.GoogleEmail,
		LastSyncAt:    config.LastSyncAt,
	})
}

// Disconnect removes Google Calendar connection
// DELETE /google-calendar/config
func (h *Handler) Disconnect(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	// Remove all Google event IDs from tasks
	_, err := h.db.Exec(r.Context(), `
		UPDATE tasks SET google_event_id = NULL, google_calendar_id = NULL, last_synced_at = NULL
		WHERE user_id = $1
	`, userID)
	if err != nil {
		log.Printf("[Disconnect] Error clearing task google IDs: %v", err)
	}

	// Delete all routine Google events from the new table
	_, err = h.db.Exec(r.Context(), `
		DELETE FROM routine_google_events WHERE user_id = $1
	`, userID)
	if err != nil {
		log.Printf("[Disconnect] Error deleting routine_google_events: %v", err)
	}

	// Delete the config
	result, err := h.db.Exec(r.Context(), `
		DELETE FROM google_calendar_config WHERE user_id = $1
	`, userID)

	if err != nil || result.RowsAffected() == 0 {
		http.Error(w, "Config not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// SyncNow triggers an immediate sync including weekly routine check
// POST /google-calendar/sync
func (h *Handler) SyncNow(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	// Get config with tokens
	var config GoogleCalendarConfig
	err := h.db.QueryRow(r.Context(), `
		SELECT id, user_id, access_token, refresh_token, token_expiry, is_enabled, sync_direction, calendar_id, google_email
		FROM google_calendar_config
		WHERE user_id = $1
	`, userID).Scan(
		&config.ID, &config.UserID, &config.AccessToken, &config.RefreshToken,
		&config.TokenExpiry, &config.IsEnabled, &config.SyncDirection,
		&config.CalendarID, &config.GoogleEmail,
	)

	if err != nil {
		http.Error(w, "Google Calendar not connected", http.StatusNotFound)
		return
	}

	if !config.IsEnabled {
		http.Error(w, "Google Calendar sync is disabled", http.StatusBadRequest)
		return
	}

	// Check if token needs refresh
	if time.Now().After(config.TokenExpiry) {
		// Token expired - client needs to refresh
		http.Error(w, "Token expired, please reconnect", http.StatusUnauthorized)
		return
	}

	// Perform sync for tasks
	result, err := h.performSync(r.Context(), userID, config)
	if err != nil {
		log.Printf("[SyncNow] ERROR: %v", err)
		http.Error(w, "Sync failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Check and sync routines if a week has passed
	routinesSynced, wasTriggered, _ := h.CheckAndSyncRoutinesWeekly(r.Context(), userID)
	if wasTriggered {
		log.Printf("[SyncNow] Weekly routine sync triggered: %d routines synced", routinesSynced)
	}

	// Update last sync time
	h.db.Exec(r.Context(), `
		UPDATE google_calendar_config SET last_sync_at = now(), updated_at = now() WHERE user_id = $1
	`, userID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// CheckWeeklySync checks if routines need weekly re-sync and performs it
// GET /google-calendar/check-weekly
func (h *Handler) CheckWeeklySync(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	synced, wasTriggered, err := h.CheckAndSyncRoutinesWeekly(r.Context(), userID)
	if err != nil {
		log.Printf("[CheckWeeklySync] ERROR: %v", err)
		http.Error(w, "Failed to check weekly sync", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"syncTriggered":   wasTriggered,
		"routinesSynced":  synced,
		"message":         fmt.Sprintf("Weekly sync %s", map[bool]string{true: "triggered", false: "not needed"}[wasTriggered]),
	})
}

// SyncTask syncs a single task to Google Calendar
// POST /google-calendar/sync-task/{id}
func (h *Handler) SyncTask(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	taskID := chi.URLParam(r, "id")

	// Get config
	config, err := h.getConfig(r.Context(), userID)
	if err != nil {
		http.Error(w, "Google Calendar not connected", http.StatusNotFound)
		return
	}

	if !config.IsEnabled {
		http.Error(w, "Google Calendar sync is disabled", http.StatusBadRequest)
		return
	}

	// Get task
	var task struct {
		ID             string
		Title          string
		Description    *string
		Date           string
		ScheduledStart *string
		ScheduledEnd   *string
		GoogleEventID  *string
	}

	err = h.db.QueryRow(r.Context(), `
		SELECT id, title, description, date, scheduled_start::text, scheduled_end::text, google_event_id
		FROM tasks
		WHERE id = $1 AND user_id = $2
	`, taskID, userID).Scan(
		&task.ID, &task.Title, &task.Description, &task.Date,
		&task.ScheduledStart, &task.ScheduledEnd, &task.GoogleEventID,
	)

	if err != nil {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}

	// Create or update event in Google Calendar
	eventID, err := h.createOrUpdateGoogleEvent(r.Context(), *config, task.GoogleEventID, task.Title, task.Description, task.Date, task.ScheduledStart, task.ScheduledEnd)
	if err != nil {
		log.Printf("[SyncTask] ERROR: %v", err)
		http.Error(w, "Failed to sync to Google Calendar", http.StatusInternalServerError)
		return
	}

	// Update task with Google event ID
	h.db.Exec(r.Context(), `
		UPDATE tasks SET google_event_id = $1, google_calendar_id = $2, last_synced_at = now()
		WHERE id = $3 AND user_id = $4
	`, eventID, config.CalendarID, taskID, userID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"googleEventId": eventID,
		"status":        "synced",
	})
}

// ==========================================
// HELPER METHODS
// ==========================================

func (h *Handler) getConfig(ctx context.Context, userID string) (*GoogleCalendarConfig, error) {
	var config GoogleCalendarConfig
	err := h.db.QueryRow(ctx, `
		SELECT id, user_id, access_token, refresh_token, token_expiry, is_enabled, sync_direction, calendar_id, google_email
		FROM google_calendar_config
		WHERE user_id = $1
	`, userID).Scan(
		&config.ID, &config.UserID, &config.AccessToken, &config.RefreshToken,
		&config.TokenExpiry, &config.IsEnabled, &config.SyncDirection,
		&config.CalendarID, &config.GoogleEmail,
	)

	if err != nil {
		return nil, err
	}

	return &config, nil
}

func (h *Handler) performSync(ctx context.Context, userID string, config GoogleCalendarConfig) (*SyncResult, error) {
	result := &SyncResult{
		LastSyncAt: time.Now().Format(time.RFC3339),
		Errors:     []string{},
	}

	// Sync tasks to Google Calendar
	if config.SyncDirection == "bidirectional" || config.SyncDirection == "to_google" {
		synced, errors := h.syncTasksToGoogle(ctx, userID, config)
		result.TasksSynced = synced
		result.Errors = append(result.Errors, errors...)
	}

	// Import events from Google Calendar
	if config.SyncDirection == "bidirectional" || config.SyncDirection == "from_google" {
		imported, errors := h.importEventsFromGoogle(ctx, userID, config)
		result.EventsImported = imported
		result.Errors = append(result.Errors, errors...)
	}

	return result, nil
}

func (h *Handler) syncTasksToGoogle(ctx context.Context, userID string, config GoogleCalendarConfig) (int, []string) {
	var synced int
	var errors []string

	// Get tasks that need syncing (no google_event_id or updated since last sync)
	rows, err := h.db.Query(ctx, `
		SELECT id, title, description, date, scheduled_start::text, scheduled_end::text, google_event_id
		FROM tasks
		WHERE user_id = $1
		AND date >= CURRENT_DATE
		AND (google_event_id IS NULL OR last_synced_at < updated_at)
		ORDER BY date, scheduled_start
		LIMIT 50
	`, userID)

	if err != nil {
		return 0, []string{err.Error()}
	}
	defer rows.Close()

	for rows.Next() {
		var task struct {
			ID             string
			Title          string
			Description    *string
			Date           string
			ScheduledStart *string
			ScheduledEnd   *string
			GoogleEventID  *string
		}

		if err := rows.Scan(&task.ID, &task.Title, &task.Description, &task.Date, &task.ScheduledStart, &task.ScheduledEnd, &task.GoogleEventID); err != nil {
			errors = append(errors, fmt.Sprintf("Failed to scan task: %v", err))
			continue
		}

		eventID, err := h.createOrUpdateGoogleEvent(ctx, config, task.GoogleEventID, task.Title, task.Description, task.Date, task.ScheduledStart, task.ScheduledEnd)
		if err != nil {
			errors = append(errors, fmt.Sprintf("Failed to sync task %s: %v", task.Title, err))
			continue
		}

		// Update task with Google event ID
		h.db.Exec(ctx, `
			UPDATE tasks SET google_event_id = $1, google_calendar_id = $2, last_synced_at = now()
			WHERE id = $3 AND user_id = $4
		`, eventID, config.CalendarID, task.ID, userID)

		synced++
	}

	return synced, errors
}

func (h *Handler) importEventsFromGoogle(ctx context.Context, userID string, config GoogleCalendarConfig) (int, []string) {
	var imported int
	var errors []string

	log.Printf("[ImportFromGoogle] Starting import for user %s, direction: %s, calendar: %s", userID, config.SyncDirection, config.CalendarID)

	// Fetch events from Google Calendar for the next 30 days
	now := time.Now()
	timeMin := now.Format(time.RFC3339)
	timeMax := now.AddDate(0, 0, 30).Format(time.RFC3339)

	url := fmt.Sprintf(
		"https://www.googleapis.com/calendar/v3/calendars/%s/events?timeMin=%s&timeMax=%s&singleEvents=true&orderBy=startTime&maxResults=100",
		config.CalendarID, timeMin, timeMax,
	)

	log.Printf("[ImportFromGoogle] Fetching from URL: %s", url)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		log.Printf("[ImportFromGoogle] Failed to create request: %v", err)
		return 0, []string{fmt.Sprintf("Failed to create request: %v", err)}
	}

	req.Header.Set("Authorization", "Bearer "+config.AccessToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[ImportFromGoogle] Failed to fetch events: %v", err)
		return 0, []string{fmt.Sprintf("Failed to fetch events: %v", err)}
	}
	defer resp.Body.Close()

	log.Printf("[ImportFromGoogle] Google API response status: %d", resp.StatusCode)

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[ImportFromGoogle] Google API error: %s", string(body))
		return 0, []string{fmt.Sprintf("Google API error %d: %s", resp.StatusCode, string(body))}
	}

	// Parse response
	var eventsResponse struct {
		Items []struct {
			ID          string `json:"id"`
			Summary     string `json:"summary"`
			Description string `json:"description"`
			Status      string `json:"status"` // "confirmed", "tentative", "cancelled"
			Start       struct {
				DateTime string `json:"dateTime"`
				Date     string `json:"date"` // All-day events
			} `json:"start"`
			End struct {
				DateTime string `json:"dateTime"`
				Date     string `json:"date"`
			} `json:"end"`
			Updated string `json:"updated"` // RFC3339 timestamp
		} `json:"items"`
	}

	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &eventsResponse); err != nil {
		return 0, []string{fmt.Sprintf("Failed to parse response: %v", err)}
	}

	log.Printf("[ImportFromGoogle] Found %d events for user %s", len(eventsResponse.Items), userID)

	for _, event := range eventsResponse.Items {
		log.Printf("[ImportFromGoogle] Processing event: ID=%s, Summary=%s, Status=%s", event.ID, event.Summary, event.Status)

		// Skip events created by our app (they have our prefix or are already synced)
		if event.Summary == "" {
			log.Printf("[ImportFromGoogle] Skipping event with empty summary")
			continue
		}

		// Skip cancelled events
		if event.Status == "cancelled" {
			// Check if we have this event in our DB and delete it
			result, err := h.db.Exec(ctx, `
				DELETE FROM tasks WHERE user_id = $1 AND google_event_id = $2
			`, userID, event.ID)
			if err == nil && result.RowsAffected() > 0 {
				log.Printf("[ImportFromGoogle] Deleted cancelled event: %s", event.Summary)
			}
			continue
		}

		// Parse event date and times
		var eventDate, startTime, endTime string

		if event.Start.DateTime != "" {
			// Timed event
			startParsed, err := time.Parse(time.RFC3339, event.Start.DateTime)
			if err != nil {
				errors = append(errors, fmt.Sprintf("Failed to parse start time for %s: %v", event.Summary, err))
				continue
			}
			eventDate = startParsed.Format("2006-01-02")
			startTime = startParsed.Format("15:04")

			if event.End.DateTime != "" {
				endParsed, _ := time.Parse(time.RFC3339, event.End.DateTime)
				endTime = endParsed.Format("15:04")
			}
		} else if event.Start.Date != "" {
			// All-day event
			eventDate = event.Start.Date
		} else {
			continue
		}

		// First check if this is a routine event (stored in routine_google_events)
		var routineID string
		var routineEventDate string
		errRoutine := h.db.QueryRow(ctx, `
			SELECT routine_id, event_date FROM routine_google_events
			WHERE user_id = $1 AND google_event_id = $2
		`, userID, event.ID).Scan(&routineID, &routineEventDate)

		if errRoutine == nil {
			// This IS a routine event - always skip creating a task for it
			log.Printf("[ImportFromGoogle] Event %s is a routine event (routine_id=%s), checking for title change", event.ID, routineID)

			// Remove the 🔄 prefix if present to get clean title
			cleanTitle := event.Summary
			if strings.HasPrefix(cleanTitle, "🔄 ") {
				cleanTitle = strings.TrimPrefix(cleanTitle, "🔄 ")
			} else if strings.HasPrefix(cleanTitle, "🔄") {
				cleanTitle = strings.TrimPrefix(cleanTitle, "🔄")
				cleanTitle = strings.TrimSpace(cleanTitle)
			}

			// Update routine title in our DB only if it changed
			result, err := h.db.Exec(ctx, `
				UPDATE routines SET title = $1
				WHERE id = $2 AND user_id = $3 AND title != $1
			`, cleanTitle, routineID, userID)
			if err != nil {
				errors = append(errors, fmt.Sprintf("Failed to update routine %s: %v", cleanTitle, err))
			} else if result.RowsAffected() > 0 {
				imported++
				log.Printf("[ImportFromGoogle] Updated routine title from Google: %s", cleanTitle)

				// Also update ALL other Google Calendar events for this routine
				go func(rID, uID, title string, cfg GoogleCalendarConfig) {
					ctx := context.Background()
					newTitle := "🔄 " + title

					rows, err := h.db.Query(ctx, `
						SELECT google_event_id, google_calendar_id FROM routine_google_events
						WHERE routine_id = $1 AND user_id = $2
					`, rID, uID)
					if err != nil {
						log.Printf("[ImportFromGoogle] Failed to get routine events: %v", err)
						return
					}
					defer rows.Close()

					for rows.Next() {
						var eventID, calendarID string
						if err := rows.Scan(&eventID, &calendarID); err != nil {
							continue
						}

						payload := map[string]interface{}{"summary": newTitle}
						payloadBytes, _ := json.Marshal(payload)

						url := fmt.Sprintf("https://www.googleapis.com/calendar/v3/calendars/%s/events/%s", calendarID, eventID)
						req, err := http.NewRequestWithContext(ctx, "PATCH", url, bytes.NewReader(payloadBytes))
						if err != nil {
							continue
						}
						req.Header.Set("Authorization", "Bearer "+cfg.AccessToken)
						req.Header.Set("Content-Type", "application/json")

						client := &http.Client{Timeout: 10 * time.Second}
						resp, err := client.Do(req)
						if err != nil {
							continue
						}
						resp.Body.Close()

						if resp.StatusCode == 200 {
							log.Printf("[ImportFromGoogle] Updated Google event %s with new title: %s", eventID, newTitle)
						}
					}
				}(routineID, userID, cleanTitle, config)
			} else {
				log.Printf("[ImportFromGoogle] Routine %s title unchanged, skipping", routineID)
			}
			// ALWAYS continue here - never create a task for a routine event
			continue
		}

		// Check if this event already exists in our tasks table (by google_event_id)
		var existingTaskID string
		var existingUpdatedAt time.Time
		err := h.db.QueryRow(ctx, `
			SELECT id, updated_at FROM tasks WHERE user_id = $1 AND google_event_id = $2
		`, userID, event.ID).Scan(&existingTaskID, &existingUpdatedAt)

		if err == nil {
			// Event exists - check if Google version is newer
			googleUpdated, _ := time.Parse(time.RFC3339, event.Updated)
			if googleUpdated.After(existingUpdatedAt) {
				// Google version is newer - update our task
				_, err = h.db.Exec(ctx, `
					UPDATE tasks SET
						title = $1,
						description = $2,
						date = $3,
						scheduled_start = $4::time,
						scheduled_end = $5::time,
						updated_at = now(),
						last_synced_at = now()
					WHERE id = $6 AND user_id = $7
				`, event.Summary, event.Description, eventDate, nullIfEmpty(startTime), nullIfEmpty(endTime), existingTaskID, userID)
				if err != nil {
					errors = append(errors, fmt.Sprintf("Failed to update task %s: %v", event.Summary, err))
				} else {
					imported++
					log.Printf("[ImportFromGoogle] Updated task from Google: %s", event.Summary)
				}
			}
		} else {
			// New event from Google - create task in our DB
			// Skip if it looks like it was created by our app (has routine prefix)
			if len(event.Summary) > 2 && (event.Summary[:2] == "🔄" || strings.HasPrefix(event.Summary, "🔄")) {
				continue // Skip routine events that we don't have tracked
			}

			var description *string
			if event.Description != "" {
				description = &event.Description
			}

			_, err = h.db.Exec(ctx, `
				INSERT INTO tasks (user_id, title, description, date, scheduled_start, scheduled_end, google_event_id, google_calendar_id, last_synced_at)
				VALUES ($1, $2, $3, $4, $5::time, $6::time, $7, $8, now())
			`, userID, event.Summary, description, eventDate, nullIfEmpty(startTime), nullIfEmpty(endTime), event.ID, config.CalendarID)

			if err != nil {
				errors = append(errors, fmt.Sprintf("Failed to create task %s: %v", event.Summary, err))
			} else {
				imported++
				log.Printf("[ImportFromGoogle] Created task from Google: %s on %s", event.Summary, eventDate)
			}
		}
	}

	log.Printf("[ImportFromGoogle] Imported %d events for user %s", imported, userID)
	return imported, errors
}

// nullIfEmpty returns nil if the string is empty, otherwise returns a pointer to it
func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (h *Handler) createOrUpdateGoogleEvent(ctx context.Context, config GoogleCalendarConfig, existingEventID *string, title string, description *string, date string, startTime, endTime *string) (string, error) {
	// Build event times - Google Calendar requires RFC3339 format
	startDateTime := date + "T09:00:00"
	endDateTime := date + "T10:00:00"

	if startTime != nil && *startTime != "" {
		// Handle various time formats from DB (HH:MM, HH:MM:SS, HH:MM:SS.microseconds)
		t := *startTime
		// Remove microseconds if present (e.g., "09:30:00.000000" -> "09:30:00")
		if idx := len(t); idx > 8 {
			t = t[:8]
		}
		// Ensure we have HH:MM:SS format
		if len(t) == 5 {
			t = t + ":00" // HH:MM -> HH:MM:SS
		}
		startDateTime = date + "T" + t
	}
	if endTime != nil && *endTime != "" {
		t := *endTime
		if idx := len(t); idx > 8 {
			t = t[:8]
		}
		if len(t) == 5 {
			t = t + ":00"
		}
		endDateTime = date + "T" + t
	}

	// Build event payload
	eventPayload := map[string]interface{}{
		"summary": title,
		"start": map[string]string{
			"dateTime": startDateTime,
			"timeZone": "Europe/Paris",
		},
		"end": map[string]string{
			"dateTime": endDateTime,
			"timeZone": "Europe/Paris",
		},
	}

	if description != nil && *description != "" {
		eventPayload["description"] = *description
	}

	// Marshal payload to JSON
	payloadBytes, err := json.Marshal(eventPayload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal event payload: %v", err)
	}

	// Make API call to Google Calendar
	var url string
	var method string

	if existingEventID != nil && *existingEventID != "" {
		// Update existing event
		url = fmt.Sprintf("https://www.googleapis.com/calendar/v3/calendars/%s/events/%s", config.CalendarID, *existingEventID)
		method = "PATCH"
	} else {
		// Create new event
		url = fmt.Sprintf("https://www.googleapis.com/calendar/v3/calendars/%s/events", config.CalendarID)
		method = "POST"
	}

	log.Printf("[GoogleCalendar] %s %s - title: %s, start: %s, end: %s, payload: %s", method, url, title, startDateTime, endDateTime, string(payloadBytes))

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(payloadBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+config.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call Google Calendar API: %v", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %v", err)
	}

	log.Printf("[GoogleCalendar] Response status: %d", resp.StatusCode)

	if resp.StatusCode == 401 {
		return "", fmt.Errorf("token expired, please reconnect to Google Calendar")
	}

	if resp.StatusCode != 200 {
		log.Printf("[GoogleCalendar] Error response: %s", string(body))
		return "", fmt.Errorf("Google Calendar API error: %d - %s", resp.StatusCode, string(body))
	}

	// Parse response to get event ID
	var eventResponse struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &eventResponse); err != nil {
		return "", fmt.Errorf("failed to parse response: %v", err)
	}

	log.Printf("[GoogleCalendar] Created/updated event: %s", eventResponse.ID)
	return eventResponse.ID, nil
}

// createOrUpdateWeeklyRoutineEvents creates or updates events for a routine (one per day for the next 7 days)
// If an event already exists for a date, it updates it. Otherwise, it creates a new one.
func (h *Handler) createOrUpdateWeeklyRoutineEvents(ctx context.Context, config GoogleCalendarConfig, userID, routineID, title string, startTime, endTime *string) (int, int, error) {
	eventsCreated := 0
	eventsUpdated := 0

	// Create/update events for the next 7 days
	for i := 0; i < 7; i++ {
		eventDate := time.Now().AddDate(0, 0, i)
		date := eventDate.Format("2006-01-02")

		// Build event times - ensure proper HH:MM:SS format
		startDateTime := date + "T09:00:00"
		endDateTime := date + "T09:30:00"

		if startTime != nil && *startTime != "" {
			t := *startTime
			// Remove microseconds if present
			if len(t) > 8 {
				t = t[:8]
			}
			// Ensure HH:MM:SS format
			if len(t) == 5 {
				t = t + ":00"
			}
			startDateTime = date + "T" + t
		}
		if endTime != nil && *endTime != "" {
			t := *endTime
			if len(t) > 8 {
				t = t[:8]
			}
			if len(t) == 5 {
				t = t + ":00"
			}
			endDateTime = date + "T" + t
		}

		// Build event payload
		eventPayload := map[string]interface{}{
			"summary": title,
			"start": map[string]string{
				"dateTime": startDateTime,
				"timeZone": "Europe/Paris",
			},
			"end": map[string]string{
				"dateTime": endDateTime,
				"timeZone": "Europe/Paris",
			},
		}

		// Marshal payload to JSON
		payloadBytes, err := json.Marshal(eventPayload)
		if err != nil {
			log.Printf("[GoogleCalendar] Failed to marshal event payload for day %d: %v", i, err)
			continue
		}

		// Check if event already exists for this date
		var existingEventID, existingCalendarID string
		err = h.db.QueryRow(ctx, `
			SELECT google_event_id, google_calendar_id FROM routine_google_events
			WHERE routine_id = $1 AND event_date = $2
		`, routineID, date).Scan(&existingEventID, &existingCalendarID)

		var url, method string
		if err == nil && existingEventID != "" {
			// Event exists - UPDATE it
			url = fmt.Sprintf("https://www.googleapis.com/calendar/v3/calendars/%s/events/%s", existingCalendarID, existingEventID)
			method = "PATCH"
			log.Printf("[GoogleCalendar] PATCH %s - updating event for %s", url, date)
		} else {
			// Event doesn't exist - CREATE it
			url = fmt.Sprintf("https://www.googleapis.com/calendar/v3/calendars/%s/events", config.CalendarID)
			method = "POST"
			log.Printf("[GoogleCalendar] POST %s - creating event for %s (day %d/7)", url, date, i+1)
		}

		req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(payloadBytes))
		if err != nil {
			log.Printf("[GoogleCalendar] Failed to create request for day %d: %v", i, err)
			continue
		}

		req.Header.Set("Authorization", "Bearer "+config.AccessToken)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("[GoogleCalendar] Failed to call API for day %d: %v", i, err)
			continue
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == 401 {
			return eventsCreated, eventsUpdated, fmt.Errorf("token expired, please reconnect to Google Calendar")
		}

		if resp.StatusCode != 200 {
			log.Printf("[GoogleCalendar] Error response for day %d: %s", i, string(body))
			continue
		}

		// Parse response to get event ID
		var eventResponse struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(body, &eventResponse); err != nil {
			log.Printf("[GoogleCalendar] Failed to parse response for day %d: %v", i, err)
			continue
		}

		// Save to routine_google_events table (insert or update)
		_, err = h.db.Exec(ctx, `
			INSERT INTO routine_google_events (routine_id, user_id, google_event_id, google_calendar_id, event_date)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (routine_id, event_date) DO UPDATE SET
				google_event_id = EXCLUDED.google_event_id,
				google_calendar_id = EXCLUDED.google_calendar_id
		`, routineID, userID, eventResponse.ID, config.CalendarID, date)

		if err != nil {
			log.Printf("[GoogleCalendar] Failed to save event to DB for day %d: %v", i, err)
		}

		if method == "POST" {
			eventsCreated++
			log.Printf("[GoogleCalendar] Created event for %s: %s", date, eventResponse.ID)
		} else {
			eventsUpdated++
			log.Printf("[GoogleCalendar] Updated event for %s: %s", date, eventResponse.ID)
		}
	}

	log.Printf("[GoogleCalendar] Routine %s: %d created, %d updated", routineID, eventsCreated, eventsUpdated)
	return eventsCreated, eventsUpdated, nil
}

// deleteRoutineGoogleEvents deletes all Google Calendar events for a routine
func (h *Handler) deleteRoutineGoogleEvents(ctx context.Context, config GoogleCalendarConfig, userID, routineID string) (int, error) {
	// Get all events for this routine
	rows, err := h.db.Query(ctx, `
		SELECT google_event_id, google_calendar_id FROM routine_google_events
		WHERE routine_id = $1 AND user_id = $2
	`, routineID, userID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	deleted := 0
	for rows.Next() {
		var googleEventID, googleCalendarID string
		if err := rows.Scan(&googleEventID, &googleCalendarID); err != nil {
			continue
		}

		// Delete from Google Calendar
		url := fmt.Sprintf("https://www.googleapis.com/calendar/v3/calendars/%s/events/%s", googleCalendarID, googleEventID)
		req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
		if err != nil {
			log.Printf("[GoogleCalendar] Failed to create delete request: %v", err)
			continue
		}

		req.Header.Set("Authorization", "Bearer "+config.AccessToken)

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("[GoogleCalendar] Failed to delete event %s: %v", googleEventID, err)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == 204 || resp.StatusCode == 200 || resp.StatusCode == 404 {
			deleted++
			log.Printf("[GoogleCalendar] Deleted event %s", googleEventID)
		}
	}

	// Delete from database
	_, err = h.db.Exec(ctx, `DELETE FROM routine_google_events WHERE routine_id = $1 AND user_id = $2`, routineID, userID)
	if err != nil {
		log.Printf("[GoogleCalendar] Failed to delete events from DB: %v", err)
	}

	return deleted, nil
}

// deleteOldRoutineEvents deletes Google Calendar events for past dates
func (h *Handler) deleteOldRoutineEvents(ctx context.Context, config GoogleCalendarConfig, userID string) (int, error) {
	// Get all events for dates before today
	rows, err := h.db.Query(ctx, `
		SELECT id, google_event_id, google_calendar_id FROM routine_google_events
		WHERE user_id = $1 AND event_date < CURRENT_DATE
	`, userID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	deleted := 0
	var idsToDelete []string
	for rows.Next() {
		var id, googleEventID, googleCalendarID string
		if err := rows.Scan(&id, &googleEventID, &googleCalendarID); err != nil {
			continue
		}

		// Delete from Google Calendar
		url := fmt.Sprintf("https://www.googleapis.com/calendar/v3/calendars/%s/events/%s", googleCalendarID, googleEventID)
		req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
		if err != nil {
			continue
		}

		req.Header.Set("Authorization", "Bearer "+config.AccessToken)

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == 204 || resp.StatusCode == 200 || resp.StatusCode == 404 {
			idsToDelete = append(idsToDelete, id)
			deleted++
		}
	}

	// Delete from database
	if len(idsToDelete) > 0 {
		_, err = h.db.Exec(ctx, `DELETE FROM routine_google_events WHERE user_id = $1 AND event_date < CURRENT_DATE`, userID)
		if err != nil {
			log.Printf("[GoogleCalendar] Failed to delete old events from DB: %v", err)
		}
	}

	log.Printf("[GoogleCalendar] Deleted %d old routine events for user %s", deleted, userID)
	return deleted, nil
}

// SyncTaskToGoogleCalendar syncs a task to Google Calendar (called from calendar handler)
// This is an exported function that can be called when a task is created/updated
func (h *Handler) SyncTaskToGoogleCalendar(ctx context.Context, userID, taskID, title string, description *string, date string, startTime, endTime *string) error {
	// Get config
	config, err := h.getConfig(ctx, userID)
	if err != nil {
		log.Printf("[SyncTaskToGoogleCalendar] No Google config for user %s: %v", userID, err)
		return nil // Not an error - user just doesn't have Google Calendar connected
	}

	if !config.IsEnabled {
		log.Printf("[SyncTaskToGoogleCalendar] Google Calendar sync is disabled for user %s", userID)
		return nil
	}

	// Check sync direction
	if config.SyncDirection != "bidirectional" && config.SyncDirection != "to_google" {
		log.Printf("[SyncTaskToGoogleCalendar] Sync direction is %s, not syncing to Google", config.SyncDirection)
		return nil
	}

	// Check if token is expired
	if time.Now().After(config.TokenExpiry) {
		log.Printf("[SyncTaskToGoogleCalendar] Token expired for user %s", userID)
		return nil // Don't fail the task creation, just skip sync
	}

	// Get existing google_event_id if any
	var googleEventID *string
	h.db.QueryRow(ctx, `SELECT google_event_id FROM tasks WHERE id = $1 AND user_id = $2`, taskID, userID).Scan(&googleEventID)

	// Create or update event
	eventID, err := h.createOrUpdateGoogleEvent(ctx, *config, googleEventID, title, description, date, startTime, endTime)
	if err != nil {
		log.Printf("[SyncTaskToGoogleCalendar] Failed to sync task %s: %v", taskID, err)
		return err
	}

	// Update task with Google event ID
	_, err = h.db.Exec(ctx, `
		UPDATE tasks SET google_event_id = $1, google_calendar_id = $2, last_synced_at = now()
		WHERE id = $3 AND user_id = $4
	`, eventID, config.CalendarID, taskID, userID)

	if err != nil {
		log.Printf("[SyncTaskToGoogleCalendar] Failed to update task with event ID: %v", err)
	}

	log.Printf("[SyncTaskToGoogleCalendar] Successfully synced task %s to Google Calendar event %s", taskID, eventID)
	return nil
}

// DeleteGoogleCalendarEvent deletes an event from Google Calendar
func (h *Handler) DeleteGoogleCalendarEvent(ctx context.Context, userID, googleEventID string) error {
	if googleEventID == "" {
		return nil
	}

	config, err := h.getConfig(ctx, userID)
	if err != nil || !config.IsEnabled {
		return nil
	}

	url := fmt.Sprintf("https://www.googleapis.com/calendar/v3/calendars/%s/events/%s", config.CalendarID, googleEventID)

	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+config.AccessToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 204 && resp.StatusCode != 200 && resp.StatusCode != 404 {
		return fmt.Errorf("failed to delete event: %d", resp.StatusCode)
	}

	log.Printf("[GoogleCalendar] Deleted event %s", googleEventID)
	return nil
}

// DeleteRoutineGoogleEvents deletes all Google Calendar events for a routine (exported version)
func (h *Handler) DeleteRoutineGoogleEvents(ctx context.Context, userID, routineID string) error {
	config, err := h.getConfig(ctx, userID)
	if err != nil || !config.IsEnabled {
		return nil
	}

	deleted, err := h.deleteRoutineGoogleEvents(ctx, *config, userID, routineID)
	if err != nil {
		log.Printf("[DeleteRoutineGoogleEvents] Error deleting events for routine %s: %v", routineID, err)
		return err
	}

	log.Printf("[DeleteRoutineGoogleEvents] Deleted %d events for routine %s", deleted, routineID)
	return nil
}

// SyncRoutineToGoogleCalendar syncs a routine to Google Calendar for the next 7 days
// Creates new events or updates existing ones - does NOT delete old events
func (h *Handler) SyncRoutineToGoogleCalendar(ctx context.Context, userID, routineID, title string, scheduledTime *string, durationMinutes *int) error {
	config, err := h.getConfig(ctx, userID)
	if err != nil {
		log.Printf("[SyncRoutineToGoogleCalendar] No Google config for user %s: %v", userID, err)
		return nil
	}

	if !config.IsEnabled {
		log.Printf("[SyncRoutineToGoogleCalendar] Google Calendar sync is disabled for user %s", userID)
		return nil
	}

	if config.SyncDirection != "bidirectional" && config.SyncDirection != "to_google" {
		log.Printf("[SyncRoutineToGoogleCalendar] Sync direction is %s, not syncing to Google", config.SyncDirection)
		return nil
	}

	if time.Now().After(config.TokenExpiry) {
		log.Printf("[SyncRoutineToGoogleCalendar] Token expired for user %s", userID)
		return nil
	}

	// Build event times - default to 09:00 if no scheduled time
	startTime := "09:00"
	if scheduledTime != nil && *scheduledTime != "" {
		startTime = *scheduledTime
	}

	// Parse start time to calculate end time (use duration or default 30 minutes)
	duration := 30
	if durationMinutes != nil && *durationMinutes > 0 {
		duration = *durationMinutes
	}
	startParsed, _ := time.Parse("15:04", startTime)
	endParsed := startParsed.Add(time.Duration(duration) * time.Minute)
	endTime := endParsed.Format("15:04")

	// Create event with routine prefix
	eventTitle := "🔄 " + title

	// Create or update events for the next 7 days
	eventsCreated, eventsUpdated, err := h.createOrUpdateWeeklyRoutineEvents(ctx, *config, userID, routineID, eventTitle, &startTime, &endTime)
	if err != nil {
		log.Printf("[SyncRoutineToGoogleCalendar] Failed to sync routine %s: %v", routineID, err)
		return err
	}

	log.Printf("[SyncRoutineToGoogleCalendar] Routine %s synced: %d created, %d updated (start: %s, duration: %d min)", routineID, eventsCreated, eventsUpdated, startTime, duration)
	return nil
}

// SyncAllRoutinesToGoogleCalendar syncs all routines for a user (called when enabling sync)
// First cleans up old events (past dates), then creates new events for the next 7 days
func (h *Handler) SyncAllRoutinesToGoogleCalendar(ctx context.Context, userID string) (int, error) {
	config, err := h.getConfig(ctx, userID)
	if err != nil || !config.IsEnabled {
		return 0, nil
	}

	if config.SyncDirection != "bidirectional" && config.SyncDirection != "to_google" {
		return 0, nil
	}

	// First, clean up old events (past dates)
	deletedOld, _ := h.deleteOldRoutineEvents(ctx, *config, userID)
	if deletedOld > 0 {
		log.Printf("[SyncAllRoutines] Cleaned up %d old events for user %s", deletedOld, userID)
	}

	// Get all routines for user
	rows, err := h.db.Query(ctx, `
		SELECT id, title, scheduled_time, duration_minutes
		FROM routines
		WHERE user_id = $1
		ORDER BY created_at
	`, userID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	synced := 0
	for rows.Next() {
		var id, title string
		var scheduledTime *string
		var durationMinutes *int
		if err := rows.Scan(&id, &title, &scheduledTime, &durationMinutes); err != nil {
			log.Printf("[SyncAllRoutines] Failed to scan routine: %v", err)
			continue
		}

		// Sync routine
		if err := h.SyncRoutineToGoogleCalendar(ctx, userID, id, title, scheduledTime, durationMinutes); err != nil {
			log.Printf("[SyncAllRoutines] Failed to sync routine %s: %v", id, err)
			continue
		}
		synced++
	}

	// Update last routine sync timestamp
	h.db.Exec(ctx, `UPDATE google_calendar_config SET last_routine_sync_at = now(), last_sync_at = now() WHERE user_id = $1`, userID)

	log.Printf("[SyncAllRoutines] Synced %d routines for user %s", synced, userID)
	return synced, nil
}

// CheckAndSyncRoutinesWeekly checks if a week has passed since last sync and re-syncs routines
// This should be called periodically (e.g., when app launches or daily)
func (h *Handler) CheckAndSyncRoutinesWeekly(ctx context.Context, userID string) (int, bool, error) {
	config, err := h.getConfig(ctx, userID)
	if err != nil || !config.IsEnabled {
		return 0, false, nil
	}

	// Check if last routine sync was more than 6 days ago
	var lastRoutineSyncAt *time.Time
	h.db.QueryRow(ctx, `SELECT last_routine_sync_at FROM google_calendar_config WHERE user_id = $1`, userID).Scan(&lastRoutineSyncAt)

	// If never synced or last sync was more than 6 days ago, sync again
	needsSync := lastRoutineSyncAt == nil || time.Since(*lastRoutineSyncAt) > (6*24*time.Hour)

	if !needsSync {
		log.Printf("[CheckAndSyncRoutinesWeekly] No sync needed for user %s (last routine sync: %v)", userID, lastRoutineSyncAt)
		return 0, false, nil
	}

	log.Printf("[CheckAndSyncRoutinesWeekly] Weekly sync triggered for user %s (last routine sync: %v)", userID, lastRoutineSyncAt)
	synced, err := h.SyncAllRoutinesToGoogleCalendar(ctx, userID)
	return synced, true, err
}

// SyncAllTasksToGoogleCalendar syncs all future tasks for a user (called when enabling sync)
func (h *Handler) SyncAllTasksToGoogleCalendar(ctx context.Context, userID string) (int, error) {
	config, err := h.getConfig(ctx, userID)
	if err != nil || !config.IsEnabled {
		return 0, nil
	}

	if config.SyncDirection != "bidirectional" && config.SyncDirection != "to_google" {
		return 0, nil
	}

	// Get all future tasks for user
	rows, err := h.db.Query(ctx, `
		SELECT id, title, description, date, scheduled_start::text, scheduled_end::text, google_event_id
		FROM tasks
		WHERE user_id = $1 AND date >= CURRENT_DATE
		ORDER BY date, scheduled_start
	`, userID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	synced := 0
	for rows.Next() {
		var id, title, date string
		var description, scheduledStart, scheduledEnd, googleEventID *string
		if err := rows.Scan(&id, &title, &description, &date, &scheduledStart, &scheduledEnd, &googleEventID); err != nil {
			continue
		}

		// Sync task
		if err := h.SyncTaskToGoogleCalendar(ctx, userID, id, title, description, date, scheduledStart, scheduledEnd); err != nil {
			log.Printf("[SyncAllTasks] Failed to sync task %s: %v", id, err)
			continue
		}
		synced++
	}

	log.Printf("[SyncAllTasks] Synced %d tasks for user %s", synced, userID)
	return synced, nil
}
