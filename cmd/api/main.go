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
	"firelevel-backend/internal/crew"
	"firelevel-backend/internal/database"
	"firelevel-backend/internal/focus"
	"firelevel-backend/internal/googlecalendar"
	"firelevel-backend/internal/intentions"
	"firelevel-backend/internal/onboarding"
	"firelevel-backend/internal/quests"
	"firelevel-backend/internal/reflections"
	"firelevel-backend/internal/routines"
	"firelevel-backend/internal/stats"
	"firelevel-backend/internal/streaks"
	"firelevel-backend/internal/users"
	"firelevel-backend/internal/voice"
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
	calendarHandler := calendar.NewHandler(pool)
	calendarAIHandler := calendar.NewAIHandler(pool)
	voiceHandler := voice.NewHandler(pool)
	googleCalendarHandler := googlecalendar.NewHandler(pool)

	// Connect Google Calendar sync to handlers
	calendarHandler.SetGoogleCalendarSyncer(googleCalendarHandler)
	routinesHandler.SetGoogleCalendarSyncer(googleCalendarHandler)

	// 4. Setup Router
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Public Routes
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
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
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Server starting on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}