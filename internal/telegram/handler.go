package telegram

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Handler handles Telegram-related HTTP requests
type Handler struct {
	db      *pgxpool.Pool
	service *Service
}

// NewHandler creates a new telegram handler
func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{
		db:      db,
		service: Get(),
	}
}

// SendDailySummary sends the daily KPI summary to Telegram
// POST /jobs/telegram/daily-summary
func (h *Handler) SendDailySummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	stats, err := h.getDailyStats(ctx)
	if err != nil {
		log.Printf("❌ Failed to get daily stats: %v", err)
		http.Error(w, "Failed to get stats", http.StatusInternalServerError)
		return
	}

	h.service.Send(Event{
		Type: EventDailySummary,
		Data: stats,
	})

	log.Println("✅ Daily summary sent to Telegram")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"stats":   stats,
	})
}

// getDailyStats retrieves daily statistics from the database
func (h *Handler) getDailyStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// New users today
	var newUsers int
	err := h.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM users
		WHERE created_at >= CURRENT_DATE
	`).Scan(&newUsers)
	if err != nil {
		log.Printf("⚠️ Error getting new users: %v", err)
	}
	stats["new_users"] = newUsers

	// Active users today (had any activity)
	var activeUsers int
	err = h.db.QueryRow(ctx, `
		SELECT COUNT(DISTINCT user_id) FROM (
			SELECT user_id FROM routine_completions WHERE completed_at >= CURRENT_DATE
			UNION
			SELECT user_id FROM tasks WHERE completed_at >= CURRENT_DATE
			UNION
			SELECT user_id FROM focus_sessions WHERE created_at >= CURRENT_DATE
		) active
	`).Scan(&activeUsers)
	if err != nil {
		log.Printf("⚠️ Error getting active users: %v", err)
	}
	stats["active_users"] = activeUsers

	// Focus sessions today
	var focusSessions, focusMinutes int
	err = h.db.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE(SUM(duration_minutes), 0)
		FROM focus_sessions
		WHERE created_at >= CURRENT_DATE AND status = 'completed'
	`).Scan(&focusSessions, &focusMinutes)
	if err != nil {
		log.Printf("⚠️ Error getting focus stats: %v", err)
	}
	stats["focus_sessions"] = focusSessions
	stats["focus_minutes"] = focusMinutes

	// Routines completed today
	var routinesCompleted int
	err = h.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM routine_completions
		WHERE completed_at >= CURRENT_DATE
	`).Scan(&routinesCompleted)
	if err != nil {
		log.Printf("⚠️ Error getting routine completions: %v", err)
	}
	stats["routines_completed"] = routinesCompleted

	// Streaks broken today (streak was > 0 yesterday, is 0 today)
	var streaksBroken int
	err = h.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM user_streaks
		WHERE current_streak = 0
		AND updated_at >= CURRENT_DATE
		AND longest_streak > 0
	`).Scan(&streaksBroken)
	if err != nil {
		log.Printf("⚠️ Error getting broken streaks: %v", err)
	}
	stats["streaks_broken"] = streaksBroken

	// Flame level ups today (check if flame_level increased)
	var flameLevelUps int
	err = h.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM user_streaks
		WHERE updated_at >= CURRENT_DATE
		AND current_streak IN (3, 7, 14, 30, 60, 100)
	`).Scan(&flameLevelUps)
	if err != nil {
		log.Printf("⚠️ Error getting flame level ups: %v", err)
	}
	stats["flame_level_ups"] = flameLevelUps

	// Community posts today
	var communityPosts int
	err = h.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM community_posts
		WHERE created_at >= CURRENT_DATE
	`).Scan(&communityPosts)
	if err != nil {
		log.Printf("⚠️ Error getting community posts: %v", err)
	}
	stats["community_posts"] = communityPosts

	// Referrals this month
	var referralsThisMonth int
	err = h.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM referrals
		WHERE referred_at >= DATE_TRUNC('month', CURRENT_DATE)
	`).Scan(&referralsThisMonth)
	if err != nil {
		log.Printf("⚠️ Error getting referrals: %v", err)
	}
	stats["referrals_this_month"] = referralsThisMonth

	return stats, nil
}

// TestNotification sends a test notification
// POST /admin/telegram/test
func (h *Handler) TestNotification(w http.ResponseWriter, r *http.Request) {
	h.service.SendRaw("🧪 *Test Notification*\n\nLe système Telegram fonctionne correctement !")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// CheckInactiveUsers checks for inactive users and sends alerts
// POST /jobs/telegram/check-inactive
func (h *Handler) CheckInactiveUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Users inactive for 3 days
	rows, err := h.db.Query(ctx, `
		SELECT u.id, COALESCE(u.pseudo, u.first_name, 'User') as name, COALESCE(u.email, '') as email
		FROM users u
		WHERE u.id NOT IN (
			SELECT DISTINCT user_id FROM routine_completions WHERE completed_at >= CURRENT_DATE - INTERVAL '3 days'
			UNION
			SELECT DISTINCT user_id FROM tasks WHERE completed_at >= CURRENT_DATE - INTERVAL '3 days'
			UNION
			SELECT DISTINCT user_id FROM focus_sessions WHERE created_at >= CURRENT_DATE - INTERVAL '3 days'
		)
		AND u.created_at < CURRENT_DATE - INTERVAL '3 days'
		AND u.id IN (
			SELECT DISTINCT user_id FROM routine_completions
			UNION
			SELECT DISTINCT user_id FROM tasks
		)
		LIMIT 10
	`)
	if err != nil {
		log.Printf("❌ Error checking inactive users: %v", err)
		http.Error(w, "Failed to check", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var userID, name, email string
		if err := rows.Scan(&userID, &name, &email); err != nil {
			continue
		}

		h.service.Send(Event{
			Type:      EventUserInactive3Days,
			UserID:    userID,
			UserName:  name,
			UserEmail: email,
		})
		count++
	}

	log.Printf("✅ Checked inactive users, sent %d alerts", count)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"alerts_sent": count})
}
