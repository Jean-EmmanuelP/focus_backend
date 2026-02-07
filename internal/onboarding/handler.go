package onboarding

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"firelevel-backend/internal/auth"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ===========================================
// ONBOARDING - 22 Steps (PRD v2)
// ===========================================
// Steps:
//  0 - User name (first_name, last_name)
//  1 - Age range
//  2 - Pronouns
//  3 - Companion role
//  4 - Companion gender
//  5 - Companion name
//  6 - Social proof (user count)
//  7 - Wellness goals (multi-select)
//  8 - Life improvements (multi-select)
//  9 - Guide expectations (multi-select)
// 10 - Companion presentation (fullscreen)
// 11 - Personal development areas (multi-select)
// 12 - Additional activities (multi-select)
// 13 - Agreement question #1 (yes/no)
// 14 - Agreement question #2 (yes/no)
// 15 - Agreement question #3 (yes/no)
// 16 - Agreement question #4 (yes/no)
// 17 - Agreement question #5 (yes/no)
// 18 - Social proof (ratings)
// 19 - Loading / companion creation
// 20 - Paywall / subscription
// 21 - Final screen - meet companion
// ===========================================

const totalOnboardingSteps = 22

// OnboardingData represents user onboarding responses stored in the database
type OnboardingData struct {
	ID            string     `json:"id"`
	UserID        string     `json:"user_id"`
	CurrentStep   int        `json:"current_step"`
	CompletedAt   *time.Time `json:"completed_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`

	// Legacy fields (kept for backward compat)
	ProjectStatus *string  `json:"project_status"`
	TimeAvailable *string  `json:"time_available"`
	Goals         []string `json:"goals"`

	// New PRD v2 fields (stored as JSONB)
	Responses json.RawMessage `json:"responses"` // Full onboarding responses
}

// OnboardingStatus represents the current onboarding state
type OnboardingStatus struct {
	IsCompleted   bool                   `json:"is_completed"`
	CurrentStep   int                    `json:"current_step"`
	TotalSteps    int                    `json:"total_steps"`
	CompletedAt   *time.Time             `json:"completed_at,omitempty"`
	Responses     map[string]interface{} `json:"responses,omitempty"`

	// Legacy fields kept for iOS backward compat
	ProjectStatus *string  `json:"project_status,omitempty"`
	TimeAvailable *string  `json:"time_available,omitempty"`
	Goals         []string `json:"goals,omitempty"`
}

// SaveOnboardingRequest for saving onboarding progress
type SaveOnboardingRequest struct {
	CurrentStep int    `json:"current_step"`
	IsComplete  bool   `json:"is_complete"`

	// Step-specific data (all optional, sent per step)
	FirstName       *string  `json:"first_name,omitempty"`
	LastName        *string  `json:"last_name,omitempty"`
	AgeRange        *string  `json:"age_range,omitempty"`        // "less_than_18", "18-24", "25-34", etc.
	Pronouns        *string  `json:"pronouns,omitempty"`         // "elle_la", "il_lui", "iel_iels"
	CompanionRole   *string  `json:"companion_role,omitempty"`   // role chosen
	CompanionGender *string  `json:"companion_gender,omitempty"` // "female", "male", "non_binary"
	CompanionName   *string  `json:"companion_name,omitempty"`   // custom name
	AvatarStyle     *string  `json:"avatar_style,omitempty"`     // "realistic", "anime", etc.

	// Multi-select arrays
	WellnessGoals       []string `json:"wellness_goals,omitempty"`       // step 7
	LifeImprovements    []string `json:"life_improvements,omitempty"`    // step 8
	GuideExpectations   []string `json:"guide_expectations,omitempty"`   // step 9
	DevelopmentAreas    []string `json:"development_areas,omitempty"`    // step 11
	AdditionalActivities []string `json:"additional_activities,omitempty"` // step 12

	// Yes/No agreement questions (steps 13-17)
	AgreementQ1 *bool `json:"agreement_q1,omitempty"`
	AgreementQ2 *bool `json:"agreement_q2,omitempty"`
	AgreementQ3 *bool `json:"agreement_q3,omitempty"`
	AgreementQ4 *bool `json:"agreement_q4,omitempty"`
	AgreementQ5 *bool `json:"agreement_q5,omitempty"`

	// Legacy fields (backward compat)
	ProjectStatus *string  `json:"project_status,omitempty"`
	TimeAvailable *string  `json:"time_available,omitempty"`
	Goals         []string `json:"goals,omitempty"`
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
	fmt.Printf("📋 GetStatus called - userID: %s\n", userID)

	query := `
		SELECT
			current_step,
			completed_at,
			project_status,
			time_available,
			goals,
			COALESCE(responses, '{}'::jsonb) as responses
		FROM public.user_onboarding
		WHERE user_id = $1
	`

	var status OnboardingStatus
	status.TotalSteps = totalOnboardingSteps
	status.Goals = []string{}

	var goalsJSON []byte
	var responsesJSON []byte

	err := h.db.QueryRow(r.Context(), query, userID).Scan(
		&status.CurrentStep,
		&status.CompletedAt,
		&status.ProjectStatus,
		&status.TimeAvailable,
		&goalsJSON,
		&responsesJSON,
	)

	if err != nil {
		// No onboarding record exists yet - user hasn't started
		fmt.Printf("📋 GetStatus - No record found for user %s, starting onboarding\n", userID)
		status.IsCompleted = false
		status.CurrentStep = 0
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
		return
	}

	// Parse goals JSON
	if goalsJSON != nil {
		json.Unmarshal(goalsJSON, &status.Goals)
	}

	// Parse responses JSON
	if responsesJSON != nil && len(responsesJSON) > 2 { // > 2 means not just "{}"
		var responses map[string]interface{}
		if err := json.Unmarshal(responsesJSON, &responses); err == nil {
			status.Responses = responses
		}
	}

	status.IsCompleted = status.CompletedAt != nil
	fmt.Printf("📋 GetStatus - Found record for user %s: step=%d, isCompleted=%v\n",
		userID, status.CurrentStep, status.IsCompleted)

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
	fmt.Printf("📝 SaveProgress request - step: %d\n", req.CurrentStep)

	// Build the responses JSON from step-specific data
	responses := h.buildResponsesJSON(req)
	responsesBytes, _ := json.Marshal(responses)

	// Convert goals to JSON string for PostgreSQL jsonb
	var goalsJSONStr string
	if req.Goals == nil || len(req.Goals) == 0 {
		goalsJSONStr = "[]"
	} else {
		goalsBytes, _ := json.Marshal(req.Goals)
		goalsJSONStr = string(goalsBytes)
	}

	// Determine completed_at
	var completedAt *time.Time
	if req.IsComplete {
		now := time.Now()
		completedAt = &now
	}

	// Upsert onboarding data with responses merge
	query := `
		INSERT INTO public.user_onboarding (
			user_id, project_status, time_available, goals, current_step, completed_at, responses, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, NOW(), NOW())
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
			responses = COALESCE(user_onboarding.responses, '{}'::jsonb) || EXCLUDED.responses::jsonb,
			updated_at = NOW()
		RETURNING id, current_step, completed_at, project_status, time_available, goals, COALESCE(responses, '{}'::jsonb)
	`

	var data struct {
		ID            string
		CurrentStep   int
		CompletedAt   *time.Time
		ProjectStatus *string
		TimeAvailable *string
	}
	var returnedGoalsJSON []byte
	var returnedResponsesJSON []byte

	err := h.db.QueryRow(r.Context(), query,
		userID,
		req.ProjectStatus,
		req.TimeAvailable,
		goalsJSONStr,
		req.CurrentStep,
		completedAt,
		string(responsesBytes),
	).Scan(
		&data.ID,
		&data.CurrentStep,
		&data.CompletedAt,
		&data.ProjectStatus,
		&data.TimeAvailable,
		&returnedGoalsJSON,
		&returnedResponsesJSON,
	)

	if err != nil {
		fmt.Printf("❌ Onboarding save error for user %s: %v\n", userID, err)
		http.Error(w, "Failed to save onboarding progress", http.StatusInternalServerError)
		return
	}
	fmt.Printf("✅ Onboarding saved for user %s - id: %s, step: %d\n", userID, data.ID, data.CurrentStep)

	// Also update user profile with collected data (name, pronouns, etc.)
	h.updateUserProfile(r, userID, req)

	// Parse goals
	var goals []string
	if returnedGoalsJSON != nil {
		json.Unmarshal(returnedGoalsJSON, &goals)
	}
	if goals == nil {
		goals = []string{}
	}

	// Parse responses
	var respMap map[string]interface{}
	if returnedResponsesJSON != nil {
		json.Unmarshal(returnedResponsesJSON, &respMap)
	}

	// Return status
	status := OnboardingStatus{
		IsCompleted:   data.CompletedAt != nil,
		CurrentStep:   data.CurrentStep,
		TotalSteps:    totalOnboardingSteps,
		ProjectStatus: data.ProjectStatus,
		TimeAvailable: data.TimeAvailable,
		Goals:         goals,
		CompletedAt:   data.CompletedAt,
		Responses:     respMap,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// buildResponsesJSON merges step-specific data into a JSON map
func (h *Handler) buildResponsesJSON(req SaveOnboardingRequest) map[string]interface{} {
	responses := make(map[string]interface{})

	if req.FirstName != nil {
		responses["first_name"] = *req.FirstName
	}
	if req.LastName != nil {
		responses["last_name"] = *req.LastName
	}
	if req.AgeRange != nil {
		responses["age_range"] = *req.AgeRange
	}
	if req.Pronouns != nil {
		responses["pronouns"] = *req.Pronouns
	}
	if req.CompanionRole != nil {
		responses["companion_role"] = *req.CompanionRole
	}
	if req.CompanionGender != nil {
		responses["companion_gender"] = *req.CompanionGender
	}
	if req.CompanionName != nil {
		responses["companion_name"] = *req.CompanionName
	}
	if req.AvatarStyle != nil {
		responses["avatar_style"] = *req.AvatarStyle
	}
	if len(req.WellnessGoals) > 0 {
		responses["wellness_goals"] = req.WellnessGoals
	}
	if len(req.LifeImprovements) > 0 {
		responses["life_improvements"] = req.LifeImprovements
	}
	if len(req.GuideExpectations) > 0 {
		responses["guide_expectations"] = req.GuideExpectations
	}
	if len(req.DevelopmentAreas) > 0 {
		responses["development_areas"] = req.DevelopmentAreas
	}
	if len(req.AdditionalActivities) > 0 {
		responses["additional_activities"] = req.AdditionalActivities
	}
	if req.AgreementQ1 != nil {
		responses["agreement_q1"] = *req.AgreementQ1
	}
	if req.AgreementQ2 != nil {
		responses["agreement_q2"] = *req.AgreementQ2
	}
	if req.AgreementQ3 != nil {
		responses["agreement_q3"] = *req.AgreementQ3
	}
	if req.AgreementQ4 != nil {
		responses["agreement_q4"] = *req.AgreementQ4
	}
	if req.AgreementQ5 != nil {
		responses["agreement_q5"] = *req.AgreementQ5
	}

	return responses
}

// updateUserProfile syncs onboarding data to the user profile
func (h *Handler) updateUserProfile(r *http.Request, userID string, req SaveOnboardingRequest) {
	// Update first_name / last_name if provided (step 0)
	if req.FirstName != nil || req.LastName != nil {
		if req.FirstName != nil && req.LastName != nil {
			h.db.Exec(r.Context(),
				`UPDATE public.users SET first_name = $1, last_name = $2 WHERE id = $3`,
				*req.FirstName, *req.LastName, userID)
		} else if req.FirstName != nil {
			h.db.Exec(r.Context(),
				`UPDATE public.users SET first_name = $1 WHERE id = $2`,
				*req.FirstName, userID)
		} else if req.LastName != nil {
			h.db.Exec(r.Context(),
				`UPDATE public.users SET last_name = $1 WHERE id = $2`,
				*req.LastName, userID)
		}
	}

	// Update pronouns → gender field (step 2)
	if req.Pronouns != nil {
		gender := "prefer_not_to_say"
		switch *req.Pronouns {
		case "elle_la":
			gender = "female"
		case "il_lui":
			gender = "male"
		case "iel_iels":
			gender = "other"
		}
		h.db.Exec(r.Context(),
			`UPDATE public.users SET gender = $1 WHERE id = $2`,
			gender, userID)
	}

	// Update age from age range (step 1)
	if req.AgeRange != nil {
		// Store age range as approximate midpoint
		ageMap := map[string]int{
			"less_than_18": 16,
			"18-24":        21,
			"25-34":        30,
			"35-44":        40,
			"45-54":        50,
			"55-64":        60,
			"65_plus":      70,
		}
		if age, ok := ageMap[*req.AgeRange]; ok {
			h.db.Exec(r.Context(),
				`UPDATE public.users SET age = $1 WHERE id = $2`,
				age, userID)
		}
	}

	// Update companion name (step 9 - "Nommez votre Focus")
	if req.CompanionName != nil {
		h.db.Exec(r.Context(),
			`UPDATE public.users SET companion_name = $1 WHERE id = $2`,
			*req.CompanionName, userID)
		fmt.Printf("✅ Saved companion name '%s' for user %s\n", *req.CompanionName, userID)
	}

	// Update companion gender (step 8 - "Personnalisez votre Focus")
	if req.CompanionGender != nil {
		h.db.Exec(r.Context(),
			`UPDATE public.users SET companion_gender = $1 WHERE id = $2`,
			*req.CompanionGender, userID)
	}

	// Update avatar style (step 8 - "Personnalisez votre Focus")
	if req.AvatarStyle != nil {
		h.db.Exec(r.Context(),
			`UPDATE public.users SET avatar_style = $1 WHERE id = $2`,
			*req.AvatarStyle, userID)
	}

	// Update work place from life_improvements (step 7 - "Où travaillez-vous")
	if len(req.LifeImprovements) > 0 {
		// First item is typically the work place
		workPlace := req.LifeImprovements[0]
		h.db.Exec(r.Context(),
			`UPDATE public.users SET work_place = $1 WHERE id = $2`,
			workPlace, userID)
		fmt.Printf("✅ Saved work place '%s' for user %s\n", workPlace, userID)
	}
}

// CompleteOnboardingRequest for the completion endpoint
type CompleteOnboardingRequest struct {
	Goals            []string `json:"goals,omitempty"`
	ProductivityPeak string   `json:"productivity_peak,omitempty"`
	IsComplete       bool     `json:"is_complete"`
}

// Complete marks onboarding as completed
// POST /onboarding/complete
func (h *Handler) Complete(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	fmt.Printf("🏁 Complete called - userID: %s\n", userID)

	// Parse optional request body
	var req CompleteOnboardingRequest
	json.NewDecoder(r.Body).Decode(&req) // Ignore error - body might be empty

	// Convert goals to JSON string
	var goalsJSONStr string
	if req.Goals == nil || len(req.Goals) == 0 {
		goalsJSONStr = "[]"
	} else {
		goalsBytes, _ := json.Marshal(req.Goals)
		goalsJSONStr = string(goalsBytes)
	}

	// Store productivity_peak in time_available field (reusing existing column)
	var productivityPeak *string
	if req.ProductivityPeak != "" {
		productivityPeak = &req.ProductivityPeak
	}

	// Upsert: create if not exists, update if exists
	query := `
		INSERT INTO public.user_onboarding (
			user_id, goals, time_available, current_step, completed_at, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, NOW(), NOW(), NOW())
		ON CONFLICT (user_id)
		DO UPDATE SET
			goals = CASE
				WHEN EXCLUDED.goals::text != '[]' THEN EXCLUDED.goals
				ELSE user_onboarding.goals
			END,
			time_available = COALESCE(EXCLUDED.time_available, user_onboarding.time_available),
			current_step = $4,
			completed_at = NOW(),
			updated_at = NOW()
		RETURNING id, project_status, time_available, goals, current_step, completed_at
	`

	var data OnboardingData
	var goalsJSON []byte
	err := h.db.QueryRow(r.Context(), query, userID, goalsJSONStr, productivityPeak, totalOnboardingSteps).Scan(
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
	fmt.Printf("✅ Onboarding completed for user %s\n", userID)

	// Also update user productivity_peak
	if req.ProductivityPeak != "" {
		h.db.Exec(r.Context(),
			`UPDATE public.users SET productivity_peak = $1 WHERE id = $2`,
			req.ProductivityPeak, userID)
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
		CurrentStep:   totalOnboardingSteps,
		TotalSteps:    totalOnboardingSteps,
		ProjectStatus: data.ProjectStatus,
		TimeAvailable: data.TimeAvailable,
		Goals:         data.Goals,
		CompletedAt:   data.CompletedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// Reset resets onboarding for a user (useful for testing)
// DELETE /onboarding/reset
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
