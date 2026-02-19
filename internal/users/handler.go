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
	ID                   string     `json:"id"`
	Email                *string    `json:"email"`
	Pseudo               *string    `json:"pseudo"`                 // Display name / username
	FirstName            *string    `json:"first_name"`
	LastName             *string    `json:"last_name"`
	Gender               *string    `json:"gender"`                 // male, female, other, prefer_not_to_say
	Age                  *int       `json:"age"`
	Birthday             *time.Time `json:"birthday"`               // Date of birth
	Description          *string    `json:"description"`            // Bio / tagline
	Hobbies              *string    `json:"hobbies"`                // Comma-separated or free text
	LifeGoal             *string    `json:"life_goal"`              // What they want to achieve
	AvatarURL            *string    `json:"avatar_url"`
	DayVisibility        *string    `json:"day_visibility"`         // public, crew, private
	ProductivityPeak     *string    `json:"productivity_peak"`      // morning, afternoon, evening
	Language             *string    `json:"language"`               // fr, en
	Timezone             *string    `json:"timezone"`               // Europe/Paris, etc.
	NotificationsEnabled *bool      `json:"notifications_enabled"`  // true/false
	MorningReminderTime  *string    `json:"morning_reminder_time"`  // HH:MM format
	CompanionName        *string    `json:"companion_name"`         // AI companion name
	CompanionGender      *string    `json:"companion_gender"`       // AI companion gender
	AvatarStyle          *string    `json:"avatar_style"`           // Avatar style choice
	CurrentStreak        *int       `json:"current_streak"`         // Current streak count
	LongestStreak        *int       `json:"longest_streak"`         // Longest streak count
	CreatedAt            *string    `json:"created_at"`             // Account creation date
}

// 2. The DTO: Represents what a user is ALLOWED to update
// The DTO needs pointers to distinguish between "empty string" and "missing field"
type UpdateUserRequest struct {
	Pseudo               *string `json:"pseudo"`
	FirstName            *string `json:"first_name"`
	LastName             *string `json:"last_name"`
	Gender               *string `json:"gender"`
	Age                  *int    `json:"age"`
	Birthday             *string `json:"birthday"` // Format: YYYY-MM-DD
	Description          *string `json:"description"`
	Hobbies              *string `json:"hobbies"`
	LifeGoal             *string `json:"life_goal"`
	AvatarURL            *string `json:"avatar_url"`
	ProductivityPeak     *string `json:"productivity_peak"`
	Language             *string `json:"language"`
	Timezone             *string `json:"timezone"`
	NotificationsEnabled *bool   `json:"notifications_enabled"`
	MorningReminderTime  *string `json:"morning_reminder_time"`
	CompanionName        *string `json:"companion_name"`
	CompanionGender      *string `json:"companion_gender"`
	AvatarStyle          *string `json:"avatar_style"`
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
		SELECT id, email, pseudo, first_name, last_name, gender, age, birthday,
		       description, hobbies, life_goal, avatar_url,
		       COALESCE(day_visibility, 'crew') as day_visibility,
		       productivity_peak,
		       COALESCE(language, 'fr') as language,
		       COALESCE(timezone, 'Europe/Paris') as timezone,
		       COALESCE(notifications_enabled, true) as notifications_enabled,
		       COALESCE(morning_reminder_time, '08:00') as morning_reminder_time,
		       companion_name, companion_gender, avatar_style,
		       COALESCE(current_streak, 0) as current_streak,
		       COALESCE(longest_streak, 0) as longest_streak,
		       created_at
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
		&user.Birthday,
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
		&user.CompanionName,
		&user.CompanionGender,
		&user.AvatarStyle,
		&user.CurrentStreak,
		&user.LongestStreak,
		&user.CreatedAt,
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

	if req.Birthday != nil {
		setParts = append(setParts, fmt.Sprintf("birthday = $%d", argId))
		args = append(args, *req.Birthday)
		argId++
	}

	if req.CompanionName != nil {
		setParts = append(setParts, fmt.Sprintf("companion_name = $%d", argId))
		args = append(args, *req.CompanionName)
		argId++
	}

	if req.CompanionGender != nil {
		setParts = append(setParts, fmt.Sprintf("companion_gender = $%d", argId))
		args = append(args, *req.CompanionGender)
		argId++
	}

	if req.AvatarStyle != nil {
		setParts = append(setParts, fmt.Sprintf("avatar_style = $%d", argId))
		args = append(args, *req.AvatarStyle)
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
		 RETURNING id, email, pseudo, first_name, last_name, gender, age, birthday,
		           description, hobbies, life_goal, avatar_url,
		           COALESCE(day_visibility, 'crew'), productivity_peak,
		           COALESCE(language, 'fr'), COALESCE(timezone, 'Europe/Paris'),
		           COALESCE(notifications_enabled, true), COALESCE(morning_reminder_time, '08:00'),
		           companion_name, companion_gender, avatar_style,
		           COALESCE(current_streak, 0), COALESCE(longest_streak, 0), created_at`,
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
		&updatedUser.Birthday,
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
		&updatedUser.CompanionName,
		&updatedUser.CompanionGender,
		&updatedUser.AvatarStyle,
		&updatedUser.CurrentStreak,
		&updatedUser.LongestStreak,
		&updatedUser.CreatedAt,
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
// Deletes ALL user data from ALL tables, then the user record.
// Each deletion runs independently so a missing table doesn't
// block the rest.
// ---------------------------------------------------------
func (h *Handler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	fmt.Printf("🗑️ Starting account deletion for user: %s\n", userID)

	type deletion struct {
		query string
		table string
	}

	// Order: deepest child tables first, parent tables last, user record at the end
	deletions := []deletion{
		// ── Chat & AI ──
		{`DELETE FROM public.chat_messages WHERE user_id = $1`, "chat_messages"},
		{`DELETE FROM public.chat_contexts WHERE user_id = $1`, "chat_contexts"},

		// ── Journal & Reflections ──
		{`DELETE FROM public.daily_reflections WHERE user_id = $1`, "daily_reflections"},
		{`DELETE FROM public.journal_entries WHERE user_id = $1`, "journal_entries"},
		{`DELETE FROM public.journal_bilans WHERE user_id = $1`, "journal_bilans"},

		// ── Focus & Tasks ──
		{`DELETE FROM public.focus_sessions WHERE user_id = $1`, "focus_sessions"},
		{`DELETE FROM public.tasks WHERE user_id = $1`, "tasks"},
		{`DELETE FROM public.day_plans WHERE user_id = $1`, "day_plans"},

		// ── Routines (child tables first) ──
		{`DELETE FROM public.routine_completions WHERE user_id = $1`, "routine_completions"},
		{`DELETE FROM public.group_routines WHERE shared_by = $1`, "group_routines"},
		{`DELETE FROM public.routine_google_events WHERE user_id = $1`, "routine_google_events"},
		{`DELETE FROM public.routines WHERE user_id = $1`, "routines"},

		// ── Quests & Areas ──
		{`DELETE FROM public.quests WHERE user_id = $1`, "quests"},
		{`DELETE FROM public.areas WHERE user_id = $1`, "areas"},

		// ── Weekly goals (child table first) ──
		{`DELETE FROM public.weekly_goal_items WHERE weekly_goal_id IN (SELECT id FROM public.weekly_goals WHERE user_id = $1)`, "weekly_goal_items"},
		{`DELETE FROM public.weekly_goals WHERE user_id = $1`, "weekly_goals"},

		// ── Daily tracking ──
		{`DELETE FROM public.morning_checkins WHERE user_id = $1`, "morning_checkins"},
		{`DELETE FROM public.evening_checkins WHERE user_id = $1`, "evening_checkins"},
		{`DELETE FROM public.daily_intentions WHERE user_id = $1`, "daily_intentions"},

		// ── Community & Social ──
		{`DELETE FROM public.community_post_reports WHERE reporter_id = $1`, "community_post_reports"},
		{`DELETE FROM public.community_post_likes WHERE user_id = $1`, "community_post_likes"},
		{`DELETE FROM public.community_posts WHERE user_id = $1`, "community_posts"},

		// ── Friends & Groups ──
		{`DELETE FROM public.friend_group_members WHERE member_id = $1`, "friend_group_members"},
		{`DELETE FROM public.friend_groups WHERE user_id = $1`, "friend_groups"},

		// ── Integrations ──
		{`DELETE FROM public.google_calendar_config WHERE user_id = $1`, "google_calendar_config"},
		{`DELETE FROM public.gmail_config WHERE user_id = $1`, "gmail_config"},
		{`DELETE FROM public.whatsapp_users WHERE user_id = $1`, "whatsapp_users"},
		{`DELETE FROM public.whatsapp_verification_codes WHERE user_id = $1`, "whatsapp_verification_codes"},
		{`DELETE FROM public.whatsapp_otp WHERE user_id = $1`, "whatsapp_otp"},
		{`DELETE FROM public.whatsapp_pending_users WHERE converted_to_user_id = $1`, "whatsapp_pending_users"},
		{`DELETE FROM public.phone_linking_otps WHERE user_id = $1`, "phone_linking_otps"},

		// ── Device & Notifications ──
		{`DELETE FROM public.device_tokens WHERE user_id = $1`, "device_tokens"},

		// ── Onboarding ──
		{`DELETE FROM public.user_onboarding WHERE user_id = $1`, "user_onboarding"},
	}

	// Execute each deletion independently (no transaction)
	// so a missing/failing table doesn't block the rest
	var failedTables []string
	for _, d := range deletions {
		_, err := h.db.Exec(r.Context(), d.query, userID)
		if err != nil {
			fmt.Printf("⚠️ Failed to delete from %s: %v\n", d.table, err)
			failedTables = append(failedTables, d.table)
		} else {
			fmt.Printf("✅ Deleted from %s\n", d.table)
		}
	}

	// Delete from public.users
	_, err := h.db.Exec(r.Context(), `DELETE FROM public.users WHERE id = $1`, userID)
	if err != nil {
		fmt.Printf("⚠️ Failed to delete from public.users: %v\n", err)
		failedTables = append(failedTables, "public.users")
	} else {
		fmt.Printf("✅ Deleted from public.users\n")
	}

	// Delete from auth.users via Supabase Admin API
	// This triggers ON DELETE CASCADE for any remaining FK references
	supabaseURL := os.Getenv("SUPABASE_URL")
	supabaseKey := os.Getenv("SUPABASE_KEY") // Service Role key
	if supabaseURL != "" && supabaseKey != "" {
		authDeleteURL := fmt.Sprintf("%s/auth/v1/admin/users/%s", supabaseURL, userID)
		req, reqErr := http.NewRequestWithContext(r.Context(), http.MethodDelete, authDeleteURL, nil)
		if reqErr == nil {
			req.Header.Set("Authorization", "Bearer "+supabaseKey)
			req.Header.Set("apikey", supabaseKey)
			client := &http.Client{Timeout: 10 * time.Second}
			resp, apiErr := client.Do(req)
			if apiErr != nil {
				fmt.Printf("⚠️ Failed to delete auth.users via API: %v\n", apiErr)
			} else {
				resp.Body.Close()
				if resp.StatusCode >= 200 && resp.StatusCode < 300 {
					fmt.Printf("✅ Deleted from auth.users via Supabase Admin API\n")
				} else {
					fmt.Printf("⚠️ Supabase Admin API returned %d for auth user deletion\n", resp.StatusCode)
					// Fallback: try direct SQL deletion from auth.users
					_, sqlErr := h.db.Exec(r.Context(), `DELETE FROM auth.users WHERE id = $1`, userID)
					if sqlErr != nil {
						fmt.Printf("⚠️ Fallback auth.users SQL delete also failed: %v\n", sqlErr)
					} else {
						fmt.Printf("✅ Deleted from auth.users via direct SQL\n")
					}
				}
			}
		}
	} else {
		// No Supabase config: try direct SQL
		fmt.Printf("ℹ️ No SUPABASE_URL/KEY, trying direct SQL delete from auth.users\n")
		_, sqlErr := h.db.Exec(r.Context(), `DELETE FROM auth.users WHERE id = $1`, userID)
		if sqlErr != nil {
			fmt.Printf("⚠️ Direct auth.users delete failed: %v\n", sqlErr)
		} else {
			fmt.Printf("✅ Deleted from auth.users via direct SQL\n")
		}
	}

	if len(failedTables) > 0 {
		fmt.Printf("⚠️ Account deleted with some table errors for %s: %v\n", userID, failedTables)
	} else {
		fmt.Printf("✅ Account fully deleted: %s (all %d tables + auth cleaned)\n", userID, len(deletions)+1)
	}

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

// ---------------------------------------------------------
// PUT /me/email - Change email via Supabase Auth
// ---------------------------------------------------------
type ChangeEmailRequest struct {
	NewEmail string `json:"new_email"`
}

func (h *Handler) ChangeEmail(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	accessToken := r.Header.Get("Authorization")
	if strings.HasPrefix(accessToken, "Bearer ") {
		accessToken = strings.TrimPrefix(accessToken, "Bearer ")
	}

	var req ChangeEmailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.NewEmail == "" {
		http.Error(w, "Email is required", http.StatusBadRequest)
		return
	}

	// Call Supabase Auth API to update email
	supabaseURL := os.Getenv("SUPABASE_URL")
	if supabaseURL == "" {
		http.Error(w, "Server configuration error", http.StatusInternalServerError)
		return
	}

	updateURL := supabaseURL + "/auth/v1/user"
	payload := map[string]string{"email": req.NewEmail}
	payloadBytes, _ := json.Marshal(payload)

	httpReq, err := http.NewRequest("PUT", updateURL, strings.NewReader(string(payloadBytes)))
	if err != nil {
		http.Error(w, "Failed to create request", http.StatusInternalServerError)
		return
	}

	httpReq.Header.Set("Authorization", "Bearer "+accessToken)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("apikey", os.Getenv("SUPABASE_KEY"))

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		fmt.Printf("❌ Supabase auth error: %v\n", err)
		http.Error(w, "Failed to update email", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("❌ Supabase auth error: %s - %s\n", resp.Status, string(body))
		http.Error(w, "Failed to update email: "+string(body), resp.StatusCode)
		return
	}

	fmt.Printf("✅ Email change initiated for user %s to %s\n", userID, req.NewEmail)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"message": "Confirmation emails sent to both addresses",
	})
}

// ---------------------------------------------------------
// PUT /me/password - Change password via Supabase Auth
// ---------------------------------------------------------
type ChangePasswordRequest struct {
	NewPassword string `json:"new_password"`
}

func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	accessToken := r.Header.Get("Authorization")
	if strings.HasPrefix(accessToken, "Bearer ") {
		accessToken = strings.TrimPrefix(accessToken, "Bearer ")
	}

	var req ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.NewPassword == "" || len(req.NewPassword) < 6 {
		http.Error(w, "Password must be at least 6 characters", http.StatusBadRequest)
		return
	}

	// Call Supabase Auth API to update password
	supabaseURL := os.Getenv("SUPABASE_URL")
	if supabaseURL == "" {
		http.Error(w, "Server configuration error", http.StatusInternalServerError)
		return
	}

	updateURL := supabaseURL + "/auth/v1/user"
	payload := map[string]string{"password": req.NewPassword}
	payloadBytes, _ := json.Marshal(payload)

	httpReq, err := http.NewRequest("PUT", updateURL, strings.NewReader(string(payloadBytes)))
	if err != nil {
		http.Error(w, "Failed to create request", http.StatusInternalServerError)
		return
	}

	httpReq.Header.Set("Authorization", "Bearer "+accessToken)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("apikey", os.Getenv("SUPABASE_KEY"))

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		fmt.Printf("❌ Supabase auth error: %v\n", err)
		http.Error(w, "Failed to update password", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("❌ Supabase auth error: %s - %s\n", resp.Status, string(body))
		http.Error(w, "Failed to update password: "+string(body), resp.StatusCode)
		return
	}

	fmt.Printf("✅ Password changed for user %s\n", userID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"message": "Password updated successfully",
	})
}

