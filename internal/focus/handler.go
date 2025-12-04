package focus

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

type FocusSession struct {
	ID              string     `json:"id"`
	QuestID         *string    `json:"quest_id"`
	Description     *string    `json:"description"`
	DurationMinutes int        `json:"duration_minutes"`
	Status          string     `json:"status"`
	StartedAt       time.Time  `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at"`
}

type StartSessionRequest struct {
	QuestID         *string `json:"quest_id"`
	Description     *string `json:"description"`
	DurationMinutes int     `json:"duration_minutes"`
}

type UpdateSessionRequest struct {
	Status      *string `json:"status"`
	Description *string `json:"description"`
}

type Handler struct {
	db *pgxpool.Pool
}

func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
}

// Start - POST /focus-sessions
func (h *Handler) Start(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	var req StartSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	if req.DurationMinutes <= 0 {
		http.Error(w, "Duration must be positive", http.StatusBadRequest)
		return
	}

	query := `
		INSERT INTO public.focus_sessions (user_id, quest_id, description, duration_minutes, status, started_at)
		VALUES ($1, $2, $3, $4, 'active', now())
		RETURNING id, quest_id, description, duration_minutes, status, started_at, completed_at
	`

	var s FocusSession
	err := h.db.QueryRow(r.Context(), query, userID, req.QuestID, req.Description, req.DurationMinutes).Scan(
		&s.ID, &s.QuestID, &s.Description, &s.DurationMinutes, &s.Status, &s.StartedAt, &s.CompletedAt,
	)

	if err != nil {
		fmt.Println("Start session error:", err)
		http.Error(w, "Failed to start session", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s)
}

// Update - PATCH /focus-sessions/{id}
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	sessionID := chi.URLParam(r, "id")

	var req UpdateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	setParts := []string{}
	args := []interface{}{}
	argId := 1

	if req.Description != nil {
		setParts = append(setParts, fmt.Sprintf("description = $%d", argId))
		args = append(args, *req.Description)
		argId++
	}

	if req.Status != nil {
		status := *req.Status
		setParts = append(setParts, fmt.Sprintf("status = $%d", argId))
		args = append(args, status)
		argId++

		// Automatically set completed_at if status is 'completed'
		if status == "completed" {
			setParts = append(setParts, "completed_at = now()")
		}
	}

	if len(setParts) == 0 {
		http.Error(w, "No fields to update", http.StatusBadRequest)
		return
	}

	args = append(args, userID, sessionID)
	query := fmt.Sprintf(
		"UPDATE public.focus_sessions SET %s WHERE user_id = $%d AND id = $%d RETURNING id, quest_id, description, duration_minutes, status, started_at, completed_at",
		strings.Join(setParts, ", "),
		argId,
		argId+1,
	)

	var s FocusSession
	err := h.db.QueryRow(r.Context(), query, args...).Scan(
		&s.ID, &s.QuestID, &s.Description, &s.DurationMinutes, &s.Status, &s.StartedAt, &s.CompletedAt,
	)

	if err != nil {
		http.Error(w, "Failed to update session", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s)
}

// List - GET /focus-sessions
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	questID := r.URL.Query().Get("quest_id")
	status := r.URL.Query().Get("status")

	query := `SELECT id, quest_id, description, duration_minutes, status, started_at, completed_at FROM public.focus_sessions WHERE user_id = $1`
	args := []interface{}{userID}
	argId := 2

	if questID != "" {
		query += fmt.Sprintf(" AND quest_id = $%d", argId)
		args = append(args, questID)
		argId++
	}

	if status != "" {
		query += fmt.Sprintf(" AND status = $%d", argId)
		args = append(args, status)
		argId++
	}

	query += " ORDER BY started_at DESC LIMIT 20"

	rows, err := h.db.Query(r.Context(), query, args...)
	if err != nil {
		http.Error(w, "Failed to list sessions", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	sessions := []FocusSession{}
	for rows.Next() {
		var s FocusSession
		if err := rows.Scan(&s.ID, &s.QuestID, &s.Description, &s.DurationMinutes, &s.Status, &s.StartedAt, &s.CompletedAt); err != nil {
			continue
		}
		sessions = append(sessions, s)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessions)
}

// Delete - DELETE /focus-sessions/{id}
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	sessionID := chi.URLParam(r, "id")

	query := `DELETE FROM public.focus_sessions WHERE user_id = $1 AND id = $2`
	if _, err := h.db.Exec(r.Context(), query, userID, sessionID); err != nil {
		http.Error(w, "Failed to delete session", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
