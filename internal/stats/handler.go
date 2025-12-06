package stats

import (
	"encoding/json"
	"net/http"
	"time"

	"firelevel-backend/internal/auth"
	"github.com/jackc/pgx/v5/pgxpool"
)

// --- Response Structures ---

type UserInfo struct {
	ID       string  `json:"id"`
	FullName *string `json:"full_name"`
}

type Area struct {
	ID   string  `json:"id"`
	Name string  `json:"name"`
	Slug *string `json:"slug,omitempty"`
	Icon *string `json:"icon,omitempty"`
}

type Routine struct {
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	Frequency string  `json:"frequency"`
	Icon      *string `json:"icon,omitempty"`
	Completed bool    `json:"completed"`
}

type IntentionItem struct {
	ID       string  `json:"id"`
	AreaID   *string `json:"area_id,omitempty"`
	Content  string  `json:"content"`
	Position int     `json:"position"`
}

type TodayIntention struct {
	ID          string          `json:"id"`
	MoodRating  int             `json:"mood_rating"`
	MoodEmoji   string          `json:"mood_emoji"`
	SleepRating int             `json:"sleep_rating"`
	SleepEmoji  string          `json:"sleep_emoji"`
	Intentions  []IntentionItem `json:"intentions"`
}

type DaySession struct {
	Date     string `json:"date"`
	Minutes  int    `json:"minutes"`
	Sessions int    `json:"sessions"`
}

type WeekSessions struct {
	TotalMinutes  int          `json:"total_minutes"`
	TotalSessions int          `json:"total_sessions"`
	Days          []DaySession `json:"days"`
}

type DashboardStats struct {
	FocusedToday int `json:"focused_today"`
	StreakDays   int `json:"streak_days"`
}

type DashboardResponse struct {
	User            *UserInfo       `json:"user"`
	Areas           []Area          `json:"areas"`
	TodaysRoutines  []Routine       `json:"todays_routines"`
	TodayIntentions *TodayIntention `json:"today_intentions"`
	Stats           DashboardStats  `json:"stats"`
	WeekSessions    WeekSessions    `json:"week_sessions"`
}

type Handler struct {
	db *pgxpool.Pool
}

func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
}

// GetDashboard - GET /dashboard?date=YYYY-MM-DD
// Returns all data needed for the main dashboard in a single call
func (h *Handler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	ctx := r.Context()

	// Get user's local date from query param
	userDate := r.URL.Query().Get("date")
	if userDate == "" {
		userDate = time.Now().Format("2006-01-02")
	}

	response := DashboardResponse{
		Areas:          []Area{},
		TodaysRoutines: []Routine{},
		WeekSessions: WeekSessions{
			Days: []DaySession{},
		},
	}

	// 1. User Profile
	var user UserInfo
	err := h.db.QueryRow(ctx,
		"SELECT id, full_name FROM public.users WHERE id = $1",
		userID,
	).Scan(&user.ID, &user.FullName)
	if err == nil {
		response.User = &user
	}

	// 2. Areas
	areaRows, err := h.db.Query(ctx,
		"SELECT id, name, slug, icon FROM public.areas WHERE user_id = $1 ORDER BY created_at DESC",
		userID,
	)
	if err == nil {
		defer areaRows.Close()
		for areaRows.Next() {
			var a Area
			if err := areaRows.Scan(&a.ID, &a.Name, &a.Slug, &a.Icon); err == nil {
				response.Areas = append(response.Areas, a)
			}
		}
	}

	// 3. Today's Routines with completion status
	routineRows, err := h.db.Query(ctx, `
		SELECT
			r.id, r.title, r.icon, r.frequency,
			EXISTS (
				SELECT 1 FROM public.routine_completions rc
				WHERE rc.routine_id = r.id
				AND rc.user_id = r.user_id
				AND DATE(rc.completed_at) = $2::date
			) as completed
		FROM public.routines r
		WHERE r.user_id = $1
		ORDER BY r.created_at
	`, userID, userDate)
	if err == nil {
		defer routineRows.Close()
		for routineRows.Next() {
			var rt Routine
			if err := routineRows.Scan(&rt.ID, &rt.Title, &rt.Icon, &rt.Frequency, &rt.Completed); err == nil {
				response.TodaysRoutines = append(response.TodaysRoutines, rt)
			}
		}
	}

	// 4. Stats: Focused Today
	var focusedToday *int
	h.db.QueryRow(ctx,
		"SELECT COALESCE(SUM(duration_minutes), 0) FROM public.focus_sessions WHERE user_id = $1 AND status = 'completed' AND DATE(started_at) = $2::date",
		userID, userDate,
	).Scan(&focusedToday)
	if focusedToday != nil {
		response.Stats.FocusedToday = *focusedToday
	}

	// 5. Stats: Streak (simplified - count consecutive days)
	var streak *int
	h.db.QueryRow(ctx, `
		WITH daily_activity AS (
			SELECT DISTINCT DATE(started_at) as activity_date
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
		SELECT COALESCE(MAX(streak_val), 0) FROM streak
	`, userID, userDate).Scan(&streak)
	if streak != nil {
		response.Stats.StreakDays = *streak
	}

	// 6. Week Sessions (Monday to Sunday)
	sessionRows, err := h.db.Query(ctx, `
		SELECT
			to_char(started_at, 'YYYY-MM-DD') as date,
			COALESCE(SUM(duration_minutes), 0) as minutes,
			COUNT(id) as sessions
		FROM public.focus_sessions
		WHERE user_id = $1
		  AND status = 'completed'
		  AND started_at >= date_trunc('week', $2::date)
		GROUP BY to_char(started_at, 'YYYY-MM-DD')
		ORDER BY date ASC
	`, userID, userDate)
	if err == nil {
		defer sessionRows.Close()
		for sessionRows.Next() {
			var ds DaySession
			if err := sessionRows.Scan(&ds.Date, &ds.Minutes, &ds.Sessions); err == nil {
				response.WeekSessions.Days = append(response.WeekSessions.Days, ds)
				response.WeekSessions.TotalMinutes += ds.Minutes
				response.WeekSessions.TotalSessions += ds.Sessions
			}
		}
	}

	// 7. Today's Intentions
	var intentionID string
	var intentionDate time.Time
	var moodRating, sleepRating int
	var moodEmoji, sleepEmoji string

	err = h.db.QueryRow(ctx,
		"SELECT id, date, mood_rating, mood_emoji, sleep_rating, sleep_emoji FROM public.daily_intentions WHERE user_id = $1 AND date = $2::date",
		userID, userDate,
	).Scan(&intentionID, &intentionDate, &moodRating, &moodEmoji, &sleepRating, &sleepEmoji)

	if err == nil {
		intention := TodayIntention{
			ID:          intentionID,
			MoodRating:  moodRating,
			MoodEmoji:   moodEmoji,
			SleepRating: sleepRating,
			SleepEmoji:  sleepEmoji,
			Intentions:  []IntentionItem{},
		}

		// Get intention items
		itemRows, err := h.db.Query(ctx,
			"SELECT id, area_id, content, position FROM public.intention_items WHERE daily_intention_id = $1 ORDER BY position ASC",
			intentionID,
		)
		if err == nil {
			defer itemRows.Close()
			for itemRows.Next() {
				var item IntentionItem
				if err := itemRows.Scan(&item.ID, &item.AreaID, &item.Content, &item.Position); err == nil {
					intention.Intentions = append(intention.Intentions, item)
				}
			}
		}

		response.TodayIntentions = &intention
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetFireMode - GET /firemode
func (h *Handler) GetFireMode(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	ctx := r.Context()

	type FireModeResponse struct {
		MinutesToday  int `json:"minutes_today"`
		SessionsToday int `json:"sessions_today"`
		SessionsWeek  int `json:"sessions_week"`
		MinutesLast7  int `json:"minutes_last_7"`
		SessionsLast7 int `json:"sessions_last_7"`
	}

	response := FireModeResponse{}

	// Today's stats
	h.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(duration_minutes), 0), COUNT(id)
		FROM public.focus_sessions
		WHERE user_id = $1 AND status = 'completed' AND DATE(started_at) = CURRENT_DATE
	`, userID).Scan(&response.MinutesToday, &response.SessionsToday)

	// This week's sessions
	h.db.QueryRow(ctx, `
		SELECT COUNT(id)
		FROM public.focus_sessions
		WHERE user_id = $1 AND status = 'completed' AND started_at >= date_trunc('week', CURRENT_DATE)
	`, userID).Scan(&response.SessionsWeek)

	// Last 7 days
	h.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(duration_minutes), 0), COUNT(id)
		FROM public.focus_sessions
		WHERE user_id = $1 AND status = 'completed' AND started_at >= CURRENT_DATE - INTERVAL '7 days'
	`, userID).Scan(&response.MinutesLast7, &response.SessionsLast7)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetQuestsTab - GET /quests-tab
func (h *Handler) GetQuestsTab(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	ctx := r.Context()

	type QuestsTabResponse struct {
		Areas    []map[string]interface{} `json:"areas"`
		Quests   []map[string]interface{} `json:"quests"`
		Routines []map[string]interface{} `json:"routines"`
	}

	response := QuestsTabResponse{
		Areas:    []map[string]interface{}{},
		Quests:   []map[string]interface{}{},
		Routines: []map[string]interface{}{},
	}

	// Areas
	areaRows, _ := h.db.Query(ctx, "SELECT id, name, slug, icon FROM public.areas WHERE user_id = $1", userID)
	if areaRows != nil {
		defer areaRows.Close()
		for areaRows.Next() {
			var id, name string
			var slug, icon *string
			areaRows.Scan(&id, &name, &slug, &icon)
			area := map[string]interface{}{"id": id, "name": name}
			if slug != nil {
				area["slug"] = *slug
			}
			if icon != nil {
				area["icon"] = *icon
			}
			response.Areas = append(response.Areas, area)
		}
	}

	// Quests
	questRows, _ := h.db.Query(ctx, "SELECT id, area_id, title, status, current_value, target_value FROM public.quests WHERE user_id = $1", userID)
	if questRows != nil {
		defer questRows.Close()
		for questRows.Next() {
			var id, title, status string
			var areaID *string
			var cur, tgt int
			questRows.Scan(&id, &areaID, &title, &status, &cur, &tgt)
			quest := map[string]interface{}{
				"id": id, "title": title, "status": status, "current_value": cur, "target_value": tgt,
			}
			if areaID != nil {
				quest["area_id"] = *areaID
			}
			response.Quests = append(response.Quests, quest)
		}
	}

	// Routines
	routineRows, _ := h.db.Query(ctx, "SELECT id, area_id, title, frequency, icon FROM public.routines WHERE user_id = $1", userID)
	if routineRows != nil {
		defer routineRows.Close()
		for routineRows.Next() {
			var id, title, freq string
			var areaID, icon *string
			routineRows.Scan(&id, &areaID, &title, &freq, &icon)
			rt := map[string]interface{}{
				"id": id, "title": title, "frequency": freq,
			}
			if areaID != nil {
				rt["area_id"] = *areaID
			}
			if icon != nil {
				rt["icon"] = *icon
			}
			response.Routines = append(response.Routines, rt)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Stub handlers for backwards compatibility
func (h *Handler) GetFocusStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte("{}"))
}

func (h *Handler) GetRoutineStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte("{}"))
}
