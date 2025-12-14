package routines

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"firelevel-backend/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Routine struct {
	ID            string  `json:"id"`
	AreaID        *string `json:"area_id,omitempty"`
	Title         string  `json:"title"`
	Frequency     string  `json:"frequency"`
	Icon          string  `json:"icon,omitempty"`
	ScheduledTime *string `json:"scheduled_time,omitempty"` // HH:mm format
}

type CreateRoutineRequest struct {
	AreaID        string  `json:"area_id"`
	Title         string  `json:"title"`
	Frequency     string  `json:"frequency"` // default 'daily'
	Icon          string  `json:"icon"`
	ScheduledTime *string `json:"scheduled_time"` // HH:mm format
}

type BatchCompleteRequest struct {
	RoutineIDs []string `json:"routine_ids"`
}

type UpdateRoutineRequest struct {
	Title         *string `json:"title"`
	Frequency     *string `json:"frequency"`
	Icon          *string `json:"icon"`
	ScheduledTime *string `json:"scheduled_time"`
}

type Handler struct {
	db *pgxpool.Pool
}

func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	areaID := r.URL.Query().Get("area_id")

	query := `SELECT id, area_id, title, frequency, icon, scheduled_time FROM public.routines WHERE user_id = $1`
	args := []interface{}{userID}

	if areaID != "" {
		query += " AND area_id = $2"
		args = append(args, areaID)
	}
	query += " ORDER BY created_at DESC"

	rows, err := h.db.Query(r.Context(), query, args...)
	if err != nil {
		http.Error(w, "Failed to list routines", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	routines := []Routine{}
	for rows.Next() {
		var rt Routine
		var icon *string
		if err := rows.Scan(&rt.ID, &rt.AreaID, &rt.Title, &rt.Frequency, &icon, &rt.ScheduledTime); err != nil {
			fmt.Println("Scan error:", err)
			continue
		}
		if icon != nil {
			rt.Icon = *icon
		}
		routines = append(routines, rt)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(routines)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	var req CreateRoutineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	if req.Frequency == "" {
		req.Frequency = "daily"
	}

	query := `
		INSERT INTO public.routines (user_id, area_id, title, frequency, icon, scheduled_time)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, area_id, title, frequency, icon, scheduled_time
	`

	var rt Routine
	var icon *string
	var areaID *string
	if req.AreaID != "" {
		areaID = &req.AreaID
	}
	err := h.db.QueryRow(r.Context(), query, userID, areaID, req.Title, req.Frequency, req.Icon, req.ScheduledTime).Scan(
		&rt.ID, &rt.AreaID, &rt.Title, &rt.Frequency, &icon, &rt.ScheduledTime,
	)
	if err != nil {
		fmt.Println("Create error:", err)
		http.Error(w, "Failed to create routine", http.StatusInternalServerError)
		return
	}
	if icon != nil {
		rt.Icon = *icon
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rt)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	routineID := chi.URLParam(r, "id")

	var req UpdateRoutineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	setParts := []string{}
	args := []interface{}{}
	argId := 1

	if req.Title != nil {
		setParts = append(setParts, fmt.Sprintf("title = $%d", argId))
		args = append(args, *req.Title)
		argId++
	}
	if req.Frequency != nil {
		setParts = append(setParts, fmt.Sprintf("frequency = $%d", argId))
		args = append(args, *req.Frequency)
		argId++
	}
	if req.Icon != nil {
		setParts = append(setParts, fmt.Sprintf("icon = $%d", argId))
		args = append(args, *req.Icon)
		argId++
	}
	if req.ScheduledTime != nil {
		setParts = append(setParts, fmt.Sprintf("scheduled_time = $%d", argId))
		args = append(args, *req.ScheduledTime)
		argId++
	}

	if len(setParts) == 0 {
		http.Error(w, "No fields to update", http.StatusBadRequest)
		return
	}

	args = append(args, userID, routineID)
	query := fmt.Sprintf(
		"UPDATE public.routines SET %s WHERE user_id = $%d AND id = $%d RETURNING id, area_id, title, frequency, icon, scheduled_time",
		strings.Join(setParts, ", "),
		argId,
		argId+1,
	)

	var rt Routine
	var icon *string
	err := h.db.QueryRow(r.Context(), query, args...).Scan(
		&rt.ID, &rt.AreaID, &rt.Title, &rt.Frequency, &icon, &rt.ScheduledTime,
	)
	if err != nil {
		http.Error(w, "Failed to update routine", http.StatusInternalServerError)
		return
	}
	if icon != nil {
		rt.Icon = *icon
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rt)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	routineID := chi.URLParam(r, "id")

	query := `DELETE FROM public.routines WHERE user_id = $1 AND id = $2`
	if _, err := h.db.Exec(r.Context(), query, userID, routineID); err != nil {
		http.Error(w, "Failed to delete routine", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// --- COMPLETIONS ---

type CompleteRoutineRequest struct {
	Date string `json:"date"` // Optional: YYYY-MM-DD format, defaults to today
}

func (h *Handler) Complete(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	routineID := chi.URLParam(r, "id")

	// Parse optional date from request body
	var req CompleteRoutineRequest
	if r.Body != nil && r.ContentLength > 0 {
		json.NewDecoder(r.Body).Decode(&req) // Ignore errors, date is optional
	}

	// Also check query param for backwards compatibility
	if req.Date == "" {
		req.Date = r.URL.Query().Get("date")
	}

	// Default to today if no date provided
	completionDate := req.Date
	if completionDate == "" {
		completionDate = time.Now().Format("2006-01-02")
	}

	fmt.Printf("📅 Completing routine %s for user %s on date %s\n", routineID, userID, completionDate)

	// Insert with completion_date for unique per-day constraint
	query := `
		INSERT INTO public.routine_completions (user_id, routine_id, completed_at, completion_date)
		VALUES ($1, $2, ($3::date + interval '12 hours'), $3::date)
		ON CONFLICT (user_id, routine_id, completion_date) DO NOTHING
	`

	result, err := h.db.Exec(r.Context(), query, userID, routineID, completionDate)
	if err != nil {
		fmt.Printf("❌ Failed to complete routine: %v\n", err)
		http.Error(w, "Failed to complete routine", http.StatusInternalServerError)
		return
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		fmt.Printf("⚠️ Routine completion already exists for %s (no rows inserted)\n", completionDate)
	} else {
		fmt.Printf("✅ Routine completion created successfully for %s\n", completionDate)
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) BatchComplete(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	var req BatchCompleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	if len(req.RoutineIDs) == 0 {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Use a transaction for batch insert
	tx, err := h.db.Begin(r.Context())
	if err != nil {
		http.Error(w, "Failed to start transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())
	
	now := time.Now()
	for _, id := range req.RoutineIDs {
		// Verify the routine belongs to the user implicitly via foreign key constraints?
		// No, we should verify ownership or rely on the fact that user_id is part of the insert.
		// However, if routine_id does not exist or belong to user, we might want to handle that.
		// But strictly speaking, if we insert (user_id, routine_id), and routine_id is foreign key to public.routines,
		// we only need to ensure public.routines has that ID.
		// AND we should ensure the user owns that routine to avoid completing someone else's routine.
		// The RLS policy on 'insert' should prevent inserting a completion for a routine you don't own?
		// Wait, the RLS on routine_completions says "using (auth.uid() = user_id)". 
		// It doesn't check if routine_id belongs to user_id.
		// So we should add a WHERE clause check or just trust the input if RLS handles user_id correctly.
		// A safe way is:
		// INSERT INTO routine_completions ... SELECT ... FROM routines WHERE id = $2 AND user_id = $1
		
		safeQuery := `
			INSERT INTO public.routine_completions (user_id, routine_id, completed_at)
			SELECT $1, id, $3 FROM public.routines WHERE id = $2 AND user_id = $1
			ON CONFLICT DO NOTHING
		`

		if _, err := tx.Exec(r.Context(), safeQuery, userID, id, now); err != nil {
			// If one fails, fail all? Or ignore?
			// Let's fail all for consistency.
			http.Error(w, "Failed to batch complete", http.StatusInternalServerError)
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "Failed to commit transaction", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

type UncompleteRoutineRequest struct {
	Date string `json:"date"` // Optional: YYYY-MM-DD format, defaults to today
}

func (h *Handler) Uncomplete(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	routineID := chi.URLParam(r, "id")

	// Parse optional date from request body
	var req UncompleteRoutineRequest
	if r.Body != nil && r.ContentLength > 0 {
		json.NewDecoder(r.Body).Decode(&req) // Ignore errors, date is optional
	}

	// Also check query param for backwards compatibility
	if req.Date == "" {
		req.Date = r.URL.Query().Get("date")
	}

	var query string
	var args []interface{}

	if req.Date != "" {
		// Delete completion for specific date
		query = `
			DELETE FROM public.routine_completions
			WHERE user_id = $1 AND routine_id = $2 AND completion_date = $3::date
		`
		args = []interface{}{userID, routineID, req.Date}
		fmt.Printf("📅 Uncompleting routine %s for user %s on date %s\n", routineID, userID, req.Date)
	} else {
		// Delete the most recent completion (backwards compatibility)
		query = `
			DELETE FROM public.routine_completions
			WHERE id = (
				SELECT id FROM public.routine_completions
				WHERE user_id = $1 AND routine_id = $2
				ORDER BY completed_at DESC
				LIMIT 1
			)
		`
		args = []interface{}{userID, routineID}
		fmt.Printf("📅 Uncompleting most recent completion for routine %s, user %s\n", routineID, userID)
	}

	result, err := h.db.Exec(r.Context(), query, args...)
	if err != nil {
		fmt.Printf("❌ Failed to uncomplete routine: %v\n", err)
		http.Error(w, "Failed to undo completion", http.StatusInternalServerError)
		return
	}

	rowsAffected := result.RowsAffected()
	fmt.Printf("✅ Uncomplete: %d rows deleted\n", rowsAffected)

	w.WriteHeader(http.StatusOK)
}