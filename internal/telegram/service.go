package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

// Service handles Telegram notifications for KPI events
type Service struct {
	botToken string
	chatID   string
	enabled  bool
}

// Global instance
var instance *Service

// Init initializes the Telegram service
func Init() {
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID := os.Getenv("TELEGRAM_CHAT_ID")

	instance = &Service{
		botToken: botToken,
		chatID:   chatID,
		enabled:  botToken != "" && chatID != "",
	}

	if instance.enabled {
		log.Println("✅ Telegram notifications enabled")
	} else {
		log.Println("⚠️ Telegram notifications disabled (missing TELEGRAM_BOT_TOKEN or TELEGRAM_CHAT_ID)")
	}
}

// Get returns the singleton instance
func Get() *Service {
	if instance == nil {
		Init()
	}
	return instance
}

// ===== Event Types =====

type EventType string

const (
	// Acquisition
	EventUserSignup          EventType = "user_signup"
	EventOnboardingCompleted EventType = "onboarding_completed"

	// Engagement
	EventFirstRoutineCreated EventType = "first_routine_created"
	EventFirstQuestCreated   EventType = "first_quest_created"
	EventFirstTaskCreated    EventType = "first_task_created"

	// Streaks & Milestones
	EventStreakDayValidated EventType = "streak_day_validated"
	EventStreakBroken       EventType = "streak_broken"
	EventFlameLevelUnlocked EventType = "flame_level_unlocked"
	EventStreak100Days      EventType = "streak_100_days"

	// Focus
	EventFocusSessionCompleted EventType = "focus_session_completed"
	EventFocusMinuteMilestone  EventType = "focus_minute_milestone"

	// Quests
	EventQuestCompleted EventType = "quest_completed"

	// Community
	EventCommunityPostCreated EventType = "community_post_created"
	EventFriendRequestAccepted EventType = "friend_request_accepted"

	// Referrals
	EventReferralApplied   EventType = "referral_applied"
	EventReferralActivated EventType = "referral_activated"
	EventCommissionEarned  EventType = "commission_earned"

	// At-Risk
	EventUserInactive3Days EventType = "user_inactive_3_days"
	EventUserInactive7Days EventType = "user_inactive_7_days"

	// Admin
	EventDailySummary EventType = "daily_summary"
)

// Event represents a KPI event to send
type Event struct {
	Type      EventType
	UserID    string
	UserName  string
	UserEmail string
	Data      map[string]interface{}
	Timestamp time.Time
}

// ===== Send Methods =====

// Send sends an event notification to Telegram
func (s *Service) Send(event Event) {
	if !s.enabled {
		return
	}

	message := s.formatMessage(event)
	go s.sendMessage(message)
}

// SendRaw sends a raw message to Telegram
func (s *Service) SendRaw(message string) {
	if !s.enabled {
		return
	}
	go s.sendMessage(message)
}

// formatMessage formats an event into a Telegram message
func (s *Service) formatMessage(event Event) string {
	event.Timestamp = time.Now()

	switch event.Type {
	// ===== ACQUISITION =====
	case EventUserSignup:
		return fmt.Sprintf("🎉 *Nouveau User !*\n\n👤 %s\n📧 %s\n🕐 %s",
			event.UserName, event.UserEmail, event.Timestamp.Format("15:04"))

	case EventOnboardingCompleted:
		return fmt.Sprintf("✅ *Onboarding terminé*\n\n👤 %s\n🎯 Prêt à commencer !",
			event.UserName)

	// ===== ENGAGEMENT =====
	case EventFirstRoutineCreated:
		routineName := getString(event.Data, "routine_name")
		return fmt.Sprintf("🔄 *Première routine créée !*\n\n👤 %s\n📋 %s",
			event.UserName, routineName)

	case EventFirstQuestCreated:
		questName := getString(event.Data, "quest_name")
		return fmt.Sprintf("🎯 *Première quête créée !*\n\n👤 %s\n🏆 %s",
			event.UserName, questName)

	case EventFirstTaskCreated:
		return fmt.Sprintf("📝 *Première tâche créée !*\n\n👤 %s",
			event.UserName)

	// ===== STREAKS & MILESTONES =====
	case EventStreakDayValidated:
		streak := getInt(event.Data, "current_streak")
		return fmt.Sprintf("🔥 *Jour validé !*\n\n👤 %s\n📊 Streak: %d jours",
			event.UserName, streak)

	case EventStreakBroken:
		wasStreak := getInt(event.Data, "was_streak")
		return fmt.Sprintf("💔 *Streak cassé !*\n\n👤 %s\n📉 Était à %d jours",
			event.UserName, wasStreak)

	case EventFlameLevelUnlocked:
		level := getInt(event.Data, "level")
		levelName := getString(event.Data, "level_name")
		return fmt.Sprintf("🏆 *Nouveau niveau Flame !*\n\n👤 %s\n🔥 Niveau %d: %s",
			event.UserName, level, levelName)

	case EventStreak100Days:
		return fmt.Sprintf("🌟 *LEGEND STATUS !*\n\n👤 %s\n🔥 100 jours de streak !\n\n🎉🎉🎉",
			event.UserName)

	// ===== FOCUS =====
	case EventFocusSessionCompleted:
		duration := getInt(event.Data, "duration_minutes")
		return fmt.Sprintf("⏱️ *Session focus terminée*\n\n👤 %s\n⏰ %d minutes",
			event.UserName, duration)

	case EventFocusMinuteMilestone:
		totalMinutes := getInt(event.Data, "total_minutes")
		return fmt.Sprintf("🎯 *Milestone Focus !*\n\n👤 %s\n⏱️ %d minutes total cette semaine",
			event.UserName, totalMinutes)

	// ===== QUESTS =====
	case EventQuestCompleted:
		questName := getString(event.Data, "quest_name")
		return fmt.Sprintf("🏆 *Quête complétée !*\n\n👤 %s\n🎯 %s",
			event.UserName, questName)

	// ===== COMMUNITY =====
	case EventCommunityPostCreated:
		return fmt.Sprintf("📸 *Nouveau post communauté*\n\n👤 %s",
			event.UserName)

	case EventFriendRequestAccepted:
		friendName := getString(event.Data, "friend_name")
		return fmt.Sprintf("🤝 *Nouvelle connexion*\n\n👤 %s ↔️ %s",
			event.UserName, friendName)

	// ===== REFERRALS =====
	case EventReferralApplied:
		referrerName := getString(event.Data, "referrer_name")
		return fmt.Sprintf("🔗 *Code parrain utilisé !*\n\n👤 Nouveau: %s\n👑 Parrain: %s",
			event.UserName, referrerName)

	case EventReferralActivated:
		referrerName := getString(event.Data, "referrer_name")
		return fmt.Sprintf("💰 *Parrainage activé !*\n\n👤 %s a souscrit\n👑 Parrain: %s\n💵 Commission: 20%%",
			event.UserName, referrerName)

	case EventCommissionEarned:
		amount := getFloat(event.Data, "amount")
		referredName := getString(event.Data, "referred_name")
		return fmt.Sprintf("💵 *Commission gagnée !*\n\n👑 %s\n💰 +%.2f€\n👤 Grâce à: %s",
			event.UserName, amount, referredName)

	// ===== AT-RISK =====
	case EventUserInactive3Days:
		return fmt.Sprintf("⚠️ *User inactif 3 jours*\n\n👤 %s\n📧 %s",
			event.UserName, event.UserEmail)

	case EventUserInactive7Days:
		return fmt.Sprintf("🚨 *User inactif 7 jours !*\n\n👤 %s\n📧 %s\n\n⚠️ Risque de churn",
			event.UserName, event.UserEmail)

	// ===== ADMIN =====
	case EventDailySummary:
		return s.formatDailySummary(event.Data)

	default:
		return fmt.Sprintf("📊 *Event: %s*\n\n👤 %s\n📦 %v",
			event.Type, event.UserName, event.Data)
	}
}

// formatDailySummary formats a daily summary
func (s *Service) formatDailySummary(data map[string]interface{}) string {
	newUsers := getInt(data, "new_users")
	activeUsers := getInt(data, "active_users")
	focusSessions := getInt(data, "focus_sessions")
	focusMinutes := getInt(data, "focus_minutes")
	routinesCompleted := getInt(data, "routines_completed")
	streaksBroken := getInt(data, "streaks_broken")
	flameLevelUps := getInt(data, "flame_level_ups")
	communityPosts := getInt(data, "community_posts")
	referralsThisMonth := getInt(data, "referrals_this_month")

	return fmt.Sprintf(`📊 *Résumé Quotidien Firelevel*

👥 *Utilisateurs*
• Nouveaux: %d
• Actifs aujourd'hui: %d

🔥 *Streaks*
• Streaks cassés: %d
• Level ups: %d

⏱️ *Focus*
• Sessions: %d
• Minutes: %d

✅ *Routines complétées*: %d

📸 *Posts communauté*: %d

🔗 *Parrainages ce mois*: %d`,
		newUsers, activeUsers,
		streaksBroken, flameLevelUps,
		focusSessions, focusMinutes,
		routinesCompleted,
		communityPosts,
		referralsThisMonth)
}

// sendMessage sends a message via Telegram Bot API
func (s *Service) sendMessage(text string) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", s.botToken)

	payload := map[string]interface{}{
		"chat_id":    s.chatID,
		"text":       text,
		"parse_mode": "Markdown",
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Printf("❌ Telegram marshal error: %v", err)
		return
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("❌ Telegram send error: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("❌ Telegram API error: status %d", resp.StatusCode)
	}
}

// ===== Helper functions =====

func getString(data map[string]interface{}, key string) string {
	if v, ok := data[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getInt(data map[string]interface{}, key string) int {
	if v, ok := data[key]; ok {
		switch n := v.(type) {
		case int:
			return n
		case int64:
			return int(n)
		case float64:
			return int(n)
		}
	}
	return 0
}

func getFloat(data map[string]interface{}, key string) float64 {
	if v, ok := data[key]; ok {
		switch n := v.(type) {
		case float64:
			return n
		case int:
			return float64(n)
		case int64:
			return float64(n)
		}
	}
	return 0
}
