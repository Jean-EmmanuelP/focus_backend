package notifications

import (
	"encoding/json"
	"log"
	"net/http"

	"firelevel-backend/internal/auth"
)

// Handler handles notification-related HTTP requests
type Handler struct {
	repo     *Repository
	firebase *FirebaseService
}

// NewHandler creates a new notifications handler
func NewHandler(repo *Repository) *Handler {
	return &Handler{
		repo:     repo,
		firebase: GetFirebaseService(),
	}
}

// ===== Request/Response Types =====

type RegisterTokenRequest struct {
	FCMToken string `json:"fcm_token"`
	Platform string `json:"platform"` // ios, android
}

type UnregisterTokenRequest struct {
	FCMToken string `json:"fcm_token"`
}

type TrackNotificationRequest struct {
	NotificationID string `json:"notification_id"`
	Event          string `json:"event"`  // opened, converted
	Action         string `json:"action"` // optional, for converted events
}

type UpdatePreferencesRequest struct {
	MorningReminderEnabled    *bool   `json:"morning_reminder_enabled,omitempty"`
	MorningReminderTime       *string `json:"morning_reminder_time,omitempty"`
	TaskRemindersEnabled      *bool   `json:"task_reminders_enabled,omitempty"`
	TaskReminderMinutesBefore *int    `json:"task_reminder_minutes_before,omitempty"`
	EveningReminderEnabled    *bool   `json:"evening_reminder_enabled,omitempty"`
	EveningReminderTime       *string `json:"evening_reminder_time,omitempty"`
	StreakAlertEnabled        *bool   `json:"streak_alert_enabled,omitempty"`
	WeeklySummaryEnabled      *bool   `json:"weekly_summary_enabled,omitempty"`
	Language                  *string `json:"language,omitempty"`
	Timezone                  *string `json:"timezone,omitempty"`
}

// ===== Handlers =====

// RegisterToken registers a device FCM token
// POST /notifications/token
func (h *Handler) RegisterToken(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req RegisterTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.FCMToken == "" {
		http.Error(w, "FCM token is required", http.StatusBadRequest)
		return
	}

	if req.Platform == "" {
		req.Platform = "ios" // default
	}

	if err := h.repo.SaveDeviceToken(r.Context(), userID, req.FCMToken, req.Platform); err != nil {
		log.Printf("❌ Failed to save device token: %v", err)
		http.Error(w, "Failed to register token", http.StatusInternalServerError)
		return
	}

	log.Printf("✅ FCM token registered for user %s", userID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "Token registered successfully",
	})
}

// UnregisterToken removes a device FCM token
// POST /notifications/token/unregister
func (h *Handler) UnregisterToken(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req UnregisterTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.FCMToken == "" {
		http.Error(w, "FCM token is required", http.StatusBadRequest)
		return
	}

	if err := h.repo.DeactivateDeviceToken(r.Context(), userID, req.FCMToken); err != nil {
		log.Printf("❌ Failed to deactivate device token: %v", err)
		http.Error(w, "Failed to unregister token", http.StatusInternalServerError)
		return
	}

	log.Printf("✅ FCM token unregistered for user %s", userID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "Token unregistered successfully",
	})
}

// TrackNotification tracks notification events (opened, converted)
// POST /notifications/track
func (h *Handler) TrackNotification(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req TrackNotificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.NotificationID == "" || req.Event == "" {
		http.Error(w, "notification_id and event are required", http.StatusBadRequest)
		return
	}

	var err error
	switch req.Event {
	case "opened":
		err = h.repo.UpdateNotificationStatus(r.Context(), req.NotificationID, "opened")
	case "converted":
		err = h.repo.UpdateNotificationConverted(r.Context(), req.NotificationID, req.Action)
	default:
		http.Error(w, "Invalid event type", http.StatusBadRequest)
		return
	}

	if err != nil {
		log.Printf("❌ Failed to track notification: %v", err)
		http.Error(w, "Failed to track notification", http.StatusInternalServerError)
		return
	}

	log.Printf("📊 Notification %s tracked: %s (user: %s)", req.NotificationID, req.Event, userID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "success",
	})
}

// GetPreferences returns user notification preferences
// GET /notifications/preferences
func (h *Handler) GetPreferences(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	prefs, err := h.repo.GetPreferences(r.Context(), userID)
	if err != nil {
		log.Printf("❌ Failed to get preferences: %v", err)
		http.Error(w, "Failed to get preferences", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(prefs)
}

// UpdatePreferences updates user notification preferences
// PUT /notifications/preferences
func (h *Handler) UpdatePreferences(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req UpdatePreferencesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Get existing preferences
	prefs, err := h.repo.GetPreferences(r.Context(), userID)
	if err != nil {
		log.Printf("❌ Failed to get preferences: %v", err)
		http.Error(w, "Failed to get preferences", http.StatusInternalServerError)
		return
	}

	// Update only provided fields
	if req.MorningReminderEnabled != nil {
		prefs.MorningReminderEnabled = *req.MorningReminderEnabled
	}
	if req.MorningReminderTime != nil {
		prefs.MorningReminderTime = *req.MorningReminderTime
	}
	if req.TaskRemindersEnabled != nil {
		prefs.TaskRemindersEnabled = *req.TaskRemindersEnabled
	}
	if req.TaskReminderMinutesBefore != nil {
		prefs.TaskReminderMinutesBefore = *req.TaskReminderMinutesBefore
	}
	if req.EveningReminderEnabled != nil {
		prefs.EveningReminderEnabled = *req.EveningReminderEnabled
	}
	if req.EveningReminderTime != nil {
		prefs.EveningReminderTime = *req.EveningReminderTime
	}
	if req.StreakAlertEnabled != nil {
		prefs.StreakAlertEnabled = *req.StreakAlertEnabled
	}
	if req.WeeklySummaryEnabled != nil {
		prefs.WeeklySummaryEnabled = *req.WeeklySummaryEnabled
	}
	if req.Language != nil {
		prefs.Language = *req.Language
	}
	if req.Timezone != nil {
		prefs.Timezone = *req.Timezone
	}

	// Save updated preferences
	if err := h.repo.SavePreferences(r.Context(), prefs); err != nil {
		log.Printf("❌ Failed to save preferences: %v", err)
		http.Error(w, "Failed to save preferences", http.StatusInternalServerError)
		return
	}

	log.Printf("✅ Preferences updated for user %s", userID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(prefs)
}

// GetStats returns notification analytics for the user
// GET /notifications/stats
func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Default to last 30 days
	days := 30

	stats, err := h.repo.GetNotificationStats(r.Context(), userID, days)
	if err != nil {
		log.Printf("❌ Failed to get notification stats: %v", err)
		http.Error(w, "Failed to get stats", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// GetStatus returns Firebase status (for health check)
// GET /notifications/status
func (h *Handler) GetStatus(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"firebase_available": h.firebase.IsAvailable(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}
