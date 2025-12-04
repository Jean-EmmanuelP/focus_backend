package users

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"firelevel-backend/internal/auth"

	"github.com/jackc/pgx/v5/pgxpool"
)

// 1. The Model: Represents the database row in 'public.users'
type User struct {
	ID        string  `json:"id"`
	Email     *string `json:"email"` // Use pointer for nullable fields if necessary, or string if always present
	FullName  *string `json:"full_name"`
	AvatarURL *string `json:"avatar_url"`
}

// 2. The DTO: Represents what a user is ALLOWED to update
// The DTO needs pointers to distinguish between "empty string" and "missing field"
type UpdateUserRequest struct {
	FullName  *string `json:"full_name"`
	AvatarURL *string `json:"avatar_url"`
}

// 3. The Handler: Holds dependencies (the database client)
type Handler struct {
	db *pgxpool.Pool // Changed from *supabase.Client
}

// Factory function to create a new Handler
func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
}

// ---------------------------------------------------------
// GET /me - Retrieve current user profile
// ---------------------------------------------------------
func (h *Handler) GetProfile(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	// RAW SQL Query
	query := `
		SELECT id, email, full_name, avatar_url 
		FROM public.users 
		WHERE id = $1
	`

	var user User
	// QueryRow scans directly into variables
	err := h.db.QueryRow(r.Context(), query, userID).Scan(
		&user.ID,
		&user.Email,
		&user.FullName,
		&user.AvatarURL,
	)

	if err != nil {
		fmt.Println("❌ Database Error:", err)
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

// ---------------------------------------------------------
// PATCH /me - Update current user profile
// ---------------------------------------------------------
func (h *Handler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	var req UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Dynamic SQL Builder
	// We only want to update fields that were actually sent (not nil)
	
	setParts := []string{}
	args := []interface{}{}
	argId := 1

	if req.FullName != nil {
		setParts = append(setParts, fmt.Sprintf("full_name = $%d", argId))
		args = append(args, *req.FullName)
		argId++
	}

	if req.AvatarURL != nil {
		setParts = append(setParts, fmt.Sprintf("avatar_url = $%d", argId))
		args = append(args, *req.AvatarURL)
		argId++
	}

	if len(setParts) == 0 {
		http.Error(w, "No fields to update", http.StatusBadRequest)
		return
	}

	// Add the WHERE clause
	args = append(args, userID)
	query := fmt.Sprintf(
		"UPDATE public.users SET %s WHERE id = $%d RETURNING id, email, full_name, avatar_url",
		strings.Join(setParts, ", "),
		argId,
	)

	// Execute and Scan back the updated user
	var updatedUser User
	err := h.db.QueryRow(r.Context(), query, args...).Scan(
		&updatedUser.ID,
		&updatedUser.Email,
		&updatedUser.FullName,
		&updatedUser.AvatarURL,
	)

	if err != nil {
		fmt.Println("❌ Update Error:", err)
		http.Error(w, "Failed to update profile", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updatedUser)
}
