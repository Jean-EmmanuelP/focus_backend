package googlecalendar

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

	// Remove all Google event IDs from routines
	_, err = h.db.Exec(r.Context(), `
		UPDATE routines SET google_event_id = NULL, google_calendar_id = NULL
		WHERE user_id = $1
	`, userID)
	if err != nil {
		log.Printf("[Disconnect] Error clearing routine google IDs: %v", err)
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

// SyncNow triggers an immediate sync
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

	// Perform sync
	result, err := h.performSync(r.Context(), userID, config)
	if err != nil {
		log.Printf("[SyncNow] ERROR: %v", err)
		http.Error(w, "Sync failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Update last sync time
	h.db.Exec(r.Context(), `
		UPDATE google_calendar_config SET last_sync_at = now(), updated_at = now() WHERE user_id = $1
	`, userID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
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
	// This would call Google Calendar API to get events
	// For now, return 0 as this requires the Google API client
	// The iOS app handles this directly with the Google SDK
	return 0, nil
}

func (h *Handler) createOrUpdateGoogleEvent(ctx context.Context, config GoogleCalendarConfig, existingEventID *string, title string, description *string, date string, startTime, endTime *string) (string, error) {
	// Build event times
	startDateTime := date + "T09:00:00"
	endDateTime := date + "T10:00:00"

	if startTime != nil && *startTime != "" {
		startDateTime = date + "T" + *startTime + ":00"
	}
	if endTime != nil && *endTime != "" {
		endDateTime = date + "T" + *endTime + ":00"
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

	log.Printf("[GoogleCalendar] %s %s - title: %s, start: %s, end: %s", method, url, title, startDateTime, endDateTime)

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

// createWeeklyRoutineEvents creates 7 individual events for a routine (one per day for the next 7 days)
func (h *Handler) createWeeklyRoutineEvents(ctx context.Context, config GoogleCalendarConfig, title string, startTime, endTime *string) (string, error) {
	var firstEventID string

	// Create events for the next 7 days
	for i := 0; i < 7; i++ {
		date := time.Now().AddDate(0, 0, i).Format("2006-01-02")

		// Build event times
		startDateTime := date + "T09:00:00"
		endDateTime := date + "T09:30:00"

		if startTime != nil && *startTime != "" {
			startDateTime = date + "T" + *startTime + ":00"
		}
		if endTime != nil && *endTime != "" {
			endDateTime = date + "T" + *endTime + ":00"
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

		// Create new event
		url := fmt.Sprintf("https://www.googleapis.com/calendar/v3/calendars/%s/events", config.CalendarID)

		log.Printf("[GoogleCalendar] POST %s - title: %s, date: %s (day %d/7)", url, title, date, i+1)

		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payloadBytes))
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
			return "", fmt.Errorf("token expired, please reconnect to Google Calendar")
		}

		if resp.StatusCode != 200 {
			log.Printf("[GoogleCalendar] Error response for day %d: %s", i, string(body))
			continue
		}

		// Parse response to get event ID
		var eventResponse struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(body, &eventResponse); err == nil && firstEventID == "" {
			firstEventID = eventResponse.ID
		}

		log.Printf("[GoogleCalendar] Created event for %s: %s", date, eventResponse.ID)
	}

	log.Printf("[GoogleCalendar] Created 7 daily events for routine, first event ID: %s", firstEventID)
	return firstEventID, nil
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

// SyncRoutineToGoogleCalendar syncs a routine to Google Calendar for the next 7 days
// Creates 7 individual events (one per day) for easy viewing
func (h *Handler) SyncRoutineToGoogleCalendar(ctx context.Context, userID, routineID, title string, scheduledTime *string) error {
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

	// Parse start time to calculate end time (30 minutes duration)
	startParsed, _ := time.Parse("15:04", startTime)
	endParsed := startParsed.Add(30 * time.Minute)
	endTime := endParsed.Format("15:04")

	// Create event with routine prefix
	eventTitle := "🔄 " + title

	// Create 7 daily events for the routine
	eventID, err := h.createWeeklyRoutineEvents(ctx, *config, eventTitle, &startTime, &endTime)
	if err != nil {
		log.Printf("[SyncRoutineToGoogleCalendar] Failed to sync routine %s: %v", routineID, err)
		return err
	}

	// Update routine with first Google event ID (for reference)
	_, err = h.db.Exec(ctx, `
		UPDATE routines SET google_event_id = $1, google_calendar_id = $2
		WHERE id = $3 AND user_id = $4
	`, eventID, config.CalendarID, routineID, userID)

	if err != nil {
		log.Printf("[SyncRoutineToGoogleCalendar] Failed to update routine with event ID: %v", err)
	}

	log.Printf("[SyncRoutineToGoogleCalendar] Successfully synced routine %s to Google Calendar (7 days, start: %s)", routineID, startTime)
	return nil
}

// SyncAllRoutinesToGoogleCalendar syncs all routines for a user (called when enabling sync)
func (h *Handler) SyncAllRoutinesToGoogleCalendar(ctx context.Context, userID string) (int, error) {
	config, err := h.getConfig(ctx, userID)
	if err != nil || !config.IsEnabled {
		return 0, nil
	}

	if config.SyncDirection != "bidirectional" && config.SyncDirection != "to_google" {
		return 0, nil
	}

	// Get all routines for user
	rows, err := h.db.Query(ctx, `
		SELECT id, title, scheduled_time, google_event_id
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
		var scheduledTime, googleEventID *string
		if err := rows.Scan(&id, &title, &scheduledTime, &googleEventID); err != nil {
			continue
		}

		// Sync routine
		if err := h.SyncRoutineToGoogleCalendar(ctx, userID, id, title, scheduledTime); err != nil {
			log.Printf("[SyncAllRoutines] Failed to sync routine %s: %v", id, err)
			continue
		}
		synced++
	}

	log.Printf("[SyncAllRoutines] Synced %d routines for user %s", synced, userID)
	return synced, nil
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
