package users

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"firelevel-backend/internal/auth"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// 1. The Model: Represents the database row in 'public.users'
type User struct {
	ID                   string  `json:"id"`
	Email                *string `json:"email"`
	Pseudo               *string `json:"pseudo"`                 // Display name / username
	FirstName            *string `json:"first_name"`
	LastName             *string `json:"last_name"`
	Gender               *string `json:"gender"`                 // male, female, other, prefer_not_to_say
	Age                  *int    `json:"age"`
	Description          *string `json:"description"`            // Bio / tagline
	Hobbies              *string `json:"hobbies"`                // Comma-separated or free text
	LifeGoal             *string `json:"life_goal"`              // What they want to achieve
	AvatarURL            *string `json:"avatar_url"`
	DayVisibility        *string `json:"day_visibility"`         // public, crew, private
	ProductivityPeak     *string `json:"productivity_peak"`      // morning, afternoon, evening
	Language             *string `json:"language"`               // fr, en
	Timezone             *string `json:"timezone"`               // Europe/Paris, etc.
	NotificationsEnabled *bool   `json:"notifications_enabled"`  // true/false
	MorningReminderTime  *string `json:"morning_reminder_time"`  // HH:MM format
}

// 2. The DTO: Represents what a user is ALLOWED to update
// The DTO needs pointers to distinguish between "empty string" and "missing field"
type UpdateUserRequest struct {
	Pseudo               *string `json:"pseudo"`
	FirstName            *string `json:"first_name"`
	LastName             *string `json:"last_name"`
	Gender               *string `json:"gender"`
	Age                  *int    `json:"age"`
	Description          *string `json:"description"`
	Hobbies              *string `json:"hobbies"`
	LifeGoal             *string `json:"life_goal"`
	AvatarURL            *string `json:"avatar_url"`
	ProductivityPeak     *string `json:"productivity_peak"`
	Language             *string `json:"language"`
	Timezone             *string `json:"timezone"`
	NotificationsEnabled *bool   `json:"notifications_enabled"`
	MorningReminderTime  *string `json:"morning_reminder_time"`
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

	// RAW SQL Query with all profile fields
	query := `
		SELECT id, email, pseudo, first_name, last_name, gender, age,
		       description, hobbies, life_goal, avatar_url,
		       COALESCE(day_visibility, 'crew') as day_visibility,
		       productivity_peak,
		       COALESCE(language, 'fr') as language,
		       COALESCE(timezone, 'Europe/Paris') as timezone,
		       COALESCE(notifications_enabled, true) as notifications_enabled,
		       COALESCE(morning_reminder_time, '08:00') as morning_reminder_time
		FROM public.users
		WHERE id = $1
	`

	var user User
	err := h.db.QueryRow(r.Context(), query, userID).Scan(
		&user.ID,
		&user.Email,
		&user.Pseudo,
		&user.FirstName,
		&user.LastName,
		&user.Gender,
		&user.Age,
		&user.Description,
		&user.Hobbies,
		&user.LifeGoal,
		&user.AvatarURL,
		&user.DayVisibility,
		&user.ProductivityPeak,
		&user.Language,
		&user.Timezone,
		&user.NotificationsEnabled,
		&user.MorningReminderTime,
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

	// Debug log for productivity peak
	if req.ProductivityPeak != nil {
		fmt.Println("📊 Updating ProductivityPeak to:", *req.ProductivityPeak)
	}

	// Dynamic SQL Builder - only update fields that were sent
	setParts := []string{}
	args := []interface{}{}
	argId := 1

	if req.Pseudo != nil {
		setParts = append(setParts, fmt.Sprintf("pseudo = $%d", argId))
		args = append(args, *req.Pseudo)
		argId++
	}

	if req.FirstName != nil {
		setParts = append(setParts, fmt.Sprintf("first_name = $%d", argId))
		args = append(args, *req.FirstName)
		argId++
	}

	if req.LastName != nil {
		setParts = append(setParts, fmt.Sprintf("last_name = $%d", argId))
		args = append(args, *req.LastName)
		argId++
	}

	if req.Gender != nil {
		setParts = append(setParts, fmt.Sprintf("gender = $%d", argId))
		args = append(args, *req.Gender)
		argId++
	}

	if req.Age != nil {
		setParts = append(setParts, fmt.Sprintf("age = $%d", argId))
		args = append(args, *req.Age)
		argId++
	}

	if req.Description != nil {
		setParts = append(setParts, fmt.Sprintf("description = $%d", argId))
		args = append(args, *req.Description)
		argId++
	}

	if req.Hobbies != nil {
		setParts = append(setParts, fmt.Sprintf("hobbies = $%d", argId))
		args = append(args, *req.Hobbies)
		argId++
	}

	if req.LifeGoal != nil {
		setParts = append(setParts, fmt.Sprintf("life_goal = $%d", argId))
		args = append(args, *req.LifeGoal)
		argId++
	}

	if req.AvatarURL != nil {
		setParts = append(setParts, fmt.Sprintf("avatar_url = $%d", argId))
		args = append(args, *req.AvatarURL)
		argId++
	}

	if req.ProductivityPeak != nil {
		setParts = append(setParts, fmt.Sprintf("productivity_peak = $%d", argId))
		args = append(args, *req.ProductivityPeak)
		argId++
	}

	if req.Language != nil {
		setParts = append(setParts, fmt.Sprintf("language = $%d", argId))
		args = append(args, *req.Language)
		argId++
	}

	if req.Timezone != nil {
		setParts = append(setParts, fmt.Sprintf("timezone = $%d", argId))
		args = append(args, *req.Timezone)
		argId++
	}

	if req.NotificationsEnabled != nil {
		setParts = append(setParts, fmt.Sprintf("notifications_enabled = $%d", argId))
		args = append(args, *req.NotificationsEnabled)
		argId++
	}

	if req.MorningReminderTime != nil {
		setParts = append(setParts, fmt.Sprintf("morning_reminder_time = $%d", argId))
		args = append(args, *req.MorningReminderTime)
		argId++
	}

	if len(setParts) == 0 {
		http.Error(w, "No fields to update", http.StatusBadRequest)
		return
	}

	// Add WHERE clause and RETURNING all fields
	args = append(args, userID)
	query := fmt.Sprintf(
		`UPDATE public.users SET %s WHERE id = $%d
		 RETURNING id, email, pseudo, first_name, last_name, gender, age, description, hobbies, life_goal, avatar_url,
		           COALESCE(day_visibility, 'crew'), productivity_peak,
		           COALESCE(language, 'fr'), COALESCE(timezone, 'Europe/Paris'),
		           COALESCE(notifications_enabled, true), COALESCE(morning_reminder_time, '08:00')`,
		strings.Join(setParts, ", "),
		argId,
	)

	// Execute and scan back the updated user
	var updatedUser User
	err := h.db.QueryRow(r.Context(), query, args...).Scan(
		&updatedUser.ID,
		&updatedUser.Email,
		&updatedUser.Pseudo,
		&updatedUser.FirstName,
		&updatedUser.LastName,
		&updatedUser.Gender,
		&updatedUser.Age,
		&updatedUser.Description,
		&updatedUser.Hobbies,
		&updatedUser.LifeGoal,
		&updatedUser.AvatarURL,
		&updatedUser.DayVisibility,
		&updatedUser.ProductivityPeak,
		&updatedUser.Language,
		&updatedUser.Timezone,
		&updatedUser.NotificationsEnabled,
		&updatedUser.MorningReminderTime,
	)

	if err != nil {
		fmt.Println("❌ Update Error:", err)
		http.Error(w, "Failed to update profile", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updatedUser)
}

// ---------------------------------------------------------
// POST /me/avatar - Upload profile photo
// Accepts multipart/form-data with "file" field
// Or JSON with base64 encoded image
// ---------------------------------------------------------
type UploadAvatarRequest struct {
	ImageBase64 string `json:"image_base64"`
	ContentType string `json:"content_type"` // e.g., "image/jpeg", "image/png"
}

type UploadAvatarResponse struct {
	AvatarURL string `json:"avatar_url"`
}

func (h *Handler) UploadAvatar(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	var imageData []byte
	var contentType string

	// Check if it's multipart form data or JSON
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		// Handle multipart form upload
		err := r.ParseMultipartForm(10 << 20) // 10 MB max
		if err != nil {
			http.Error(w, "File too large (max 10MB)", http.StatusBadRequest)
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "Missing file field", http.StatusBadRequest)
			return
		}
		defer file.Close()

		contentType = header.Header.Get("Content-Type")
		if !strings.HasPrefix(contentType, "image/") {
			http.Error(w, "File must be an image", http.StatusBadRequest)
			return
		}

		imageData, err = io.ReadAll(file)
		if err != nil {
			http.Error(w, "Failed to read file", http.StatusInternalServerError)
			return
		}
	} else {
		// Handle JSON with base64
		var req UploadAvatarRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if req.ImageBase64 == "" {
			http.Error(w, "Missing image_base64 field", http.StatusBadRequest)
			return
		}

		// Handle data URL format (data:image/jpeg;base64,...)
		base64Data := req.ImageBase64
		if strings.Contains(base64Data, ",") {
			parts := strings.SplitN(base64Data, ",", 2)
			if len(parts) == 2 {
				base64Data = parts[1]
				// Extract content type from data URL
				if strings.Contains(parts[0], "image/jpeg") {
					contentType = "image/jpeg"
				} else if strings.Contains(parts[0], "image/png") {
					contentType = "image/png"
				}
			}
		}

		if contentType == "" {
			contentType = req.ContentType
			if contentType == "" {
				contentType = "image/jpeg" // Default
			}
		}

		var err error
		imageData, err = base64.StdEncoding.DecodeString(base64Data)
		if err != nil {
			http.Error(w, "Invalid base64 encoding", http.StatusBadRequest)
			return
		}
	}

	// Validate image size
	if len(imageData) > 10*1024*1024 {
		http.Error(w, "Image too large (max 10MB)", http.StatusBadRequest)
		return
	}

	// Generate unique filename
	extension := ".jpg"
	if strings.Contains(contentType, "png") {
		extension = ".png"
	}
	filename := fmt.Sprintf("avatars/%s/%s%s", userID, uuid.New().String(), extension)

	// Upload to Supabase Storage
	avatarURL, err := uploadToSupabaseStorage(filename, imageData, contentType)
	if err != nil {
		fmt.Println("❌ Storage upload error:", err)
		http.Error(w, "Failed to upload image", http.StatusInternalServerError)
		return
	}

	// Update user's avatar_url in database
	query := `UPDATE public.users SET avatar_url = $1 WHERE id = $2 RETURNING avatar_url`
	var updatedAvatarURL *string
	err = h.db.QueryRow(r.Context(), query, avatarURL, userID).Scan(&updatedAvatarURL)
	if err != nil {
		fmt.Println("❌ Database update error:", err)
		http.Error(w, "Failed to update profile", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(UploadAvatarResponse{AvatarURL: avatarURL})
}

// uploadToSupabaseStorage uploads a file to Supabase Storage
func uploadToSupabaseStorage(path string, data []byte, contentType string) (string, error) {
	supabaseURL := os.Getenv("SUPABASE_URL")
	supabaseKey := os.Getenv("SUPABASE_KEY") // Service Role key to bypass RLS
	bucketName := "avatars"

	if supabaseURL == "" || supabaseKey == "" {
		return "", fmt.Errorf("missing Supabase configuration")
	}

	// Build the storage API URL
	storageURL := fmt.Sprintf("%s/storage/v1/object/%s/%s", supabaseURL, bucketName, path)

	// Create the request
	req, err := http.NewRequest("POST", storageURL, strings.NewReader(string(data)))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+supabaseKey)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("x-upsert", "true") // Overwrite if exists

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("storage error: %s - %s", resp.Status, string(body))
	}

	// Return the public URL
	publicURL := fmt.Sprintf("%s/storage/v1/object/public/%s/%s", supabaseURL, bucketName, path)
	return publicURL, nil
}

// ---------------------------------------------------------
// DELETE /me/avatar - Remove profile photo
// ---------------------------------------------------------
func (h *Handler) DeleteAvatar(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	// Set avatar_url to NULL
	query := `UPDATE public.users SET avatar_url = NULL WHERE id = $1`
	_, err := h.db.Exec(r.Context(), query, userID)
	if err != nil {
		fmt.Println("❌ Delete avatar error:", err)
		http.Error(w, "Failed to remove avatar", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------
// DELETE /me - Delete user account (GDPR compliant)
// ---------------------------------------------------------
func (h *Handler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	// Start a transaction to delete all user data
	tx, err := h.db.Begin(r.Context())
	if err != nil {
		http.Error(w, "Failed to start transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	// Delete all related data first (cascade may not cover everything)
	deletions := []string{
		`DELETE FROM public.chat_messages WHERE user_id = $1`,
		`DELETE FROM public.chat_contexts WHERE user_id = $1`,
		`DELETE FROM public.focus_sessions WHERE user_id = $1`,
		`DELETE FROM public.tasks WHERE user_id = $1`,
		`DELETE FROM public.routine_completions WHERE user_id = $1`,
		`DELETE FROM public.routines WHERE user_id = $1`,
		// Finally delete the user
		`DELETE FROM public.users WHERE id = $1`,
	}

	for _, query := range deletions {
		_, err := tx.Exec(r.Context(), query, userID)
		if err != nil {
			// Log but continue - some tables might not exist
			fmt.Printf("⚠️ Delete query failed (might not exist): %v\n", err)
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "Failed to delete account", http.StatusInternalServerError)
		return
	}

	fmt.Printf("🗑️ Account deleted: %s\n", userID)
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------
// POST /me/location - Update user location
// ---------------------------------------------------------
type LocationUpdateRequest struct {
	Latitude     float64  `json:"latitude"`
	Longitude    float64  `json:"longitude"`
	City         *string  `json:"city"`
	Country      *string  `json:"country"`
	Neighborhood *string  `json:"neighborhood"`
	Timezone     *string  `json:"timezone"`
}

func (h *Handler) UpdateLocation(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	var req LocationUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Update user location in database
	query := `
		UPDATE public.users
		SET latitude = $2,
		    longitude = $3,
		    city = COALESCE($4, city),
		    country = COALESCE($5, country),
		    neighborhood = COALESCE($6, neighborhood),
		    timezone = COALESCE($7, timezone),
		    location_updated_at = NOW()
		WHERE id = $1
	`

	_, err := h.db.Exec(r.Context(), query,
		userID,
		req.Latitude,
		req.Longitude,
		req.City,
		req.Country,
		req.Neighborhood,
		req.Timezone,
	)

	if err != nil {
		fmt.Printf("❌ Failed to update location: %v\n", err)
		http.Error(w, "Failed to update location", http.StatusInternalServerError)
		return
	}

	fmt.Printf("📍 Location updated for user %s: %s, %s\n", userID, stringValue(req.City), stringValue(req.Country))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func stringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

