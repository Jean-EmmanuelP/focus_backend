package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"

	"firelevel-backend/internal/areas"
	"firelevel-backend/internal/auth"
	"firelevel-backend/internal/calendar"
	"firelevel-backend/internal/community"
	"firelevel-backend/internal/crew"
	"firelevel-backend/internal/database"
	"firelevel-backend/internal/focus"
	"firelevel-backend/internal/googlecalendar"
	"firelevel-backend/internal/intentions"
	"firelevel-backend/internal/journal"
	"firelevel-backend/internal/motivation"
	"firelevel-backend/internal/notifications"
	"firelevel-backend/internal/onboarding"
	"firelevel-backend/internal/quests"
	"firelevel-backend/internal/reflections"
	"firelevel-backend/internal/routines"
	"firelevel-backend/internal/stats"
	"firelevel-backend/internal/streaks"
	"firelevel-backend/internal/users"
	"firelevel-backend/internal/voice"
	ws "firelevel-backend/internal/websocket"
	"firelevel-backend/internal/referral"
	"firelevel-backend/internal/telegram"
)

func main() {
	_ = godotenv.Load()

	// 1. Initialize Postgres Pool (pgx)
	pool, err := database.NewPool(context.Background())
	if err != nil {
		log.Fatalf("Failed to init DB pool: %v", err)
	}
	defer pool.Close()

	// 2. Setup JWT Middleware
	jwtSecret := os.Getenv("SUPABASE_JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("SUPABASE_JWT_SECRET is not set in .env")
	}

	authMW, err := auth.AuthMiddleware(jwtSecret)
	if err != nil {
		log.Fatalf("Failed to setup auth middleware: %v", err)
	}

	// 3. Initialize Handlers
	usersHandler := users.NewHandler(pool)
	areasHandler := areas.NewHandler(pool)
	questsHandler := quests.NewHandler(pool)
	routinesHandler := routines.NewHandler(pool)
	reflectionsHandler := reflections.NewHandler(pool)
	completionsHandler := routines.NewCompletionHandler(pool)
	focusHandler := focus.NewHandler(pool)
	intentionsHandler := intentions.NewHandler(pool)
	statsHandler := stats.NewHandler(pool)
	crewHandler := crew.NewHandler(pool)
	streaksHandler := streaks.NewHandler(pool)
	onboardingHandler := onboarding.NewHandler(pool)
	communityHandler := community.NewHandler(pool)
	calendarHandler := calendar.NewHandler(pool)
	calendarAIHandler := calendar.NewAIHandler(pool)
	voiceHandler := voice.NewHandler(pool)
	googleCalendarHandler := googlecalendar.NewHandler(pool)
	journalHandler := journal.NewHandler(pool)
	motivationHandler := motivation.NewHandler(pool)
	notificationsRepo := notifications.NewRepository(pool)
	notificationsHandler := notifications.NewHandler(notificationsRepo)
	referralRepo := referral.NewRepository(pool)
	referralHandler := referral.NewHandler(referralRepo)

	// Connect Google Calendar sync to calendar handler (tasks only, routines stay local)
	calendarHandler.SetGoogleCalendarSyncer(googleCalendarHandler)

	// Initialize WebSocket hub for real-time updates
	ws.InitGlobalHub()

	// Initialize Telegram notifications
	telegram.Init()
	telegramHandler := telegram.NewHandler(pool)
	telegramWebhook := telegram.NewWebhookHandler()

	// 4. Setup Router
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Public Routes
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})
	r.Get("/notifications/status", notificationsHandler.GetStatus)

	// Webhooks (called by Supabase triggers)
	r.Post("/webhooks/user-created", func(w http.ResponseWriter, r *http.Request) {
		// Verify webhook secret
		webhookSecret := os.Getenv("WEBHOOK_SECRET")
		if webhookSecret != "" && r.Header.Get("X-Webhook-Secret") != webhookSecret {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		telegramWebhook.HandleUserCreated(w, r)
	})

	// Protected Routes
	r.Group(func(r chi.Router) {
		r.Use(authMW)

		// Users
		r.Get("/me", usersHandler.GetProfile)
		r.Patch("/me", usersHandler.UpdateProfile)
		r.Post("/me/avatar", usersHandler.UploadAvatar)
		r.Delete("/me/avatar", usersHandler.DeleteAvatar)

		// Areas
		r.Get("/areas", areasHandler.List)
		r.Post("/areas", areasHandler.Create)
		r.Patch("/areas/{id}", areasHandler.Update)
		r.Delete("/areas/{id}", areasHandler.Delete)

		// Quests
		r.Get("/quests", questsHandler.List)
		r.Post("/quests", questsHandler.Create)
		r.Patch("/quests/{id}", questsHandler.Update)
		r.Delete("/quests/{id}", questsHandler.Delete)

		// Routines
		r.Get("/routines", routinesHandler.List)
		r.Post("/routines", routinesHandler.Create)
		r.Patch("/routines/{id}", routinesHandler.Update)
		r.Delete("/routines/{id}", routinesHandler.Delete)
		
		// Routine Completions (Actions)
		r.Post("/routines/{id}/complete", routinesHandler.Complete)
		r.Post("/routines/complete-batch", routinesHandler.BatchComplete) // New Batch Endpoint
		r.Delete("/routines/{id}/complete", routinesHandler.Uncomplete) // Undo
		
		// Routine Completions (History)
		r.Get("/completions", completionsHandler.List)

		// Daily Reflections
		r.Get("/reflections", reflectionsHandler.List)
		r.Get("/reflections/{date}", reflectionsHandler.GetByDate)
		r.Put("/reflections/{date}", reflectionsHandler.Upsert)

		// Daily Intentions (Start Day)
		r.Get("/intentions", intentionsHandler.List)
		r.Get("/intentions/today", intentionsHandler.GetToday)
		r.Get("/intentions/{date}", intentionsHandler.GetByDate)
		r.Put("/intentions/{date}", intentionsHandler.Upsert)

		// Focus Sessions
		r.Get("/focus-sessions", focusHandler.List)
		r.Post("/focus-sessions", focusHandler.Start)
		r.Patch("/focus-sessions/{id}", focusHandler.Update)
		r.Delete("/focus-sessions/{id}", focusHandler.Delete)

		// Stats & Dashboard
		r.Get("/dashboard", statsHandler.GetDashboard)
		r.Get("/firemode", statsHandler.GetFireMode)       // New
		r.Get("/quests-tab", statsHandler.GetQuestsTab)    // New
		r.Get("/stats/focus", statsHandler.GetFocusStats)
		r.Get("/stats/routines", statsHandler.GetRoutineStats)

		// Streaks
		r.Get("/streak", streaksHandler.GetStreak)
		r.Get("/streak/day", streaksHandler.GetDayValidation)
		r.Post("/streak/recalculate", streaksHandler.RecalculateStreak)

		// Friends / Social
		r.Get("/friends", crewHandler.ListMembers)
		r.Delete("/friends/{id}", crewHandler.RemoveMember)
		r.Get("/friends/{id}/day", crewHandler.GetMemberDay)
		r.Get("/friends/leaderboard", crewHandler.GetLeaderboard)

		// Friend Requests
		r.Get("/friend-requests/received", crewHandler.ListReceivedRequests)
		r.Get("/friend-requests/sent", crewHandler.ListSentRequests)
		r.Post("/friend-requests", crewHandler.SendRequest)
		r.Post("/friend-requests/{id}/accept", crewHandler.AcceptRequest)
		r.Post("/friend-requests/{id}/reject", crewHandler.RejectRequest)

		// User Search & Suggestions
		r.Get("/users/search", crewHandler.SearchUsers)
		r.Get("/users/suggestions", crewHandler.GetSuggestedUsers)

		// Profile
		r.Patch("/me/visibility", crewHandler.UpdateVisibility)
		r.Get("/me/stats", crewHandler.GetMyStats)

		// Friend Groups (custom friend grouping)
		r.Get("/friend-groups", crewHandler.ListGroupsShared) // Shows groups where user is owner OR member
		r.Post("/friend-groups", crewHandler.CreateGroup)
		r.Get("/friend-groups/{id}", crewHandler.GetGroup)
		r.Patch("/friend-groups/{id}", crewHandler.UpdateGroup)
		r.Delete("/friend-groups/{id}", crewHandler.DeleteGroup)
		r.Post("/friend-groups/{id}/members", crewHandler.AddGroupMembers)
		r.Delete("/friend-groups/{id}/members/{memberId}", crewHandler.RemoveGroupMember)
		r.Post("/friend-groups/{id}/invite", crewHandler.InviteToGroup)
		r.Post("/friend-groups/{id}/leave", crewHandler.LeaveGroup)

		// Group Routines (shared routines for accountability)
		r.Get("/friend-groups/{id}/routines", crewHandler.ListGroupRoutines)
		r.Post("/friend-groups/{id}/routines", crewHandler.ShareRoutineWithGroup)
		r.Delete("/friend-groups/{id}/routines/{groupRoutineId}", crewHandler.RemoveGroupRoutine)

		// Group Invitations
		r.Get("/group-invitations/received", crewHandler.ListReceivedGroupInvitations)
		r.Get("/group-invitations/sent", crewHandler.ListSentGroupInvitations)
		r.Post("/group-invitations/{id}/accept", crewHandler.AcceptGroupInvitation)
		r.Post("/group-invitations/{id}/reject", crewHandler.RejectGroupInvitation)
		r.Delete("/group-invitations/{id}", crewHandler.CancelGroupInvitation)

		// Routine Likes
		r.Post("/completions/{id}/like", crewHandler.LikeCompletion)
		r.Delete("/completions/{id}/like", crewHandler.UnlikeCompletion)

		// Onboarding
		r.Get("/onboarding/status", onboardingHandler.GetStatus)
		r.Put("/onboarding/progress", onboardingHandler.SaveProgress)
		r.Post("/onboarding/complete", onboardingHandler.Complete)
		r.Delete("/onboarding", onboardingHandler.Reset)

		// Calendar - Day Plans
		r.Get("/calendar/day", calendarHandler.GetDayPlan)
		r.Post("/calendar/day", calendarHandler.CreateDayPlan)
		r.Patch("/calendar/day/{id}", calendarHandler.UpdateDayPlan)

		// Calendar - Time Blocks
		r.Get("/calendar/blocks", calendarHandler.ListTimeBlocks)
		r.Post("/calendar/blocks", calendarHandler.CreateTimeBlock)
		r.Patch("/calendar/blocks/{id}", calendarHandler.UpdateTimeBlock)
		r.Delete("/calendar/blocks/{id}", calendarHandler.DeleteTimeBlock)

		// Calendar - Tasks
		r.Get("/calendar/tasks", calendarHandler.ListTasks)
		r.Post("/calendar/tasks", calendarHandler.CreateTask)
		r.Patch("/calendar/tasks/{id}", calendarHandler.UpdateTask)
		r.Patch("/calendar/tasks/{id}/reschedule", calendarHandler.RescheduleTask)
		r.Post("/calendar/tasks/{id}/complete", calendarHandler.CompleteTask)
		r.Post("/calendar/tasks/{id}/uncomplete", calendarHandler.UncompleteTask)
		r.Delete("/calendar/tasks/{id}", calendarHandler.DeleteTask)

		// Calendar - Week View
		r.Get("/calendar/week", calendarHandler.GetWeekView)
		r.Patch("/calendar/goals/{id}/reschedule", calendarHandler.RescheduleGoal)

		// Calendar - AI Generation
		r.Post("/calendar/ai/generate-day", calendarAIHandler.GenerateDayPlan)
		r.Post("/calendar/ai/generate-tasks", calendarAIHandler.GenerateTasksForBlock)

		// Voice / Intentions AI
		r.Post("/voice/process", voiceHandler.ProcessVoiceIntent)
		r.Post("/assistant/voice", voiceHandler.VoiceAssistant)
		r.Post("/assistant/analyze", voiceHandler.AnalyzeVoiceIntent)
		r.Get("/voice/intentions", voiceHandler.GetIntentLogs)
		r.Get("/daily-goals", voiceHandler.GetDailyGoals)
		r.Get("/daily-goals/{date}", voiceHandler.GetDailyGoalsByDate)
		r.Post("/daily-goals", voiceHandler.CreateDailyGoal)
		r.Patch("/daily-goals/{id}", voiceHandler.UpdateDailyGoal)
		r.Delete("/daily-goals/{id}", voiceHandler.DeleteDailyGoal)
		r.Post("/daily-goals/{id}/complete", voiceHandler.CompleteDailyGoal)
		r.Get("/daily-goals/{id}/subtasks", voiceHandler.GetGoalSubtasks)
		r.Post("/calendar/schedule-goals", voiceHandler.ScheduleGoalsToCalendar)

		// Google Calendar Integration
		r.Get("/google-calendar/config", googleCalendarHandler.GetConfig)
		r.Post("/google-calendar/tokens", googleCalendarHandler.SaveTokens)
		r.Patch("/google-calendar/config", googleCalendarHandler.UpdateConfig)
		r.Delete("/google-calendar/config", googleCalendarHandler.Disconnect)
		r.Post("/google-calendar/sync", googleCalendarHandler.SyncNow)
		r.Get("/google-calendar/check-weekly", googleCalendarHandler.CheckWeeklySync)

		// Community Feed
		r.Post("/community/posts", communityHandler.CreatePost)
		r.Get("/community/feed", communityHandler.GetFeed)
		r.Get("/community/posts/{id}", communityHandler.GetPost)
		r.Delete("/community/posts/{id}", communityHandler.DeletePost)
		r.Post("/community/posts/{id}/like", communityHandler.LikePost)
		r.Delete("/community/posts/{id}/like", communityHandler.UnlikePost)
		r.Post("/community/posts/{id}/report", communityHandler.ReportPost)
		r.Get("/community/my-posts", communityHandler.GetMyPosts)
		r.Get("/tasks/{id}/posts", communityHandler.GetTaskPosts)
		r.Get("/routines/{id}/posts", communityHandler.GetRoutinePosts)

		// Journal - Audio/Video Progress Journal
		r.Post("/journal/entries", journalHandler.CreateEntry)
		r.Get("/journal/entries", journalHandler.ListEntries)
		r.Get("/journal/entries/today", journalHandler.GetTodayEntry)
		r.Get("/journal/entries/streak", journalHandler.GetStreak)
		r.Get("/journal/entries/{id}", journalHandler.GetEntry)
		r.Delete("/journal/entries/{id}", journalHandler.DeleteEntry)
		r.Get("/journal/stats", journalHandler.GetStats)
		r.Post("/journal/bilans/weekly", journalHandler.GenerateWeeklyBilan)
		r.Post("/journal/bilans/monthly", journalHandler.GenerateMonthlyBilan)
		r.Get("/journal/bilans", journalHandler.ListBilans)

		// Motivation - Phrases for notifications
		r.Get("/motivation/morning", motivationHandler.GetMorningPhrase)
		r.Get("/motivation/task", motivationHandler.GetTaskReminderPhrase)
		r.Get("/motivation/all", motivationHandler.GetAllPhrases)

		// Push Notifications
		r.Post("/notifications/token", notificationsHandler.RegisterToken)
		r.Post("/notifications/token/unregister", notificationsHandler.UnregisterToken)
		r.Post("/notifications/track", notificationsHandler.TrackNotification)
		r.Get("/notifications/preferences", notificationsHandler.GetPreferences)
		r.Put("/notifications/preferences", notificationsHandler.UpdatePreferences)
		r.Get("/notifications/stats", notificationsHandler.GetStats)

		// Referral / Parrainage
		r.Get("/referral/stats", referralHandler.GetStats)
		r.Get("/referral/list", referralHandler.GetReferrals)
		r.Get("/referral/earnings", referralHandler.GetEarnings)
		r.Post("/referral/apply", referralHandler.ApplyCode)
		r.Post("/referral/activate", referralHandler.ActivateUserReferral)
	})

	// Public referral validation (no auth required)
	r.Get("/referral/validate", referralHandler.ValidateCode)

	// Admin endpoints (protected by X-Admin-Secret header)
	r.Route("/admin", func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				adminSecret := os.Getenv("ADMIN_SECRET")
				if adminSecret == "" {
					adminSecret = "focus-admin-2024" // Default for dev
				}
				if req.Header.Get("X-Admin-Secret") != adminSecret {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}
				next.ServeHTTP(w, req)
			})
		})

		// Referral admin
		r.Get("/referral/balances", referralHandler.GetAllBalances)
		r.Post("/referral/mark-paid", referralHandler.MarkUserPaid)

		// Telegram admin
		r.Post("/telegram/test", telegramHandler.TestNotification)
	})

	// Cron/Job endpoints (protected by X-Cron-Secret header)
	r.Post("/jobs/journal/monthly-analysis", journalHandler.RunMonthlyAnalysis)

	// Telegram cron jobs
	r.Post("/jobs/telegram/daily-summary", func(w http.ResponseWriter, r *http.Request) {
		cronSecret := os.Getenv("CRON_SECRET")
		if cronSecret != "" && r.Header.Get("X-Cron-Secret") != cronSecret {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		telegramHandler.SendDailySummary(w, r)
	})
	r.Post("/jobs/telegram/check-inactive", func(w http.ResponseWriter, r *http.Request) {
		cronSecret := os.Getenv("CRON_SECRET")
		if cronSecret != "" && r.Header.Get("X-Cron-Secret") != cronSecret {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		telegramHandler.CheckInactiveUsers(w, r)
	})

	// Notification cron jobs
	notificationScheduler := notifications.NewScheduler(notificationsRepo)
	r.Post("/jobs/notifications/morning", func(w http.ResponseWriter, r *http.Request) {
		// Verify cron secret
		cronSecret := os.Getenv("CRON_SECRET")
		if cronSecret != "" && r.Header.Get("X-Cron-Secret") != cronSecret {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		if err := notificationScheduler.SendMorningNotifications(r.Context()); err != nil {
			log.Printf("❌ Morning notifications failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Write([]byte("OK"))
	})

	r.Post("/jobs/notifications/evening", func(w http.ResponseWriter, r *http.Request) {
		cronSecret := os.Getenv("CRON_SECRET")
		if cronSecret != "" && r.Header.Get("X-Cron-Secret") != cronSecret {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		if err := notificationScheduler.SendEveningNotifications(r.Context()); err != nil {
			log.Printf("❌ Evening notifications failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Write([]byte("OK"))
	})

	r.Post("/jobs/notifications/streak-danger", func(w http.ResponseWriter, r *http.Request) {
		cronSecret := os.Getenv("CRON_SECRET")
		if cronSecret != "" && r.Header.Get("X-Cron-Secret") != cronSecret {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		if err := notificationScheduler.SendStreakDangerNotifications(r.Context()); err != nil {
			log.Printf("❌ Streak danger notifications failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Write([]byte("OK"))
	})

	r.Post("/jobs/notifications/task-reminders", func(w http.ResponseWriter, r *http.Request) {
		cronSecret := os.Getenv("CRON_SECRET")
		if cronSecret != "" && r.Header.Get("X-Cron-Secret") != cronSecret {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		if err := notificationScheduler.SendTaskReminders(r.Context()); err != nil {
			log.Printf("❌ Task reminders failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Write([]byte("OK"))
	})

	// WebSocket endpoint for real-time updates (requires auth)
	r.Group(func(r chi.Router) {
		r.Use(authMW)
		r.Get("/ws", func(w http.ResponseWriter, r *http.Request) {
			userID := r.Context().Value(auth.UserContextKey).(string)
			ws.ServeWs(ws.GlobalHub, w, r, userID)
		})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Server starting on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}