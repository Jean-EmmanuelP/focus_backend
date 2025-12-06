package areas

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"firelevel-backend/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn" // Add pgconn import
	"github.com/jackc/pgx/v5/pgxpool"
)

type Area struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug,omitempty"`
	Icon string `json:"icon,omitempty"`
}

type CreateAreaRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
	Icon string `json:"icon"`
}

type UpdateAreaRequest struct {
	Name *string `json:"name"`
	Slug *string `json:"slug"`
	Icon *string `json:"icon"`
}

type Handler struct {
	db *pgxpool.Pool
}

func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	query := `SELECT id, name, slug, icon FROM public.areas WHERE user_id = $1 ORDER BY created_at DESC`
	rows, err := h.db.Query(r.Context(), query, userID)
	if err != nil {
		fmt.Println("List areas error:", err)
		http.Error(w, "Failed to list areas", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	areas := []Area{}
	for rows.Next() {
		var a Area
		var slug, icon *string

		if err := rows.Scan(&a.ID, &a.Name, &slug, &icon); err != nil {
			fmt.Println("Scan area error:", err)
			continue
		}
		if slug != nil {
			a.Slug = *slug
		}
		if icon != nil {
			a.Icon = *icon
		}
		areas = append(areas, a)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(areas)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	var req CreateAreaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	query := `
		INSERT INTO public.areas (user_id, name, slug, icon)
		VALUES ($1, $2, $3, $4)
		RETURNING id, name, slug, icon
	`

	var a Area
	var slug, icon *string

	err := h.db.QueryRow(r.Context(), query, userID, req.Name, req.Slug, req.Icon).Scan(
		&a.ID, &a.Name, &slug, &icon,
	)
	if err != nil {
		// 1. Check if the error is a Postgres "Unique Violation"
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" {
			http.Error(w, "An area with this name or slug already exists.", http.StatusConflict) // 409 Conflict
			return
		}

		// 2. Handle other errors
		fmt.Println("Create error:", err)
		http.Error(w, "Failed to create area", http.StatusInternalServerError)
		return
	}

	if slug != nil {
		a.Slug = *slug
	}
	if icon != nil {
		a.Icon = *icon
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(a)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	areaID := chi.URLParam(r, "id")

	var req UpdateAreaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	setParts := []string{}
	args := []interface{}{}
	argId := 1

	if req.Name != nil {
		setParts = append(setParts, fmt.Sprintf("name = $%d", argId))
		args = append(args, *req.Name)
		argId++
	}
	if req.Slug != nil {
		setParts = append(setParts, fmt.Sprintf("slug = $%d", argId))
		args = append(args, *req.Slug)
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

	args = append(args, userID, areaID)
	query := fmt.Sprintf(
		"UPDATE public.areas SET %s WHERE user_id = $%d AND id = $%d RETURNING id, name, slug, icon",
		strings.Join(setParts, ", "),
		argId,
		argId+1,
	)

	var a Area
	var slug, icon *string
	err := h.db.QueryRow(r.Context(), query, args...).Scan(
		&a.ID, &a.Name, &slug, &icon,
	)
	if err != nil {
		// 1. Check if the error is a Postgres "Unique Violation" (for name or slug)
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" {
			http.Error(w, "An area with this name or slug already exists.", http.StatusConflict) // 409 Conflict
			return
		}
		http.Error(w, "Failed to update area", http.StatusInternalServerError)
		return
	}

	if slug != nil {
		a.Slug = *slug
	}
	if icon != nil {
		a.Icon = *icon
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(a)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	areaID := chi.URLParam(r, "id")

	query := `DELETE FROM public.areas WHERE user_id = $1 AND id = $2`
	if _, err := h.db.Exec(r.Context(), query, userID, areaID); err != nil {
		http.Error(w, "Failed to delete area", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}