package crew

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"firelevel-backend/internal/auth"

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
}

type SearchUserResult struct {
	ID                string  `json:"id"`
	Pseudo            *string `json:"pseudo"`
	FirstName         *string `json:"first_name"`
	LastName          *string `json:"last_name"`
	AvatarUrl         *string `json:"avatar_url"`
	DayVisibility     *string `json:"day_visibility"`
	TotalSessions7d   *int    `json:"total_sessions_7d"`
	TotalMinutes7d    *int    `json:"total_minutes_7d"`
	ActivityScore     *int    `json:"activity_score"`
	IsCrewMember      bool    `json:"is_crew_member"`
	HasPendingRequest bool    `json:"has_pending_request"`
	RequestDirection  *string `json:"request_direction"`
}

type CrewMemberDay struct {
	User              *CrewUserInfo          `json:"user"`
	Intentions        []CrewIntention        `json:"intentions"`
	FocusSessions     []CrewFocusSession     `json:"focus_sessions"`
	CompletedRoutines []CrewCompletedRoutine `json:"completed_routines"`
	Routines          []CrewRoutine          `json:"routines"`
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
			u.avatar_url,
			COALESCE(u.day_visibility, 'crew') as day_visibility,
			COALESCE(fs.total_sessions, 0)::int as total_sessions_7d,
			COALESCE(fs.total_minutes, 0)::int as total_minutes_7d,
			(COALESCE(fs.total_minutes, 0) + (COALESCE(rc.completed_count, 0) * 10))::int as activity_score,
			cm.created_at
		FROM crew_members cm
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
			&m.AvatarUrl, &m.DayVisibility, &m.TotalSessions7d, &m.TotalMinutes7d,
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
		DELETE FROM crew_members
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
		UPDATE crew_requests SET status = 'rejected'
		WHERE (from_user_id = $1 AND to_user_id = $2)
		   OR (from_user_id = $2 AND to_user_id = $1)
	`
	h.db.Exec(r.Context(), updateQuery, userID, memberID)

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
			u.id, u.pseudo, u.first_name, u.last_name, u.avatar_url
		FROM crew_requests cr
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
			&fromUser.ID, &fromUser.Pseudo, &fromUser.FirstName, &fromUser.LastName, &fromUser.AvatarUrl,
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
			u.id, u.pseudo, u.first_name, u.last_name, u.avatar_url
		FROM crew_requests cr
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
			&toUser.ID, &toUser.Pseudo, &toUser.FirstName, &toUser.LastName, &toUser.AvatarUrl,
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
	checkQuery := `SELECT EXISTS(SELECT 1 FROM crew_members WHERE user_id = $1 AND member_id = $2)`
	var alreadyMember bool
	h.db.QueryRow(r.Context(), checkQuery, userID, req.ToUserID).Scan(&alreadyMember)
	if alreadyMember {
		http.Error(w, "Already crew members", http.StatusConflict)
		return
	}

	query := `
		INSERT INTO crew_requests (from_user_id, to_user_id, message, status)
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
		SELECT from_user_id, to_user_id FROM crew_requests
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
	updateQuery := `UPDATE crew_requests SET status = 'accepted', updated_at = NOW() WHERE id = $1`
	_, err = tx.Exec(r.Context(), updateQuery, requestID)
	if err != nil {
		http.Error(w, "Failed to update request", http.StatusInternalServerError)
		return
	}

	// Create bidirectional crew membership
	insertQuery := `
		INSERT INTO crew_members (user_id, member_id) VALUES ($1, $2)
		ON CONFLICT (user_id, member_id) DO NOTHING
	`
	tx.Exec(r.Context(), insertQuery, toUserID, fromUserID)
	tx.Exec(r.Context(), insertQuery, fromUserID, toUserID)

	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "Failed to commit transaction", http.StatusInternalServerError)
		return
	}

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
		UPDATE crew_requests SET status = 'rejected', updated_at = NOW()
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
			u.avatar_url,
			COALESCE(u.day_visibility, 'crew') as day_visibility,
			COALESCE(fs.total_sessions, 0)::int as total_sessions_7d,
			COALESCE(fs.total_minutes, 0)::int as total_minutes_7d,
			(COALESCE(fs.total_minutes, 0) + (COALESCE(rc.completed_count, 0) * 10))::int as activity_score,
			EXISTS(SELECT 1 FROM crew_members cm WHERE cm.user_id = $1 AND cm.member_id = u.id) as is_crew_member,
			EXISTS(
				SELECT 1 FROM crew_requests cr
				WHERE cr.status = 'pending'
				AND ((cr.from_user_id = $1 AND cr.to_user_id = u.id) OR (cr.from_user_id = u.id AND cr.to_user_id = $1))
			) as has_pending_request,
			(
				SELECT CASE
					WHEN cr.from_user_id = $1 THEN 'outgoing'
					WHEN cr.to_user_id = $1 THEN 'incoming'
					ELSE NULL
				END
				FROM crew_requests cr
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
		AND (u.pseudo ILIKE $2 OR u.first_name ILIKE $2 OR u.last_name ILIKE $2)
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
			&u.ID, &u.Pseudo, &u.FirstName, &u.LastName, &u.AvatarUrl,
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
		)
		SELECT
			ROW_NUMBER() OVER (ORDER BY activity_score DESC, total_minutes_7d DESC)::bigint as rank,
			us.id,
			us.pseudo,
			us.first_name,
			us.last_name,
			us.email,
			us.avatar_url,
			us.day_visibility,
			us.total_sessions_7d,
			us.total_minutes_7d,
			us.completed_routines_7d,
			us.activity_score,
			COALESCE((SELECT longest_streak FROM user_streaks WHERE user_id = us.id), 0)::int as current_streak,
			us.last_active,
			EXISTS(SELECT 1 FROM crew_members cm WHERE cm.user_id = $1 AND cm.member_id = us.id) as is_crew_member,
			EXISTS(
				SELECT 1 FROM crew_requests cr
				WHERE cr.status = 'pending'
				AND ((cr.from_user_id = $1 AND cr.to_user_id = us.id) OR (cr.from_user_id = us.id AND cr.to_user_id = $1))
			) as has_pending_request,
			(
				SELECT CASE
					WHEN cr.from_user_id = $1 THEN 'outgoing'
					WHEN cr.to_user_id = $1 THEN 'incoming'
					ELSE NULL
				END
				FROM crew_requests cr
				WHERE cr.status = 'pending'
				AND ((cr.from_user_id = $1 AND cr.to_user_id = us.id) OR (cr.from_user_id = us.id AND cr.to_user_id = $1))
				LIMIT 1
			) as request_direction
		FROM user_stats us
		ORDER BY activity_score DESC, total_minutes_7d DESC
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
		if err := rows.Scan(
			&rank, &e.ID, &e.Pseudo, &e.FirstName, &e.LastName, &e.Email, &e.AvatarUrl,
			&e.DayVisibility, &e.TotalSessions7d, &e.TotalMinutes7d, &e.CompletedRoutines7d,
			&e.ActivityScore, &e.CurrentStreak, &lastActive, &e.IsCrewMember, &e.HasPendingRequest, &e.RequestDirection,
		); err != nil {
			fmt.Println("Scan leaderboard entry error:", err)
			continue
		}
		e.Rank = int(rank)
		if lastActive != nil {
			formatted := lastActive.Format(time.RFC3339)
			e.LastActive = &formatted
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

	// Check if current user is in their crew
	var isCrewMember bool
	crewQuery := `SELECT EXISTS(SELECT 1 FROM crew_members WHERE user_id = $1 AND member_id = $2)`
	h.db.QueryRow(r.Context(), crewQuery, userID, memberID).Scan(&isCrewMember)

	// Check permission
	if visibility == "private" {
		http.Error(w, "Day is private", http.StatusForbidden)
		return
	}
	if visibility == "crew" && !isCrewMember {
		http.Error(w, "Not a crew member", http.StatusForbidden)
		return
	}

	// Get user info
	var user CrewUserInfo
	userQuery := `SELECT id, pseudo, first_name, last_name, avatar_url FROM users WHERE id = $1`
	h.db.QueryRow(r.Context(), userQuery, memberID).Scan(
		&user.ID, &user.Pseudo, &user.FirstName, &user.LastName, &user.AvatarUrl,
	)

	// Get intentions
	intentionsQuery := `
		SELECT i.id, i.content, i.position
		FROM intentions i
		JOIN daily_intentions di ON i.daily_intention_id = di.id
		WHERE di.user_id = $1 AND di.date = $2
		ORDER BY i.position
	`
	intentionRows, _ := h.db.Query(r.Context(), intentionsQuery, memberID, dateStr)
	defer intentionRows.Close()

	intentions := []CrewIntention{}
	for intentionRows.Next() {
		var i CrewIntention
		intentionRows.Scan(&i.ID, &i.Content, &i.Position)
		intentions = append(intentions, i)
	}

	// Get focus sessions
	sessionsQuery := `
		SELECT id, description, duration_minutes, started_at, completed_at, status
		FROM focus_sessions
		WHERE user_id = $1 AND DATE(started_at) = $2 AND status = 'completed'
		ORDER BY started_at DESC
	`
	sessionRows, _ := h.db.Query(r.Context(), sessionsQuery, memberID, dateStr)
	defer sessionRows.Close()

	sessions := []CrewFocusSession{}
	for sessionRows.Next() {
		var s CrewFocusSession
		sessionRows.Scan(&s.ID, &s.Description, &s.DurationMinutes, &s.StartedAt, &s.CompletedAt, &s.Status)
		sessions = append(sessions, s)
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
	completedRoutineRows, _ := h.db.Query(r.Context(), completedRoutinesQuery, memberID, dateStr, userID)
	defer completedRoutineRows.Close()

	completedRoutines := []CrewCompletedRoutine{}
	for completedRoutineRows.Next() {
		var cr CrewCompletedRoutine
		completedRoutineRows.Scan(&cr.ID, &cr.Title, &cr.Icon, &cr.CompletedAt, &cr.LikeCount, &cr.IsLikedByMe)
		completedRoutines = append(completedRoutines, cr)
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

	// Get stats for the member
	stats := h.getMemberStats(r.Context(), memberID)

	day := CrewMemberDay{
		User:              &user,
		Intentions:        intentions,
		FocusSessions:     sessions,
		CompletedRoutines: completedRoutines,
		Routines:          allRoutines,
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
			AND NOT EXISTS (SELECT 1 FROM crew_members cm WHERE cm.user_id = $1 AND cm.member_id = u.id)
			AND NOT EXISTS (
				SELECT 1 FROM crew_requests cr
				WHERE cr.status = 'pending'
				AND ((cr.from_user_id = $1 AND cr.to_user_id = u.id) OR (cr.from_user_id = u.id AND cr.to_user_id = $1))
			)
			AND (COALESCE(fs.total_minutes, 0) + (COALESCE(rc.completed_count, 0) * 10)) > 0
		)
		SELECT
			id, pseudo, first_name, last_name, avatar_url, day_visibility,
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
			&u.ID, &u.Pseudo, &u.FirstName, &u.LastName, &u.AvatarUrl,
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
