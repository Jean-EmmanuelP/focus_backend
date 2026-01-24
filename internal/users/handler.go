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
	ID              string  `json:"id"`
	Email           *string `json:"email"`
	Pseudo          *string `json:"pseudo"`           // Display name / username
	FirstName       *string `json:"first_name"`
	LastName        *string `json:"last_name"`
	Gender          *string `json:"gender"`           // male, female, other, prefer_not_to_say
	Age             *int    `json:"age"`
	Description     *string `json:"description"`      // Bio / tagline
	Hobbies         *string `json:"hobbies"`          // Comma-separated or free text
	LifeGoal        *string `json:"life_goal"`        // What they want to achieve
	AvatarURL       *string `json:"avatar_url"`
	DayVisibility   *string `json:"day_visibility"`   // public, crew, private
	ProductivityPeak *string `json:"productivity_peak"` // morning, afternoon, evening
}

// 2. The DTO: Represents what a user is ALLOWED to update
// The DTO needs pointers to distinguish between "empty string" and "missing field"
type UpdateUserRequest struct {
	Pseudo           *string `json:"pseudo"`
	FirstName        *string `json:"first_name"`
	LastName         *string `json:"last_name"`
	Gender           *string `json:"gender"`
	Age              *int    `json:"age"`
	Description      *string `json:"description"`
	Hobbies          *string `json:"hobbies"`
	LifeGoal         *string `json:"life_goal"`
	AvatarURL        *string `json:"avatar_url"`
	ProductivityPeak *string `json:"productivity_peak"`
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
		       productivity_peak
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

	if len(setParts) == 0 {
		http.Error(w, "No fields to update", http.StatusBadRequest)
		return
	}

	// Add WHERE clause and RETURNING all fields
	args = append(args, userID)
	query := fmt.Sprintf(
		`UPDATE public.users SET %s WHERE id = $%d
		 RETURNING id, email, pseudo, first_name, last_name, gender, age, description, hobbies, life_goal, avatar_url, day_visibility, productivity_peak`,
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
// GET /me/whatsapp - Get WhatsApp linking status
// ---------------------------------------------------------
type WhatsAppStatusResponse struct {
	IsLinked    bool    `json:"is_linked"`
	PhoneNumber *string `json:"phone_number,omitempty"`
	LinkedAt    *string `json:"linked_at,omitempty"`
}

func (h *Handler) GetWhatsAppStatus(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	var phoneNumber *string
	var phoneVerified bool
	var linkedAt *time.Time

	err := h.db.QueryRow(r.Context(), `
		SELECT phone_number, COALESCE(phone_verified, false), whatsapp_linked_at
		FROM public.users WHERE id = $1
	`, userID).Scan(&phoneNumber, &phoneVerified, &linkedAt)

	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	resp := WhatsAppStatusResponse{
		IsLinked: phoneNumber != nil && phoneVerified,
	}

	if resp.IsLinked {
		resp.PhoneNumber = phoneNumber
		if linkedAt != nil {
			t := linkedAt.Format(time.RFC3339)
			resp.LinkedAt = &t
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ---------------------------------------------------------
// POST /me/whatsapp/link - Initiate phone linking
// ---------------------------------------------------------
type LinkPhoneRequest struct {
	PhoneNumber string `json:"phone_number"` // E.164 format: +33612345678
}

type LinkPhoneResponse struct {
	Message string `json:"message"`
	CodeSent bool   `json:"code_sent"`
}

func (h *Handler) InitiatePhoneLink(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	var req LinkPhoneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.PhoneNumber == "" {
		http.Error(w, "Phone number is required", http.StatusBadRequest)
		return
	}

	// Check if phone is already used by another user
	var existingUserID *string
	_ = h.db.QueryRow(r.Context(), `
		SELECT id FROM public.users WHERE phone_number = $1 AND phone_verified = true AND id != $2
	`, req.PhoneNumber, userID).Scan(&existingUserID)

	if existingUserID != nil {
		http.Error(w, "Phone number already linked to another account", http.StatusConflict)
		return
	}

	// Generate OTP
	otp := generateOTP()
	expiresAt := time.Now().Add(10 * time.Minute)

	// Delete existing OTPs for this user
	_, _ = h.db.Exec(r.Context(), `DELETE FROM public.phone_linking_otps WHERE user_id = $1`, userID)

	// Save new OTP
	_, err := h.db.Exec(r.Context(), `
		INSERT INTO public.phone_linking_otps (user_id, phone_number, otp_code, expires_at)
		VALUES ($1, $2, $3, $4)
	`, userID, req.PhoneNumber, otp, expiresAt)

	if err != nil {
		http.Error(w, "Failed to generate code", http.StatusInternalServerError)
		return
	}

	// In production: send OTP via SMS or WhatsApp message
	// For now, we'll return a success message (and log the OTP for testing)
	fmt.Printf("📱 OTP for %s: %s\n", req.PhoneNumber, otp)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(LinkPhoneResponse{
		Message:  "Code sent to your phone",
		CodeSent: true,
	})
}

// ---------------------------------------------------------
// POST /me/whatsapp/verify - Verify OTP and complete linking
// ---------------------------------------------------------
type VerifyOTPRequest struct {
	Code string `json:"code"`
}

func (h *Handler) VerifyPhoneLink(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	var req VerifyOTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Code == "" {
		http.Error(w, "Code is required", http.StatusBadRequest)
		return
	}

	// Verify OTP
	var phoneNumber string
	var expiresAt time.Time
	err := h.db.QueryRow(r.Context(), `
		SELECT phone_number, expires_at
		FROM public.phone_linking_otps
		WHERE user_id = $1 AND otp_code = $2 AND verified = false
	`, userID, req.Code).Scan(&phoneNumber, &expiresAt)

	if err != nil {
		http.Error(w, "Invalid code", http.StatusBadRequest)
		return
	}

	if time.Now().After(expiresAt) {
		http.Error(w, "Code expired", http.StatusBadRequest)
		return
	}

	// Mark OTP as verified
	_, _ = h.db.Exec(r.Context(), `
		UPDATE public.phone_linking_otps SET verified = true WHERE user_id = $1
	`, userID)

	// Link phone to user
	_, err = h.db.Exec(r.Context(), `
		UPDATE public.users
		SET phone_number = $1, phone_verified = true, whatsapp_linked_at = now()
		WHERE id = $2
	`, phoneNumber, userID)

	if err != nil {
		http.Error(w, "Failed to link phone", http.StatusInternalServerError)
		return
	}

	// Convert pending WhatsApp user if exists
	_, _ = h.db.Exec(r.Context(), `
		UPDATE public.whatsapp_pending_users
		SET converted_to_user_id = $1
		WHERE phone_number = $2
	`, userID, phoneNumber)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"phone_number": phoneNumber,
		"message":      "WhatsApp linked successfully!",
	})
}

// ---------------------------------------------------------
// DELETE /me/whatsapp - Unlink WhatsApp
// ---------------------------------------------------------
func (h *Handler) UnlinkWhatsApp(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	_, err := h.db.Exec(r.Context(), `
		UPDATE public.users
		SET phone_number = NULL, phone_verified = false, whatsapp_linked_at = NULL
		WHERE id = $1
	`, userID)

	if err != nil {
		http.Error(w, "Failed to unlink WhatsApp", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// generateOTP creates a 6-digit OTP
func generateOTP() string {
	return fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
}
