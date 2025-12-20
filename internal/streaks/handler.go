package streaks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"firelevel-backend/internal/auth"
	"github.com/jackc/pgx/v5/pgxpool"
)

// FlameLevel represents a flame tier that users can unlock
type FlameLevel struct {
	Level       int    `json:"level"`
	Name        string `json:"name"`
	Icon        string `json:"icon"`
	DaysRequired int   `json:"days_required"`
	IsUnlocked  bool   `json:"is_unlocked"`
	IsCurrent   bool   `json:"is_current"`
}

// StreakInfo contains streak data for a user
type StreakInfo struct {
	CurrentStreak   int          `json:"current_streak"`
	LongestStreak   int          `json:"longest_streak"`
	LastValidDate   *string      `json:"last_valid_date,omitempty"`
	StreakStart     *string      `json:"streak_start,omitempty"`
	TodayValidation *DayValidation `json:"today_validation,omitempty"`
	FlameLevels     []FlameLevel `json:"flame_levels"`
	CurrentFlameLevel int        `json:"current_flame_level"`
}

// DayValidation contains the validation status for a specific day
type DayValidation struct {
	Date              string `json:"date"`
	HasIntention      bool   `json:"has_intention"`
	TotalRoutines     int    `json:"total_routines"`
	CompletedRoutines int    `json:"completed_routines"`
	RoutineRate       int    `json:"routine_rate"` // percentage
	TotalTasks        int    `json:"total_tasks"`
	CompletedTasks    int    `json:"completed_tasks"`
	TaskRate          int    `json:"task_rate"` // percentage
	TotalItems        int    `json:"total_items"` // tasks + routines
	CompletedItems    int    `json:"completed_items"`
	OverallRate       int    `json:"overall_rate"` // combined percentage
	IsValid           bool   `json:"is_valid"`
	// Validation requirements (simplified: 60% completion + at least 1 task)
	RequiredCompletionRate int  `json:"required_completion_rate"` // 60%
	RequiredMinTasks       int  `json:"required_min_tasks"`       // 1
	MeetsCompletionRate    bool `json:"meets_completion_rate"`
	MeetsMinTasks          bool `json:"meets_min_tasks"`
}

type Handler struct {
	db *pgxpool.Pool
}

func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
}

// GetFlameLevels returns the flame level definitions
func GetFlameLevels() []FlameLevel {
	return []FlameLevel{
		{Level: 1, Name: "Spark", Icon: "🔥", DaysRequired: 0},
		{Level: 2, Name: "Ember", Icon: "🔥🔥", DaysRequired: 3},
		{Level: 3, Name: "Blaze", Icon: "🔥🔥🔥", DaysRequired: 7},
		{Level: 4, Name: "Inferno", Icon: "🌟🔥", DaysRequired: 14},
		{Level: 5, Name: "Phoenix", Icon: "🌟🔥🔥", DaysRequired: 30},
		{Level: 6, Name: "Supernova", Icon: "🌟🔥🔥🔥", DaysRequired: 60},
		{Level: 7, Name: "Legend", Icon: "👑🔥", DaysRequired: 100},
	}
}

// GetStreak - GET /streak
// Returns the user's current streak information
func (h *Handler) GetStreak(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	ctx := r.Context()

	// Get user's local date from query param
	userDate := r.URL.Query().Get("date")
	if userDate == "" {
		userDate = time.Now().Format("2006-01-02")
	}

	streak := h.calculateStreak(ctx, userID, userDate)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(streak)
}

// GetDayValidation - GET /streak/day?date=YYYY-MM-DD
// Returns validation status for a specific day
func (h *Handler) GetDayValidation(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	ctx := r.Context()

	date := r.URL.Query().Get("date")
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	validation := h.validateDay(ctx, userID, date)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(validation)
}

// validateDay checks if a specific day counts towards the streak
// Rules (simplified):
// 1. At least 60% of daily items (tasks + routines) completed
// 2. At least 1 task in the calendar for the day
func (h *Handler) validateDay(ctx context.Context, userID string, date string) DayValidation {
	validation := DayValidation{
		Date:                   date,
		IsValid:                false,
		RequiredCompletionRate: 60,
		RequiredMinTasks:       1,
	}

	// Parse date for day of week calculation
	parsedDate, err := time.Parse("2006-01-02", date)
	if err != nil {
		fmt.Printf("❌ Streak: Error parsing date: %v\n", err)
		return validation
	}
	dayOfWeek := int(parsedDate.Weekday()) // 0 = Sunday, 1 = Monday, etc.

	// 1. Check if daily intention exists for this date
	var intentionExists bool
	err = h.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM public.daily_intentions WHERE user_id = $1 AND date = $2::date)`,
		userID, date,
	).Scan(&intentionExists)
	if err != nil {
		fmt.Printf("❌ Streak: Error checking intention: %v\n", err)
	}
	validation.HasIntention = intentionExists

	// 2. Count routines and completed routines for the day
	rows, err := h.db.Query(ctx, `
		SELECT r.id, r.frequency,
			EXISTS (
				SELECT 1 FROM public.routine_completions rc
				WHERE rc.routine_id = r.id
				AND rc.user_id = r.user_id
				AND DATE(rc.completed_at) = $2::date
			) as completed
		FROM public.routines r
		WHERE r.user_id = $1
	`, userID, date)
	if err != nil {
		fmt.Printf("❌ Streak: Error querying routines: %v\n", err)
		return validation
	}
	defer rows.Close()

	totalRoutines := 0
	completedRoutines := 0

	for rows.Next() {
		var routineID, frequency string
		var completed bool
		if err := rows.Scan(&routineID, &frequency, &completed); err != nil {
			continue
		}

		// Check if this routine applies to this day based on frequency
		appliesForDay := false
		switch frequency {
		case "daily":
			appliesForDay = true
		case "weekdays":
			appliesForDay = dayOfWeek >= 1 && dayOfWeek <= 5
		case "weekends":
			appliesForDay = dayOfWeek == 0 || dayOfWeek == 6
		case "monday":
			appliesForDay = dayOfWeek == 1
		case "tuesday":
			appliesForDay = dayOfWeek == 2
		case "wednesday":
			appliesForDay = dayOfWeek == 3
		case "thursday":
			appliesForDay = dayOfWeek == 4
		case "friday":
			appliesForDay = dayOfWeek == 5
		case "saturday":
			appliesForDay = dayOfWeek == 6
		case "sunday":
			appliesForDay = dayOfWeek == 0
		default:
			appliesForDay = true
		}

		if appliesForDay {
			totalRoutines++
			if completed {
				completedRoutines++
			}
		}
	}

	validation.TotalRoutines = totalRoutines
	validation.CompletedRoutines = completedRoutines

	// Calculate routine completion rate
	if totalRoutines > 0 {
		validation.RoutineRate = (completedRoutines * 100) / totalRoutines
	} else {
		validation.RoutineRate = 100
	}

	// 3. Count tasks for the day
	var totalTasks, completedTasks int
	err = h.db.QueryRow(ctx, `
		SELECT
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE status = 'completed') as completed
		FROM public.tasks
		WHERE user_id = $1 AND date = $2::date
	`, userID, date).Scan(&totalTasks, &completedTasks)
	if err != nil {
		fmt.Printf("❌ Streak: Error counting tasks: %v\n", err)
	}

	validation.TotalTasks = totalTasks
	validation.CompletedTasks = completedTasks

	// Calculate task completion rate
	if totalTasks > 0 {
		validation.TaskRate = (completedTasks * 100) / totalTasks
	} else {
		validation.TaskRate = 100
	}

	// Calculate overall completion rate (tasks + routines)
	validation.TotalItems = totalTasks + totalRoutines
	validation.CompletedItems = completedTasks + completedRoutines

	if validation.TotalItems > 0 {
		validation.OverallRate = (validation.CompletedItems * 100) / validation.TotalItems
	} else {
		// If no items, can't validate the day
		validation.OverallRate = 0
	}

	// Check validation requirements (simplified)
	validation.MeetsCompletionRate = validation.OverallRate >= validation.RequiredCompletionRate
	validation.MeetsMinTasks = validation.TotalItems >= validation.RequiredMinTasks

	// Day is valid if:
	// 1. At least 60% of items completed
	// 2. At least 1 task/routine in calendar
	validation.IsValid = validation.MeetsCompletionRate && validation.MeetsMinTasks

	return validation
}

// calculateStreak calculates the current streak by checking consecutive valid days
func (h *Handler) calculateStreak(ctx context.Context, userID string, currentDate string) StreakInfo {
	streak := StreakInfo{
		CurrentStreak: 0,
		LongestStreak: 0,
		FlameLevels:   GetFlameLevels(),
	}

	// Parse current date
	today, err := time.Parse("2006-01-02", currentDate)
	if err != nil {
		fmt.Printf("❌ Streak: Error parsing current date: %v\n", err)
		return streak
	}

	// Check today first
	todayValidation := h.validateDay(ctx, userID, currentDate)
	streak.TodayValidation = &todayValidation

	// Start checking from yesterday and go backwards
	consecutiveDays := 0
	var streakStartDate string
	var lastValidDate string

	// If today is valid, count it and continue from yesterday
	if todayValidation.IsValid {
		consecutiveDays = 1
		streakStartDate = currentDate
		lastValidDate = currentDate
	}

	// Check up to 365 days back (or until we find an invalid day)
	for i := 1; i <= 365; i++ {
		checkDate := today.AddDate(0, 0, -i)
		dateStr := checkDate.Format("2006-01-02")

		validation := h.validateDay(ctx, userID, dateStr)

		if validation.IsValid {
			consecutiveDays++
			streakStartDate = dateStr
			if lastValidDate == "" {
				lastValidDate = dateStr
			}
		} else {
			break
		}
	}

	streak.CurrentStreak = consecutiveDays
	if streakStartDate != "" {
		streak.StreakStart = &streakStartDate
	}
	if lastValidDate != "" {
		streak.LastValidDate = &lastValidDate
	}

	// Get longest streak from database
	var longestStreak *int
	err = h.db.QueryRow(ctx,
		`SELECT longest_streak FROM public.user_streaks WHERE user_id = $1`,
		userID,
	).Scan(&longestStreak)
	if err == nil && longestStreak != nil {
		streak.LongestStreak = *longestStreak
	}

	// Update longest streak if current is higher
	if streak.CurrentStreak > streak.LongestStreak {
		streak.LongestStreak = streak.CurrentStreak
	}

	// Calculate flame levels based on current streak
	currentFlameLevel := 1
	for i := range streak.FlameLevels {
		if streak.CurrentStreak >= streak.FlameLevels[i].DaysRequired {
			streak.FlameLevels[i].IsUnlocked = true
			currentFlameLevel = streak.FlameLevels[i].Level
		}
	}
	streak.CurrentFlameLevel = currentFlameLevel

	// Mark current flame level
	for i := range streak.FlameLevels {
		if streak.FlameLevels[i].Level == currentFlameLevel {
			streak.FlameLevels[i].IsCurrent = true
		}
	}

	// Update streaks in DB
	h.updateStreaks(ctx, userID, streak.CurrentStreak, streak.LongestStreak)

	fmt.Printf("✅ Streak calculated for user %s: current=%d, longest=%d, flameLevel=%d, todayValid=%v\n",
		userID, streak.CurrentStreak, streak.LongestStreak, currentFlameLevel, todayValidation.IsValid)

	return streak
}

// updateStreaks updates the user's current and longest streak in the database
func (h *Handler) updateStreaks(ctx context.Context, userID string, currentStreak int, longestStreak int) {
	_, err := h.db.Exec(ctx, `
		INSERT INTO public.user_streaks (user_id, current_streak, longest_streak, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (user_id)
		DO UPDATE SET current_streak = $2, longest_streak = $3, updated_at = NOW()
	`, userID, currentStreak, longestStreak)
	if err != nil {
		fmt.Printf("❌ Streak: Error updating streaks: %v\n", err)
	}
}

// RecalculateStreak - POST /streak/recalculate
// Forces a recalculation of the streak
func (h *Handler) RecalculateStreak(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	ctx := r.Context()

	userDate := r.URL.Query().Get("date")
	if userDate == "" {
		userDate = time.Now().Format("2006-01-02")
	}

	streak := h.calculateStreak(ctx, userID, userDate)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(streak)
}
