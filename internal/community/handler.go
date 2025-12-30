package community

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"firelevel-backend/internal/auth"
	"firelevel-backend/internal/telegram"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ==========================================
// TYPES
// ==========================================

type Handler struct {
	db *pgxpool.Pool
}

func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
}

// CommunityPost represents a post in the feed
type CommunityPost struct {
	ID         string     `json:"id"`
	UserID     string     `json:"userId"`
	TaskID     *string    `json:"taskId,omitempty"`
	RoutineID  *string    `json:"routineId,omitempty"`
	ImageURL   string     `json:"imageUrl"`
	Caption    *string    `json:"caption,omitempty"`
	LikesCount int        `json:"likesCount"`
	CreatedAt  time.Time  `json:"createdAt"`

	// Joined fields
	User       *PostUser  `json:"user,omitempty"`
	TaskTitle  *string    `json:"taskTitle,omitempty"`
	RoutineTitle *string  `json:"routineTitle,omitempty"`
	IsLikedByMe bool      `json:"isLikedByMe"`
}

type PostUser struct {
	ID        string  `json:"id"`
	Pseudo    *string `json:"pseudo,omitempty"`
	AvatarURL *string `json:"avatarUrl,omitempty"`
}

type CreatePostRequest struct {
	ImageBase64 string  `json:"image_base64"` // Base64 encoded image
	Caption     *string `json:"caption,omitempty"`
	TaskID      *string `json:"task_id,omitempty"`
	RoutineID   *string `json:"routine_id,omitempty"`
	ContentType string  `json:"content_type"` // image/jpeg or image/png
}

type ReportPostRequest struct {
	Reason  string  `json:"reason"`  // inappropriate, spam, harassment, other
	Details *string `json:"details,omitempty"`
}

type FeedResponse struct {
	Posts      []CommunityPost `json:"posts"`
	HasMore    bool            `json:"hasMore"`
	NextOffset int             `json:"nextOffset"`
}

// ==========================================
// HANDLERS
// ==========================================

// CreatePost creates a new community post with image upload
// POST /community/posts
func (h *Handler) CreatePost(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	var req CreatePostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[CreatePost] Failed to decode request body: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Task/routine link is now optional
	log.Printf("[CreatePost] Received request - imageBase64 length: %d, caption: %v, taskId: %v, routineId: %v",
		len(req.ImageBase64), req.Caption, req.TaskID, req.RoutineID)

	// Validate image
	if req.ImageBase64 == "" {
		log.Printf("[CreatePost] Image is empty!")
		http.Error(w, "Image is required", http.StatusBadRequest)
		return
	}

	// Decode base64 image
	imageData, err := base64.StdEncoding.DecodeString(req.ImageBase64)
	if err != nil {
		http.Error(w, "Invalid base64 image", http.StatusBadRequest)
		return
	}

	// Upload to Supabase Storage
	imageURL, err := h.uploadImageToStorage(userID, imageData, req.ContentType)
	if err != nil {
		log.Printf("[CreatePost] Failed to upload image: %v", err)
		http.Error(w, "Failed to upload image", http.StatusInternalServerError)
		return
	}

	// Insert post into database
	var postID string
	err = h.db.QueryRow(r.Context(), `
		INSERT INTO community_posts (user_id, task_id, routine_id, image_url, caption)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, userID, req.TaskID, req.RoutineID, imageURL, req.Caption).Scan(&postID)

	if err != nil {
		log.Printf("[CreatePost] Failed to insert post: %v", err)
		http.Error(w, "Failed to create post", http.StatusInternalServerError)
		return
	}

	// Fetch the created post with user info
	post, err := h.getPostByID(r.Context(), postID, userID)
	if err != nil {
		log.Printf("[CreatePost] Failed to fetch created post: %v", err)
		http.Error(w, "Post created but failed to fetch", http.StatusInternalServerError)
		return
	}

	// Send Telegram notification
	go telegram.Get().Send(telegram.Event{
		Type:     telegram.EventCommunityPostCreated,
		UserID:   userID,
		UserName: "User",
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(post)
}

// GetFeed returns the public feed of posts
// GET /community/feed?offset=0&limit=20
func (h *Handler) GetFeed(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	// Parse pagination
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	// Query posts with user info and like status
	rows, err := h.db.Query(r.Context(), `
		SELECT
			p.id, p.user_id, p.task_id, p.routine_id, p.image_url, p.caption, p.likes_count, p.created_at,
			u.pseudo, u.avatar_url,
			t.title as task_title,
			ro.title as routine_title,
			EXISTS(SELECT 1 FROM community_post_likes l WHERE l.post_id = p.id AND l.user_id = $1) as is_liked
		FROM community_posts p
		JOIN public.users u ON u.id = p.user_id
		LEFT JOIN tasks t ON t.id = p.task_id
		LEFT JOIN routines ro ON ro.id = p.routine_id
		WHERE p.is_hidden = false
		ORDER BY p.created_at DESC
		LIMIT $2 OFFSET $3
	`, userID, limit+1, offset) // Fetch one extra to check if there's more

	if err != nil {
		log.Printf("[GetFeed] Query error: %v", err)
		http.Error(w, "Failed to fetch feed", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	posts := []CommunityPost{}
	for rows.Next() {
		var post CommunityPost
		var user PostUser
		var taskTitle, routineTitle *string
		var isLiked bool

		err := rows.Scan(
			&post.ID, &post.UserID, &post.TaskID, &post.RoutineID, &post.ImageURL, &post.Caption, &post.LikesCount, &post.CreatedAt,
			&user.Pseudo, &user.AvatarURL,
			&taskTitle, &routineTitle,
			&isLiked,
		)
		if err != nil {
			log.Printf("[GetFeed] Scan error: %v", err)
			continue
		}

		user.ID = post.UserID
		post.User = &user
		post.TaskTitle = taskTitle
		post.RoutineTitle = routineTitle
		post.IsLikedByMe = isLiked

		posts = append(posts, post)
	}

	// Check if there's more
	hasMore := len(posts) > limit
	if hasMore {
		posts = posts[:limit] // Remove the extra one
	}

	response := FeedResponse{
		Posts:      posts,
		HasMore:    hasMore,
		NextOffset: offset + limit,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetPost returns a single post by ID
// GET /community/posts/{id}
func (h *Handler) GetPost(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	postID := chi.URLParam(r, "id")

	post, err := h.getPostByID(r.Context(), postID, userID)
	if err != nil {
		http.Error(w, "Post not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(post)
}

// DeletePost deletes a user's own post
// DELETE /community/posts/{id}
func (h *Handler) DeletePost(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	postID := chi.URLParam(r, "id")

	// Get the post's image URL first to delete from storage
	var imageURL string
	err := h.db.QueryRow(r.Context(), `
		SELECT image_url FROM community_posts WHERE id = $1 AND user_id = $2
	`, postID, userID).Scan(&imageURL)

	if err != nil {
		http.Error(w, "Post not found or not yours", http.StatusNotFound)
		return
	}

	// Delete the post
	result, err := h.db.Exec(r.Context(), `
		DELETE FROM community_posts WHERE id = $1 AND user_id = $2
	`, postID, userID)

	if err != nil || result.RowsAffected() == 0 {
		http.Error(w, "Failed to delete post", http.StatusInternalServerError)
		return
	}

	// TODO: Delete image from Supabase Storage (optional cleanup)

	w.WriteHeader(http.StatusNoContent)
}

// LikePost likes a post
// POST /community/posts/{id}/like
func (h *Handler) LikePost(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	postID := chi.URLParam(r, "id")

	// Insert like (ignore if already exists)
	_, err := h.db.Exec(r.Context(), `
		INSERT INTO community_post_likes (post_id, user_id)
		VALUES ($1, $2)
		ON CONFLICT (post_id, user_id) DO NOTHING
	`, postID, userID)

	if err != nil {
		log.Printf("[LikePost] Error: %v", err)
		http.Error(w, "Failed to like post", http.StatusInternalServerError)
		return
	}

	// Update likes count
	h.db.Exec(r.Context(), `
		UPDATE community_posts SET likes_count = (
			SELECT COUNT(*) FROM community_post_likes WHERE post_id = $1
		) WHERE id = $1
	`, postID)

	// Return updated post
	post, _ := h.getPostByID(r.Context(), postID, userID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(post)
}

// UnlikePost removes a like from a post
// DELETE /community/posts/{id}/like
func (h *Handler) UnlikePost(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	postID := chi.URLParam(r, "id")

	// Delete like
	_, err := h.db.Exec(r.Context(), `
		DELETE FROM community_post_likes WHERE post_id = $1 AND user_id = $2
	`, postID, userID)

	if err != nil {
		log.Printf("[UnlikePost] Error: %v", err)
		http.Error(w, "Failed to unlike post", http.StatusInternalServerError)
		return
	}

	// Update likes count
	h.db.Exec(r.Context(), `
		UPDATE community_posts SET likes_count = (
			SELECT COUNT(*) FROM community_post_likes WHERE post_id = $1
		) WHERE id = $1
	`, postID)

	// Return updated post
	post, _ := h.getPostByID(r.Context(), postID, userID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(post)
}

// ReportPost reports a post
// POST /community/posts/{id}/report
func (h *Handler) ReportPost(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	postID := chi.URLParam(r, "id")

	var req ReportPostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate reason
	validReasons := map[string]bool{
		"inappropriate": true,
		"spam":          true,
		"harassment":    true,
		"other":         true,
	}
	if !validReasons[req.Reason] {
		http.Error(w, "Invalid reason", http.StatusBadRequest)
		return
	}

	// Insert report (ignore if already reported by this user)
	_, err := h.db.Exec(r.Context(), `
		INSERT INTO community_post_reports (post_id, reporter_id, reason, details)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (post_id, reporter_id) DO UPDATE SET
			reason = EXCLUDED.reason,
			details = EXCLUDED.details
	`, postID, userID, req.Reason, req.Details)

	if err != nil {
		log.Printf("[ReportPost] Error: %v", err)
		http.Error(w, "Failed to report post", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "reported",
		"message": "Report submitted successfully",
	})
}

// GetMyPosts returns the current user's posts
// GET /community/my-posts
func (h *Handler) GetMyPosts(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	// Parse pagination (optional, defaults to all posts)
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 50 {
		limit = 50
	}

	rows, err := h.db.Query(r.Context(), `
		SELECT
			p.id, p.user_id, p.task_id, p.routine_id, p.image_url, p.caption, p.likes_count, p.created_at,
			u.pseudo, u.avatar_url,
			t.title as task_title,
			ro.title as routine_title
		FROM community_posts p
		JOIN public.users u ON u.id = p.user_id
		LEFT JOIN tasks t ON t.id = p.task_id
		LEFT JOIN routines ro ON ro.id = p.routine_id
		WHERE p.user_id = $1
		ORDER BY p.created_at DESC
		LIMIT $2 OFFSET $3
	`, userID, limit+1, offset)

	if err != nil {
		log.Printf("[GetMyPosts] Query error: %v", err)
		http.Error(w, "Failed to fetch posts", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	posts := []CommunityPost{}
	for rows.Next() {
		var post CommunityPost
		var user PostUser
		var taskTitle, routineTitle *string

		err := rows.Scan(
			&post.ID, &post.UserID, &post.TaskID, &post.RoutineID, &post.ImageURL, &post.Caption, &post.LikesCount, &post.CreatedAt,
			&user.Pseudo, &user.AvatarURL,
			&taskTitle, &routineTitle,
		)
		if err != nil {
			continue
		}

		user.ID = post.UserID
		post.User = &user
		post.TaskTitle = taskTitle
		post.RoutineTitle = routineTitle
		post.IsLikedByMe = true // Own posts

		posts = append(posts, post)
	}

	// Check if there's more
	hasMore := len(posts) > limit
	if hasMore {
		posts = posts[:limit]
	}

	// Return FeedResponse format (same as GetFeed)
	response := FeedResponse{
		Posts:      posts,
		HasMore:    hasMore,
		NextOffset: offset + limit,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetTaskPosts returns posts linked to a specific task
// GET /tasks/{id}/posts
func (h *Handler) GetTaskPosts(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	taskID := chi.URLParam(r, "id")

	rows, err := h.db.Query(r.Context(), `
		SELECT
			p.id, p.user_id, p.task_id, p.routine_id, p.image_url, p.caption, p.likes_count, p.created_at,
			u.pseudo, u.avatar_url,
			EXISTS(SELECT 1 FROM community_post_likes l WHERE l.post_id = p.id AND l.user_id = $1) as is_liked
		FROM community_posts p
		JOIN public.users u ON u.id = p.user_id
		WHERE p.task_id = $2 AND p.is_hidden = false
		ORDER BY p.created_at DESC
	`, userID, taskID)

	if err != nil {
		log.Printf("[GetTaskPosts] Query error: %v", err)
		http.Error(w, "Failed to fetch posts", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	posts := []CommunityPost{}
	for rows.Next() {
		var post CommunityPost
		var user PostUser
		var isLiked bool

		err := rows.Scan(
			&post.ID, &post.UserID, &post.TaskID, &post.RoutineID, &post.ImageURL, &post.Caption, &post.LikesCount, &post.CreatedAt,
			&user.Pseudo, &user.AvatarURL,
			&isLiked,
		)
		if err != nil {
			continue
		}

		user.ID = post.UserID
		post.User = &user
		post.IsLikedByMe = isLiked

		posts = append(posts, post)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(posts)
}

// GetRoutinePosts returns posts linked to a specific routine
// GET /routines/{id}/posts
func (h *Handler) GetRoutinePosts(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	routineID := chi.URLParam(r, "id")

	rows, err := h.db.Query(r.Context(), `
		SELECT
			p.id, p.user_id, p.task_id, p.routine_id, p.image_url, p.caption, p.likes_count, p.created_at,
			u.pseudo, u.avatar_url,
			EXISTS(SELECT 1 FROM community_post_likes l WHERE l.post_id = p.id AND l.user_id = $1) as is_liked
		FROM community_posts p
		JOIN public.users u ON u.id = p.user_id
		WHERE p.routine_id = $2 AND p.is_hidden = false
		ORDER BY p.created_at DESC
	`, userID, routineID)

	if err != nil {
		log.Printf("[GetRoutinePosts] Query error: %v", err)
		http.Error(w, "Failed to fetch posts", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	posts := []CommunityPost{}
	for rows.Next() {
		var post CommunityPost
		var user PostUser
		var isLiked bool

		err := rows.Scan(
			&post.ID, &post.UserID, &post.TaskID, &post.RoutineID, &post.ImageURL, &post.Caption, &post.LikesCount, &post.CreatedAt,
			&user.Pseudo, &user.AvatarURL,
			&isLiked,
		)
		if err != nil {
			continue
		}

		user.ID = post.UserID
		post.User = &user
		post.IsLikedByMe = isLiked

		posts = append(posts, post)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(posts)
}

// ==========================================
// HELPER METHODS
// ==========================================

func (h *Handler) getPostByID(ctx context.Context, postID, userID string) (*CommunityPost, error) {
	var post CommunityPost
	var user PostUser
	var taskTitle, routineTitle *string
	var isLiked bool

	err := h.db.QueryRow(ctx, `
		SELECT
			p.id, p.user_id, p.task_id, p.routine_id, p.image_url, p.caption, p.likes_count, p.created_at,
			u.pseudo, u.avatar_url,
			t.title as task_title,
			ro.title as routine_title,
			EXISTS(SELECT 1 FROM community_post_likes l WHERE l.post_id = p.id AND l.user_id = $2) as is_liked
		FROM community_posts p
		JOIN public.users u ON u.id = p.user_id
		LEFT JOIN tasks t ON t.id = p.task_id
		LEFT JOIN routines ro ON ro.id = p.routine_id
		WHERE p.id = $1
	`, postID, userID).Scan(
		&post.ID, &post.UserID, &post.TaskID, &post.RoutineID, &post.ImageURL, &post.Caption, &post.LikesCount, &post.CreatedAt,
		&user.Pseudo, &user.AvatarURL,
		&taskTitle, &routineTitle,
		&isLiked,
	)

	if err != nil {
		return nil, err
	}

	user.ID = post.UserID
	post.User = &user
	post.TaskTitle = taskTitle
	post.RoutineTitle = routineTitle
	post.IsLikedByMe = isLiked

	return &post, nil
}

func (h *Handler) uploadImageToStorage(userID string, imageData []byte, contentType string) (string, error) {
	supabaseURL := os.Getenv("SUPABASE_URL")
	// Try SUPABASE_SERVICE_ROLE_KEY first, fallback to SUPABASE_KEY
	serviceRoleKey := os.Getenv("SUPABASE_SERVICE_ROLE_KEY")
	if serviceRoleKey == "" {
		serviceRoleKey = os.Getenv("SUPABASE_KEY")
	}

	log.Printf("[uploadImageToStorage] Starting upload - userID: %s, imageSize: %d bytes, contentType: %s", userID, len(imageData), contentType)

	if supabaseURL == "" || serviceRoleKey == "" {
		log.Printf("[uploadImageToStorage] Missing config - URL empty: %v, KEY empty: %v", supabaseURL == "", serviceRoleKey == "")
		return "", fmt.Errorf("missing Supabase configuration")
	}

	// Determine file extension
	ext := "jpg"
	if contentType == "image/png" {
		ext = "png"
	}

	// Generate unique filename (stored in community-images bucket)
	filename := fmt.Sprintf("%s/%s.%s", userID, uuid.New().String(), ext)

	// Upload to Supabase Storage (community-images bucket)
	url := fmt.Sprintf("%s/storage/v1/object/community-images/%s", supabaseURL, filename)

	req, err := http.NewRequest("POST", url, bytes.NewReader(imageData))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+serviceRoleKey)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("x-upsert", "true")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[uploadImageToStorage] Storage error %d: %s (url: %s)", resp.StatusCode, string(body), url)
		return "", fmt.Errorf("storage error %d: %s", resp.StatusCode, string(body))
	}

	// Return public URL
	publicURL := fmt.Sprintf("%s/storage/v1/object/public/community-images/%s", supabaseURL, filename)
	return publicURL, nil
}
