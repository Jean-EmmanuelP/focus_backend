package gcalendar

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"firelevel-backend/internal/auth"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	googlecalendar "google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

// =============================================
// GOOGLE CALENDAR INTEGRATION
// =============================================
// Endpoints that iOS GoogleCalendarService calls:
//   GET    /google-calendar/config
//   POST   /google-calendar/tokens
//   PATCH  /google-calendar/config
//   DELETE /google-calendar/config
//   POST   /google-calendar/sync
// =============================================

// ConfigResponse matches iOS GoogleCalendarConfigResponse
type ConfigResponse struct {
	IsConnected   bool    `json:"is_connected"`
	IsEnabled     bool    `json:"is_enabled"`
	SyncDirection string  `json:"sync_direction"`
	CalendarID    string  `json:"calendar_id"`
	GoogleEmail   *string `json:"google_email"`
	LastSyncAt    *string `json:"last_sync_at,omitempty"`
}

// SaveTokensRequest from iOS
type SaveTokensRequest struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	GoogleEmail  string `json:"google_email"`
}

// UpdateConfigRequest from iOS
type UpdateConfigRequest struct {
	IsEnabled     *bool   `json:"is_enabled,omitempty"`
	SyncDirection *string `json:"sync_direction,omitempty"`
	CalendarID    *string `json:"calendar_id,omitempty"`
}

// SyncResult matches iOS GoogleSyncResult
type SyncResult struct {
	TasksSynced    int      `json:"tasks_synced"`
	EventsImported int      `json:"events_imported"`
	Errors         []string `json:"errors,omitempty"`
	LastSyncAt     string   `json:"last_sync_at"`
}

// Handler holds dependencies
type Handler struct {
	db          *pgxpool.Pool
	oauthConfig *oauth2.Config
}

// NewHandler creates a new Google Calendar handler
func NewHandler(db *pgxpool.Pool) *Handler {
	config := &oauth2.Config{
		ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		Scopes: []string{
			"https://www.googleapis.com/auth/calendar",
			"https://www.googleapis.com/auth/userinfo.email",
		},
		Endpoint: google.Endpoint,
	}
	return &Handler{db: db, oauthConfig: config}
}

// GetConfig returns Google Calendar config for the user
// GET /google-calendar/config
func (h *Handler) GetConfig(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	config := h.getConfigFromDB(r.Context(), userID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

// SaveTokens stores OAuth tokens after Google Sign-In on iOS
// POST /google-calendar/tokens
func (h *Handler) SaveTokens(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	var req SaveTokensRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.AccessToken == "" {
		http.Error(w, "access_token is required", http.StatusBadRequest)
		return
	}

	// Calculate token expiry
	var expiry time.Time
	if req.ExpiresIn > 0 {
		expiry = time.Now().Add(time.Duration(req.ExpiresIn) * time.Second)
	} else {
		expiry = time.Now().Add(1 * time.Hour)
	}

	// Upsert into calendar_providers table
	// COALESCE(NULLIF(...)) preserves existing refresh_token if new one is empty
	query := `
		INSERT INTO public.calendar_providers (
			user_id, provider_type, provider_email, access_token,
			refresh_token, token_expiry, is_active, is_connected,
			sync_direction, created_at, updated_at
		)
		VALUES ($1, 'google', $2, $3, $4, $5, true, true, 'from_provider', NOW(), NOW())
		ON CONFLICT (user_id, provider_type, provider_email)
		DO UPDATE SET
			is_connected = true,
			is_active = true,
			access_token = EXCLUDED.access_token,
			refresh_token = COALESCE(NULLIF(EXCLUDED.refresh_token, ''), calendar_providers.refresh_token),
			token_expiry = EXCLUDED.token_expiry,
			updated_at = NOW()
	`

	_, err := h.db.Exec(r.Context(), query,
		userID, req.GoogleEmail, req.AccessToken, req.RefreshToken, expiry,
	)
	if err != nil {
		fmt.Printf("❌ Google Calendar tokens save error: %v\n", err)
		http.Error(w, "Failed to save tokens", http.StatusInternalServerError)
		return
	}

	fmt.Printf("✅ Google Calendar connected for user %s: %s\n", userID, req.GoogleEmail)

	config := h.getConfigFromDB(r.Context(), userID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
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

	// Build dynamic update query
	query := `
		UPDATE public.calendar_providers SET updated_at = NOW()
	`
	args := []interface{}{userID}
	argIdx := 2

	if req.IsEnabled != nil {
		query += fmt.Sprintf(", is_active = $%d", argIdx)
		args = append(args, *req.IsEnabled)
		argIdx++
	}
	if req.SyncDirection != nil {
		query += fmt.Sprintf(", sync_direction = $%d", argIdx)
		args = append(args, *req.SyncDirection)
		argIdx++
	}
	if req.CalendarID != nil {
		query += fmt.Sprintf(", provider_config = jsonb_set(COALESCE(provider_config, '{}'), '{calendar_id}', to_jsonb($%d::text))", argIdx)
		args = append(args, *req.CalendarID)
		argIdx++
	}

	query += " WHERE user_id = $1 AND provider_type = 'google'"

	h.db.Exec(r.Context(), query, args...)

	config := h.getConfigFromDB(r.Context(), userID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

// Disconnect removes Google Calendar connection
// DELETE /google-calendar/config
func (h *Handler) Disconnect(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	query := `
		UPDATE public.calendar_providers
		SET is_connected = false,
			is_active = false,
			access_token = NULL,
			refresh_token = NULL,
			updated_at = NOW()
		WHERE user_id = $1 AND provider_type = 'google'
	`
	h.db.Exec(r.Context(), query, userID)

	// Clean up cached events
	cleanQuery := `
		DELETE FROM public.calendar_events
		WHERE provider_id IN (
			SELECT id FROM public.calendar_providers
			WHERE user_id = $1 AND provider_type = 'google'
		)
	`
	h.db.Exec(r.Context(), cleanQuery, userID)

	w.WriteHeader(http.StatusNoContent)
}

// Sync fetches events from Google Calendar API and caches them
// POST /google-calendar/sync
func (h *Handler) Sync(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	fmt.Printf("📅 Starting Google Calendar sync for user %s\n", userID)

	// Get provider info and tokens
	var providerID, accessToken, refreshToken string
	var tokenExpiry time.Time
	var calendarID string

	query := `
		SELECT id, access_token, COALESCE(refresh_token, ''), token_expiry,
			   COALESCE(provider_config->>'calendar_id', 'primary')
		FROM public.calendar_providers
		WHERE user_id = $1 AND provider_type = 'google' AND is_connected = true
	`
	err := h.db.QueryRow(r.Context(), query, userID).Scan(
		&providerID, &accessToken, &refreshToken, &tokenExpiry, &calendarID,
	)
	if err != nil {
		fmt.Printf("❌ Google Calendar not connected: %v\n", err)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SyncResult{
			Errors:     []string{"Google Calendar not connected"},
			LastSyncAt: time.Now().Format(time.RFC3339),
		})
		return
	}

	// Mark sync in progress
	h.db.Exec(r.Context(), `
		UPDATE public.calendar_providers
		SET last_sync_status = 'in_progress', updated_at = NOW()
		WHERE id = $1
	`, providerID)

	// Create Google Calendar client
	token := &oauth2.Token{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		Expiry:       tokenExpiry,
	}

	ctx := context.Background()
	client := h.oauthConfig.Client(ctx, token)

	calService, err := googlecalendar.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		h.markSyncError(r.Context(), providerID, err)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SyncResult{
			Errors:     []string{"Failed to create Calendar service"},
			LastSyncAt: time.Now().Format(time.RFC3339),
		})
		return
	}

	// Fetch events for the next 7 days
	now := time.Now()
	timeMin := now.Format(time.RFC3339)
	timeMax := now.AddDate(0, 0, 7).Format(time.RFC3339)

	events, err := calService.Events.List(calendarID).
		TimeMin(timeMin).
		TimeMax(timeMax).
		SingleEvents(true).
		OrderBy("startTime").
		MaxResults(100).
		Do()

	if err != nil {
		h.markSyncError(r.Context(), providerID, err)
		fmt.Printf("❌ Google Calendar fetch error: %v\n", err)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SyncResult{
			Errors:     []string{fmt.Sprintf("Failed to fetch events: %v", err)},
			LastSyncAt: time.Now().Format(time.RFC3339),
		})
		return
	}

	// If token was refreshed by the OAuth client, save the new token
	newToken, err := client.Transport.(*oauth2.Transport).Source.Token()
	if err == nil && newToken.AccessToken != accessToken {
		h.db.Exec(r.Context(), `
			UPDATE public.calendar_providers
			SET access_token = $1, token_expiry = $2, updated_at = NOW()
			WHERE id = $3
		`, newToken.AccessToken, newToken.Expiry, providerID)
		fmt.Printf("🔄 Google Calendar token refreshed for user %s\n", userID)
	}

	// Upsert events into calendar_events
	imported := 0
	var syncErrors []string

	for _, event := range events.Items {
		if event.Status == "cancelled" {
			// Mark cancelled events
			h.db.Exec(r.Context(), `
				UPDATE public.calendar_events
				SET event_status = 'cancelled', updated_at = NOW()
				WHERE provider_id = $1 AND external_event_id = $2
			`, providerID, event.Id)
			continue
		}

		startAt, endAt, isAllDay := parseGoogleDateTime(event.Start, event.End)
		if startAt.IsZero() {
			continue
		}

		eventType := "default"
		if event.EventType != "" {
			eventType = event.EventType
		}

		// Auto-enable blocking for focusTime events
		autoBlock := eventType == "focusTime"

		rawData, _ := json.Marshal(event)

		upsertQuery := `
			INSERT INTO public.calendar_events (
				user_id, provider_id, external_event_id, external_calendar_id,
				title, description, location, start_at, end_at,
				is_all_day, timezone, event_status, is_busy, event_type,
				synced_at, raw_data, created_at, updated_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, NOW(), $15, NOW(), NOW())
			ON CONFLICT (provider_id, external_event_id)
			DO UPDATE SET
				title = EXCLUDED.title,
				description = EXCLUDED.description,
				location = EXCLUDED.location,
				start_at = EXCLUDED.start_at,
				end_at = EXCLUDED.end_at,
				is_all_day = EXCLUDED.is_all_day,
				timezone = EXCLUDED.timezone,
				event_status = EXCLUDED.event_status,
				is_busy = EXCLUDED.is_busy,
				event_type = EXCLUDED.event_type,
				synced_at = NOW(),
				raw_data = EXCLUDED.raw_data,
				updated_at = NOW()
		`
		// Note: block_apps is NOT overwritten during sync — user control is preserved

		status := "confirmed"
		if event.Status == "tentative" {
			status = "tentative"
		}

		isBusy := event.Transparency != "transparent"

		_, err := h.db.Exec(r.Context(), upsertQuery,
			userID, providerID, event.Id, calendarID,
			event.Summary, event.Description, event.Location,
			startAt, endAt, isAllDay,
			getTimezone(event.Start), status, isBusy, eventType,
			rawData,
		)

		if err != nil {
			syncErrors = append(syncErrors, fmt.Sprintf("Event %s: %v", event.Id, err))
			continue
		}

		// If auto-block and event is new (not updated), set block_apps
		if autoBlock {
			h.db.Exec(r.Context(), `
				UPDATE public.calendar_events
				SET block_apps = true, block_apps_source = 'auto'
				WHERE provider_id = $1 AND external_event_id = $2
				  AND block_apps = false AND block_apps_source = 'manual'
			`, providerID, event.Id)
		}

		imported++
	}

	// Mark sync success
	h.db.Exec(r.Context(), `
		UPDATE public.calendar_providers
		SET last_sync_at = NOW(),
			last_sync_status = 'success',
			last_sync_error = NULL,
			updated_at = NOW()
		WHERE id = $1
	`, providerID)

	fmt.Printf("✅ Google Calendar sync: %d events imported for user %s\n", imported, userID)

	result := SyncResult{
		TasksSynced:    0,
		EventsImported: imported,
		LastSyncAt:     time.Now().Format(time.RFC3339),
	}
	if len(syncErrors) > 0 {
		result.Errors = syncErrors
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// CheckWeekly is a no-op endpoint for weekly sync checks
// GET /google-calendar/check-weekly
func (h *Handler) CheckWeekly(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	// Check if sync is needed (last sync > 24h ago)
	var lastSyncAt *time.Time
	h.db.QueryRow(r.Context(), `
		SELECT last_sync_at FROM public.calendar_providers
		WHERE user_id = $1 AND provider_type = 'google' AND is_connected = true
	`, userID).Scan(&lastSyncAt)

	needsSync := lastSyncAt == nil || time.Since(*lastSyncAt) > 24*time.Hour

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"needs_sync": needsSync,
	})
}

// --- Helpers ---

func (h *Handler) getConfigFromDB(ctx context.Context, userID string) ConfigResponse {
	var isConnected, isActive bool
	var email *string
	var syncDirection string
	var calendarID string
	var lastSyncAt *time.Time

	query := `
		SELECT is_connected, is_active, COALESCE(provider_email, ''),
			   COALESCE(sync_direction, 'from_provider'),
			   COALESCE(provider_config->>'calendar_id', 'primary'),
			   last_sync_at
		FROM public.calendar_providers
		WHERE user_id = $1 AND provider_type = 'google'
		ORDER BY updated_at DESC LIMIT 1
	`
	err := h.db.QueryRow(ctx, query, userID).Scan(
		&isConnected, &isActive, &email, &syncDirection, &calendarID, &lastSyncAt,
	)

	if err != nil {
		return ConfigResponse{
			IsConnected:   false,
			IsEnabled:     false,
			SyncDirection: "from_provider",
			CalendarID:    "primary",
		}
	}

	var lastSyncStr *string
	if lastSyncAt != nil {
		s := lastSyncAt.Format(time.RFC3339)
		lastSyncStr = &s
	}

	return ConfigResponse{
		IsConnected:   isConnected,
		IsEnabled:     isActive,
		SyncDirection: syncDirection,
		CalendarID:    calendarID,
		GoogleEmail:   email,
		LastSyncAt:    lastSyncStr,
	}
}

func (h *Handler) markSyncError(ctx context.Context, providerID string, err error) {
	h.db.Exec(ctx, `
		UPDATE public.calendar_providers
		SET last_sync_status = 'error',
			last_sync_error = $1,
			updated_at = NOW()
		WHERE id = $2
	`, err.Error(), providerID)
}

// parseGoogleDateTime parses Google Calendar event start/end times
func parseGoogleDateTime(start, end *googlecalendar.EventDateTime) (time.Time, time.Time, bool) {
	if start == nil || end == nil {
		return time.Time{}, time.Time{}, false
	}

	// All-day event (date only, no time)
	if start.Date != "" {
		startDate, _ := time.Parse("2006-01-02", start.Date)
		endDate, _ := time.Parse("2006-01-02", end.Date)
		return startDate, endDate, true
	}

	// Timed event
	if start.DateTime != "" {
		startTime, _ := time.Parse(time.RFC3339, start.DateTime)
		endTime, _ := time.Parse(time.RFC3339, end.DateTime)
		return startTime, endTime, false
	}

	return time.Time{}, time.Time{}, false
}

func getTimezone(dt *googlecalendar.EventDateTime) string {
	if dt != nil && dt.TimeZone != "" {
		return dt.TimeZone
	}
	return ""
}
