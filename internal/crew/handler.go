package crew

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"firelevel-backend/internal/auth"
	"firelevel-backend/internal/telegram"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ============================================================================
// Models
// ============================================================================

type CrewMember struct {
	ID              string  `json:"id"`
	MemberID        string  `json:"member_id"`
	Pseudo          *string `json:"pseudo"`
	FirstName       *string `json:"first_name"`
	LastName        *string `json:"last_name"`
	Email           *string `json:"email"`
	AvatarUrl       *string `json:"avatar_url"`
	DayVisibility   *string `json:"day_visibility"`
	TotalSessions7d *int    `json:"total_sessions_7d"`
	TotalMinutes7d  *int    `json:"total_minutes_7d"`
	ActivityScore   *int    `json:"activity_score"`
	CreatedAt       *string `json:"created_at"`
}

type CrewRequest struct {
	ID         string        `json:"id"`
	FromUserID string        `json:"from_user_id"`
	ToUserID   string        `json:"to_user_id"`
	Status     string        `json:"status"`
	Message    *string       `json:"message"`
	CreatedAt  time.Time     `json:"created_at"`
	UpdatedAt  *time.Time    `json:"updated_at"`
	FromUser   *CrewUserInfo `json:"from_user,omitempty"`
	ToUser     *CrewUserInfo `json:"to_user,omitempty"`
}

type CrewUserInfo struct {
	ID        string  `json:"id"`
	Pseudo    *string `json:"pseudo"`
	FirstName *string `json:"first_name"`
	LastName  *string `json:"last_name"`
	Email     *string `json:"email"`
	AvatarUrl *string `json:"avatar_url"`
}

type LeaderboardEntry struct {
	Rank                int     `json:"rank"`
	ID                  string  `json:"id"`
	Pseudo              *string `json:"pseudo"`
	FirstName           *string `json:"first_name"`
	LastName            *string `json:"last_name"`
	Email               *string `json:"email"`
	AvatarUrl           *string `json:"avatar_url"`
	DayVisibility       *string `json:"day_visibility"`
	TotalSessions7d     int     `json:"total_sessions_7d"`
	TotalMinutes7d      int     `json:"total_minutes_7d"`
	CompletedRoutines7d int     `json:"completed_routines_7d"`
	ActivityScore       int     `json:"activity_score"`
	CurrentStreak       int     `json:"current_streak"`
	LastActive          *string `json:"last_active"`
	IsCrewMember        bool    `json:"is_crew_member"`
	HasPendingRequest   bool    `json:"has_pending_request"`
	RequestDirection    *string `json:"request_direction"`
	IsSelf              bool    `json:"is_self"`
	// Live focus session fields
	IsLive               bool    `json:"is_live"`
	LiveSessionStartedAt *string `json:"live_session_started_at,omitempty"`
	LiveSessionDuration  *int    `json:"live_session_duration,omitempty"`
}

type SearchUserResult struct {
	ID                string  `json:"id"`
	Pseudo            *string `json:"pseudo"`
	FirstName         *string `json:"first_name"`
	LastName          *string `json:"last_name"`
	Email             *string `json:"email"`
	AvatarUrl         *string `json:"avatar_url"`
	DayVisibility     *string `json:"day_visibility"`
	TotalSessions7d   *int    `json:"total_sessions_7d"`
	TotalMinutes7d    *int    `json:"total_minutes_7d"`
	ActivityScore     *int    `json:"activity_score"`
	IsCrewMember      bool    `json:"is_crew_member"`
	HasPendingRequest bool    `json:"has_pending_request"`
	RequestDirection  *string `json:"request_direction"`
	IsSelf            bool    `json:"is_self"`
}

type CrewTask struct {
	ID             string  `json:"id"`
	Title          string  `json:"title"`
	Description    *string `json:"description,omitempty"`
	ScheduledStart *string `json:"scheduled_start,omitempty"`
	ScheduledEnd   *string `json:"scheduled_end,omitempty"`
	TimeBlock      string  `json:"time_block"`
	Priority       string  `json:"priority"`
	Status         string  `json:"status"`
	AreaName       *string `json:"area_name,omitempty"`
	AreaIcon       *string `json:"area_icon,omitempty"`
	IsPrivate      bool    `json:"is_private"`
}

type CrewMemberDay struct {
	User              *CrewUserInfo          `json:"user"`
	Intentions        []CrewIntention        `json:"intentions"`
	FocusSessions     []CrewFocusSession     `json:"focus_sessions"`
	CompletedRoutines []CrewCompletedRoutine `json:"completed_routines"`
	Routines          []CrewRoutine          `json:"routines"`
	Tasks             []CrewTask             `json:"tasks"`
	Stats             *CrewMemberStats       `json:"stats"`
}

type CrewMemberStats struct {
	// Weekly stats (last 7 days)
	WeeklyFocusMinutes   []DailyStat `json:"weekly_focus_minutes"`
	WeeklyRoutinesDone   []DailyStat `json:"weekly_routines_done"`
	WeeklyTotalFocus     int         `json:"weekly_total_focus"`
	WeeklyTotalRoutines  int         `json:"weekly_total_routines"`
	WeeklyAvgFocus       int         `json:"weekly_avg_focus"`
	WeeklyRoutineRate    int         `json:"weekly_routine_rate"` // percentage

	// Monthly stats (last 30 days)
	MonthlyFocusMinutes  []DailyStat `json:"monthly_focus_minutes"`
	MonthlyRoutinesDone  []DailyStat `json:"monthly_routines_done"`
	MonthlyTotalFocus    int         `json:"monthly_total_focus"`
	MonthlyTotalRoutines int         `json:"monthly_total_routines"`
}

type DailyStat struct {
	Date  string `json:"date"`
	Value int    `json:"value"`
}

type CrewRoutine struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Icon        *string    `json:"icon"`
	Completed   bool       `json:"completed"`
	CompletedAt *time.Time `json:"completed_at"`
	LikeCount   int        `json:"like_count"`
	IsLikedByMe bool       `json:"is_liked_by_me"`
}

type CrewIntention struct {
	ID       string `json:"id"`
	Content  string `json:"content"`
	Position int    `json:"position"`
}

type CrewFocusSession struct {
	ID              string     `json:"id"`
	Description     *string    `json:"description"`
	DurationMinutes int        `json:"duration_minutes"`
	StartedAt       time.Time  `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at"`
	Status          string     `json:"status"`
}

type CrewCompletedRoutine struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Icon        *string   `json:"icon"`
	CompletedAt time.Time `json:"completed_at"`
	LikeCount   int       `json:"like_count"`
	IsLikedByMe bool      `json:"is_liked_by_me"`
}

// ============================================================================
// Request DTOs
// ============================================================================

type SendCrewRequestDTO struct {
	ToUserID string  `json:"to_user_id"`
	Message  *string `json:"message"`
}

type UpdateVisibilityRequest struct {
	DayVisibility string `json:"day_visibility"`
}

// ============================================================================
// Handler
// ============================================================================

type Handler struct {
	db *pgxpool.Pool
}

func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
}

// ============================================================================
// GET /crew/members - List crew members
// ============================================================================

func (h *Handler) ListMembers(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	query := `
		SELECT
			cm.id,
			cm.member_id,
			u.pseudo,
			u.first_name,
			u.last_name,
			u.email,
			u.avatar_url,
			COALESCE(u.day_visibility, 'crew') as day_visibility,
			COALESCE(fs.total_sessions, 0)::int as total_sessions_7d,
			COALESCE(fs.total_minutes, 0)::int as total_minutes_7d,
			(COALESCE(fs.total_minutes, 0) + (COALESCE(rc.completed_count, 0) * 10))::int as activity_score,
			cm.created_at
		FROM friendships cm
		JOIN users u ON cm.member_id = u.id
		LEFT JOIN (
			SELECT user_id, COUNT(*)::int as total_sessions, COALESCE(SUM(duration_minutes), 0)::int as total_minutes
			FROM focus_sessions
			WHERE started_at >= NOW() - INTERVAL '7 days' AND status = 'completed'
			GROUP BY user_id
		) fs ON u.id = fs.user_id
		LEFT JOIN (
			SELECT r.user_id, COUNT(*)::int as completed_count
			FROM routine_completions c
			JOIN routines r ON c.routine_id = r.id
			WHERE c.completed_at >= NOW() - INTERVAL '7 days'
			GROUP BY r.user_id
		) rc ON u.id = rc.user_id
		WHERE cm.user_id = $1
		ORDER BY cm.created_at DESC
	`

	rows, err := h.db.Query(r.Context(), query, userID)
	if err != nil {
		fmt.Println("List crew members error:", err)
		http.Error(w, "Failed to list crew members", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	members := []CrewMember{}
	for rows.Next() {
		var m CrewMember
		var createdAt time.Time
		if err := rows.Scan(
			&m.ID, &m.MemberID, &m.Pseudo, &m.FirstName, &m.LastName,
			&m.Email, &m.AvatarUrl, &m.DayVisibility, &m.TotalSessions7d, &m.TotalMinutes7d,
			&m.ActivityScore, &createdAt,
		); err != nil {
			fmt.Println("Scan crew member error:", err)
			continue
		}
		createdAtStr := createdAt.Format(time.RFC3339)
		m.CreatedAt = &createdAtStr
		members = append(members, m)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(members)
}

// ============================================================================
// DELETE /crew/members/{id} - Remove a crew member
// ============================================================================

func (h *Handler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	memberID := chi.URLParam(r, "id")

	// Remove bidirectional membership
	query := `
		DELETE FROM friendships
		WHERE (user_id = $1 AND member_id = $2)
		   OR (user_id = $2 AND member_id = $1)
	`
	_, err := h.db.Exec(r.Context(), query, userID, memberID)
	if err != nil {
		fmt.Println("Remove crew member error:", err)
		http.Error(w, "Failed to remove crew member", http.StatusInternalServerError)
		return
	}

	// Update related requests to allow re-requesting
	updateQuery := `
		UPDATE friend_requests SET status = 'rejected'
		WHERE (from_user_id = $1 AND to_user_id = $2)
		   OR (from_user_id = $2 AND to_user_id = $1)
	`
	if _, err := h.db.Exec(r.Context(), updateQuery, userID, memberID); err != nil {
		fmt.Println("Update crew requests error:", err)
		// Non-critical, continue anyway
	}

	w.WriteHeader(http.StatusOK)
}

// ============================================================================
// GET /crew/requests/received - List received requests
// ============================================================================

func (h *Handler) ListReceivedRequests(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	query := `
		SELECT
			cr.id, cr.from_user_id, cr.to_user_id, cr.status, cr.message,
			cr.created_at, cr.updated_at,
			u.id, u.pseudo, u.first_name, u.last_name, u.email, u.avatar_url
		FROM friend_requests cr
		JOIN users u ON cr.from_user_id = u.id
		WHERE cr.to_user_id = $1 AND cr.status = 'pending'
		ORDER BY cr.created_at DESC
	`

	rows, err := h.db.Query(r.Context(), query, userID)
	if err != nil {
		fmt.Println("List received requests error:", err)
		http.Error(w, "Failed to list requests", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	requests := []CrewRequest{}
	for rows.Next() {
		var req CrewRequest
		var fromUser CrewUserInfo
		if err := rows.Scan(
			&req.ID, &req.FromUserID, &req.ToUserID, &req.Status, &req.Message,
			&req.CreatedAt, &req.UpdatedAt,
			&fromUser.ID, &fromUser.Pseudo, &fromUser.FirstName, &fromUser.LastName, &fromUser.Email, &fromUser.AvatarUrl,
		); err != nil {
			fmt.Println("Scan request error:", err)
			continue
		}
		req.FromUser = &fromUser
		requests = append(requests, req)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(requests)
}

// ============================================================================
// GET /crew/requests/sent - List sent requests
// ============================================================================

func (h *Handler) ListSentRequests(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	query := `
		SELECT
			cr.id, cr.from_user_id, cr.to_user_id, cr.status, cr.message,
			cr.created_at, cr.updated_at,
			u.id, u.pseudo, u.first_name, u.last_name, u.email, u.avatar_url
		FROM friend_requests cr
		JOIN users u ON cr.to_user_id = u.id
		WHERE cr.from_user_id = $1
		ORDER BY cr.created_at DESC
	`

	rows, err := h.db.Query(r.Context(), query, userID)
	if err != nil {
		fmt.Println("List sent requests error:", err)
		http.Error(w, "Failed to list requests", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	requests := []CrewRequest{}
	for rows.Next() {
		var req CrewRequest
		var toUser CrewUserInfo
		if err := rows.Scan(
			&req.ID, &req.FromUserID, &req.ToUserID, &req.Status, &req.Message,
			&req.CreatedAt, &req.UpdatedAt,
			&toUser.ID, &toUser.Pseudo, &toUser.FirstName, &toUser.LastName, &toUser.Email, &toUser.AvatarUrl,
		); err != nil {
			fmt.Println("Scan request error:", err)
			continue
		}
		req.ToUser = &toUser
		requests = append(requests, req)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(requests)
}

// ============================================================================
// POST /crew/requests - Send a crew request
// ============================================================================

func (h *Handler) SendRequest(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	var req SendCrewRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	if req.ToUserID == "" {
		http.Error(w, "to_user_id is required", http.StatusBadRequest)
		return
	}

	if req.ToUserID == userID {
		http.Error(w, "Cannot send request to yourself", http.StatusBadRequest)
		return
	}

	// Check if already crew members
	checkQuery := `SELECT EXISTS(SELECT 1 FROM friendships WHERE user_id = $1 AND member_id = $2)`
	var alreadyMember bool
	if err := h.db.QueryRow(r.Context(), checkQuery, userID, req.ToUserID).Scan(&alreadyMember); err != nil {
		fmt.Println("Check crew member error:", err)
		http.Error(w, "Failed to check membership", http.StatusInternalServerError)
		return
	}
	if alreadyMember {
		http.Error(w, "Already crew members", http.StatusConflict)
		return
	}

	query := `
		INSERT INTO friend_requests (from_user_id, to_user_id, message, status)
		VALUES ($1, $2, $3, 'pending')
		RETURNING id, from_user_id, to_user_id, status, message, created_at, updated_at
	`

	var crewReq CrewRequest
	err := h.db.QueryRow(r.Context(), query, userID, req.ToUserID, req.Message).Scan(
		&crewReq.ID, &crewReq.FromUserID, &crewReq.ToUserID, &crewReq.Status,
		&crewReq.Message, &crewReq.CreatedAt, &crewReq.UpdatedAt,
	)
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" {
			http.Error(w, "Request already exists", http.StatusConflict)
			return
		}
		fmt.Println("Send request error:", err)
		http.Error(w, "Failed to send request", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(crewReq)
}

// ============================================================================
// POST /crew/requests/{id}/accept - Accept a crew request
// ============================================================================

func (h *Handler) AcceptRequest(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	requestID := chi.URLParam(r, "id")

	// Start transaction
	tx, err := h.db.Begin(r.Context())
	if err != nil {
		http.Error(w, "Failed to start transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	// Get the request and verify it's pending and belongs to current user
	var fromUserID, toUserID string
	checkQuery := `
		SELECT from_user_id, to_user_id FROM friend_requests
		WHERE id = $1 AND status = 'pending' AND to_user_id = $2
	`
	err = tx.QueryRow(r.Context(), checkQuery, requestID, userID).Scan(&fromUserID, &toUserID)
	if err != nil {
		if err == pgx.ErrNoRows {
			http.Error(w, "Request not found or already processed", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to check request", http.StatusInternalServerError)
		return
	}

	// Update request status
	updateQuery := `UPDATE friend_requests SET status = 'accepted', updated_at = NOW() WHERE id = $1`
	_, err = tx.Exec(r.Context(), updateQuery, requestID)
	if err != nil {
		http.Error(w, "Failed to update request", http.StatusInternalServerError)
		return
	}

	// Create bidirectional crew membership
	insertQuery := `
		INSERT INTO friendships (user_id, member_id) VALUES ($1, $2)
		ON CONFLICT (user_id, member_id) DO NOTHING
	`
	if _, err := tx.Exec(r.Context(), insertQuery, toUserID, fromUserID); err != nil {
		fmt.Println("Insert crew member (direction 1) error:", err)
		http.Error(w, "Failed to create crew membership", http.StatusInternalServerError)
		return
	}
	if _, err := tx.Exec(r.Context(), insertQuery, fromUserID, toUserID); err != nil {
		fmt.Println("Insert crew member (direction 2) error:", err)
		http.Error(w, "Failed to create crew membership", http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "Failed to commit transaction", http.StatusInternalServerError)
		return
	}

	// Send Telegram notification for new friendship
	go telegram.Get().Send(telegram.Event{
		Type:     telegram.EventFriendRequestAccepted,
		UserID:   userID,
		UserName: "User",
		Data: map[string]interface{}{
			"friend_name": "New friend",
		},
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// ============================================================================
// POST /crew/requests/{id}/reject - Reject a crew request
// ============================================================================

func (h *Handler) RejectRequest(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	requestID := chi.URLParam(r, "id")

	query := `
		UPDATE friend_requests SET status = 'rejected', updated_at = NOW()
		WHERE id = $1 AND status = 'pending' AND to_user_id = $2
	`
	result, err := h.db.Exec(r.Context(), query, requestID, userID)
	if err != nil {
		fmt.Println("Reject request error:", err)
		http.Error(w, "Failed to reject request", http.StatusInternalServerError)
		return
	}

	if result.RowsAffected() == 0 {
		http.Error(w, "Request not found or already processed", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// ============================================================================
// GET /crew/search?q=...&limit=20 - Search users
// ============================================================================

func (h *Handler) SearchUsers(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	searchQuery := r.URL.Query().Get("q")
	limitStr := r.URL.Query().Get("limit")

	limit := 20
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 50 {
			limit = l
		}
	}

	if searchQuery == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]SearchUserResult{})
		return
	}

	searchPattern := "%" + searchQuery + "%"

	query := `
		SELECT
			u.id,
			u.pseudo,
			u.first_name,
			u.last_name,
			u.email,
			u.avatar_url,
			COALESCE(u.day_visibility, 'crew') as day_visibility,
			COALESCE(fs.total_sessions, 0)::int as total_sessions_7d,
			COALESCE(fs.total_minutes, 0)::int as total_minutes_7d,
			(COALESCE(fs.total_minutes, 0) + (COALESCE(rc.completed_count, 0) * 10))::int as activity_score,
			EXISTS(SELECT 1 FROM friendships cm WHERE cm.user_id = $1 AND cm.member_id = u.id) as is_crew_member,
			EXISTS(
				SELECT 1 FROM friend_requests cr
				WHERE cr.status = 'pending'
				AND ((cr.from_user_id = $1 AND cr.to_user_id = u.id) OR (cr.from_user_id = u.id AND cr.to_user_id = $1))
			) as has_pending_request,
			(
				SELECT CASE
					WHEN cr.from_user_id = $1 THEN 'outgoing'
					WHEN cr.to_user_id = $1 THEN 'incoming'
					ELSE NULL
				END
				FROM friend_requests cr
				WHERE cr.status = 'pending'
				AND ((cr.from_user_id = $1 AND cr.to_user_id = u.id) OR (cr.from_user_id = u.id AND cr.to_user_id = $1))
				LIMIT 1
			) as request_direction
		FROM users u
		LEFT JOIN (
			SELECT user_id, COUNT(*)::int as total_sessions, COALESCE(SUM(duration_minutes), 0)::int as total_minutes
			FROM focus_sessions
			WHERE started_at >= NOW() - INTERVAL '7 days' AND status = 'completed'
			GROUP BY user_id
		) fs ON u.id = fs.user_id
		LEFT JOIN (
			SELECT r.user_id, COUNT(*)::int as completed_count
			FROM routine_completions c
			JOIN routines r ON c.routine_id = r.id
			WHERE c.completed_at >= NOW() - INTERVAL '7 days'
			GROUP BY r.user_id
		) rc ON u.id = rc.user_id
		WHERE u.id != $1
		AND (u.pseudo ILIKE $2 OR u.first_name ILIKE $2 OR u.last_name ILIKE $2 OR u.email ILIKE $2)
		ORDER BY activity_score DESC
		LIMIT $3
	`

	rows, err := h.db.Query(r.Context(), query, userID, searchPattern, limit)
	if err != nil {
		fmt.Println("Search users error:", err)
		http.Error(w, "Failed to search users", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	results := []SearchUserResult{}
	for rows.Next() {
		var u SearchUserResult
		if err := rows.Scan(
			&u.ID, &u.Pseudo, &u.FirstName, &u.LastName, &u.Email, &u.AvatarUrl,
			&u.DayVisibility, &u.TotalSessions7d, &u.TotalMinutes7d, &u.ActivityScore,
			&u.IsCrewMember, &u.HasPendingRequest, &u.RequestDirection,
		); err != nil {
			fmt.Println("Scan search result error:", err)
			continue
		}
		results = append(results, u)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// ============================================================================
// GET /crew/leaderboard?limit=50 - Get leaderboard
// ============================================================================

func (h *Handler) GetLeaderboard(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	limitStr := r.URL.Query().Get("limit")

	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	query := `
		WITH user_stats AS (
			SELECT
				u.id,
				u.pseudo,
				u.first_name,
				u.last_name,
				u.email,
				u.avatar_url,
				COALESCE(u.day_visibility, 'crew') as day_visibility,
				COALESCE(fs.total_sessions, 0)::int as total_sessions_7d,
				COALESCE(fs.total_minutes, 0)::int as total_minutes_7d,
				COALESCE(rc.completed_count, 0)::int as completed_routines_7d,
				(COALESCE(fs.total_minutes, 0) + (COALESCE(rc.completed_count, 0) * 10))::int as activity_score,
				GREATEST(fs.last_session, rc.last_completion) as last_active
			FROM users u
			LEFT JOIN (
				SELECT user_id, COUNT(*)::int as total_sessions, COALESCE(SUM(duration_minutes), 0)::int as total_minutes, MAX(started_at) as last_session
				FROM focus_sessions
				WHERE started_at >= NOW() - INTERVAL '7 days' AND status = 'completed'
				GROUP BY user_id
			) fs ON u.id = fs.user_id
			LEFT JOIN (
				SELECT r.user_id, COUNT(*)::int as completed_count, MAX(c.completed_at) as last_completion
				FROM routine_completions c
				JOIN routines r ON c.routine_id = r.id
				WHERE c.completed_at >= NOW() - INTERVAL '7 days'
				GROUP BY r.user_id
			) rc ON u.id = rc.user_id
			WHERE (COALESCE(fs.total_minutes, 0) + (COALESCE(rc.completed_count, 0) * 10)) > 0
		),
		live_sessions AS (
			-- Get only the most recent active session per user
			-- Only consider sessions that:
			-- 1. Have status = 'active'
			-- 2. Started within the last 4 hours
			-- 3. Haven't exceeded their planned duration by more than 30 minutes (stale session protection)
			SELECT DISTINCT ON (user_id)
				user_id,
				started_at as live_started_at,
				duration_minutes as live_duration
			FROM focus_sessions
			WHERE status = 'active'
			  AND started_at >= NOW() - INTERVAL '4 hours'
			  AND started_at + (duration_minutes + 30) * INTERVAL '1 minute' > NOW()
			ORDER BY user_id, started_at DESC
		),
		ranked_users AS (
			SELECT
				us.*,
				COALESCE((SELECT current_streak FROM user_streaks WHERE user_id = us.id), 0)::int as current_streak,
				EXISTS(SELECT 1 FROM friendships cm WHERE cm.user_id = $1 AND cm.member_id = us.id) as is_crew_member,
				EXISTS(
					SELECT 1 FROM friend_requests cr
					WHERE cr.status = 'pending'
					AND ((cr.from_user_id = $1 AND cr.to_user_id = us.id) OR (cr.from_user_id = us.id AND cr.to_user_id = $1))
				) as has_pending_request,
				(
					SELECT CASE
						WHEN cr.from_user_id = $1 THEN 'outgoing'
						WHEN cr.to_user_id = $1 THEN 'incoming'
						ELSE NULL
					END
					FROM friend_requests cr
					WHERE cr.status = 'pending'
					AND ((cr.from_user_id = $1 AND cr.to_user_id = us.id) OR (cr.from_user_id = us.id AND cr.to_user_id = $1))
					LIMIT 1
				) as request_direction,
				ls.live_started_at IS NOT NULL as is_live,
				ls.live_started_at,
				ls.live_duration
			FROM user_stats us
			LEFT JOIN live_sessions ls ON us.id = ls.user_id
		)
		SELECT
			ROW_NUMBER() OVER (ORDER BY is_live DESC, activity_score DESC, total_minutes_7d DESC)::bigint as rank,
			id,
			pseudo,
			first_name,
			last_name,
			email,
			avatar_url,
			day_visibility,
			total_sessions_7d,
			total_minutes_7d,
			completed_routines_7d,
			activity_score,
			current_streak,
			last_active,
			is_crew_member,
			has_pending_request,
			request_direction,
			is_live,
			live_started_at,
			live_duration
		FROM ranked_users
		ORDER BY is_live DESC, activity_score DESC, total_minutes_7d DESC
		LIMIT $2
	`

	rows, err := h.db.Query(r.Context(), query, userID, limit)
	if err != nil {
		fmt.Println("Get leaderboard error:", err)
		http.Error(w, "Failed to get leaderboard", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	entries := []LeaderboardEntry{}
	for rows.Next() {
		var e LeaderboardEntry
		var rank int64
		var lastActive *time.Time
		var liveStartedAt *time.Time
		var liveDuration *int
		if err := rows.Scan(
			&rank, &e.ID, &e.Pseudo, &e.FirstName, &e.LastName, &e.Email, &e.AvatarUrl,
			&e.DayVisibility, &e.TotalSessions7d, &e.TotalMinutes7d, &e.CompletedRoutines7d,
			&e.ActivityScore, &e.CurrentStreak, &lastActive, &e.IsCrewMember, &e.HasPendingRequest, &e.RequestDirection,
			&e.IsLive, &liveStartedAt, &liveDuration,
		); err != nil {
			fmt.Println("Scan leaderboard entry error:", err)
			continue
		}
		e.Rank = int(rank)
		e.IsSelf = (e.ID == userID)
		if lastActive != nil {
			formatted := lastActive.Format(time.RFC3339)
			e.LastActive = &formatted
		}
		// Set live session fields
		if liveStartedAt != nil {
			formatted := liveStartedAt.Format(time.RFC3339)
			e.LiveSessionStartedAt = &formatted
		}
		if liveDuration != nil {
			e.LiveSessionDuration = liveDuration
		}
		entries = append(entries, e)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

// ============================================================================
// GET /crew/members/{id}/day?date=YYYY-MM-DD - Get a crew member's day
// ============================================================================

func (h *Handler) GetMemberDay(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	memberID := chi.URLParam(r, "id")
	dateStr := r.URL.Query().Get("date")

	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}

	// Check member's visibility
	var visibility string
	visQuery := `SELECT COALESCE(day_visibility, 'crew') FROM users WHERE id = $1`
	err := h.db.QueryRow(r.Context(), visQuery, memberID).Scan(&visibility)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// Check if current user is in their crew (friends)
	var isCrewMember bool
	crewQuery := `SELECT EXISTS(SELECT 1 FROM friendships WHERE user_id = $1 AND member_id = $2)`
	if err := h.db.QueryRow(r.Context(), crewQuery, userID, memberID).Scan(&isCrewMember); err != nil {
		fmt.Println("Check crew member error:", err)
		isCrewMember = false
	}

	// Check if users share a group (either as owner or member)
	var sharesGroup bool
	groupQuery := `
		SELECT EXISTS(
			-- Both are in same group as members
			SELECT 1 FROM friend_group_members gm1
			JOIN friend_group_members gm2 ON gm1.group_id = gm2.group_id
			WHERE gm1.member_id = $1 AND gm2.member_id = $2
			UNION
			-- User is member, target is owner
			SELECT 1 FROM friend_group_members gm
			JOIN friend_groups g ON gm.group_id = g.id
			WHERE gm.member_id = $1 AND g.user_id = $2
			UNION
			-- User is owner, target is member
			SELECT 1 FROM friend_groups g
			JOIN friend_group_members gm ON g.id = gm.group_id
			WHERE g.user_id = $1 AND gm.member_id = $2
			UNION
			-- Both are owners of same group (edge case)
			SELECT 1 FROM friend_groups g1
			JOIN friend_groups g2 ON g1.id = g2.id
			WHERE g1.user_id = $1 AND g2.user_id = $2
		)
	`
	if err := h.db.QueryRow(r.Context(), groupQuery, userID, memberID).Scan(&sharesGroup); err != nil {
		fmt.Println("Check group membership error:", err)
		sharesGroup = false
	}

	// Check permission
	if visibility == "private" {
		http.Error(w, "Day is private", http.StatusForbidden)
		return
	}
	if visibility == "crew" && !isCrewMember && !sharesGroup {
		http.Error(w, "Not a crew member or group member", http.StatusForbidden)
		return
	}

	// Get user info
	var user CrewUserInfo
	userQuery := `SELECT id, pseudo, first_name, last_name, email, avatar_url FROM users WHERE id = $1`
	if err := h.db.QueryRow(r.Context(), userQuery, memberID).Scan(
		&user.ID, &user.Pseudo, &user.FirstName, &user.LastName, &user.Email, &user.AvatarUrl,
	); err != nil {
		fmt.Println("Get user info error:", err)
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// Get intentions
	intentionsQuery := `
		SELECT i.id, i.content, i.position
		FROM intentions i
		JOIN daily_intentions di ON i.daily_intention_id = di.id
		WHERE di.user_id = $1 AND di.date = $2
		ORDER BY i.position
	`
	intentionRows, err := h.db.Query(r.Context(), intentionsQuery, memberID, dateStr)
	if err != nil {
		fmt.Println("Get intentions error:", err)
		// Continue with empty intentions
		intentionRows = nil
	}
	if intentionRows != nil {
		defer intentionRows.Close()
	}

	intentions := []CrewIntention{}
	if intentionRows != nil {
		for intentionRows.Next() {
			var i CrewIntention
			if err := intentionRows.Scan(&i.ID, &i.Content, &i.Position); err != nil {
				fmt.Println("Scan intention error:", err)
				continue
			}
			intentions = append(intentions, i)
		}
	}

	// Get focus sessions
	sessionsQuery := `
		SELECT id, description, duration_minutes, started_at, completed_at, status
		FROM focus_sessions
		WHERE user_id = $1 AND DATE(started_at) = $2 AND status = 'completed'
		ORDER BY started_at DESC
	`
	sessionRows, err := h.db.Query(r.Context(), sessionsQuery, memberID, dateStr)
	if err != nil {
		fmt.Println("Get sessions error:", err)
		sessionRows = nil
	}
	if sessionRows != nil {
		defer sessionRows.Close()
	}

	sessions := []CrewFocusSession{}
	if sessionRows != nil {
		for sessionRows.Next() {
			var s CrewFocusSession
			if err := sessionRows.Scan(&s.ID, &s.Description, &s.DurationMinutes, &s.StartedAt, &s.CompletedAt, &s.Status); err != nil {
				fmt.Println("Scan session error:", err)
				continue
			}
			sessions = append(sessions, s)
		}
	}

	// Get completed routines with like counts
	completedRoutinesQuery := `
		SELECT
			c.id,
			r.title,
			r.icon,
			c.completed_at,
			COALESCE((SELECT COUNT(*) FROM routine_likes rl WHERE rl.completion_id = c.id), 0)::int as like_count,
			EXISTS(SELECT 1 FROM routine_likes rl WHERE rl.completion_id = c.id AND rl.user_id = $3) as is_liked_by_me
		FROM routine_completions c
		JOIN routines r ON c.routine_id = r.id
		WHERE r.user_id = $1 AND DATE(c.completed_at) = $2
		ORDER BY c.completed_at
	`
	completedRoutineRows, err := h.db.Query(r.Context(), completedRoutinesQuery, memberID, dateStr, userID)
	if err != nil {
		fmt.Println("Get completed routines error:", err)
		completedRoutineRows = nil
	}
	if completedRoutineRows != nil {
		defer completedRoutineRows.Close()
	}

	completedRoutines := []CrewCompletedRoutine{}
	if completedRoutineRows != nil {
		for completedRoutineRows.Next() {
			var cr CrewCompletedRoutine
			if err := completedRoutineRows.Scan(&cr.ID, &cr.Title, &cr.Icon, &cr.CompletedAt, &cr.LikeCount, &cr.IsLikedByMe); err != nil {
				fmt.Println("Scan completed routine error:", err)
				continue
			}
			completedRoutines = append(completedRoutines, cr)
		}
	}

	// Get ALL routines with completion status and like counts for this day
	allRoutinesQuery := `
		SELECT
			COALESCE(c.id, r.id) as id,
			r.title,
			r.icon,
			CASE WHEN c.id IS NOT NULL THEN true ELSE false END as completed,
			c.completed_at,
			COALESCE((SELECT COUNT(*) FROM routine_likes rl WHERE rl.completion_id = c.id), 0)::int as like_count,
			COALESCE(EXISTS(SELECT 1 FROM routine_likes rl WHERE rl.completion_id = c.id AND rl.user_id = $3), false) as is_liked_by_me
		FROM routines r
		LEFT JOIN routine_completions c ON r.id = c.routine_id AND DATE(c.completed_at) = $2
		WHERE r.user_id = $1
		ORDER BY completed DESC, r.title ASC
	`
	allRoutineRows, err := h.db.Query(r.Context(), allRoutinesQuery, memberID, dateStr, userID)
	if err != nil {
		fmt.Println("Get all routines error:", err)
	}
	defer allRoutineRows.Close()

	allRoutines := []CrewRoutine{}
	for allRoutineRows.Next() {
		var cr CrewRoutine
		if err := allRoutineRows.Scan(&cr.ID, &cr.Title, &cr.Icon, &cr.Completed, &cr.CompletedAt, &cr.LikeCount, &cr.IsLikedByMe); err != nil {
			fmt.Println("Scan routine error:", err)
			continue
		}
		allRoutines = append(allRoutines, cr)
	}
	fmt.Printf("Found %d routines for user %s on %s\n", len(allRoutines), memberID, dateStr)

	// Get calendar tasks for this day
	tasksQuery := `
		SELECT
			t.id,
			t.title,
			t.description,
			t.scheduled_start,
			t.scheduled_end,
			COALESCE(t.time_block, 'morning') as time_block,
			COALESCE(t.priority, 'medium') as priority,
			COALESCE(t.status, 'pending') as status,
			a.name as area_name,
			a.icon as area_icon,
			COALESCE(t.is_private, false) as is_private
		FROM tasks t
		LEFT JOIN areas a ON t.area_id = a.id
		WHERE t.user_id = $1 AND t.date = $2
		ORDER BY t.scheduled_start ASC NULLS LAST, t.position ASC
	`
	taskRows, err := h.db.Query(r.Context(), tasksQuery, memberID, dateStr)
	if err != nil {
		fmt.Println("Get tasks error:", err)
		taskRows = nil
	}

	tasks := []CrewTask{}
	if taskRows != nil {
		defer taskRows.Close()
		for taskRows.Next() {
			var t CrewTask
			if err := taskRows.Scan(&t.ID, &t.Title, &t.Description, &t.ScheduledStart, &t.ScheduledEnd, &t.TimeBlock, &t.Priority, &t.Status, &t.AreaName, &t.AreaIcon, &t.IsPrivate); err != nil {
				fmt.Println("Scan task error:", err)
				continue
			}
			// Mask private task content - only show that something is scheduled
			if t.IsPrivate {
				t.Title = "🔒 Tâche privée"
				t.Description = nil
				t.AreaName = nil
				t.AreaIcon = nil
			}
			tasks = append(tasks, t)
		}
	}
	fmt.Printf("Found %d tasks for user %s on %s\n", len(tasks), memberID, dateStr)

	// Get stats for the member
	stats := h.getMemberStats(r.Context(), memberID)

	day := CrewMemberDay{
		User:              &user,
		Intentions:        intentions,
		FocusSessions:     sessions,
		CompletedRoutines: completedRoutines,
		Routines:          allRoutines,
		Tasks:             tasks,
		Stats:             stats,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(day)
}

// getMemberStats calculates weekly and monthly stats for a member
func (h *Handler) getMemberStats(ctx context.Context, memberID string) *CrewMemberStats {
	stats := &CrewMemberStats{
		WeeklyFocusMinutes:  make([]DailyStat, 0),
		WeeklyRoutinesDone:  make([]DailyStat, 0),
		MonthlyFocusMinutes: make([]DailyStat, 0),
		MonthlyRoutinesDone: make([]DailyStat, 0),
	}

	// Weekly focus minutes (last 7 days)
	weeklyFocusQuery := `
		SELECT DATE(started_at) as date, COALESCE(SUM(duration_minutes), 0)::int as minutes
		FROM focus_sessions
		WHERE user_id = $1 AND started_at >= NOW() - INTERVAL '7 days' AND status = 'completed'
		GROUP BY DATE(started_at)
		ORDER BY date
	`
	rows, err := h.db.Query(ctx, weeklyFocusQuery, memberID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var date time.Time
			var minutes int
			rows.Scan(&date, &minutes)
			stats.WeeklyFocusMinutes = append(stats.WeeklyFocusMinutes, DailyStat{
				Date:  date.Format("2006-01-02"),
				Value: minutes,
			})
			stats.WeeklyTotalFocus += minutes
		}
	}
	if len(stats.WeeklyFocusMinutes) > 0 {
		stats.WeeklyAvgFocus = stats.WeeklyTotalFocus / 7
	}

	// Weekly routines done (last 7 days)
	weeklyRoutinesQuery := `
		SELECT DATE(c.completed_at) as date, COUNT(*)::int as count
		FROM routine_completions c
		JOIN routines r ON c.routine_id = r.id
		WHERE r.user_id = $1 AND c.completed_at >= NOW() - INTERVAL '7 days'
		GROUP BY DATE(c.completed_at)
		ORDER BY date
	`
	rows2, err := h.db.Query(ctx, weeklyRoutinesQuery, memberID)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var date time.Time
			var count int
			rows2.Scan(&date, &count)
			stats.WeeklyRoutinesDone = append(stats.WeeklyRoutinesDone, DailyStat{
				Date:  date.Format("2006-01-02"),
				Value: count,
			})
			stats.WeeklyTotalRoutines += count
		}
	}

	// Calculate routine completion rate (routines done / total possible)
	var totalRoutines int
	countQuery := `SELECT COUNT(*) FROM routines WHERE user_id = $1`
	h.db.QueryRow(ctx, countQuery, memberID).Scan(&totalRoutines)
	if totalRoutines > 0 {
		// Rate = (routines done in 7 days) / (total routines * 7 days) * 100
		possibleRoutines := totalRoutines * 7
		if possibleRoutines > 0 {
			stats.WeeklyRoutineRate = (stats.WeeklyTotalRoutines * 100) / possibleRoutines
		}
	}

	// Monthly focus minutes (last 30 days)
	monthlyFocusQuery := `
		SELECT DATE(started_at) as date, COALESCE(SUM(duration_minutes), 0)::int as minutes
		FROM focus_sessions
		WHERE user_id = $1 AND started_at >= NOW() - INTERVAL '30 days' AND status = 'completed'
		GROUP BY DATE(started_at)
		ORDER BY date
	`
	rows3, err := h.db.Query(ctx, monthlyFocusQuery, memberID)
	if err == nil {
		defer rows3.Close()
		for rows3.Next() {
			var date time.Time
			var minutes int
			rows3.Scan(&date, &minutes)
			stats.MonthlyFocusMinutes = append(stats.MonthlyFocusMinutes, DailyStat{
				Date:  date.Format("2006-01-02"),
				Value: minutes,
			})
			stats.MonthlyTotalFocus += minutes
		}
	}

	// Monthly routines done (last 30 days)
	monthlyRoutinesQuery := `
		SELECT DATE(c.completed_at) as date, COUNT(*)::int as count
		FROM routine_completions c
		JOIN routines r ON c.routine_id = r.id
		WHERE r.user_id = $1 AND c.completed_at >= NOW() - INTERVAL '30 days'
		GROUP BY DATE(c.completed_at)
		ORDER BY date
	`
	rows4, err := h.db.Query(ctx, monthlyRoutinesQuery, memberID)
	if err == nil {
		defer rows4.Close()
		for rows4.Next() {
			var date time.Time
			var count int
			rows4.Scan(&date, &count)
			stats.MonthlyRoutinesDone = append(stats.MonthlyRoutinesDone, DailyStat{
				Date:  date.Format("2006-01-02"),
				Value: count,
			})
			stats.MonthlyTotalRoutines += count
		}
	}

	return stats
}

// ============================================================================
// GET /me/stats - Get my own stats
// ============================================================================

func (h *Handler) GetMyStats(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	stats := h.getMemberStats(r.Context(), userID)

	// Also get total routines count for context
	var totalRoutines int
	countQuery := `SELECT COUNT(*) FROM routines WHERE user_id = $1`
	h.db.QueryRow(r.Context(), countQuery, userID).Scan(&totalRoutines)

	response := struct {
		*CrewMemberStats
		TotalRoutines int `json:"total_routines"`
	}{
		CrewMemberStats: stats,
		TotalRoutines:   totalRoutines,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ============================================================================
// PATCH /me/visibility - Update day visibility
// ============================================================================

func (h *Handler) UpdateVisibility(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	var req UpdateVisibilityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	// Validate visibility value
	if req.DayVisibility != "public" && req.DayVisibility != "crew" && req.DayVisibility != "private" {
		http.Error(w, "Invalid day_visibility value. Must be: public, crew, or private", http.StatusBadRequest)
		return
	}

	query := `UPDATE users SET day_visibility = $1 WHERE id = $2`
	_, err := h.db.Exec(r.Context(), query, req.DayVisibility, userID)
	if err != nil {
		fmt.Println("Update visibility error:", err)
		http.Error(w, "Failed to update visibility", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"day_visibility": req.DayVisibility})
}

// ============================================================================
// POST /completions/{id}/like - Like a routine completion
// ============================================================================

func (h *Handler) LikeCompletion(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	completionID := chi.URLParam(r, "id")

	query := `
		INSERT INTO routine_likes (user_id, completion_id)
		VALUES ($1, $2)
		ON CONFLICT (user_id, completion_id) DO NOTHING
	`
	_, err := h.db.Exec(r.Context(), query, userID, completionID)
	if err != nil {
		fmt.Println("Like completion error:", err)
		http.Error(w, "Failed to like completion", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// ============================================================================
// DELETE /completions/{id}/like - Unlike a routine completion
// ============================================================================

func (h *Handler) UnlikeCompletion(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	completionID := chi.URLParam(r, "id")

	query := `DELETE FROM routine_likes WHERE user_id = $1 AND completion_id = $2`
	_, err := h.db.Exec(r.Context(), query, userID, completionID)
	if err != nil {
		fmt.Println("Unlike completion error:", err)
		http.Error(w, "Failed to unlike completion", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// ============================================================================
// GET /crew/suggestions?limit=10 - Get suggested users to add to crew
// ============================================================================

func (h *Handler) GetSuggestedUsers(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	limitStr := r.URL.Query().Get("limit")

	limit := 10
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 50 {
			limit = l
		}
	}

	// Get users who are active but not in crew and no pending requests
	query := `
		WITH user_activity AS (
			SELECT
				u.id,
				u.pseudo,
				u.first_name,
				u.last_name,
				u.email,
				u.avatar_url,
				COALESCE(u.day_visibility, 'crew') as day_visibility,
				COALESCE(fs.total_sessions, 0)::int as total_sessions_7d,
				COALESCE(fs.total_minutes, 0)::int as total_minutes_7d,
				(COALESCE(fs.total_minutes, 0) + (COALESCE(rc.completed_count, 0) * 10))::int as activity_score
			FROM users u
			LEFT JOIN (
				SELECT user_id, COUNT(*)::int as total_sessions, COALESCE(SUM(duration_minutes), 0)::int as total_minutes
				FROM focus_sessions
				WHERE started_at >= NOW() - INTERVAL '7 days' AND status = 'completed'
				GROUP BY user_id
			) fs ON u.id = fs.user_id
			LEFT JOIN (
				SELECT r.user_id, COUNT(*)::int as completed_count
				FROM routine_completions c
				JOIN routines r ON c.routine_id = r.id
				WHERE c.completed_at >= NOW() - INTERVAL '7 days'
				GROUP BY r.user_id
			) rc ON u.id = rc.user_id
			WHERE u.id != $1
			AND NOT EXISTS (SELECT 1 FROM friendships cm WHERE cm.user_id = $1 AND cm.member_id = u.id)
			AND NOT EXISTS (
				SELECT 1 FROM friend_requests cr
				WHERE cr.status = 'pending'
				AND ((cr.from_user_id = $1 AND cr.to_user_id = u.id) OR (cr.from_user_id = u.id AND cr.to_user_id = $1))
			)
			AND (COALESCE(fs.total_minutes, 0) + (COALESCE(rc.completed_count, 0) * 10)) > 0
		)
		SELECT
			id, pseudo, first_name, last_name, email, avatar_url, day_visibility,
			total_sessions_7d, total_minutes_7d, activity_score,
			false as is_crew_member,
			false as has_pending_request,
			NULL::text as request_direction
		FROM user_activity
		ORDER BY activity_score DESC
		LIMIT $2
	`

	rows, err := h.db.Query(r.Context(), query, userID, limit)
	if err != nil {
		fmt.Println("Get suggested users error:", err)
		http.Error(w, "Failed to get suggestions", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	results := []SearchUserResult{}
	for rows.Next() {
		var u SearchUserResult
		if err := rows.Scan(
			&u.ID, &u.Pseudo, &u.FirstName, &u.LastName, &u.Email, &u.AvatarUrl,
			&u.DayVisibility, &u.TotalSessions7d, &u.TotalMinutes7d, &u.ActivityScore,
			&u.IsCrewMember, &u.HasPendingRequest, &u.RequestDirection,
		); err != nil {
			fmt.Println("Scan suggestion error:", err)
			continue
		}
		results = append(results, u)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// ============================================================================
// CREW GROUPS - Custom friend grouping (Crews)
// ============================================================================

// Group models
type CrewGroup struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description *string           `json:"description"`
	Icon        string            `json:"icon"`
	Color       string            `json:"color"`
	MemberCount int               `json:"member_count"`
	Members     []CrewGroupMember `json:"members,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

type CrewGroupMember struct {
	ID        string  `json:"id"`
	MemberID  string  `json:"member_id"`
	Pseudo    *string `json:"pseudo"`
	FirstName *string `json:"first_name"`
	LastName  *string `json:"last_name"`
	Email     *string `json:"email"`
	AvatarUrl *string `json:"avatar_url"`
	AddedAt   string  `json:"added_at"`
	IsOwner   bool    `json:"is_owner"`
}

type CreateGroupDTO struct {
	Name        string   `json:"name"`
	Description *string  `json:"description"`
	Icon        *string  `json:"icon"`
	Color       *string  `json:"color"`
	MemberIDs   []string `json:"member_ids"`
}

type UpdateGroupDTO struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Icon        *string `json:"icon"`
	Color       *string `json:"color"`
}

type AddGroupMembersDTO struct {
	MemberIDs []string `json:"member_ids"`
}

// ============================================================================
// GET /crew/groups - List user's groups (crews)
// ============================================================================

func (h *Handler) ListGroups(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	// Get groups where user is owner OR member
	query := `
		SELECT DISTINCT
			g.id,
			g.name,
			g.description,
			g.icon,
			g.color,
			g.created_at,
			g.updated_at,
			COALESCE(mc.member_count, 0) + 1 as member_count,
			(g.user_id = $1) as is_owner
		FROM friend_groups g
		LEFT JOIN (
			SELECT group_id, COUNT(*)::int as member_count
			FROM friend_group_members
			GROUP BY group_id
		) mc ON g.id = mc.group_id
		WHERE g.user_id = $1
		   OR g.id IN (SELECT group_id FROM friend_group_members WHERE member_id = $1)
		ORDER BY g.name
	`

	rows, err := h.db.Query(r.Context(), query, userID)
	if err != nil {
		http.Error(w, "Failed to list groups", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	groups := []CrewGroup{}
	for rows.Next() {
		var g CrewGroup
		var isOwner bool
		if err := rows.Scan(&g.ID, &g.Name, &g.Description, &g.Icon, &g.Color, &g.CreatedAt, &g.UpdatedAt, &g.MemberCount, &isOwner); err != nil {
			continue
		}
		groups = append(groups, g)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(groups)
}

// ============================================================================
// POST /crew/groups - Create a new group (crew)
// ============================================================================

func (h *Handler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	var dto CreateGroupDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if dto.Name == "" {
		http.Error(w, "Group name is required", http.StatusBadRequest)
		return
	}

	// Default values
	icon := "👥"
	color := "#6366F1"
	if dto.Icon != nil && *dto.Icon != "" {
		icon = *dto.Icon
	}
	if dto.Color != nil && *dto.Color != "" {
		color = *dto.Color
	}

	// Start transaction
	ctx := r.Context()
	tx, err := h.db.Begin(ctx)
	if err != nil {
		http.Error(w, "Failed to start transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(ctx)

	// Create group
	var group CrewGroup
	err = tx.QueryRow(ctx, `
		INSERT INTO friend_groups (user_id, name, description, icon, color)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, name, description, icon, color, created_at, updated_at
	`, userID, dto.Name, dto.Description, icon, color).Scan(
		&group.ID, &group.Name, &group.Description, &group.Icon, &group.Color, &group.CreatedAt, &group.UpdatedAt,
	)
	if err != nil {
		http.Error(w, "Failed to create group", http.StatusInternalServerError)
		return
	}

	// Add members if provided (must be existing friends/friendships)
	if len(dto.MemberIDs) > 0 {
		for _, memberID := range dto.MemberIDs {
			// Verify member is in user's friends (friendships)
			var exists bool
			err = tx.QueryRow(ctx, `
				SELECT EXISTS(SELECT 1 FROM friendships WHERE user_id = $1 AND member_id = $2)
			`, userID, memberID).Scan(&exists)
			if err != nil || !exists {
				continue // Skip non-friends
			}

			_, err = tx.Exec(ctx, `
				INSERT INTO friend_group_members (group_id, member_id)
				VALUES ($1, $2)
				ON CONFLICT (group_id, member_id) DO NOTHING
			`, group.ID, memberID)
			if err != nil {
				continue
			}
		}
	}

	// Commit transaction
	if err = tx.Commit(ctx); err != nil {
		http.Error(w, "Failed to commit transaction", http.StatusInternalServerError)
		return
	}

	// Get member count
	h.db.QueryRow(ctx, `SELECT COUNT(*) FROM friend_group_members WHERE group_id = $1`, group.ID).Scan(&group.MemberCount)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(group)
}

// ============================================================================
// GET /crew/groups/{id} - Get group with members
// ============================================================================

func (h *Handler) GetGroup(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	groupID := chi.URLParam(r, "id")

	// Verify user is owner OR member of the group
	var hasAccess bool
	err := h.db.QueryRow(r.Context(), `
		SELECT EXISTS(
			SELECT 1 FROM friend_groups WHERE id = $1 AND user_id = $2
			UNION
			SELECT 1 FROM friend_group_members WHERE group_id = $1 AND member_id = $2
		)
	`, groupID, userID).Scan(&hasAccess)
	if err != nil || !hasAccess {
		http.Error(w, "Group not found", http.StatusNotFound)
		return
	}

	// Get group details with owner info
	var group CrewGroup
	var ownerID string
	var ownerPseudo, ownerFirstName, ownerLastName, ownerEmail, ownerAvatarUrl *string
	var groupCreatedAt time.Time
	err = h.db.QueryRow(r.Context(), `
		SELECT g.id, g.name, g.description, g.icon, g.color, g.created_at, g.updated_at,
		       g.user_id, u.pseudo, u.first_name, u.last_name, u.email, u.avatar_url
		FROM friend_groups g
		JOIN users u ON g.user_id = u.id
		WHERE g.id = $1
	`, groupID).Scan(&group.ID, &group.Name, &group.Description, &group.Icon, &group.Color, &group.CreatedAt, &group.UpdatedAt,
		&ownerID, &ownerPseudo, &ownerFirstName, &ownerLastName, &ownerEmail, &ownerAvatarUrl)
	if err != nil {
		if err == pgx.ErrNoRows {
			http.Error(w, "Group not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to get group", http.StatusInternalServerError)
		return
	}
	groupCreatedAt = group.CreatedAt

	// Start with owner as first member
	members := []CrewGroupMember{
		{
			ID:        ownerID, // Use owner's user ID as member ID
			MemberID:  ownerID,
			Pseudo:    ownerPseudo,
			FirstName: ownerFirstName,
			LastName:  ownerLastName,
			Email:     ownerEmail,
			AvatarUrl: ownerAvatarUrl,
			AddedAt:   groupCreatedAt.Format(time.RFC3339),
			IsOwner:   true,
		},
	}

	// Get other members
	rows, err := h.db.Query(r.Context(), `
		SELECT
			gm.id,
			gm.member_id,
			u.pseudo,
			u.first_name,
			u.last_name,
			u.email,
			u.avatar_url,
			gm.added_at
		FROM friend_group_members gm
		JOIN users u ON gm.member_id = u.id
		WHERE gm.group_id = $1
		ORDER BY u.pseudo, u.first_name
	`, groupID)
	if err != nil {
		http.Error(w, "Failed to get members", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var m CrewGroupMember
		var addedAt time.Time
		if err := rows.Scan(&m.ID, &m.MemberID, &m.Pseudo, &m.FirstName, &m.LastName, &m.Email, &m.AvatarUrl, &addedAt); err != nil {
			continue
		}
		m.AddedAt = addedAt.Format(time.RFC3339)
		m.IsOwner = false
		members = append(members, m)
	}

	group.Members = members
	group.MemberCount = len(members)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(group)
}

// ============================================================================
// PATCH /crew/groups/{id} - Update group
// ============================================================================

func (h *Handler) UpdateGroup(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	groupID := chi.URLParam(r, "id")

	var dto UpdateGroupDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Build dynamic update query
	updates := []string{}
	args := []interface{}{}
	argIndex := 1

	if dto.Name != nil {
		updates = append(updates, fmt.Sprintf("name = $%d", argIndex))
		args = append(args, *dto.Name)
		argIndex++
	}
	if dto.Description != nil {
		updates = append(updates, fmt.Sprintf("description = $%d", argIndex))
		args = append(args, *dto.Description)
		argIndex++
	}
	if dto.Icon != nil {
		updates = append(updates, fmt.Sprintf("icon = $%d", argIndex))
		args = append(args, *dto.Icon)
		argIndex++
	}
	if dto.Color != nil {
		updates = append(updates, fmt.Sprintf("color = $%d", argIndex))
		args = append(args, *dto.Color)
		argIndex++
	}

	if len(updates) == 0 {
		http.Error(w, "No fields to update", http.StatusBadRequest)
		return
	}

	updates = append(updates, "updated_at = NOW()")

	query := fmt.Sprintf(`
		UPDATE friend_groups
		SET %s
		WHERE id = $%d AND user_id = $%d
		RETURNING id, name, description, icon, color, created_at, updated_at
	`, joinStrings(updates, ", "), argIndex, argIndex+1)

	args = append(args, groupID, userID)

	var group CrewGroup
	err := h.db.QueryRow(r.Context(), query, args...).Scan(
		&group.ID, &group.Name, &group.Description, &group.Icon, &group.Color, &group.CreatedAt, &group.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			http.Error(w, "Group not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to update group", http.StatusInternalServerError)
		return
	}

	// Get member count
	h.db.QueryRow(r.Context(), `SELECT COUNT(*) FROM friend_group_members WHERE group_id = $1`, groupID).Scan(&group.MemberCount)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(group)
}

// ============================================================================
// DELETE /crew/groups/{id} - Delete group
// ============================================================================

func (h *Handler) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	groupID := chi.URLParam(r, "id")

	result, err := h.db.Exec(r.Context(), `
		DELETE FROM friend_groups
		WHERE id = $1 AND user_id = $2
	`, groupID, userID)
	if err != nil {
		http.Error(w, "Failed to delete group", http.StatusInternalServerError)
		return
	}

	if result.RowsAffected() == 0 {
		http.Error(w, "Group not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ============================================================================
// POST /crew/groups/{id}/members - Add members to group
// ============================================================================

func (h *Handler) AddGroupMembers(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	groupID := chi.URLParam(r, "id")

	var dto AddGroupMembersDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if len(dto.MemberIDs) == 0 {
		http.Error(w, "No members specified", http.StatusBadRequest)
		return
	}

	// Verify group ownership
	var exists bool
	err := h.db.QueryRow(r.Context(), `
		SELECT EXISTS(SELECT 1 FROM friend_groups WHERE id = $1 AND user_id = $2)
	`, groupID, userID).Scan(&exists)
	if err != nil || !exists {
		http.Error(w, "Group not found", http.StatusNotFound)
		return
	}

	added := 0
	for _, memberID := range dto.MemberIDs {
		// Verify member is in user's friends (friendships)
		var isFriend bool
		err = h.db.QueryRow(r.Context(), `
			SELECT EXISTS(SELECT 1 FROM friendships WHERE user_id = $1 AND member_id = $2)
		`, userID, memberID).Scan(&isFriend)
		if err != nil || !isFriend {
			continue
		}

		_, err = h.db.Exec(r.Context(), `
			INSERT INTO friend_group_members (group_id, member_id)
			VALUES ($1, $2)
			ON CONFLICT (group_id, member_id) DO NOTHING
		`, groupID, memberID)
		if err == nil {
			added++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"added": added})
}

// ============================================================================
// DELETE /crew/groups/{id}/members/{memberId} - Remove member from group
// ============================================================================

func (h *Handler) RemoveGroupMember(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	groupID := chi.URLParam(r, "id")
	memberID := chi.URLParam(r, "memberId")

	// Verify group ownership
	var exists bool
	err := h.db.QueryRow(r.Context(), `
		SELECT EXISTS(SELECT 1 FROM friend_groups WHERE id = $1 AND user_id = $2)
	`, groupID, userID).Scan(&exists)
	if err != nil || !exists {
		http.Error(w, "Group not found", http.StatusNotFound)
		return
	}

	result, err := h.db.Exec(r.Context(), `
		DELETE FROM friend_group_members
		WHERE group_id = $1 AND member_id = $2
	`, groupID, memberID)
	if err != nil {
		http.Error(w, "Failed to remove member", http.StatusInternalServerError)
		return
	}

	if result.RowsAffected() == 0 {
		http.Error(w, "Member not in group", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Helper function for joining strings
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}

// ============================================================================
// GROUP INVITATIONS - Invite members to join shared groups
// ============================================================================

type GroupInvitation struct {
	ID         string         `json:"id"`
	GroupID    string         `json:"group_id"`
	FromUserID string         `json:"from_user_id"`
	ToUserID   string         `json:"to_user_id"`
	Status     string         `json:"status"`
	Message    *string        `json:"message"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  *time.Time     `json:"updated_at"`
	FromUser   *CrewUserInfo  `json:"from_user,omitempty"`
	ToUser     *CrewUserInfo  `json:"to_user,omitempty"`
	Group      *GroupInfoBrief `json:"group,omitempty"`
}

type GroupInfoBrief struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Icon  string  `json:"icon"`
	Color string  `json:"color"`
}

type InviteToGroupDTO struct {
	UserIDs []string `json:"user_ids"`
	Message *string  `json:"message"`
}

// ============================================================================
// GET /group-invitations/received - List received group invitations
// ============================================================================

func (h *Handler) ListReceivedGroupInvitations(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	query := `
		SELECT
			gi.id, gi.group_id, gi.from_user_id, gi.to_user_id, gi.status, gi.message,
			gi.created_at, gi.updated_at,
			u.id, u.pseudo, u.first_name, u.last_name, u.email, u.avatar_url,
			g.id, g.name, g.icon, g.color
		FROM group_invitations gi
		JOIN users u ON gi.from_user_id = u.id
		JOIN friend_groups g ON gi.group_id = g.id
		WHERE gi.to_user_id = $1 AND gi.status = 'pending'
		ORDER BY gi.created_at DESC
	`

	rows, err := h.db.Query(r.Context(), query, userID)
	if err != nil {
		fmt.Println("List received group invitations error:", err)
		http.Error(w, "Failed to list invitations", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	invitations := []GroupInvitation{}
	for rows.Next() {
		var inv GroupInvitation
		var fromUser CrewUserInfo
		var group GroupInfoBrief
		if err := rows.Scan(
			&inv.ID, &inv.GroupID, &inv.FromUserID, &inv.ToUserID, &inv.Status, &inv.Message,
			&inv.CreatedAt, &inv.UpdatedAt,
			&fromUser.ID, &fromUser.Pseudo, &fromUser.FirstName, &fromUser.LastName, &fromUser.Email, &fromUser.AvatarUrl,
			&group.ID, &group.Name, &group.Icon, &group.Color,
		); err != nil {
			fmt.Println("Scan group invitation error:", err)
			continue
		}
		inv.FromUser = &fromUser
		inv.Group = &group
		invitations = append(invitations, inv)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(invitations)
}

// ============================================================================
// GET /group-invitations/sent - List sent group invitations
// ============================================================================

func (h *Handler) ListSentGroupInvitations(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	query := `
		SELECT
			gi.id, gi.group_id, gi.from_user_id, gi.to_user_id, gi.status, gi.message,
			gi.created_at, gi.updated_at,
			u.id, u.pseudo, u.first_name, u.last_name, u.email, u.avatar_url,
			g.id, g.name, g.icon, g.color
		FROM group_invitations gi
		JOIN users u ON gi.to_user_id = u.id
		JOIN friend_groups g ON gi.group_id = g.id
		WHERE gi.from_user_id = $1
		ORDER BY gi.created_at DESC
	`

	rows, err := h.db.Query(r.Context(), query, userID)
	if err != nil {
		fmt.Println("List sent group invitations error:", err)
		http.Error(w, "Failed to list invitations", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	invitations := []GroupInvitation{}
	for rows.Next() {
		var inv GroupInvitation
		var toUser CrewUserInfo
		var group GroupInfoBrief
		if err := rows.Scan(
			&inv.ID, &inv.GroupID, &inv.FromUserID, &inv.ToUserID, &inv.Status, &inv.Message,
			&inv.CreatedAt, &inv.UpdatedAt,
			&toUser.ID, &toUser.Pseudo, &toUser.FirstName, &toUser.LastName, &toUser.Email, &toUser.AvatarUrl,
			&group.ID, &group.Name, &group.Icon, &group.Color,
		); err != nil {
			fmt.Println("Scan group invitation error:", err)
			continue
		}
		inv.ToUser = &toUser
		inv.Group = &group
		invitations = append(invitations, inv)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(invitations)
}

// ============================================================================
// POST /friend-groups/{id}/invite - Invite users to group
// ============================================================================

func (h *Handler) InviteToGroup(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	groupID := chi.URLParam(r, "id")

	var dto InviteToGroupDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if len(dto.UserIDs) == 0 {
		http.Error(w, "No users specified", http.StatusBadRequest)
		return
	}

	// Verify user is a member of the group (either owner or member)
	var isMember bool
	err := h.db.QueryRow(r.Context(), `
		SELECT EXISTS(
			SELECT 1 FROM friend_groups WHERE id = $1 AND user_id = $2
			UNION
			SELECT 1 FROM friend_group_members WHERE group_id = $1 AND member_id = $2
		)
	`, groupID, userID).Scan(&isMember)
	if err != nil || !isMember {
		http.Error(w, "Not a member of this group", http.StatusForbidden)
		return
	}

	invited := 0
	for _, inviteeID := range dto.UserIDs {
		// Can't invite yourself
		if inviteeID == userID {
			continue
		}

		// Check if already a member
		var alreadyMember bool
		h.db.QueryRow(r.Context(), `
			SELECT EXISTS(
				SELECT 1 FROM friend_groups WHERE id = $1 AND user_id = $2
				UNION
				SELECT 1 FROM friend_group_members WHERE group_id = $1 AND member_id = $2
			)
		`, groupID, inviteeID).Scan(&alreadyMember)
		if alreadyMember {
			continue
		}

		// Must be a friend to invite
		var isFriend bool
		h.db.QueryRow(r.Context(), `
			SELECT EXISTS(SELECT 1 FROM friendships WHERE user_id = $1 AND member_id = $2)
		`, userID, inviteeID).Scan(&isFriend)
		if !isFriend {
			continue
		}

		// Create invitation (ON CONFLICT handles duplicate pending invites)
		_, err = h.db.Exec(r.Context(), `
			INSERT INTO group_invitations (group_id, from_user_id, to_user_id, message, status)
			VALUES ($1, $2, $3, $4, 'pending')
			ON CONFLICT (group_id, to_user_id, status)
			WHERE status = 'pending'
			DO NOTHING
		`, groupID, userID, inviteeID, dto.Message)
		if err != nil {
			fmt.Printf("Error inviting user %s to group %s: %v\n", inviteeID, groupID, err)
			continue
		}
		invited++
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"invited": invited})
}

// ============================================================================
// POST /group-invitations/{id}/accept - Accept a group invitation
// ============================================================================

func (h *Handler) AcceptGroupInvitation(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	invitationID := chi.URLParam(r, "id")

	ctx := r.Context()
	tx, err := h.db.Begin(ctx)
	if err != nil {
		http.Error(w, "Failed to start transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(ctx)

	// Get invitation and verify it's for this user
	var groupID string
	err = tx.QueryRow(ctx, `
		SELECT group_id FROM group_invitations
		WHERE id = $1 AND to_user_id = $2 AND status = 'pending'
	`, invitationID, userID).Scan(&groupID)
	if err != nil {
		if err == pgx.ErrNoRows {
			http.Error(w, "Invitation not found or already processed", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to get invitation", http.StatusInternalServerError)
		return
	}

	// Update invitation status
	_, err = tx.Exec(ctx, `
		UPDATE group_invitations SET status = 'accepted', updated_at = NOW()
		WHERE id = $1
	`, invitationID)
	if err != nil {
		http.Error(w, "Failed to update invitation", http.StatusInternalServerError)
		return
	}

	// Add user to group members
	_, err = tx.Exec(ctx, `
		INSERT INTO friend_group_members (group_id, member_id)
		VALUES ($1, $2)
		ON CONFLICT (group_id, member_id) DO NOTHING
	`, groupID, userID)
	if err != nil {
		http.Error(w, "Failed to add to group", http.StatusInternalServerError)
		return
	}

	if err = tx.Commit(ctx); err != nil {
		http.Error(w, "Failed to commit transaction", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"group_id": groupID,
	})
}

// ============================================================================
// POST /group-invitations/{id}/reject - Reject a group invitation
// ============================================================================

func (h *Handler) RejectGroupInvitation(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	invitationID := chi.URLParam(r, "id")

	result, err := h.db.Exec(r.Context(), `
		UPDATE group_invitations SET status = 'rejected', updated_at = NOW()
		WHERE id = $1 AND to_user_id = $2 AND status = 'pending'
	`, invitationID, userID)
	if err != nil {
		http.Error(w, "Failed to reject invitation", http.StatusInternalServerError)
		return
	}

	if result.RowsAffected() == 0 {
		http.Error(w, "Invitation not found or already processed", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// ============================================================================
// DELETE /group-invitations/{id} - Cancel a sent invitation
// ============================================================================

func (h *Handler) CancelGroupInvitation(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	invitationID := chi.URLParam(r, "id")

	result, err := h.db.Exec(r.Context(), `
		DELETE FROM group_invitations
		WHERE id = $1 AND from_user_id = $2 AND status = 'pending'
	`, invitationID, userID)
	if err != nil {
		http.Error(w, "Failed to cancel invitation", http.StatusInternalServerError)
		return
	}

	if result.RowsAffected() == 0 {
		http.Error(w, "Invitation not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ============================================================================
// POST /friend-groups/{id}/leave - Leave a group (for non-owners)
// ============================================================================

func (h *Handler) LeaveGroup(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	groupID := chi.URLParam(r, "id")

	// Check if user is the owner
	var isOwner bool
	err := h.db.QueryRow(r.Context(), `
		SELECT EXISTS(SELECT 1 FROM friend_groups WHERE id = $1 AND user_id = $2)
	`, groupID, userID).Scan(&isOwner)
	if err != nil {
		http.Error(w, "Failed to check ownership", http.StatusInternalServerError)
		return
	}

	if isOwner {
		http.Error(w, "Owner cannot leave group. Transfer ownership or delete the group.", http.StatusForbidden)
		return
	}

	// Remove from group members
	result, err := h.db.Exec(r.Context(), `
		DELETE FROM friend_group_members
		WHERE group_id = $1 AND member_id = $2
	`, groupID, userID)
	if err != nil {
		http.Error(w, "Failed to leave group", http.StatusInternalServerError)
		return
	}

	if result.RowsAffected() == 0 {
		http.Error(w, "Not a member of this group", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ============================================================================
// Updated GET /crew/groups - List groups where user is owner OR member
// ============================================================================

func (h *Handler) ListGroupsShared(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	query := `
		SELECT
			g.id,
			g.name,
			g.description,
			g.icon,
			g.color,
			g.user_id as owner_id,
			g.created_at,
			g.updated_at,
			COALESCE(mc.member_count, 0) + 1 as member_count,
			(g.user_id = $1) as is_owner
		FROM friend_groups g
		LEFT JOIN (
			SELECT group_id, COUNT(*)::int as member_count
			FROM friend_group_members
			GROUP BY group_id
		) mc ON g.id = mc.group_id
		WHERE g.user_id = $1
		   OR EXISTS (SELECT 1 FROM friend_group_members gm WHERE gm.group_id = g.id AND gm.member_id = $1)
		ORDER BY g.name
	`

	rows, err := h.db.Query(r.Context(), query, userID)
	if err != nil {
		fmt.Println("List groups error:", err)
		http.Error(w, "Failed to list groups", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type SharedGroup struct {
		ID          string    `json:"id"`
		Name        string    `json:"name"`
		Description *string   `json:"description"`
		Icon        string    `json:"icon"`
		Color       string    `json:"color"`
		OwnerID     string    `json:"owner_id"`
		MemberCount int       `json:"member_count"`
		IsOwner     bool      `json:"is_owner"`
		CreatedAt   time.Time `json:"created_at"`
		UpdatedAt   time.Time `json:"updated_at"`
	}

	groups := []SharedGroup{}
	for rows.Next() {
		var g SharedGroup
		if err := rows.Scan(&g.ID, &g.Name, &g.Description, &g.Icon, &g.Color, &g.OwnerID, &g.CreatedAt, &g.UpdatedAt, &g.MemberCount, &g.IsOwner); err != nil {
			fmt.Println("Scan group error:", err)
			continue
		}
		groups = append(groups, g)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(groups)
}

// ============================================================================
// GROUP ROUTINES - Shared routines for group accountability
// ============================================================================

// Models for group routines
type GroupRoutine struct {
	ID                string                         `json:"id"`
	GroupID           string                         `json:"group_id"`
	RoutineID         string                         `json:"routine_id"`
	Title             string                         `json:"title"`
	Icon              *string                        `json:"icon"`
	Frequency         string                         `json:"frequency"`
	ScheduledTime     *string                        `json:"scheduled_time"`
	SharedBy          *CrewUserInfo                  `json:"shared_by"`
	MemberCompletions []GroupRoutineMemberCompletion `json:"member_completions"`
	CompletionCount   int                            `json:"completion_count"`
	TotalMembers      int                            `json:"total_members"`
	CreatedAt         time.Time                      `json:"created_at"`
}

type GroupRoutineMemberCompletion struct {
	UserID      string     `json:"user_id"`
	Pseudo      *string    `json:"pseudo"`
	FirstName   *string    `json:"first_name"`
	AvatarUrl   *string    `json:"avatar_url"`
	Completed   bool       `json:"completed"`
	CompletedAt *time.Time `json:"completed_at"`
}

type ShareRoutineDTO struct {
	RoutineID string `json:"routine_id"`
}

type GroupRoutinesResponse struct {
	Routines []GroupRoutine `json:"routines"`
}

// ============================================================================
// POST /friend-groups/{id}/routines - Share a routine with a group
// ============================================================================

func (h *Handler) ShareRoutineWithGroup(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	groupID := chi.URLParam(r, "id")

	var dto ShareRoutineDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if dto.RoutineID == "" {
		http.Error(w, "routine_id is required", http.StatusBadRequest)
		return
	}

	// Verify user is member or owner of the group
	var hasAccess bool
	err := h.db.QueryRow(r.Context(), `
		SELECT EXISTS(
			SELECT 1 FROM friend_groups WHERE id = $1 AND user_id = $2
			UNION
			SELECT 1 FROM friend_group_members WHERE group_id = $1 AND member_id = $2
		)
	`, groupID, userID).Scan(&hasAccess)
	if err != nil || !hasAccess {
		http.Error(w, "Not a member of this group", http.StatusForbidden)
		return
	}

	// Verify user owns the routine
	var routineExists bool
	err = h.db.QueryRow(r.Context(), `
		SELECT EXISTS(SELECT 1 FROM routines WHERE id = $1 AND user_id = $2)
	`, dto.RoutineID, userID).Scan(&routineExists)
	if err != nil || !routineExists {
		http.Error(w, "Routine not found or not owned by you", http.StatusNotFound)
		return
	}

	// Insert into group_routines
	var groupRoutineID string
	var createdAt time.Time
	err = h.db.QueryRow(r.Context(), `
		INSERT INTO group_routines (group_id, routine_id, shared_by)
		VALUES ($1, $2, $3)
		ON CONFLICT (group_id, routine_id) DO NOTHING
		RETURNING id, created_at
	`, groupID, dto.RoutineID, userID).Scan(&groupRoutineID, &createdAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			http.Error(w, "Routine already shared with this group", http.StatusConflict)
			return
		}
		fmt.Println("Share routine error:", err)
		http.Error(w, "Failed to share routine", http.StatusInternalServerError)
		return
	}

	// Fetch the routine details and return the full response
	var routine GroupRoutine
	routine.ID = groupRoutineID
	routine.GroupID = groupID
	routine.RoutineID = dto.RoutineID
	routine.CreatedAt = createdAt

	var sharer CrewUserInfo
	err = h.db.QueryRow(r.Context(), `
		SELECT r.title, r.icon, r.frequency, r.scheduled_time,
		       u.id, u.pseudo, u.first_name, u.last_name, u.avatar_url
		FROM routines r
		JOIN users u ON r.user_id = u.id
		WHERE r.id = $1
	`, dto.RoutineID).Scan(
		&routine.Title, &routine.Icon, &routine.Frequency, &routine.ScheduledTime,
		&sharer.ID, &sharer.Pseudo, &sharer.FirstName, &sharer.LastName, &sharer.AvatarUrl,
	)
	if err != nil {
		fmt.Println("Fetch routine details error:", err)
	}
	routine.SharedBy = &sharer

	routine.MemberCompletions = []GroupRoutineMemberCompletion{}
	routine.CompletionCount = 0
	routine.TotalMembers = 0

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(routine)
}

// ============================================================================
// GET /friend-groups/{id}/routines - List group's shared routines with completions
// ============================================================================

func (h *Handler) ListGroupRoutines(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	groupID := chi.URLParam(r, "id")
	dateStr := r.URL.Query().Get("date")

	// Default to today if no date provided
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}

	// Verify user is member or owner of the group
	var hasAccess bool
	err := h.db.QueryRow(r.Context(), `
		SELECT EXISTS(
			SELECT 1 FROM friend_groups WHERE id = $1 AND user_id = $2
			UNION
			SELECT 1 FROM friend_group_members WHERE group_id = $1 AND member_id = $2
		)
	`, groupID, userID).Scan(&hasAccess)
	if err != nil || !hasAccess {
		http.Error(w, "Not a member of this group", http.StatusForbidden)
		return
	}

	// Fetch all shared routines for this group
	routinesQuery := `
		SELECT gr.id, gr.group_id, gr.routine_id, gr.created_at,
		       r.title, r.icon, r.frequency, r.scheduled_time,
		       u.id as sharer_id, u.pseudo as sharer_pseudo, u.first_name as sharer_first_name,
		       u.last_name as sharer_last_name, u.avatar_url as sharer_avatar
		FROM group_routines gr
		JOIN routines r ON gr.routine_id = r.id
		JOIN users u ON gr.shared_by = u.id
		WHERE gr.group_id = $1
		ORDER BY r.scheduled_time NULLS LAST, r.title
	`

	rows, err := h.db.Query(r.Context(), routinesQuery, groupID)
	if err != nil {
		fmt.Println("List group routines error:", err)
		http.Error(w, "Failed to list group routines", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	routines := []GroupRoutine{}
	for rows.Next() {
		var gr GroupRoutine
		var sharer CrewUserInfo
		if err := rows.Scan(
			&gr.ID, &gr.GroupID, &gr.RoutineID, &gr.CreatedAt,
			&gr.Title, &gr.Icon, &gr.Frequency, &gr.ScheduledTime,
			&sharer.ID, &sharer.Pseudo, &sharer.FirstName, &sharer.LastName, &sharer.AvatarUrl,
		); err != nil {
			fmt.Println("Scan group routine error:", err)
			continue
		}
		gr.SharedBy = &sharer
		gr.MemberCompletions = []GroupRoutineMemberCompletion{}
		routines = append(routines, gr)
	}

	// For each routine, fetch member completions
	for i := range routines {
		completionsQuery := `
			SELECT
				u.id,
				u.pseudo,
				u.first_name,
				u.avatar_url,
				CASE WHEN rc.id IS NOT NULL THEN true ELSE false END as completed,
				rc.completed_at
			FROM (
				SELECT member_id as user_id FROM friend_group_members WHERE group_id = $1
				UNION
				SELECT user_id FROM friend_groups WHERE id = $1
			) members
			JOIN users u ON members.user_id = u.id
			LEFT JOIN routine_completions rc
				ON rc.user_id = u.id
				AND rc.routine_id = $2
				AND rc.completion_date = $3::date
			ORDER BY completed DESC, u.pseudo, u.first_name
		`

		compRows, err := h.db.Query(r.Context(), completionsQuery, groupID, routines[i].RoutineID, dateStr)
		if err != nil {
			fmt.Println("Fetch completions error:", err)
			continue
		}

		completionCount := 0
		totalMembers := 0
		for compRows.Next() {
			var mc GroupRoutineMemberCompletion
			if err := compRows.Scan(&mc.UserID, &mc.Pseudo, &mc.FirstName, &mc.AvatarUrl, &mc.Completed, &mc.CompletedAt); err != nil {
				fmt.Println("Scan completion error:", err)
				continue
			}
			routines[i].MemberCompletions = append(routines[i].MemberCompletions, mc)
			totalMembers++
			if mc.Completed {
				completionCount++
			}
		}
		compRows.Close()

		routines[i].CompletionCount = completionCount
		routines[i].TotalMembers = totalMembers
	}

	response := GroupRoutinesResponse{Routines: routines}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ============================================================================
// DELETE /friend-groups/{groupId}/routines/{groupRoutineId} - Remove routine from group
// ============================================================================

func (h *Handler) RemoveGroupRoutine(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	groupID := chi.URLParam(r, "id")
	groupRoutineID := chi.URLParam(r, "groupRoutineId")

	// Check if user is the sharer OR the group owner
	var canDelete bool
	err := h.db.QueryRow(r.Context(), `
		SELECT EXISTS(
			SELECT 1 FROM group_routines gr
			WHERE gr.id = $1 AND gr.group_id = $2 AND gr.shared_by = $3
			UNION
			SELECT 1 FROM group_routines gr
			JOIN friend_groups g ON gr.group_id = g.id
			WHERE gr.id = $1 AND gr.group_id = $2 AND g.user_id = $3
		)
	`, groupRoutineID, groupID, userID).Scan(&canDelete)
	if err != nil {
		fmt.Println("Check delete permission error:", err)
		http.Error(w, "Failed to check permissions", http.StatusInternalServerError)
		return
	}

	if !canDelete {
		http.Error(w, "Only the sharer or group owner can remove this routine", http.StatusForbidden)
		return
	}

	// Delete the group routine
	result, err := h.db.Exec(r.Context(), `
		DELETE FROM group_routines
		WHERE id = $1 AND group_id = $2
	`, groupRoutineID, groupID)
	if err != nil {
		fmt.Println("Delete group routine error:", err)
		http.Error(w, "Failed to remove routine from group", http.StatusInternalServerError)
		return
	}

	if result.RowsAffected() == 0 {
		http.Error(w, "Group routine not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
