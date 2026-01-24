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
	"firelevel-backend/internal/chat"
	"firelevel-backend/internal/database"
	"firelevel-backend/internal/focus"
	"firelevel-backend/internal/onboarding"
	"firelevel-backend/internal/routines"
	"firelevel-backend/internal/users"
)

// ===========================================
// CLEAN BACKEND - Focus on Kai as AI Friend
// ===========================================
// Core features:
// 1. Chat with Kai (infinite memory, focus intent detection)
// 2. Tasks (with app blocking)
// 3. Focus sessions
// 4. Routines/habits
// 5. User profile
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

	// 3. Initialize Handlers (only essential ones)
	usersHandler := users.NewHandler(pool)
	routinesHandler := routines.NewHandler(pool)
	completionsHandler := routines.NewCompletionHandler(pool)
	focusHandler := focus.NewHandler(pool)
	onboardingHandler := onboarding.NewHandler(pool)
	calendarHandler := calendar.NewHandler(pool)
	chatHandler := chat.NewHandler(pool)

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

	// Protected Routes
	r.Group(func(r chi.Router) {
		r.Use(authMW)

		// =====================
		// CHAT WITH KAI (Core)
		// =====================
		r.Post("/chat/message", chatHandler.SendMessage)
		r.Post("/chat/voice", chatHandler.SendVoiceMessage)
		r.Get("/chat/history", chatHandler.GetHistory)
		r.Delete("/chat/history", chatHandler.ClearHistory)

		// =====================
		// USER PROFILE
		// =====================
		r.Get("/me", usersHandler.GetProfile)
		r.Patch("/me", usersHandler.UpdateProfile)
		r.Delete("/me", usersHandler.DeleteAccount) // Account deletion (GDPR)
		r.Post("/me/avatar", usersHandler.UploadAvatar)
		r.Delete("/me/avatar", usersHandler.DeleteAvatar)

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
		// ONBOARDING
		// =====================
		r.Get("/onboarding/status", onboardingHandler.GetStatus)
		r.Put("/onboarding/progress", onboardingHandler.SaveProgress)
		r.Post("/onboarding/complete", onboardingHandler.Complete)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("🔥 Kai Backend starting on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}
