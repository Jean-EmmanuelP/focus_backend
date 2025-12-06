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
	supabaseKey := os.Getenv("SUPABASE_SERVICE_KEY")
	bucketName := "user-content" // You'll need to create this bucket in Supabase

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
