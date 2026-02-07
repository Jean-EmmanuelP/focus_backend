package notifications

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"firelevel-backend/internal/auth"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ===========================================
// NOTIFICATIONS - Device Token Management
// For APNs push notifications
// ===========================================

type Handler struct {
	db *pgxpool.Pool
}

func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
}

// DeviceToken represents a registered APNs device
type DeviceToken struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Token     string    `json:"token"`
	Platform  string    `json:"platform"`  // "ios", "ipados"
	AppVersion string   `json:"app_version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// RegisterTokenRequest for registering a device token
type RegisterTokenRequest struct {
	Token      string `json:"token"`
	Platform   string `json:"platform,omitempty"`   // defaults to "ios"
	AppVersion string `json:"app_version,omitempty"`
}

// RegisterToken registers or updates a device token for push notifications
// POST /notifications/device-token
func (h *Handler) RegisterToken(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	var req RegisterTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Token == "" {
		http.Error(w, "Token is required", http.StatusBadRequest)
		return
	}

	platform := req.Platform
	if platform == "" {
		platform = "ios"
	}

	// Upsert: one token per user per platform
	query := `
		INSERT INTO public.device_tokens (user_id, token, platform, app_version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		ON CONFLICT (user_id, platform)
		DO UPDATE SET
			token = EXCLUDED.token,
			app_version = EXCLUDED.app_version,
			updated_at = NOW()
		RETURNING id, token, platform, app_version, created_at, updated_at
	`

	var dt DeviceToken
	err := h.db.QueryRow(r.Context(), query,
		userID, req.Token, platform, req.AppVersion,
	).Scan(&dt.ID, &dt.Token, &dt.Platform, &dt.AppVersion, &dt.CreatedAt, &dt.UpdatedAt)

	if err != nil {
		fmt.Printf("❌ Device token register error: %v\n", err)
		http.Error(w, "Failed to register device token", http.StatusInternalServerError)
		return
	}

	dt.UserID = userID
	fmt.Printf("📱 Device token registered for user %s (platform: %s)\n", userID, platform)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dt)
}

// DeleteToken removes a device token (user logged out or uninstalled)
// DELETE /notifications/device-token
func (h *Handler) DeleteToken(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		// If no body, delete all tokens for this user
		h.db.Exec(r.Context(), `DELETE FROM public.device_tokens WHERE user_id = $1`, userID)
	} else {
		h.db.Exec(r.Context(), `DELETE FROM public.device_tokens WHERE user_id = $1 AND token = $2`, userID, req.Token)
	}

	fmt.Printf("🗑️ Device token deleted for user %s\n", userID)
	w.WriteHeader(http.StatusNoContent)
}

// GetSettings returns notification preferences for the user
// GET /notifications/settings
func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	query := `
		SELECT COALESCE(notification_settings, '{
			"focus_reminders": true,
			"ritual_reminders": true,
			"morning_checkin": true,
			"evening_checkin": true,
			"streak_alerts": true,
			"quest_milestones": true,
			"crew_activity": true,
			"leaderboard_updates": false
		}'::jsonb) as settings
		FROM public.users
		WHERE id = $1
	`

	var settingsJSON []byte
	err := h.db.QueryRow(r.Context(), query, userID).Scan(&settingsJSON)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(settingsJSON)
}

// UpdateSettings updates notification preferences
// PATCH /notifications/settings
func (h *Handler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	var settings json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	query := `
		UPDATE public.users
		SET notification_settings = COALESCE(notification_settings, '{}'::jsonb) || $1::jsonb
		WHERE id = $2
		RETURNING notification_settings
	`

	var updatedJSON []byte
	err := h.db.QueryRow(r.Context(), query, string(settings), userID).Scan(&updatedJSON)
	if err != nil {
		fmt.Printf("❌ Notification settings update error: %v\n", err)
		http.Error(w, "Failed to update settings", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(updatedJSON)
}
