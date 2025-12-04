package quests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"firelevel-backend/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Quest struct {
	ID           string `json:"id"`
	AreaID       string `json:"area_id"`
	Title        string `json:"title"`
	Status       string `json:"status"`
	CurrentValue int    `json:"current_value"`
	TargetValue  int    `json:"target_value"`
}

type CreateQuestRequest struct {
	AreaID      string `json:"area_id"`
	Title       string `json:"title"`
	TargetValue int    `json:"target_value"`
}

type UpdateQuestRequest struct {
	Title        *string `json:"title"`
	Status       *string `json:"status"`
	CurrentValue *int    `json:"current_value"`
	TargetValue  *int    `json:"target_value"`
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

	query := `SELECT id, area_id, title, status, current_value, target_value FROM public.quests WHERE user_id = $1`
	args := []interface{}{userID}

	if areaID != "" {
		query += " AND area_id = $2"
		args = append(args, areaID)
	}
	query += " ORDER BY created_at DESC"

	rows, err := h.db.Query(r.Context(), query, args...)
	if err != nil {
		http.Error(w, "Failed to list quests", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	quests := []Quest{}
	for rows.Next() {
		var q Quest
		if err := rows.Scan(&q.ID, &q.AreaID, &q.Title, &q.Status, &q.CurrentValue, &q.TargetValue); err != nil {
			continue
		}
		quests = append(quests, q)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(quests)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	var req CreateQuestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	query := `
		INSERT INTO public.quests (user_id, area_id, title, target_value) 
		VALUES ($1, $2, $3, $4) 
		RETURNING id, area_id, title, status, current_value, target_value
	`

	var q Quest
	err := h.db.QueryRow(r.Context(), query, userID, req.AreaID, req.Title, req.TargetValue).Scan(
		&q.ID, &q.AreaID, &q.Title, &q.Status, &q.CurrentValue, &q.TargetValue,
	)
	if err != nil {
		fmt.Println("Create error:", err)
		http.Error(w, "Failed to create quest", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(q)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	questID := chi.URLParam(r, "id")

	var req UpdateQuestRequest
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
	if req.Status != nil {
		setParts = append(setParts, fmt.Sprintf("status = $%d", argId))
		args = append(args, *req.Status)
		argId++
	}
	if req.CurrentValue != nil {
		setParts = append(setParts, fmt.Sprintf("current_value = $%d", argId))
		args = append(args, *req.CurrentValue)
		argId++
	}
	if req.TargetValue != nil {
		setParts = append(setParts, fmt.Sprintf("target_value = $%d", argId))
		args = append(args, *req.TargetValue)
		argId++
	}

	if len(setParts) == 0 {
		http.Error(w, "No fields to update", http.StatusBadRequest)
		return
	}

	args = append(args, userID, questID)
	query := fmt.Sprintf(
		"UPDATE public.quests SET %s WHERE user_id = $%d AND id = $%d RETURNING id, area_id, title, status, current_value, target_value",
		strings.Join(setParts, ", "),
		argId,
		argId+1,
	)

	var q Quest
	err := h.db.QueryRow(r.Context(), query, args...).Scan(
		&q.ID, &q.AreaID, &q.Title, &q.Status, &q.CurrentValue, &q.TargetValue,
	)
	if err != nil {
		http.Error(w, "Failed to update quest", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(q)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	questID := chi.URLParam(r, "id")

	query := `DELETE FROM public.quests WHERE user_id = $1 AND id = $2`
	if _, err := h.db.Exec(r.Context(), query, userID, questID); err != nil {
		http.Error(w, "Failed to delete quest", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
