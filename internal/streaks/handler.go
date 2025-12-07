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

// StreakInfo contains streak data for a user
type StreakInfo struct {
	CurrentStreak int     `json:"current_streak"`
	LongestStreak int     `json:"longest_streak"`
	LastValidDate *string `json:"last_valid_date,omitempty"`
	StreakStart   *string `json:"streak_start,omitempty"`
}

// DayValidation contains the validation status for a specific day
type DayValidation struct {
	Date              string `json:"date"`
	HasIntention      bool   `json:"has_intention"`
	TotalRoutines     int    `json:"total_routines"`
	CompletedRoutines int    `json:"completed_routines"`
	RoutineRate       int    `json:"routine_rate"` // percentage
	IsValid           bool   `json:"is_valid"`
}

type Handler struct {
	db *pgxpool.Pool
}

func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
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
// Rules: 40% of daily routines completed + Start of day (intention) done
func (h *Handler) validateDay(ctx context.Context, userID string, date string) DayValidation {
	validation := DayValidation{
		Date:    date,
		IsValid: false,
	}

	// 1. Check if daily intention exists for this date
	var intentionExists bool
	err := h.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM public.daily_intentions WHERE user_id = $1 AND date = $2::date)`,
		userID, date,
	).Scan(&intentionExists)
	if err != nil {
		fmt.Printf("❌ Streak: Error checking intention: %v\n", err)
	}
	validation.HasIntention = intentionExists

	// 2. Count total routines and completed routines for the day
	// We need to count routines that apply to this day based on frequency
	parsedDate, err := time.Parse("2006-01-02", date)
	if err != nil {
		fmt.Printf("❌ Streak: Error parsing date: %v\n", err)
		return validation
	}
	dayOfWeek := int(parsedDate.Weekday()) // 0 = Sunday, 1 = Monday, etc.

	// Get all user's routines and filter by frequency
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
			// If frequency is not recognized, assume daily
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
		// If no routines exist, consider it as 100% (not blocking the streak)
		validation.RoutineRate = 100
	}

	// Day is valid if:
	// 1. Has intention (Start of Day done)
	// 2. At least 40% of routines completed (or no routines exist)
	validation.IsValid = validation.HasIntention && validation.RoutineRate >= 40

	return validation
}

// calculateStreak calculates the current streak by checking consecutive valid days
func (h *Handler) calculateStreak(ctx context.Context, userID string, currentDate string) StreakInfo {
	streak := StreakInfo{
		CurrentStreak: 0,
		LongestStreak: 0,
	}

	// Parse current date
	today, err := time.Parse("2006-01-02", currentDate)
	if err != nil {
		fmt.Printf("❌ Streak: Error parsing current date: %v\n", err)
		return streak
	}

	// Check today first
	todayValidation := h.validateDay(ctx, userID, currentDate)

	// Start checking from yesterday and go backwards
	consecutiveDays := 0
	var streakStartDate string
	var lastValidDate string

	// If today is valid, count it
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
			// Streak broken, stop counting
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

	// Get longest streak from database (if we're tracking it)
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
		h.updateLongestStreak(ctx, userID, streak.CurrentStreak)
	}

	fmt.Printf("✅ Streak calculated for user %s: current=%d, longest=%d, start=%v\n",
		userID, streak.CurrentStreak, streak.LongestStreak, streak.StreakStart)

	return streak
}

// updateLongestStreak updates the user's longest streak in the database
func (h *Handler) updateLongestStreak(ctx context.Context, userID string, longestStreak int) {
	_, err := h.db.Exec(ctx, `
		INSERT INTO public.user_streaks (user_id, longest_streak, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (user_id)
		DO UPDATE SET longest_streak = $2, updated_at = NOW()
	`, userID, longestStreak)
	if err != nil {
		fmt.Printf("❌ Streak: Error updating longest streak: %v\n", err)
	}
}

// RecalculateStreak - POST /streak/recalculate
// Forces a recalculation of the streak (useful after data changes)
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
