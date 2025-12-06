package routines

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"firelevel-backend/internal/auth"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Completion struct {
	ID          string    `json:"id"`
	RoutineID   string    `json:"routine_id"`
	CompletedAt time.Time `json:"completed_at"`
}

type CompletionHandler struct {
	db *pgxpool.Pool
}

func NewCompletionHandler(db *pgxpool.Pool) *CompletionHandler {
	return &CompletionHandler{db: db}
}

func (h *CompletionHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	routineID := r.URL.Query().Get("routine_id")
	fromDate := r.URL.Query().Get("from")
	toDate := r.URL.Query().Get("to")

	query := `SELECT id, routine_id, completed_at FROM public.routine_completions WHERE user_id = $1`
	args := []interface{}{userID}
	argCount := 1

	if routineID != "" {
		argCount++
		query += fmt.Sprintf(" AND routine_id = $%d", argCount)
		args = append(args, routineID)
	}

	if fromDate != "" {
		argCount++
		query += fmt.Sprintf(" AND DATE(completed_at) >= $%d::date", argCount)
		args = append(args, fromDate)
	}

	if toDate != "" {
		argCount++
		query += fmt.Sprintf(" AND DATE(completed_at) <= $%d::date", argCount)
		args = append(args, toDate)
	}

	query += " ORDER BY completed_at DESC"

	rows, err := h.db.Query(r.Context(), query, args...)
	if err != nil {
		fmt.Println("List completions error:", err)
		http.Error(w, "Failed to list completions", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	completions := []Completion{}
	for rows.Next() {
		var c Completion
		if err := rows.Scan(&c.ID, &c.RoutineID, &c.CompletedAt); err != nil {
			fmt.Println("Scan completion error:", err)
			continue
		}
		completions = append(completions, c)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(completions)
}
