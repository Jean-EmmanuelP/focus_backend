package notifications

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository handles database operations for notifications
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new notifications repository
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// DeviceToken represents a user's FCM token
type DeviceToken struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	FCMToken  string    `json:"fcm_token"`
	Platform  string    `json:"platform"` // ios, android
	DeviceID  string    `json:"device_id,omitempty"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NotificationPreferences represents user notification preferences
type NotificationPreferences struct {
	UserID                    string    `json:"user_id"`
	MorningReminderEnabled    bool      `json:"morning_reminder_enabled"`
	MorningReminderTime       string    `json:"morning_reminder_time"` // HH:MM format
	TaskRemindersEnabled      bool      `json:"task_reminders_enabled"`
	TaskReminderMinutesBefore int       `json:"task_reminder_minutes_before"`
	EveningReminderEnabled    bool      `json:"evening_reminder_enabled"`
	EveningReminderTime       string    `json:"evening_reminder_time"`
	StreakAlertEnabled        bool      `json:"streak_alert_enabled"`
	WeeklySummaryEnabled      bool      `json:"weekly_summary_enabled"`
	Language                  string    `json:"language"` // fr, en
	Timezone                  string    `json:"timezone"`
	CreatedAt                 time.Time `json:"created_at"`
	UpdatedAt                 time.Time `json:"updated_at"`
}

// NotificationEvent represents a tracking event for analytics
type NotificationEvent struct {
	ID             string     `json:"id"`
	UserID         string     `json:"user_id"`
	NotificationID string     `json:"notification_id"`
	Type           string     `json:"type"` // morning, task_reminder, task_missed, evening, streak_danger, weekly_summary
	Status         string     `json:"status"` // scheduled, sent, delivered, opened, converted, failed
	Title          string     `json:"title"`
	Body           string     `json:"body"`
	DeepLink       string     `json:"deep_link,omitempty"`
	Metadata       string     `json:"metadata,omitempty"` // JSON string with extra data
	ScheduledAt    *time.Time `json:"scheduled_at,omitempty"`
	SentAt         *time.Time `json:"sent_at,omitempty"`
	OpenedAt       *time.Time `json:"opened_at,omitempty"`
	ConvertedAt    *time.Time `json:"converted_at,omitempty"`
	Action         string     `json:"action,omitempty"` // what action user took after opening
	ErrorMessage   string     `json:"error_message,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// ===== Device Token Methods =====

// SaveDeviceToken saves or updates a device FCM token
func (r *Repository) SaveDeviceToken(ctx context.Context, userID, fcmToken, platform string) error {
	query := `
		INSERT INTO device_tokens (id, user_id, fcm_token, platform, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, true, NOW(), NOW())
		ON CONFLICT (user_id, fcm_token)
		DO UPDATE SET
			is_active = true,
			platform = EXCLUDED.platform,
			updated_at = NOW()
	`

	_, err := r.pool.Exec(ctx, query, uuid.New().String(), userID, fcmToken, platform)
	if err != nil {
		return fmt.Errorf("failed to save device token: %w", err)
	}
	return nil
}

// DeactivateDeviceToken marks a token as inactive
func (r *Repository) DeactivateDeviceToken(ctx context.Context, userID, fcmToken string) error {
	query := `
		UPDATE device_tokens
		SET is_active = false, updated_at = NOW()
		WHERE user_id = $1 AND fcm_token = $2
	`

	_, err := r.pool.Exec(ctx, query, userID, fcmToken)
	if err != nil {
		return fmt.Errorf("failed to deactivate device token: %w", err)
	}
	return nil
}

// GetActiveTokensForUser returns all active FCM tokens for a user
func (r *Repository) GetActiveTokensForUser(ctx context.Context, userID string) ([]DeviceToken, error) {
	query := `
		SELECT id, user_id, fcm_token, platform, COALESCE(device_id, ''), is_active, created_at, updated_at
		FROM device_tokens
		WHERE user_id = $1 AND is_active = true
		ORDER BY updated_at DESC
	`

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get device tokens: %w", err)
	}
	defer rows.Close()

	var tokens []DeviceToken
	for rows.Next() {
		var t DeviceToken
		if err := rows.Scan(&t.ID, &t.UserID, &t.FCMToken, &t.Platform, &t.DeviceID, &t.IsActive, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan device token: %w", err)
		}
		tokens = append(tokens, t)
	}
	return tokens, nil
}

// GetAllActiveTokens returns all active tokens (for broadcast notifications)
func (r *Repository) GetAllActiveTokens(ctx context.Context) ([]DeviceToken, error) {
	query := `
		SELECT id, user_id, fcm_token, platform, COALESCE(device_id, ''), is_active, created_at, updated_at
		FROM device_tokens
		WHERE is_active = true
		ORDER BY updated_at DESC
	`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get all device tokens: %w", err)
	}
	defer rows.Close()

	var tokens []DeviceToken
	for rows.Next() {
		var t DeviceToken
		if err := rows.Scan(&t.ID, &t.UserID, &t.FCMToken, &t.Platform, &t.DeviceID, &t.IsActive, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan device token: %w", err)
		}
		tokens = append(tokens, t)
	}
	return tokens, nil
}

// DeactivateInvalidToken marks a specific token as inactive (when FCM returns error)
func (r *Repository) DeactivateInvalidToken(ctx context.Context, fcmToken string) error {
	query := `
		UPDATE device_tokens
		SET is_active = false, updated_at = NOW()
		WHERE fcm_token = $1
	`

	_, err := r.pool.Exec(ctx, query, fcmToken)
	if err != nil {
		return fmt.Errorf("failed to deactivate invalid token: %w", err)
	}
	return nil
}

// ===== Notification Preferences Methods =====

// GetPreferences returns notification preferences for a user
func (r *Repository) GetPreferences(ctx context.Context, userID string) (*NotificationPreferences, error) {
	query := `
		SELECT user_id, morning_reminder_enabled, morning_reminder_time,
			   task_reminders_enabled, task_reminder_minutes_before,
			   evening_reminder_enabled, evening_reminder_time,
			   streak_alert_enabled, weekly_summary_enabled,
			   language, timezone, created_at, updated_at
		FROM notification_preferences
		WHERE user_id = $1
	`

	var p NotificationPreferences
	err := r.pool.QueryRow(ctx, query, userID).Scan(
		&p.UserID, &p.MorningReminderEnabled, &p.MorningReminderTime,
		&p.TaskRemindersEnabled, &p.TaskReminderMinutesBefore,
		&p.EveningReminderEnabled, &p.EveningReminderTime,
		&p.StreakAlertEnabled, &p.WeeklySummaryEnabled,
		&p.Language, &p.Timezone, &p.CreatedAt, &p.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		// Return default preferences
		return &NotificationPreferences{
			UserID:                    userID,
			MorningReminderEnabled:    true,
			MorningReminderTime:       "08:00",
			TaskRemindersEnabled:      true,
			TaskReminderMinutesBefore: 15,
			EveningReminderEnabled:    false,
			EveningReminderTime:       "21:00",
			StreakAlertEnabled:        true,
			WeeklySummaryEnabled:      true,
			Language:                  "fr",
			Timezone:                  "Europe/Paris",
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get preferences: %w", err)
	}

	return &p, nil
}

// SavePreferences creates or updates notification preferences
func (r *Repository) SavePreferences(ctx context.Context, p *NotificationPreferences) error {
	query := `
		INSERT INTO notification_preferences (
			user_id, morning_reminder_enabled, morning_reminder_time,
			task_reminders_enabled, task_reminder_minutes_before,
			evening_reminder_enabled, evening_reminder_time,
			streak_alert_enabled, weekly_summary_enabled,
			language, timezone, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW(), NOW())
		ON CONFLICT (user_id)
		DO UPDATE SET
			morning_reminder_enabled = EXCLUDED.morning_reminder_enabled,
			morning_reminder_time = EXCLUDED.morning_reminder_time,
			task_reminders_enabled = EXCLUDED.task_reminders_enabled,
			task_reminder_minutes_before = EXCLUDED.task_reminder_minutes_before,
			evening_reminder_enabled = EXCLUDED.evening_reminder_enabled,
			evening_reminder_time = EXCLUDED.evening_reminder_time,
			streak_alert_enabled = EXCLUDED.streak_alert_enabled,
			weekly_summary_enabled = EXCLUDED.weekly_summary_enabled,
			language = EXCLUDED.language,
			timezone = EXCLUDED.timezone,
			updated_at = NOW()
	`

	_, err := r.pool.Exec(ctx, query,
		p.UserID, p.MorningReminderEnabled, p.MorningReminderTime,
		p.TaskRemindersEnabled, p.TaskReminderMinutesBefore,
		p.EveningReminderEnabled, p.EveningReminderTime,
		p.StreakAlertEnabled, p.WeeklySummaryEnabled,
		p.Language, p.Timezone,
	)

	if err != nil {
		return fmt.Errorf("failed to save preferences: %w", err)
	}
	return nil
}

// ===== Notification Event Methods =====

// CreateNotificationEvent creates a new notification tracking event
func (r *Repository) CreateNotificationEvent(ctx context.Context, event *NotificationEvent) error {
	if event.ID == "" {
		event.ID = uuid.New().String()
	}
	if event.NotificationID == "" {
		event.NotificationID = uuid.New().String()
	}

	query := `
		INSERT INTO notification_events (
			id, user_id, notification_id, type, status, title, body,
			deep_link, metadata, scheduled_at, sent_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
	`

	_, err := r.pool.Exec(ctx, query,
		event.ID, event.UserID, event.NotificationID, event.Type, event.Status,
		event.Title, event.Body, event.DeepLink, event.Metadata,
		event.ScheduledAt, event.SentAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create notification event: %w", err)
	}
	return nil
}

// UpdateNotificationStatus updates the status of a notification
func (r *Repository) UpdateNotificationStatus(ctx context.Context, notificationID, status string) error {
	var query string

	switch status {
	case "sent":
		query = `UPDATE notification_events SET status = $2, sent_at = NOW() WHERE notification_id = $1`
	case "opened":
		query = `UPDATE notification_events SET status = $2, opened_at = NOW() WHERE notification_id = $1`
	case "converted":
		query = `UPDATE notification_events SET status = $2, converted_at = NOW() WHERE notification_id = $1`
	default:
		query = `UPDATE notification_events SET status = $2 WHERE notification_id = $1`
	}

	_, err := r.pool.Exec(ctx, query, notificationID, status)
	if err != nil {
		return fmt.Errorf("failed to update notification status: %w", err)
	}
	return nil
}

// UpdateNotificationConverted marks notification as converted with action
func (r *Repository) UpdateNotificationConverted(ctx context.Context, notificationID, action string) error {
	query := `
		UPDATE notification_events
		SET status = 'converted', converted_at = NOW(), action = $2
		WHERE notification_id = $1
	`

	_, err := r.pool.Exec(ctx, query, notificationID, action)
	if err != nil {
		return fmt.Errorf("failed to update notification conversion: %w", err)
	}
	return nil
}

// MarkNotificationFailed marks a notification as failed with error message
func (r *Repository) MarkNotificationFailed(ctx context.Context, notificationID, errorMessage string) error {
	query := `
		UPDATE notification_events
		SET status = 'failed', error_message = $2
		WHERE notification_id = $1
	`

	_, err := r.pool.Exec(ctx, query, notificationID, errorMessage)
	if err != nil {
		return fmt.Errorf("failed to mark notification as failed: %w", err)
	}
	return nil
}

// GetNotificationStats returns notification analytics for a user
func (r *Repository) GetNotificationStats(ctx context.Context, userID string, days int) (*NotificationStats, error) {
	query := `
		SELECT
			COUNT(*) FILTER (WHERE status = 'sent') as total_sent,
			COUNT(*) FILTER (WHERE status = 'opened') as total_opened,
			COUNT(*) FILTER (WHERE status = 'converted') as total_converted,
			COUNT(*) FILTER (WHERE status = 'failed') as total_failed,
			COUNT(*) FILTER (WHERE type = 'morning') as morning_count,
			COUNT(*) FILTER (WHERE type = 'task_reminder') as task_reminder_count,
			COUNT(*) FILTER (WHERE type = 'task_missed') as task_missed_count,
			COUNT(*) FILTER (WHERE type = 'evening') as evening_count,
			COUNT(*) FILTER (WHERE type = 'streak_danger') as streak_danger_count
		FROM notification_events
		WHERE user_id = $1 AND created_at >= NOW() - INTERVAL '1 day' * $2
	`

	var stats NotificationStats
	err := r.pool.QueryRow(ctx, query, userID, days).Scan(
		&stats.TotalSent, &stats.TotalOpened, &stats.TotalConverted, &stats.TotalFailed,
		&stats.MorningCount, &stats.TaskReminderCount, &stats.TaskMissedCount,
		&stats.EveningCount, &stats.StreakDangerCount,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get notification stats: %w", err)
	}

	// Calculate rates
	if stats.TotalSent > 0 {
		stats.OpenRate = float64(stats.TotalOpened) / float64(stats.TotalSent) * 100
		stats.ConversionRate = float64(stats.TotalConverted) / float64(stats.TotalSent) * 100
	}

	return &stats, nil
}

// GetUsersForMorningNotification returns users who should receive morning notifications
func (r *Repository) GetUsersForMorningNotification(ctx context.Context, hour, minute int) ([]UserNotificationInfo, error) {
	query := `
		SELECT DISTINCT
			dt.user_id,
			dt.fcm_token,
			np.language,
			np.timezone,
			u.first_name
		FROM device_tokens dt
		JOIN notification_preferences np ON dt.user_id = np.user_id
		JOIN users u ON dt.user_id = u.id
		WHERE dt.is_active = true
		  AND np.morning_reminder_enabled = true
		  AND np.morning_reminder_time = $1
	`

	timeStr := fmt.Sprintf("%02d:%02d", hour, minute)
	rows, err := r.pool.Query(ctx, query, timeStr)
	if err != nil {
		return nil, fmt.Errorf("failed to get users for morning notification: %w", err)
	}
	defer rows.Close()

	var users []UserNotificationInfo
	for rows.Next() {
		var u UserNotificationInfo
		var firstName *string
		if err := rows.Scan(&u.UserID, &u.FCMToken, &u.Language, &u.Timezone, &firstName); err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}
		if firstName != nil {
			u.FirstName = *firstName
		}
		users = append(users, u)
	}

	return users, nil
}

// NotificationStats holds analytics data
type NotificationStats struct {
	TotalSent         int     `json:"total_sent"`
	TotalOpened       int     `json:"total_opened"`
	TotalConverted    int     `json:"total_converted"`
	TotalFailed       int     `json:"total_failed"`
	OpenRate          float64 `json:"open_rate"`
	ConversionRate    float64 `json:"conversion_rate"`
	MorningCount      int     `json:"morning_count"`
	TaskReminderCount int     `json:"task_reminder_count"`
	TaskMissedCount   int     `json:"task_missed_count"`
	EveningCount      int     `json:"evening_count"`
	StreakDangerCount int     `json:"streak_danger_count"`
}

// UserNotificationInfo holds user info needed for sending notifications
type UserNotificationInfo struct {
	UserID    string `json:"user_id"`
	FCMToken  string `json:"fcm_token"`
	Language  string `json:"language"`
	Timezone  string `json:"timezone"`
	FirstName string `json:"first_name"`
}
