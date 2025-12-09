package onboarding

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"firelevel-backend/internal/auth"

	"github.com/jackc/pgx/v5/pgxpool"
)

// OnboardingData represents user onboarding responses stored in the database
type OnboardingData struct {
	ID              string     `json:"id"`
	UserID          string     `json:"user_id"`
	ProjectStatus   *string    `json:"project_status"`   // idea, in_progress, launched
	TimeAvailable   *string    `json:"time_available"`   // less_than_2h, 2_to_5h, 5_to_10h, more_than_10h
	Goals           []string   `json:"goals"`            // Array of goal IDs
	CompletedAt     *time.Time `json:"completed_at"`     // When onboarding was completed
	CurrentStep     int        `json:"current_step"`     // Current step in onboarding (1-12)
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// OnboardingStatus represents the current onboarding state
type OnboardingStatus struct {
	IsCompleted   bool       `json:"is_completed"`
	CurrentStep   int        `json:"current_step"`
	TotalSteps    int        `json:"total_steps"`
	ProjectStatus *string    `json:"project_status,omitempty"`
	TimeAvailable *string    `json:"time_available,omitempty"`
	Goals         []string   `json:"goals,omitempty"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
}

// SaveOnboardingRequest for saving onboarding progress
type SaveOnboardingRequest struct {
	ProjectStatus *string  `json:"project_status,omitempty"`
	TimeAvailable *string  `json:"time_available,omitempty"`
	Goals         []string `json:"goals,omitempty"`
	CurrentStep   int      `json:"current_step"`
	IsComplete    bool     `json:"is_complete"`
}

// Handler holds database connection
type Handler struct {
	db *pgxpool.Pool
}

// NewHandler creates a new onboarding handler
func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
}

// GetStatus returns the current onboarding status for a user
// GET /onboarding/status
func (h *Handler) GetStatus(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	query := `
		SELECT
			project_status,
			time_available,
			goals,
			current_step,
			completed_at
		FROM public.user_onboarding
		WHERE user_id = $1
	`

	var status OnboardingStatus
	status.TotalSteps = 13 // Total onboarding steps
	status.Goals = []string{}

	var goalsJSON []byte
	err := h.db.QueryRow(r.Context(), query, userID).Scan(
		&status.ProjectStatus,
		&status.TimeAvailable,
		&goalsJSON,
		&status.CurrentStep,
		&status.CompletedAt,
	)

	if err != nil {
		// No onboarding record exists yet - user hasn't started
		status.IsCompleted = false
		status.CurrentStep = 1
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
		return
	}

	// Parse goals JSON
	if goalsJSON != nil {
		json.Unmarshal(goalsJSON, &status.Goals)
	}

	status.IsCompleted = status.CompletedAt != nil

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// SaveProgress saves or updates onboarding progress
// PUT /onboarding/progress
func (h *Handler) SaveProgress(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	fmt.Printf("📝 SaveProgress called - userID: %s\n", userID)

	var req SaveOnboardingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fmt.Printf("❌ SaveProgress decode error: %v\n", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	fmt.Printf("📝 SaveProgress request - step: %d, projectStatus: %v, timeAvailable: %v, goals: %v\n", req.CurrentStep, req.ProjectStatus, req.TimeAvailable, req.Goals)

	// Convert goals to JSON string for PostgreSQL jsonb
	var goalsJSONStr string
	if req.Goals == nil || len(req.Goals) == 0 {
		goalsJSONStr = "[]"
	} else {
		goalsBytes, _ := json.Marshal(req.Goals)
		goalsJSONStr = string(goalsBytes)
	}
	fmt.Printf("📝 Goals JSON string: %s\n", goalsJSONStr)

	// Determine completed_at
	var completedAt *time.Time
	if req.IsComplete {
		now := time.Now()
		completedAt = &now
	}

	// Upsert onboarding data
	query := `
		INSERT INTO public.user_onboarding (
			user_id, project_status, time_available, goals, current_step, completed_at, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
		ON CONFLICT (user_id)
		DO UPDATE SET
			project_status = COALESCE(EXCLUDED.project_status, user_onboarding.project_status),
			time_available = COALESCE(EXCLUDED.time_available, user_onboarding.time_available),
			goals = CASE
				WHEN EXCLUDED.goals::text != '[]' THEN EXCLUDED.goals
				ELSE user_onboarding.goals
			END,
			current_step = GREATEST(EXCLUDED.current_step, user_onboarding.current_step),
			completed_at = COALESCE(EXCLUDED.completed_at, user_onboarding.completed_at),
			updated_at = NOW()
		RETURNING id, project_status, time_available, goals, current_step, completed_at
	`

	var data OnboardingData
	var returnedGoalsJSON []byte
	err := h.db.QueryRow(r.Context(), query,
		userID,
		req.ProjectStatus,
		req.TimeAvailable,
		goalsJSONStr,
		req.CurrentStep,
		completedAt,
	).Scan(
		&data.ID,
		&data.ProjectStatus,
		&data.TimeAvailable,
		&returnedGoalsJSON,
		&data.CurrentStep,
		&data.CompletedAt,
	)

	if err != nil {
		fmt.Printf("❌ Onboarding save error for user %s: %v\n", userID, err)
		http.Error(w, "Failed to save onboarding progress", http.StatusInternalServerError)
		return
	}
	fmt.Printf("✅ Onboarding saved for user %s - id: %s, step: %d\n", userID, data.ID, data.CurrentStep)

	// Parse goals
	if returnedGoalsJSON != nil {
		json.Unmarshal(returnedGoalsJSON, &data.Goals)
	}
	if data.Goals == nil {
		data.Goals = []string{}
	}

	// Return status
	status := OnboardingStatus{
		IsCompleted:   data.CompletedAt != nil,
		CurrentStep:   data.CurrentStep,
		TotalSteps:    13,
		ProjectStatus: data.ProjectStatus,
		TimeAvailable: data.TimeAvailable,
		Goals:         data.Goals,
		CompletedAt:   data.CompletedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// Complete marks onboarding as completed
// POST /onboarding/complete
func (h *Handler) Complete(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	fmt.Printf("🏁 Complete called - userID: %s\n", userID)

	query := `
		UPDATE public.user_onboarding
		SET completed_at = NOW(), current_step = 13, updated_at = NOW()
		WHERE user_id = $1
		RETURNING id, project_status, time_available, goals, current_step, completed_at
	`

	var data OnboardingData
	var goalsJSON []byte
	err := h.db.QueryRow(r.Context(), query, userID).Scan(
		&data.ID,
		&data.ProjectStatus,
		&data.TimeAvailable,
		&goalsJSON,
		&data.CurrentStep,
		&data.CompletedAt,
	)

	if err != nil {
		// If no record exists, create one and mark complete
		fmt.Printf("📝 No existing onboarding record for user %s, creating new one\n", userID)
		createQuery := `
			INSERT INTO public.user_onboarding (user_id, current_step, completed_at, created_at, updated_at)
			VALUES ($1, 13, NOW(), NOW(), NOW())
			RETURNING id, project_status, time_available, goals, current_step, completed_at
		`
		err = h.db.QueryRow(r.Context(), createQuery, userID).Scan(
			&data.ID,
			&data.ProjectStatus,
			&data.TimeAvailable,
			&goalsJSON,
			&data.CurrentStep,
			&data.CompletedAt,
		)
		if err != nil {
			fmt.Printf("❌ Onboarding complete error for user %s: %v\n", userID, err)
			http.Error(w, "Failed to complete onboarding", http.StatusInternalServerError)
			return
		}
		fmt.Printf("✅ Onboarding created and completed for user %s\n", userID)
	} else {
		fmt.Printf("✅ Onboarding updated and completed for user %s\n", userID)
	}

	// Parse goals
	if goalsJSON != nil {
		json.Unmarshal(goalsJSON, &data.Goals)
	}
	if data.Goals == nil {
		data.Goals = []string{}
	}

	status := OnboardingStatus{
		IsCompleted:   true,
		CurrentStep:   13,
		TotalSteps:    13,
		ProjectStatus: data.ProjectStatus,
		TimeAvailable: data.TimeAvailable,
		Goals:         data.Goals,
		CompletedAt:   data.CompletedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// Reset resets onboarding for a user (useful for testing)
// DELETE /onboarding
func (h *Handler) Reset(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	query := `DELETE FROM public.user_onboarding WHERE user_id = $1`
	_, err := h.db.Exec(r.Context(), query, userID)
	if err != nil {
		fmt.Println("Onboarding reset error:", err)
		http.Error(w, "Failed to reset onboarding", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
