package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"

	"firelevel-backend/internal/auth"
	"firelevel-backend/internal/calendar"
	"firelevel-backend/internal/calendarevents"
	"firelevel-backend/internal/chat"
	"firelevel-backend/internal/database"
	"firelevel-backend/internal/focus"
	"firelevel-backend/internal/gcalendar"
	"firelevel-backend/internal/gmail"
	"firelevel-backend/internal/notifications"
	"firelevel-backend/internal/onboarding"
	"firelevel-backend/internal/routines"
	"firelevel-backend/internal/users"
	"firelevel-backend/internal/quests"
	"firelevel-backend/internal/discover"
	"firelevel-backend/internal/focusrooms"
	"firelevel-backend/internal/voice"
)

// ===========================================
// FOCUS BACKEND - Kai AI Motivation Coach
// ===========================================
// Core features:
// 1. Chat with Kai (semantic memory, persona from Gmail)
// 2. Tasks (calendar)
// 3. Focus sessions (pomodoro)
// 4. Routines/habits
// 5. User profile + location
// 6. Journal (audio/video entries)
// 7. Push notifications
// 8. Onboarding
// 9. Gmail integration (AI persona building)
// ===========================================

func main() {
	_ = godotenv.Load()

	// 1. Initialize Postgres Pool
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
	routinesHandler := routines.NewHandler(pool)
	completionsHandler := routines.NewCompletionHandler(pool)
	focusHandler := focus.NewHandler(pool)
	onboardingHandler := onboarding.NewHandler(pool)
	calendarHandler := calendar.NewHandler(pool)
	chatHandler := chat.NewHandler(pool)

	// Initialize Backboard API key for the new AI chat handler
	if bbKey := os.Getenv("BACKBOARD_API_KEY"); bbKey != "" {
		chat.SetBackboardAPIKey(bbKey)
		log.Println("✅ Backboard API key loaded")
	} else {
		log.Println("⚠️ BACKBOARD_API_KEY not set — /chat/v2 endpoints will be unavailable")
	}
	notificationsHandler := notifications.NewHandler(pool)
	gmailHandler := gmail.NewHandler(pool)
	questsHandler := quests.NewHandler(pool)
	voiceHandler := voice.NewHandler(jwtSecret)
	gcalendarHandler := gcalendar.NewHandler(pool)
	calendarEventsHandler := calendarevents.NewHandler(pool)
	discoverHandler := discover.NewHandler(pool)
	focusRoomsHandler := focusrooms.NewHandler(pool)

	// 4. Setup Router
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// CORS for development
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
			next.ServeHTTP(w, r)
		})
	})

	// Public Routes
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})
	r.Get("/version", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("v3.0-backboard-server"))
	})

	// Protected Routes
	r.Group(func(r chi.Router) {
		r.Use(authMW)

		// =====================
		// CHAT WITH KAI (Core)
		// =====================
		r.Post("/chat/message", chatHandler.SendMessage)       // Legacy (Gemini)
		r.Post("/chat/voice", chatHandler.SendVoiceMessage)     // Legacy (Gemini)
		r.Post("/chat/tts", chatHandler.TextToSpeech)
		r.Get("/chat/history", chatHandler.GetHistory)           // Legacy (empty)
		r.Delete("/chat/history", chatHandler.ClearHistory)      // Legacy (noop)

		// Backboard-powered chat (v2) — tools run server-side
		r.Post("/chat/v2/message", chatHandler.SendMessageV2)
		r.Post("/chat/v2/voice", chatHandler.SendVoiceMessageV2)
		r.Get("/chat/v2/history", chatHandler.GetHistoryV2)
		r.Delete("/chat/v2/history", chatHandler.ClearHistoryV2)

		// =====================
		// USER PROFILE
		// =====================
		r.Get("/me", usersHandler.GetProfile)
		r.Patch("/me", usersHandler.UpdateProfile)
		r.Delete("/me", usersHandler.DeleteAccount)
		r.Post("/me/avatar", usersHandler.UploadAvatar)
		r.Delete("/me/avatar", usersHandler.DeleteAvatar)
		r.Put("/me/email", usersHandler.ChangeEmail)
		r.Put("/me/password", usersHandler.ChangePassword)

		// =====================
		// TASKS (Calendar)
		// =====================
		r.Get("/calendar/tasks", calendarHandler.ListTasks)
		r.Post("/calendar/tasks", calendarHandler.CreateTask)
		r.Patch("/calendar/tasks/{id}", calendarHandler.UpdateTask)
		r.Post("/calendar/tasks/{id}/complete", calendarHandler.CompleteTask)
		r.Post("/calendar/tasks/{id}/uncomplete", calendarHandler.UncompleteTask)
		r.Delete("/calendar/tasks/{id}", calendarHandler.DeleteTask)
		r.Get("/calendar/week", calendarHandler.GetWeekView)

		// =====================
		// FOCUS SESSIONS
		// =====================
		r.Get("/focus-sessions", focusHandler.List)
		r.Post("/focus-sessions", focusHandler.Start)
		r.Patch("/focus-sessions/{id}", focusHandler.Update)
		r.Delete("/focus-sessions/{id}", focusHandler.Delete)

		// =====================
		// ROUTINES (Habits)
		// =====================
		r.Get("/routines", routinesHandler.List)
		r.Post("/routines", routinesHandler.Create)
		r.Patch("/routines/{id}", routinesHandler.Update)
		r.Delete("/routines/{id}", routinesHandler.Delete)
		r.Post("/routines/{id}/complete", routinesHandler.Complete)
		r.Delete("/routines/{id}/complete", routinesHandler.Uncomplete)
		r.Get("/completions", completionsHandler.List)

		// =====================
		// ONBOARDING (22 Steps)
		// =====================
		r.Get("/onboarding/status", onboardingHandler.GetStatus)
		r.Put("/onboarding/progress", onboardingHandler.SaveProgress)
		r.Post("/onboarding/complete", onboardingHandler.Complete)
		r.Delete("/onboarding/reset", onboardingHandler.Reset)


		// =====================
		// NOTIFICATIONS
		// =====================
		r.Post("/notifications/device-token", notificationsHandler.RegisterToken)
		r.Delete("/notifications/device-token", notificationsHandler.DeleteToken)
		r.Get("/notifications/settings", notificationsHandler.GetSettings)
		r.Patch("/notifications/settings", notificationsHandler.UpdateSettings)

		// =====================
		// QUESTS
		// =====================
		r.Get("/quests", questsHandler.List)
		r.Post("/quests", questsHandler.Create)

		// =====================
		// VOICE (LiveKit)
		// =====================
		r.Post("/voice/livekit-token", voiceHandler.GenerateLiveKitToken)

		// =====================
		// GMAIL INTEGRATION
		// =====================
		r.Get("/gmail/config", gmailHandler.GetConfig)
		r.Post("/gmail/tokens", gmailHandler.SaveTokens)
		r.Post("/gmail/analyze", gmailHandler.Analyze)
		r.Delete("/gmail/config", gmailHandler.Disconnect)

		// =====================
		// GOOGLE CALENDAR
		// =====================
		r.Get("/google-calendar/config", gcalendarHandler.GetConfig)
		r.Post("/google-calendar/tokens", gcalendarHandler.SaveTokens)
		r.Patch("/google-calendar/config", gcalendarHandler.UpdateConfig)
		r.Delete("/google-calendar/config", gcalendarHandler.Disconnect)
		r.Post("/google-calendar/sync", gcalendarHandler.Sync)
		r.Get("/google-calendar/check-weekly", gcalendarHandler.CheckWeekly)

		// =====================
		// CALENDAR EVENTS (cached external events)
		// =====================
		r.Get("/calendar/events", calendarEventsHandler.ListEvents)
		r.Get("/calendar/providers", calendarEventsHandler.ListProviders)
		r.Get("/calendar/blocking-schedule", calendarEventsHandler.GetBlockingSchedule)
		r.Patch("/calendar/events/{id}/blocking", calendarEventsHandler.UpdateBlocking)

		// =====================
		// LOCATION
		// =====================
		r.Post("/me/location", usersHandler.UpdateLocation)

		// =====================
		// DISCOVER MAP
		// =====================
		r.Get("/discover/users", discoverHandler.ListNearbyUsers)

		// =====================
		// FOCUS ROOMS (Group Sessions)
		// =====================
		r.Get("/focus-rooms", focusRoomsHandler.List)
		r.Post("/focus-rooms/join", focusRoomsHandler.Join)
		r.Get("/focus-rooms/{id}", focusRoomsHandler.Get)
		r.Post("/focus-rooms/{id}/leave", focusRoomsHandler.Leave)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("🔥 Kai Backend starting on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}
