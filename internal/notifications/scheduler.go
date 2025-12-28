package notifications

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

// Scheduler handles scheduled notification sending
type Scheduler struct {
	repo     *Repository
	firebase *FirebaseService
}

// NewScheduler creates a new notification scheduler
func NewScheduler(repo *Repository) *Scheduler {
	return &Scheduler{
		repo:     repo,
		firebase: GetFirebaseService(),
	}
}

// ===== Morning Notifications =====

// SendMorningNotifications sends morning notifications to all users whose reminder time matches current time
func (s *Scheduler) SendMorningNotifications(ctx context.Context) error {
	if !s.firebase.IsAvailable() {
		return fmt.Errorf("firebase not available")
	}

	// Get current time in UTC, then we'll check each user's timezone
	now := time.Now().UTC()

	// We check for users in multiple timezones
	// Common timezones to check
	timezones := []string{
		"Europe/Paris",
		"Europe/London",
		"America/New_York",
		"America/Los_Angeles",
		"Asia/Tokyo",
		"Australia/Sydney",
	}

	totalSent := 0
	totalFailed := 0

	for _, tz := range timezones {
		loc, err := time.LoadLocation(tz)
		if err != nil {
			log.Printf("⚠️ Invalid timezone %s: %v", tz, err)
			continue
		}

		localTime := now.In(loc)
		hour := localTime.Hour()
		minute := localTime.Minute()

		// Get users for this specific time
		users, err := s.repo.GetUsersForMorningNotificationByTimezone(ctx, hour, minute, tz)
		if err != nil {
			log.Printf("⚠️ Failed to get users for morning notification (%s %02d:%02d): %v", tz, hour, minute, err)
			continue
		}

		if len(users) == 0 {
			continue
		}

		log.Printf("📬 Found %d users for morning notification at %02d:%02d %s", len(users), hour, minute, tz)

		// Send to each user
		for _, user := range users {
			sent, failed := s.sendMorningNotificationToUser(ctx, user)
			totalSent += sent
			totalFailed += failed
		}
	}

	log.Printf("✅ Morning notifications complete: %d sent, %d failed", totalSent, totalFailed)
	return nil
}

func (s *Scheduler) sendMorningNotificationToUser(ctx context.Context, user UserNotificationInfo) (sent int, failed int) {
	// Get random phrase for user's language
	phrase, err := s.repo.GetRandomPhrase(ctx, "morning", user.Language)
	if err != nil || phrase == "" {
		// Fallback phrase
		if user.Language == "fr" {
			phrase = "C'est le moment de planifier ta journée et d'atteindre tes objectifs !"
		} else {
			phrase = "Time to plan your day and crush your goals!"
		}
	}

	// Build title with personalization
	var title string
	if user.FirstName != "" {
		if user.Language == "fr" {
			title = fmt.Sprintf("Bonjour %s !", user.FirstName)
		} else {
			title = fmt.Sprintf("Good morning %s!", user.FirstName)
		}
	} else {
		if user.Language == "fr" {
			title = "Bonjour !"
		} else {
			title = "Good morning!"
		}
	}

	// Create notification ID for tracking
	notificationID := uuid.New().String()

	// Create notification event for tracking
	event := &NotificationEvent{
		UserID:         user.UserID,
		NotificationID: notificationID,
		Type:           "morning",
		Status:         "sent",
		Title:          title,
		Body:           phrase,
		DeepLink:       "focus://starttheday",
	}
	now := time.Now()
	event.SentAt = &now

	// Send via FCM
	payload := NotificationPayload{
		Title:          title,
		Body:           phrase,
		DeepLink:       "focus://starttheday",
		NotificationID: notificationID,
		Type:           "morning",
	}

	_, err = s.firebase.SendToDevice(ctx, user.FCMToken, payload)
	if err != nil {
		log.Printf("❌ Failed to send morning notification to user %s: %v", user.UserID, err)
		event.Status = "failed"
		event.ErrorMessage = err.Error()
		s.repo.CreateNotificationEvent(ctx, event)

		// Check if token is invalid and deactivate
		if isInvalidTokenError(err) {
			s.repo.DeactivateInvalidToken(ctx, user.FCMToken)
		}
		return 0, 1
	}

	// Save successful event
	s.repo.CreateNotificationEvent(ctx, event)
	log.Printf("✅ Morning notification sent to %s", user.UserID)
	return 1, 0
}

// ===== Evening Notifications =====

// SendEveningNotifications sends evening reflection reminders
func (s *Scheduler) SendEveningNotifications(ctx context.Context) error {
	if !s.firebase.IsAvailable() {
		return fmt.Errorf("firebase not available")
	}

	now := time.Now().UTC()
	timezones := []string{"Europe/Paris", "Europe/London", "America/New_York", "America/Los_Angeles"}

	totalSent := 0
	totalFailed := 0

	for _, tz := range timezones {
		loc, err := time.LoadLocation(tz)
		if err != nil {
			continue
		}

		localTime := now.In(loc)
		hour := localTime.Hour()
		minute := localTime.Minute()

		users, err := s.repo.GetUsersForEveningNotificationByTimezone(ctx, hour, minute, tz)
		if err != nil {
			log.Printf("⚠️ Failed to get users for evening notification: %v", err)
			continue
		}

		for _, user := range users {
			sent, failed := s.sendEveningNotificationToUser(ctx, user)
			totalSent += sent
			totalFailed += failed
		}
	}

	log.Printf("✅ Evening notifications complete: %d sent, %d failed", totalSent, totalFailed)
	return nil
}

func (s *Scheduler) sendEveningNotificationToUser(ctx context.Context, user UserNotificationInfo) (sent int, failed int) {
	phrase, err := s.repo.GetRandomPhrase(ctx, "evening", user.Language)
	if err != nil || phrase == "" {
		if user.Language == "fr" {
			phrase = "Comment s'est passée ta journée ? Prends un moment pour faire le point."
		} else {
			phrase = "How was your day? Take a moment to reflect."
		}
	}

	var title string
	if user.Language == "fr" {
		title = "Bilan du jour"
	} else {
		title = "Daily Review"
	}

	notificationID := uuid.New().String()

	event := &NotificationEvent{
		UserID:         user.UserID,
		NotificationID: notificationID,
		Type:           "evening",
		Status:         "sent",
		Title:          title,
		Body:           phrase,
		DeepLink:       "focus://endofday",
	}
	now := time.Now()
	event.SentAt = &now

	payload := NotificationPayload{
		Title:          title,
		Body:           phrase,
		DeepLink:       "focus://endofday",
		NotificationID: notificationID,
		Type:           "evening",
	}

	_, err = s.firebase.SendToDevice(ctx, user.FCMToken, payload)
	if err != nil {
		log.Printf("❌ Failed to send evening notification to user %s: %v", user.UserID, err)
		event.Status = "failed"
		event.ErrorMessage = err.Error()
		s.repo.CreateNotificationEvent(ctx, event)
		if isInvalidTokenError(err) {
			s.repo.DeactivateInvalidToken(ctx, user.FCMToken)
		}
		return 0, 1
	}

	s.repo.CreateNotificationEvent(ctx, event)
	return 1, 0
}

// ===== Streak Danger Notifications =====

// SendStreakDangerNotifications sends alerts to users who might lose their streak
func (s *Scheduler) SendStreakDangerNotifications(ctx context.Context) error {
	if !s.firebase.IsAvailable() {
		return fmt.Errorf("firebase not available")
	}

	// Get users with active streaks who haven't completed any routine today
	users, err := s.repo.GetUsersWithStreakInDanger(ctx)
	if err != nil {
		return fmt.Errorf("failed to get users with streak in danger: %w", err)
	}

	if len(users) == 0 {
		log.Println("📊 No users with streak in danger")
		return nil
	}

	log.Printf("⚠️ Found %d users with streak in danger", len(users))

	totalSent := 0
	totalFailed := 0

	for _, user := range users {
		phrase, _ := s.repo.GetRandomPhrase(ctx, "streak_danger", user.Language)
		if phrase == "" {
			if user.Language == "fr" {
				phrase = "Ta streak est en danger ! Complete une routine pour la maintenir."
			} else {
				phrase = "Your streak is at risk! Complete a routine to keep it going."
			}
		}

		var title string
		if user.Language == "fr" {
			title = fmt.Sprintf("🔥 Streak de %d jours en danger !", user.CurrentStreak)
		} else {
			title = fmt.Sprintf("🔥 %d day streak at risk!", user.CurrentStreak)
		}

		notificationID := uuid.New().String()

		payload := NotificationPayload{
			Title:          title,
			Body:           phrase,
			DeepLink:       "focus://firemode",
			NotificationID: notificationID,
			Type:           "streak_danger",
		}

		_, err := s.firebase.SendToDevice(ctx, user.FCMToken, payload)
		if err != nil {
			log.Printf("❌ Failed to send streak danger notification: %v", err)
			totalFailed++
			if isInvalidTokenError(err) {
				s.repo.DeactivateInvalidToken(ctx, user.FCMToken)
			}
			continue
		}

		// Track event
		now := time.Now()
		event := &NotificationEvent{
			UserID:         user.UserID,
			NotificationID: notificationID,
			Type:           "streak_danger",
			Status:         "sent",
			Title:          title,
			Body:           phrase,
			DeepLink:       "focus://firemode",
			SentAt:         &now,
		}
		s.repo.CreateNotificationEvent(ctx, event)
		totalSent++
	}

	log.Printf("✅ Streak danger notifications complete: %d sent, %d failed", totalSent, totalFailed)
	return nil
}

// ===== Task Reminder Notifications =====

// SendTaskReminders sends reminders for upcoming tasks
func (s *Scheduler) SendTaskReminders(ctx context.Context) error {
	if !s.firebase.IsAvailable() {
		return fmt.Errorf("firebase not available")
	}

	// Get tasks starting in the next 15 minutes for users who have task reminders enabled
	tasks, err := s.repo.GetUpcomingTasksForReminder(ctx, 15) // 15 minutes ahead
	if err != nil {
		return fmt.Errorf("failed to get upcoming tasks: %w", err)
	}

	if len(tasks) == 0 {
		return nil
	}

	log.Printf("📋 Found %d tasks to remind", len(tasks))

	totalSent := 0
	totalFailed := 0

	for _, task := range tasks {
		phrase, _ := s.repo.GetRandomPhrase(ctx, "task_reminder", task.Language)
		if phrase == "" {
			if task.Language == "fr" {
				phrase = "C'est l'heure de te concentrer !"
			} else {
				phrase = "Time to focus!"
			}
		}

		notificationID := uuid.New().String()

		payload := NotificationPayload{
			Title:          task.TaskTitle,
			Body:           phrase,
			DeepLink:       "focus://calendar",
			NotificationID: notificationID,
			Type:           "task_reminder",
			Data: map[string]string{
				"task_id": task.TaskID,
			},
		}

		_, err := s.firebase.SendToDevice(ctx, task.FCMToken, payload)
		if err != nil {
			log.Printf("❌ Failed to send task reminder: %v", err)
			totalFailed++
			if isInvalidTokenError(err) {
				s.repo.DeactivateInvalidToken(ctx, task.FCMToken)
			}
			continue
		}

		// Track event
		now := time.Now()
		event := &NotificationEvent{
			UserID:         task.UserID,
			NotificationID: notificationID,
			Type:           "task_reminder",
			Status:         "sent",
			Title:          task.TaskTitle,
			Body:           phrase,
			DeepLink:       "focus://calendar",
			Metadata:       fmt.Sprintf(`{"task_id":"%s"}`, task.TaskID),
			SentAt:         &now,
		}
		s.repo.CreateNotificationEvent(ctx, event)
		totalSent++
	}

	log.Printf("✅ Task reminders complete: %d sent, %d failed", totalSent, totalFailed)
	return nil
}

// ===== Helper Functions =====

func isInvalidTokenError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return contains(errStr, "registration-token-not-registered") ||
		contains(errStr, "invalid-registration-token") ||
		contains(errStr, "NotRegistered")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Note: UserStreakInfo and TaskReminderInfo are defined in repository.go
