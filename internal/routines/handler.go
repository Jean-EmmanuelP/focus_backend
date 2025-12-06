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
	ID        string  `json:"id"`
	AreaID    *string `json:"area_id,omitempty"`
	Title     string  `json:"title"`
	Frequency string  `json:"frequency"`
	Icon      string  `json:"icon,omitempty"`
}

type CreateRoutineRequest struct {
	AreaID    string `json:"area_id"`
	Title     string `json:"title"`
	Frequency string `json:"frequency"` // default 'daily'
	Icon      string `json:"icon"`
}

type BatchCompleteRequest struct {
	RoutineIDs []string `json:"routine_ids"`
}

type UpdateRoutineRequest struct {
	Title     *string `json:"title"`
	Frequency *string `json:"frequency"`
	Icon      *string `json:"icon"`
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

	query := `SELECT id, area_id, title, frequency, icon FROM public.routines WHERE user_id = $1`
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
		if err := rows.Scan(&rt.ID, &rt.AreaID, &rt.Title, &rt.Frequency, &icon); err != nil {
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
		INSERT INTO public.routines (user_id, area_id, title, frequency, icon) 
		VALUES ($1, $2, $3, $4, $5) 
		RETURNING id, area_id, title, frequency, icon
	`

	var rt Routine
	var icon *string
	var areaID *string
	if req.AreaID != "" {
		areaID = &req.AreaID
	}
	err := h.db.QueryRow(r.Context(), query, userID, areaID, req.Title, req.Frequency, req.Icon).Scan(
		&rt.ID, &rt.AreaID, &rt.Title, &rt.Frequency, &icon,
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

	if len(setParts) == 0 {
		http.Error(w, "No fields to update", http.StatusBadRequest)
		return
	}

	args = append(args, userID, routineID)
	query := fmt.Sprintf(
		"UPDATE public.routines SET %s WHERE user_id = $%d AND id = $%d RETURNING id, area_id, title, frequency, icon",
		strings.Join(setParts, ", "),
		argId,
		argId+1,
	)

	var rt Routine
	var icon *string
	err := h.db.QueryRow(r.Context(), query, args...).Scan(
		&rt.ID, &rt.AreaID, &rt.Title, &rt.Frequency, &icon,
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

func (h *Handler) Complete(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	routineID := chi.URLParam(r, "id")

	// Insert new completion for now()
	// ON CONFLICT DO NOTHING ensures idempotency if the user spams the button
	query := `
		INSERT INTO public.routine_completions (user_id, routine_id, completed_at)
		VALUES ($1, $2, now())
		ON CONFLICT DO NOTHING
	`

	_, err := h.db.Exec(r.Context(), query, userID, routineID)
	if err != nil {
		http.Error(w, "Failed to complete routine", http.StatusInternalServerError)
		return
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

func (h *Handler) Uncomplete(w http.ResponseWriter, r *http.Request) {
	// This is tricky without a specific completion ID. 
	// For "Undo", we usually delete the *latest* completion for today.
	// For MVP, we can delete the most recent completion for this routine.
	userID := r.Context().Value(auth.UserContextKey).(string)
	routineID := chi.URLParam(r, "id")

	query := `
		DELETE FROM public.routine_completions
		WHERE id = (
			SELECT id FROM public.routine_completions
			WHERE user_id = $1 AND routine_id = $2
			ORDER BY completed_at DESC
			LIMIT 1
		)
	`

	_, err := h.db.Exec(r.Context(), query, userID, routineID)
	if err != nil {
		http.Error(w, "Failed to undo completion", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}