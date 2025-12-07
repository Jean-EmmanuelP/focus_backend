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
	"firelevel-backend/internal/crew"
	"firelevel-backend/internal/database"
	"firelevel-backend/internal/focus"
	"firelevel-backend/internal/intentions"
	"firelevel-backend/internal/quests"
	"firelevel-backend/internal/reflections"
	"firelevel-backend/internal/routines"
	"firelevel-backend/internal/stats"
	"firelevel-backend/internal/users"
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

		// Crew / Social
		r.Get("/crew/members", crewHandler.ListMembers)
		r.Delete("/crew/members/{id}", crewHandler.RemoveMember)
		r.Get("/crew/requests/received", crewHandler.ListReceivedRequests)
		r.Get("/crew/requests/sent", crewHandler.ListSentRequests)
		r.Post("/crew/requests", crewHandler.SendRequest)
		r.Post("/crew/requests/{id}/accept", crewHandler.AcceptRequest)
		r.Post("/crew/requests/{id}/reject", crewHandler.RejectRequest)
		r.Get("/crew/search", crewHandler.SearchUsers)
		r.Get("/crew/leaderboard", crewHandler.GetLeaderboard)
		r.Get("/crew/members/{id}/day", crewHandler.GetMemberDay)
		r.Patch("/me/visibility", crewHandler.UpdateVisibility)
		r.Get("/me/stats", crewHandler.GetMyStats)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Server starting on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}