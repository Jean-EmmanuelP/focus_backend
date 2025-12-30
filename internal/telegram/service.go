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
		return fmt.Sprintf(`🎉 *NOUVEAU USER !*

👤 *%s*
📧 %s
🆔 %s
🕐 %s

✨ _Bienvenue dans la famille !_`,
			event.UserName, event.UserEmail, event.UserID[:8]+"...", event.Timestamp.Format("02/01 15:04"))

	case EventOnboardingCompleted:
		return fmt.Sprintf(`✅ *ONBOARDING TERMINÉ*

👤 *%s*
📧 %s

🎯 _Prêt à commencer son aventure !_`,
			event.UserName, event.UserEmail)

	// ===== ENGAGEMENT =====
	case EventFirstRoutineCreated:
		routineName := getString(event.Data, "routine_name")
		return fmt.Sprintf(`🔄 *PREMIÈRE ROUTINE !*

👤 *%s*
📧 %s
📋 Routine: *%s*

🚀 _Signal d'adoption fort !_`,
			event.UserName, event.UserEmail, routineName)

	case EventFirstQuestCreated:
		questName := getString(event.Data, "quest_name")
		return fmt.Sprintf(`🎯 *PREMIÈRE QUÊTE !*

👤 *%s*
📧 %s
🏆 Quête: *%s*

💪 _User engagé !_`,
			event.UserName, event.UserEmail, questName)

	case EventFirstTaskCreated:
		return fmt.Sprintf(`📝 *PREMIÈRE TÂCHE !*

👤 *%s*
📧 %s

📅 _Commence à planifier !_`,
			event.UserName, event.UserEmail)

	// ===== STREAKS & MILESTONES =====
	case EventStreakDayValidated:
		streak := getInt(event.Data, "current_streak")
		return fmt.Sprintf(`🔥 *JOUR VALIDÉ*

👤 *%s*
📊 Streak actuel: *%d jours*

✅ _Continue comme ça !_`,
			event.UserName, streak)

	case EventStreakBroken:
		wasStreak := getInt(event.Data, "was_streak")
		return fmt.Sprintf(`💔 *STREAK CASSÉ*

👤 *%s*
📧 %s
📉 Était à: *%d jours*

⚠️ _À surveiller - risque de churn_`,
			event.UserName, event.UserEmail, wasStreak)

	case EventFlameLevelUnlocked:
		level := getInt(event.Data, "level")
		levelName := getString(event.Data, "level_name")
		return fmt.Sprintf(`🏆 *NIVEAU FLAME DÉBLOQUÉ !*

👤 *%s*
🔥 Niveau %d: *%s*

🎊 _Félicitations !_`,
			event.UserName, level, levelName)

	case EventStreak100Days:
		return fmt.Sprintf(`🌟🌟🌟 *LEGEND STATUS !* 🌟🌟🌟

👤 *%s*
📧 %s
🔥 *100 JOURS DE STREAK !*

👑 _Un vrai champion !_
🎉🎉🎉`,
			event.UserName, event.UserEmail)

	// ===== FOCUS =====
	case EventFocusSessionCompleted:
		duration := getInt(event.Data, "duration_minutes")
		return fmt.Sprintf(`⏱️ *SESSION FOCUS*

👤 *%s*
⏰ Durée: *%d minutes*

💪 _Deep work accompli !_`,
			event.UserName, duration)

	case EventFocusMinuteMilestone:
		totalMinutes := getInt(event.Data, "total_minutes")
		return fmt.Sprintf(`🎯 *MILESTONE FOCUS !*

👤 *%s*
⏱️ Total semaine: *%d minutes*

🚀 _Machine de productivité !_`,
			event.UserName, totalMinutes)

	// ===== QUESTS =====
	case EventQuestCompleted:
		questName := getString(event.Data, "quest_name")
		targetValue := getInt(event.Data, "target_value")
		return fmt.Sprintf(`🏆 *QUÊTE COMPLÉTÉE !*

👤 *%s*
📧 %s
🎯 Quest: *%s*
✅ Objectif: %d atteint

🎊 _Objectif accompli !_`,
			event.UserName, event.UserEmail, questName, targetValue)

	// ===== COMMUNITY =====
	case EventCommunityPostCreated:
		return fmt.Sprintf(`📸 *NOUVEAU POST COMMUNAUTÉ*

👤 *%s*
📧 %s
🕐 %s

📢 _Partage avec la communauté !_`,
			event.UserName, event.UserEmail, event.Timestamp.Format("02/01 15:04"))

	case EventFriendRequestAccepted:
		friendName := getString(event.Data, "friend_name")
		return fmt.Sprintf(`🤝 *NOUVELLE CONNEXION*

👤 *%s*
↔️ *%s*

👥 _Réseau qui grandit !_`,
			event.UserName, friendName)

	// ===== REFERRALS =====
	case EventReferralApplied:
		referrerCode := getString(event.Data, "referrer_name")
		return fmt.Sprintf(`🔗 *CODE PARRAIN UTILISÉ !*

👤 Nouveau: *%s*
📧 %s
👑 Code: *%s*

💰 _Parrainage en attente d'activation_`,
			event.UserName, event.UserEmail, referrerCode)

	case EventReferralActivated:
		referrerName := getString(event.Data, "referrer_name")
		return fmt.Sprintf(`💰💰 *PARRAINAGE ACTIVÉ !* 💰💰

👤 Filleul: *%s*
📧 %s
👑 Parrain: *%s*
💵 Commission: *20%%*

🎉 _Cha-ching ! Le parrain gagne de l'argent !_`,
			event.UserName, event.UserEmail, referrerName)

	case EventCommissionEarned:
		amount := getFloat(event.Data, "amount")
		referredName := getString(event.Data, "referred_name")
		return fmt.Sprintf(`💵 *COMMISSION GAGNÉE*

👑 Parrain: *%s*
💰 Montant: *+%.2f€*
👤 Grâce à: *%s*

🏦 _À payer ce mois !_`,
			event.UserName, amount, referredName)

	// ===== AT-RISK =====
	case EventUserInactive3Days:
		return fmt.Sprintf(`⚠️ *USER INACTIF 3 JOURS*

👤 *%s*
📧 %s

📊 _Surveiller - début de churn potentiel_`,
			event.UserName, event.UserEmail)

	case EventUserInactive7Days:
		return fmt.Sprintf(`🚨🚨 *ALERTE CHURN !* 🚨🚨

👤 *%s*
📧 %s
⏰ Inactif depuis: *7 jours*

❌ _Action urgente requise !_
📧 _Envoyer email de réactivation ?_`,
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

// UserInfo contains user details for notifications
type UserInfo struct {
	ID        string
	Email     string
	Pseudo    string
	FirstName string
}

// GetDisplayName returns the best display name
func (u UserInfo) GetDisplayName() string {
	if u.Pseudo != "" {
		return u.Pseudo
	}
	if u.FirstName != "" {
		return u.FirstName
	}
	return "User"
}

// GetEmailDisplay returns email or placeholder
func (u UserInfo) GetEmailDisplay() string {
	if u.Email != "" {
		return u.Email
	}
	return "N/A"
}
