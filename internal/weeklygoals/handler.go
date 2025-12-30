package weeklygoals

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"firelevel-backend/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// WeeklyGoal represents a user's weekly goals container (like daily_intentions)
type WeeklyGoal struct {
	ID            string           `json:"id"`
	WeekStartDate time.Time        `json:"-"`
	WeekStartStr  string           `json:"week_start_date"`
	Items         []WeeklyGoalItem `json:"items"`
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
}

// WeeklyGoalItem represents a single goal within a week (like intention_items)
type WeeklyGoalItem struct {
	ID          string     `json:"id"`
	AreaID      *string    `json:"area_id"`
	Content     string     `json:"content"`
	Position    int        `json:"position"`
	IsCompleted bool       `json:"is_completed"`
	CompletedAt *time.Time `json:"completed_at"`
}

// WeeklyGoalItemInput for creating/updating items
type WeeklyGoalItemInput struct {
	AreaID  *string `json:"area_id"`
	Content string  `json:"content"`
}

// UpsertWeeklyGoalRequest for creating/updating weekly goals
type UpsertWeeklyGoalRequest struct {
	Items []WeeklyGoalItemInput `json:"items"`
}

// ToggleItemRequest for toggling completion
type ToggleItemRequest struct {
	IsCompleted bool `json:"is_completed"`
}

// Handler handles weekly goals HTTP requests
type Handler struct {
	db *pgxpool.Pool
}

// NewHandler creates a new weekly goals handler
func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
}

// getWeekStart returns the Monday of the week containing the given date
func getWeekStart(dateStr string) (time.Time, error) {
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return time.Time{}, err
	}

	// Find Monday of this week
	weekday := date.Weekday()
	if weekday == time.Sunday {
		weekday = 7
	}
	daysToMonday := int(weekday) - 1
	monday := date.AddDate(0, 0, -daysToMonday)

	return monday, nil
}

// GetCurrent - GET /weekly-goals/current
func (h *Handler) GetCurrent(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	// Use today's date to find current week
	today := time.Now().Format("2006-01-02")
	monday, err := getWeekStart(today)
	if err != nil {
		http.Error(w, "Invalid date", http.StatusBadRequest)
		return
	}

	weeklyGoal, err := h.getWeeklyGoalByDate(r.Context(), userID, monday)
	if err != nil {
		// Return empty response instead of 404 - no goals set yet is a valid state
		emptyGoal := WeeklyGoal{
			ID:            "",
			WeekStartDate: monday,
			WeekStartStr:  monday.Format("2006-01-02"),
			Items:         []WeeklyGoalItem{},
			CreatedAt:     time.Time{},
			UpdatedAt:     time.Time{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(emptyGoal)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(weeklyGoal)
}

// GetByWeekStart - GET /weekly-goals/{weekStartDate}
func (h *Handler) GetByWeekStart(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	weekStartDate := chi.URLParam(r, "weekStartDate")

	monday, err := getWeekStart(weekStartDate)
	if err != nil {
		http.Error(w, "Invalid date format. Use YYYY-MM-DD", http.StatusBadRequest)
		return
	}

	weeklyGoal, err := h.getWeeklyGoalByDate(r.Context(), userID, monday)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(weeklyGoal)
}

// helper to get weekly goal
func (h *Handler) getWeeklyGoalByDate(ctx interface {
	Value(any) any
}, userID string, monday time.Time) (*WeeklyGoal, error) {
	httpCtx := ctx.(interface {
		Value(any) any
		Done() <-chan struct{}
		Err() error
		Deadline() (deadline time.Time, ok bool)
	})

	query := `
		SELECT id, week_start_date, created_at, updated_at
		FROM public.weekly_goals
		WHERE user_id = $1 AND week_start_date = $2
	`

	var wg WeeklyGoal
	err := h.db.QueryRow(httpCtx, query, userID, monday.Format("2006-01-02")).Scan(
		&wg.ID, &wg.WeekStartDate, &wg.CreatedAt, &wg.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	wg.WeekStartStr = wg.WeekStartDate.Format("2006-01-02")

	// Get items
	itemsQuery := `
		SELECT id, area_id, content, position, is_completed, completed_at
		FROM public.weekly_goal_items
		WHERE weekly_goal_id = $1
		ORDER BY position ASC
	`

	rows, err := h.db.Query(httpCtx, itemsQuery, wg.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	wg.Items = []WeeklyGoalItem{}
	for rows.Next() {
		var item WeeklyGoalItem
		if err := rows.Scan(&item.ID, &item.AreaID, &item.Content, &item.Position, &item.IsCompleted, &item.CompletedAt); err != nil {
			continue
		}
		wg.Items = append(wg.Items, item)
	}

	return &wg, nil
}

// Upsert - PUT /weekly-goals/{weekStartDate}
func (h *Handler) Upsert(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	weekStartDate := chi.URLParam(r, "weekStartDate")

	monday, err := getWeekStart(weekStartDate)
	if err != nil {
		http.Error(w, "Invalid date format. Use YYYY-MM-DD", http.StatusBadRequest)
		return
	}

	var req UpsertWeeklyGoalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	if len(req.Items) == 0 {
		http.Error(w, "At least one goal is required", http.StatusBadRequest)
		return
	}

	if len(req.Items) > 5 {
		http.Error(w, "Maximum 5 goals allowed", http.StatusBadRequest)
		return
	}

	// Start transaction
	tx, err := h.db.Begin(r.Context())
	if err != nil {
		http.Error(w, "Failed to start transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	// Upsert weekly_goals
	upsertQuery := `
		INSERT INTO public.weekly_goals (user_id, week_start_date)
		VALUES ($1, $2)
		ON CONFLICT (user_id, week_start_date) DO UPDATE SET
			updated_at = NOW()
		RETURNING id, week_start_date, created_at, updated_at
	`

	var wg WeeklyGoal
	err = tx.QueryRow(r.Context(), upsertQuery, userID, monday.Format("2006-01-02")).Scan(
		&wg.ID, &wg.WeekStartDate, &wg.CreatedAt, &wg.UpdatedAt,
	)
	if err != nil {
		fmt.Println("Upsert weekly_goals error:", err)
		http.Error(w, "Failed to save weekly goals", http.StatusInternalServerError)
		return
	}

	wg.WeekStartStr = wg.WeekStartDate.Format("2006-01-02")

	// Delete existing items (we'll recreate them)
	_, err = tx.Exec(r.Context(), "DELETE FROM public.weekly_goal_items WHERE weekly_goal_id = $1", wg.ID)
	if err != nil {
		http.Error(w, "Failed to update goals", http.StatusInternalServerError)
		return
	}

	// Insert new items
	wg.Items = []WeeklyGoalItem{}
	for pos, item := range req.Items {
		if item.Content == "" {
			continue
		}

		insertQuery := `
			INSERT INTO public.weekly_goal_items (weekly_goal_id, area_id, content, position)
			VALUES ($1, $2, $3, $4)
			RETURNING id, area_id, content, position, is_completed, completed_at
		`

		var i WeeklyGoalItem
		err = tx.QueryRow(r.Context(), insertQuery, wg.ID, item.AreaID, item.Content, pos+1).Scan(
			&i.ID, &i.AreaID, &i.Content, &i.Position, &i.IsCompleted, &i.CompletedAt,
		)
		if err != nil {
			fmt.Println("Insert weekly_goal_item error:", err)
			http.Error(w, "Failed to save goal item", http.StatusInternalServerError)
			return
		}

		wg.Items = append(wg.Items, i)
	}

	// Commit transaction
	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "Failed to commit transaction", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(wg)
}

// ToggleItem - POST /weekly-goals/items/{id}/toggle
func (h *Handler) ToggleItem(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	itemID := chi.URLParam(r, "id")

	var req ToggleItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	// Verify ownership and update
	var completedAt *time.Time
	if req.IsCompleted {
		now := time.Now()
		completedAt = &now
	}

	query := `
		UPDATE public.weekly_goal_items wgi
		SET is_completed = $1, completed_at = $2
		FROM public.weekly_goals wg
		WHERE wgi.id = $3
		  AND wgi.weekly_goal_id = wg.id
		  AND wg.user_id = $4
		RETURNING wgi.id, wgi.area_id, wgi.content, wgi.position, wgi.is_completed, wgi.completed_at
	`

	var item WeeklyGoalItem
	err := h.db.QueryRow(r.Context(), query, req.IsCompleted, completedAt, itemID, userID).Scan(
		&item.ID, &item.AreaID, &item.Content, &item.Position, &item.IsCompleted, &item.CompletedAt,
	)
	if err != nil {
		http.Error(w, "Goal not found or not owned by user", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(item)
}

// List - GET /weekly-goals
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	query := `
		SELECT id, week_start_date, created_at, updated_at
		FROM public.weekly_goals
		WHERE user_id = $1
		ORDER BY week_start_date DESC
		LIMIT 10
	`

	rows, err := h.db.Query(r.Context(), query, userID)
	if err != nil {
		http.Error(w, "Failed to list weekly goals", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	goals := []WeeklyGoal{}
	for rows.Next() {
		var wg WeeklyGoal
		if err := rows.Scan(&wg.ID, &wg.WeekStartDate, &wg.CreatedAt, &wg.UpdatedAt); err != nil {
			continue
		}
		wg.WeekStartStr = wg.WeekStartDate.Format("2006-01-02")

		// Get items for each goal
		itemsQuery := `
			SELECT id, area_id, content, position, is_completed, completed_at
			FROM public.weekly_goal_items
			WHERE weekly_goal_id = $1
			ORDER BY position ASC
		`
		itemRows, err := h.db.Query(r.Context(), itemsQuery, wg.ID)
		if err != nil {
			wg.Items = []WeeklyGoalItem{}
		} else {
			wg.Items = []WeeklyGoalItem{}
			for itemRows.Next() {
				var item WeeklyGoalItem
				if err := itemRows.Scan(&item.ID, &item.AreaID, &item.Content, &item.Position, &item.IsCompleted, &item.CompletedAt); err != nil {
					continue
				}
				wg.Items = append(wg.Items, item)
			}
			itemRows.Close()
		}

		goals = append(goals, wg)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(goals)
}

// Delete - DELETE /weekly-goals/{weekStartDate}
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	weekStartDate := chi.URLParam(r, "weekStartDate")

	monday, err := getWeekStart(weekStartDate)
	if err != nil {
		http.Error(w, "Invalid date format. Use YYYY-MM-DD", http.StatusBadRequest)
		return
	}

	query := `
		DELETE FROM public.weekly_goals
		WHERE user_id = $1 AND week_start_date = $2
	`

	result, err := h.db.Exec(r.Context(), query, userID, monday.Format("2006-01-02"))
	if err != nil {
		http.Error(w, "Failed to delete", http.StatusInternalServerError)
		return
	}

	if result.RowsAffected() == 0 {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// CheckNeedsSetup - GET /weekly-goals/needs-setup
// Returns true if user hasn't set up goals for current week
func (h *Handler) CheckNeedsSetup(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	today := time.Now()
	weekday := today.Weekday()

	// Only suggest setup on Sunday evening (after 18h) or Monday
	hour := today.Hour()
	shouldPrompt := (weekday == time.Sunday && hour >= 18) || weekday == time.Monday

	if !shouldPrompt {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"needs_setup": false,
			"reason":      "not_right_time",
		})
		return
	}

	// Check if goals exist for current week
	monday, _ := getWeekStart(today.Format("2006-01-02"))

	var count int
	err := h.db.QueryRow(r.Context(),
		"SELECT COUNT(*) FROM public.weekly_goals WHERE user_id = $1 AND week_start_date = $2",
		userID, monday.Format("2006-01-02"),
	).Scan(&count)

	if err != nil || count == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"needs_setup":     true,
			"week_start_date": monday.Format("2006-01-02"),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"needs_setup": false,
		"reason":      "already_set",
	})
}
