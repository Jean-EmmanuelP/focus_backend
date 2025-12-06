package stats

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"firelevel-backend/internal/auth"
	"github.com/jackc/pgx/v5/pgxpool"
)

// --- Structures ---

type DashboardStats struct {
	FocusedToday int `json:"focused_today"` // Minutes
	StreakDays   int `json:"streak_days"`
}

type DashboardData struct {
	User            interface{}    `json:"user"`
	Areas           interface{}    `json:"areas"`
	TodaysRoutines  interface{}    `json:"todays_routines"`
	TodayIntentions interface{}    `json:"today_intentions"`
	Stats           DashboardStats `json:"stats"`
	WeekSessions    interface{}    `json:"week_sessions"` // Focus sessions this week (Mon-Sun)
}

type FireModeData struct {
	MinutesToday   int `json:"minutes_today"`
	SessionsToday  int `json:"sessions_today"`
	SessionsWeek   int `json:"sessions_week"`
	MinutesLast7   int `json:"minutes_last_7"`
	SessionsLast7  int `json:"sessions_last_7"`
	ActiveQuests   interface{} `json:"active_quests"`
}

type QuestsTabData struct {
	Areas    interface{} `json:"areas"`
	Quests   interface{} `json:"quests"`
	Routines interface{} `json:"routines"`
}

// Existing structures kept for compatibility if needed by charts
type FocusStats struct {
	TotalMinutes   int          `json:"total_minutes"`
	TotalSessions  int          `json:"total_sessions"`
	DailyBreakdown []interface{} `json:"daily_breakdown"`
}

type RoutineStats struct {
	CompletionRate int           `json:"completion_rate"`
	DailyCounts    []interface{} `json:"daily_counts"`
}

type Handler struct {
	db *pgxpool.Pool
}

func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
}

// --- Handlers ---

// GetDashboard - GET /dashboard?date=YYYY-MM-DD
// All daily routines, streak (focus sessions), sessions this week
func (h *Handler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	ctx := r.Context()

	// Get user's local date from query param (required for timezone accuracy)
	userDateStr := r.URL.Query().Get("date")
	var userDate time.Time
	var err error
	if userDateStr != "" {
		userDate, err = time.Parse("2006-01-02", userDateStr)
		if err != nil {
			http.Error(w, "Invalid date format. Use YYYY-MM-DD", http.StatusBadRequest)
			return
		}
	} else {
		userDate = time.Now() // Fallback to server time
	}

	var wg sync.WaitGroup
	dashboard := DashboardData{}
	errChan := make(chan error, 6)

	// 1. User Profile
	wg.Add(1)
	go func() {
		defer wg.Done()
		var u struct { ID string `json:"id"`; FullName *string `json:"full_name"` }
		err := h.db.QueryRow(ctx, "SELECT id, full_name FROM public.users WHERE id = $1", userID).Scan(&u.ID, &u.FullName)
		if err != nil { errChan <- err; return }
		dashboard.User = u
	}()

	// 2. Areas (Simplified)
	wg.Add(1)
	go func() {
		defer wg.Done()
		rows, err := h.db.Query(ctx, "SELECT id, name, slug, icon FROM public.areas WHERE user_id = $1 ORDER BY created_at DESC", userID)
		if err != nil { errChan <- err; return }
		defer rows.Close()
		
		areas := []map[string]interface{}{}
		for rows.Next() {
			var id, name string
			var sSlug, sIcon *string
			rows.Scan(&id, &name, &sSlug, &sIcon)
			area := map[string]interface{}{"id": id, "name": name}
			if sSlug != nil { area["slug"] = *sSlug }
			if sIcon != nil { area["icon"] = *sIcon }
			areas = append(areas, area)
		}
		dashboard.Areas = areas
	}()

	// 3. Stats: Focused Today & Streak
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Focused Today
		var minutes *int
		err := h.db.QueryRow(ctx, "SELECT SUM(duration_minutes) FROM public.focus_sessions WHERE user_id = $1 AND status = 'completed' AND DATE(started_at) = $2", userID, userDate).Scan(&minutes)
		if err == nil && minutes != nil {
			dashboard.Stats.FocusedToday = *minutes
		}

		// Streak Calculation (Consecutive days with at least one completed focus session)
		// Use a recursive CTE to count backwards from user's current date
		streakQuery := `
			WITH daily_activity AS (
				SELECT DISTINCT date(started_at) as activity_date
				FROM public.focus_sessions
				WHERE user_id = $1 AND status = 'completed'
			),
			streak AS (
				SELECT activity_date, 1 as streak_val
				FROM daily_activity
				WHERE activity_date = $2::date OR activity_date = $2::date - 1
				UNION ALL
				SELECT da.activity_date, s.streak_val + 1
				FROM daily_activity da
				JOIN streak s ON da.activity_date = s.activity_date - 1
			)
			SELECT MAX(streak_val) FROM streak
		`
		var streak *int
		err = h.db.QueryRow(ctx, streakQuery, userID, userDate).Scan(&streak)
		if err == nil && streak != nil {
			dashboard.Stats.StreakDays = *streak
		}
	}()

	// 5. Today's Routines (ALL routines with completion status)
	wg.Add(1)
	go func() {
		defer wg.Done()
		query := `
			SELECT
				r.id, r.title, r.icon, r.frequency,
				EXISTS (
					SELECT 1 FROM public.routine_completions rc
					WHERE rc.routine_id = r.id
					AND rc.user_id = r.user_id
					AND DATE(rc.completed_at) = $2
				) as completed
			FROM public.routines r
			WHERE r.user_id = $1
			ORDER BY r.created_at
		`
		rows, err := h.db.Query(ctx, query, userID, userDate)
		if err != nil { errChan <- err; return }
		defer rows.Close()

		routines := []map[string]interface{}{}
		for rows.Next() {
			var id, title, frequency string
			var icon *string
			var completed bool
			rows.Scan(&id, &title, &icon, &frequency, &completed)
			rt := map[string]interface{}{
				"id": id, "title": title, "frequency": frequency, "completed": completed,
			}
			if icon != nil { rt["icon"] = *icon }
			routines = append(routines, rt)
		}
		dashboard.TodaysRoutines = routines
	}()

	// 6. Focus Sessions This Week (Monday to Sunday based on user's date)
	wg.Add(1)
	go func() {
		defer wg.Done()
		query := `
			SELECT
				to_char(started_at, 'YYYY-MM-DD') as date,
				SUM(duration_minutes) as minutes,
				COUNT(id) as sessions
			FROM public.focus_sessions
			WHERE user_id = $1
			  AND status = 'completed'
			  AND started_at >= date_trunc('week', $2::date)
			GROUP BY date
			ORDER BY date ASC
		`
		rows, err := h.db.Query(ctx, query, userID, userDate)
		if err != nil { errChan <- err; return }
		defer rows.Close()

		sessions := []map[string]interface{}{}
		totalMinutes := 0
		totalSessions := 0
		for rows.Next() {
			var date string
			var mins, sess int
			rows.Scan(&date, &mins, &sess)
			sessions = append(sessions, map[string]interface{}{"date": date, "minutes": mins, "sessions": sess})
			totalMinutes += mins
			totalSessions += sess
		}
		dashboard.WeekSessions = map[string]interface{}{
			"total_minutes":  totalMinutes,
			"total_sessions": totalSessions,
			"days":           sessions,
		}
	}()

	// 7. Today's Intentions (based on user's date)
	wg.Add(1)
	go func() {
		defer wg.Done()

		// Get daily intention for user's current date
		query := `
			SELECT id, date, mood_rating, mood_emoji, sleep_rating, sleep_emoji
			FROM public.daily_intentions
			WHERE user_id = $1 AND date = $2
		`

		var id, moodEmoji, sleepEmoji string
		var date interface{}
		var moodRating, sleepRating int

		err := h.db.QueryRow(ctx, query, userID, userDate).Scan(&id, &date, &moodRating, &moodEmoji, &sleepRating, &sleepEmoji)
		if err != nil {
			// No intention for today - that's OK
			dashboard.TodayIntentions = nil
			return
		}

		// Get intention items
		itemsQuery := `
			SELECT id, area_id, content, position
			FROM public.intention_items
			WHERE daily_intention_id = $1
			ORDER BY position ASC
		`

		itemRows, err := h.db.Query(ctx, itemsQuery, id)
		if err != nil {
			dashboard.TodayIntentions = nil
			return
		}
		defer itemRows.Close()

		intentions := []map[string]interface{}{}
		for itemRows.Next() {
			var itemID, content string
			var areaID *string
			var position int
			itemRows.Scan(&itemID, &areaID, &content, &position)
			item := map[string]interface{}{
				"id": itemID, "content": content, "position": position,
			}
			if areaID != nil {
				item["area_id"] = *areaID
			}
			intentions = append(intentions, item)
		}

		dashboard.TodayIntentions = map[string]interface{}{
			"id":            id,
			"mood_rating":   moodRating,
			"mood_emoji":    moodEmoji,
			"sleep_rating":  sleepRating,
			"sleep_emoji":   sleepEmoji,
			"intentions":    intentions,
		}
	}()

	wg.Wait()

	// Check for errors (optional: log them instead of failing entire req)
	select {
	case <-errChan:
		http.Error(w, "Failed to load dashboard", http.StatusInternalServerError)
		return
	default:
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dashboard)
}

// GetFireMode - GET /firemode
func (h *Handler) GetFireMode(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	ctx := r.Context()

	var wg sync.WaitGroup
	data := FireModeData{}
	errChan := make(chan error, 4)

	// 1. Today's Stats (Sessions & Minutes)
	wg.Add(1)
	go func() {
		defer wg.Done()
		var mins, sess *int
		err := h.db.QueryRow(ctx, `
			SELECT SUM(duration_minutes), COUNT(id) 
			FROM public.focus_sessions 
			WHERE user_id = $1 AND status = 'completed' AND started_at >= CURRENT_DATE
		`, userID).Scan(&mins, &sess)
		if err != nil { errChan <- err; return }
		if mins != nil { data.MinutesToday = *mins }
		if sess != nil { data.SessionsToday = *sess }
	}()

	// 2. This Week's Sessions (Since Monday)
	wg.Add(1)
	go func() {
		defer wg.Done()
		var sess *int
		// date_trunc('week', now()) in Postgres usually starts Monday
		err := h.db.QueryRow(ctx, `
			SELECT COUNT(id) 
			FROM public.focus_sessions 
			WHERE user_id = $1 AND status = 'completed' AND started_at >= date_trunc('week', CURRENT_DATE)
		`, userID).Scan(&sess)
		if err != nil { errChan <- err; return }
		if sess != nil { data.SessionsWeek = *sess }
	}()

	// 3. Last 7 Days Stats
	wg.Add(1)
	go func() {
		defer wg.Done()
		var mins, sess *int
		err := h.db.QueryRow(ctx, `
			SELECT SUM(duration_minutes), COUNT(id) 
			FROM public.focus_sessions 
			WHERE user_id = $1 AND status = 'completed' AND started_at >= CURRENT_DATE - INTERVAL '7 days'
		`, userID).Scan(&mins, &sess)
		if err != nil { errChan <- err; return }
		if mins != nil { data.MinutesLast7 = *mins }
		if sess != nil { data.SessionsLast7 = *sess }
	}()

	// 4. All Quests
	wg.Add(1)
	go func() {
		defer wg.Done()
		rows, err := h.db.Query(ctx, "SELECT id, title, current_value, target_value FROM public.quests WHERE user_id = $1", userID)
		if err != nil { errChan <- err; return }
		defer rows.Close()

		quests := []map[string]interface{}{}
		for rows.Next() {
			var id, title string
			var cur, tgt int
			rows.Scan(&id, &title, &cur, &tgt)
			quests = append(quests, map[string]interface{}{"id": id, "title": title, "current_value": cur, "target_value": tgt})
		}
		data.ActiveQuests = quests
	}()

	wg.Wait()
	
	select {
	case <-errChan:
		http.Error(w, "Failed to load firemode", http.StatusInternalServerError)
		return
	default:
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// GetQuestsTab - GET /quests-tab
func (h *Handler) GetQuestsTab(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	ctx := r.Context()

	var wg sync.WaitGroup
	data := QuestsTabData{}
	errChan := make(chan error, 3)

	// 1. All Areas
	wg.Add(1)
	go func() {
		defer wg.Done()
		rows, err := h.db.Query(ctx, "SELECT id, name, slug, icon FROM public.areas WHERE user_id = $1", userID)
		if err != nil { errChan <- err; return }
		defer rows.Close()

		areas := []map[string]interface{}{}
		for rows.Next() {
			var id, name string
			var sSlug, sIcon *string
			rows.Scan(&id, &name, &sSlug, &sIcon)
			area := map[string]interface{}{"id": id, "name": name}
			if sSlug != nil { area["slug"] = *sSlug }
			if sIcon != nil { area["icon"] = *sIcon }
			areas = append(areas, area)
		}
		data.Areas = areas
	}()

	// 2. All Quests
	wg.Add(1)
	go func() {
		defer wg.Done()
		rows, err := h.db.Query(ctx, "SELECT id, area_id, title, status, current_value, target_value FROM public.quests WHERE user_id = $1", userID)
		if err != nil { errChan <- err; return }
		defer rows.Close()

		quests := []map[string]interface{}{}
		for rows.Next() {
			var id, areaID, title, status string
			var cur, tgt int
			rows.Scan(&id, &areaID, &title, &status, &cur, &tgt)
			quests = append(quests, map[string]interface{}{
				"id": id, "area_id": areaID, "title": title, "status": status, "current_value": cur, "target_value": tgt,
			})
		}
		data.Quests = quests
	}()

	// 3. All Routines
	wg.Add(1)
	go func() {
		defer wg.Done()
		rows, err := h.db.Query(ctx, "SELECT id, area_id, title, frequency, icon FROM public.routines WHERE user_id = $1", userID)
		if err != nil { errChan <- err; return }
		defer rows.Close()

		routines := []map[string]interface{}{}
		for rows.Next() {
			var id, areaID, title, freq string
			var icon *string
			rows.Scan(&id, &areaID, &title, &freq, &icon)
			rt := map[string]interface{}{
				"id": id, "area_id": areaID, "title": title, "frequency": freq,
			}
			if icon != nil { rt["icon"] = *icon }
			routines = append(routines, rt)
		}
		data.Routines = routines
	}()

	wg.Wait()

	select {
	case <-errChan:
		http.Error(w, "Failed to load quests tab", http.StatusInternalServerError)
		return
	default:
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// Keep older handlers for backwards compatibility if needed, or stub them
func (h *Handler) GetFocusStats(w http.ResponseWriter, r *http.Request) {
	// Stub or keep implementation if you still need individual charts
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("{}"))
}

func (h *Handler) GetRoutineStats(w http.ResponseWriter, r *http.Request) {
	// Stub
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("{}"))
}