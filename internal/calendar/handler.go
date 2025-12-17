package calendar

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"firelevel-backend/internal/auth"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	db           *pgxpool.Pool
	googleCalSvc GoogleCalendarSyncer
}

// GoogleCalendarSyncer interface for Google Calendar sync
type GoogleCalendarSyncer interface {
	SyncTaskToGoogleCalendar(ctx context.Context, userID, taskID, title string, description *string, date string, startTime, endTime *string) error
	DeleteGoogleCalendarEvent(ctx context.Context, userID, googleEventID string) error
}

func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
}

// SetGoogleCalendarSyncer sets the Google Calendar syncer
func (h *Handler) SetGoogleCalendarSyncer(syncer GoogleCalendarSyncer) {
	h.googleCalSvc = syncer
}

// ==========================================
// DAY PLAN TYPES
// ==========================================

type DayPlan struct {
	ID             string     `json:"id"`
	UserID         string     `json:"userId"`
	Date           string     `json:"date"`
	IdealDayPrompt *string    `json:"idealDayPrompt,omitempty"`
	AISummary      *string    `json:"aiSummary,omitempty"`
	Progress       int        `json:"progress"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
	TimeBlocks     []TimeBlock `json:"timeBlocks,omitempty"`
	Tasks          []Task      `json:"tasks,omitempty"`
}

type TimeBlock struct {
	ID                 string    `json:"id"`
	UserID             string    `json:"userId"`
	DayPlanID          *string   `json:"dayPlanId,omitempty"`
	QuestID            *string   `json:"questId,omitempty"`
	AreaID             *string   `json:"areaId,omitempty"`
	Title              string    `json:"title"`
	Description        *string   `json:"description,omitempty"`
	StartTime          time.Time `json:"startTime"`
	EndTime            time.Time `json:"endTime"`
	BlockType          string    `json:"blockType"`
	ExternalCalendarID *string   `json:"externalCalendarId,omitempty"`
	ExternalEventID    *string   `json:"externalEventId,omitempty"`
	Status             string    `json:"status"`
	Progress           int       `json:"progress"`
	IsAIGenerated      bool      `json:"isAiGenerated"`
	Color              *string   `json:"color,omitempty"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
	Tasks              []Task    `json:"tasks,omitempty"`
	QuestTitle         *string   `json:"questTitle,omitempty"`
	AreaName           *string   `json:"areaName,omitempty"`
	AreaIcon           *string   `json:"areaIcon,omitempty"`
}

type Task struct {
	ID               string     `json:"id"`
	UserID           string     `json:"userId"`
	QuestID          *string    `json:"questId,omitempty"`
	AreaID           *string    `json:"areaId,omitempty"`
	Title            string     `json:"title"`
	Description      *string    `json:"description,omitempty"`
	Date             string     `json:"date"`                       // YYYY-MM-DD
	ScheduledStart   *string    `json:"scheduledStart,omitempty"`   // HH:mm format
	ScheduledEnd     *string    `json:"scheduledEnd,omitempty"`     // HH:mm format
	TimeBlock        string     `json:"timeBlock"`                  // morning, afternoon, evening
	Position         int        `json:"position"`
	EstimatedMinutes *int       `json:"estimatedMinutes,omitempty"`
	ActualMinutes    int        `json:"actualMinutes"`
	Priority         string     `json:"priority"`
	Status           string     `json:"status"`
	DueAt            *time.Time `json:"dueAt,omitempty"`
	CompletedAt      *time.Time `json:"completedAt,omitempty"`
	IsAIGenerated    bool       `json:"isAiGenerated"`
	AINotes          *string    `json:"aiNotes,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
	QuestTitle       *string    `json:"questTitle,omitempty"`
	AreaName         *string    `json:"areaName,omitempty"`
	AreaIcon         *string    `json:"areaIcon,omitempty"`
	PhotosCount      *int       `json:"photosCount,omitempty"` // Number of community posts linked to this task
}

// ==========================================
// REQUEST/RESPONSE TYPES
// ==========================================

type CreateDayPlanRequest struct {
	Date           string `json:"date"`
	IdealDayPrompt string `json:"idealDayPrompt"`
}

type UpdateDayPlanRequest struct {
	IdealDayPrompt *string `json:"idealDayPrompt,omitempty"`
	AISummary      *string `json:"aiSummary,omitempty"`
	Status         *string `json:"status,omitempty"`
}

type CreateTimeBlockRequest struct {
	DayPlanID   *string   `json:"dayPlanId,omitempty"`
	QuestID     *string   `json:"questId,omitempty"`
	AreaID      *string   `json:"areaId,omitempty"`
	Title       string    `json:"title"`
	Description *string   `json:"description,omitempty"`
	StartTime   time.Time `json:"startTime"`
	EndTime     time.Time `json:"endTime"`
	BlockType   string    `json:"blockType"`
	Color       *string   `json:"color,omitempty"`
}

type UpdateTimeBlockRequest struct {
	Title       *string    `json:"title,omitempty"`
	Description *string    `json:"description,omitempty"`
	StartTime   *time.Time `json:"startTime,omitempty"`
	EndTime     *time.Time `json:"endTime,omitempty"`
	Status      *string    `json:"status,omitempty"`
	Progress    *int       `json:"progress,omitempty"`
	QuestID     *string    `json:"questId,omitempty"`
	AreaID      *string    `json:"areaId,omitempty"`
}

type CreateTaskRequest struct {
	QuestID          *string    `json:"quest_id,omitempty"`
	AreaID           *string    `json:"area_id,omitempty"`
	Title            string     `json:"title"`
	Description      *string    `json:"description,omitempty"`
	Date             string     `json:"date"`                       // YYYY-MM-DD
	ScheduledStart   *string    `json:"scheduled_start,omitempty"`  // HH:mm
	ScheduledEnd     *string    `json:"scheduled_end,omitempty"`    // HH:mm
	TimeBlock        *string    `json:"time_block,omitempty"`       // morning, afternoon, evening
	Position         *int       `json:"position,omitempty"`
	EstimatedMinutes *int       `json:"estimated_minutes,omitempty"`
	Priority         *string    `json:"priority,omitempty"`
	DueAt            *time.Time `json:"due_at,omitempty"`
}

type UpdateTaskRequest struct {
	Title            *string    `json:"title,omitempty"`
	Description      *string    `json:"description,omitempty"`
	Date             *string    `json:"date,omitempty"`
	ScheduledStart   *string    `json:"scheduled_start,omitempty"`
	ScheduledEnd     *string    `json:"scheduled_end,omitempty"`
	TimeBlock        *string    `json:"time_block,omitempty"`
	Position         *int       `json:"position,omitempty"`
	EstimatedMinutes *int       `json:"estimated_minutes,omitempty"`
	ActualMinutes    *int       `json:"actual_minutes,omitempty"`
	Priority         *string    `json:"priority,omitempty"`
	Status           *string    `json:"status,omitempty"`
	DueAt            *time.Time `json:"due_at,omitempty"`
	QuestID          *string    `json:"quest_id,omitempty"`
	AreaID           *string    `json:"area_id,omitempty"`
}

type CalendarDayResponse struct {
	DayPlan    *DayPlan    `json:"dayPlan,omitempty"`
	TimeBlocks []TimeBlock `json:"timeBlocks"`
	Tasks      []Task      `json:"tasks"`
	Progress   int         `json:"progress"`
}

// ==========================================
// DAY PLAN HANDLERS
// ==========================================

func (h *Handler) GetDayPlan(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	date := r.URL.Query().Get("date")
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	var plan DayPlan
	err := h.db.QueryRow(r.Context(), `
		SELECT id, user_id, date, ideal_day_prompt, ai_summary, progress, status, created_at, updated_at
		FROM day_plans
		WHERE user_id = $1 AND date = $2
	`, userID, date).Scan(
		&plan.ID, &plan.UserID, &plan.Date, &plan.IdealDayPrompt,
		&plan.AISummary, &plan.Progress, &plan.Status,
		&plan.CreatedAt, &plan.UpdatedAt,
	)

	if err != nil {
		// No plan for this day - return empty response
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(CalendarDayResponse{
			TimeBlocks: []TimeBlock{},
			Tasks:      []Task{},
			Progress:   0,
		})
		return
	}

	// Get time blocks for this day
	timeBlocks, _ := h.getTimeBlocksForDay(r.Context(), userID, date)
	plan.TimeBlocks = timeBlocks

	// Get tasks for this day
	tasks, _ := h.getTasksForDay(r.Context(), userID, date, nil)
	plan.Tasks = tasks

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(CalendarDayResponse{
		DayPlan:    &plan,
		TimeBlocks: timeBlocks,
		Tasks:      tasks,
		Progress:   plan.Progress,
	})
}

func (h *Handler) CreateDayPlan(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	var req CreateDayPlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Date == "" {
		req.Date = time.Now().Format("2006-01-02")
	}

	var plan DayPlan
	err := h.db.QueryRow(r.Context(), `
		INSERT INTO day_plans (user_id, date, ideal_day_prompt, status)
		VALUES ($1, $2, $3, 'active')
		ON CONFLICT (user_id, date)
		DO UPDATE SET ideal_day_prompt = EXCLUDED.ideal_day_prompt, updated_at = now()
		RETURNING id, user_id, date, ideal_day_prompt, ai_summary, progress, status, created_at, updated_at
	`, userID, req.Date, req.IdealDayPrompt).Scan(
		&plan.ID, &plan.UserID, &plan.Date, &plan.IdealDayPrompt,
		&plan.AISummary, &plan.Progress, &plan.Status,
		&plan.CreatedAt, &plan.UpdatedAt,
	)

	if err != nil {
		http.Error(w, "Failed to create day plan", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(plan)
}

func (h *Handler) UpdateDayPlan(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	planID := chi.URLParam(r, "id")

	var req UpdateDayPlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var plan DayPlan
	err := h.db.QueryRow(r.Context(), `
		UPDATE day_plans
		SET
			ideal_day_prompt = COALESCE($3, ideal_day_prompt),
			ai_summary = COALESCE($4, ai_summary),
			status = COALESCE($5, status),
			updated_at = now()
		WHERE id = $1 AND user_id = $2
		RETURNING id, user_id, date, ideal_day_prompt, ai_summary, progress, status, created_at, updated_at
	`, planID, userID, req.IdealDayPrompt, req.AISummary, req.Status).Scan(
		&plan.ID, &plan.UserID, &plan.Date, &plan.IdealDayPrompt,
		&plan.AISummary, &plan.Progress, &plan.Status,
		&plan.CreatedAt, &plan.UpdatedAt,
	)

	if err != nil {
		http.Error(w, "Day plan not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(plan)
}

// ==========================================
// TIME BLOCK HANDLERS
// ==========================================

func (h *Handler) ListTimeBlocks(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	date := r.URL.Query().Get("date")
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	blocks, err := h.getTimeBlocksForDay(r.Context(), userID, date)
	if err != nil {
		http.Error(w, "Failed to fetch time blocks", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(blocks)
}

func (h *Handler) CreateTimeBlock(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	var req CreateTimeBlockRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.BlockType == "" {
		req.BlockType = "focus"
	}

	var block TimeBlock
	err := h.db.QueryRow(r.Context(), `
		INSERT INTO time_blocks (user_id, day_plan_id, quest_id, area_id, title, description, start_time, end_time, block_type, color)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, user_id, day_plan_id, quest_id, area_id, title, description, start_time, end_time, block_type, status, progress, is_ai_generated, color, created_at, updated_at
	`, userID, req.DayPlanID, req.QuestID, req.AreaID, req.Title, req.Description, req.StartTime, req.EndTime, req.BlockType, req.Color).Scan(
		&block.ID, &block.UserID, &block.DayPlanID, &block.QuestID, &block.AreaID,
		&block.Title, &block.Description, &block.StartTime, &block.EndTime,
		&block.BlockType, &block.Status, &block.Progress, &block.IsAIGenerated,
		&block.Color, &block.CreatedAt, &block.UpdatedAt,
	)

	if err != nil {
		http.Error(w, "Failed to create time block: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(block)
}

func (h *Handler) UpdateTimeBlock(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	blockID := chi.URLParam(r, "id")

	var req UpdateTimeBlockRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var block TimeBlock
	err := h.db.QueryRow(r.Context(), `
		UPDATE time_blocks
		SET
			title = COALESCE($3, title),
			description = COALESCE($4, description),
			start_time = COALESCE($5, start_time),
			end_time = COALESCE($6, end_time),
			status = COALESCE($7, status),
			progress = COALESCE($8, progress),
			quest_id = COALESCE($9, quest_id),
			area_id = COALESCE($10, area_id),
			updated_at = now()
		WHERE id = $1 AND user_id = $2
		RETURNING id, user_id, day_plan_id, quest_id, area_id, title, description, start_time, end_time, block_type, status, progress, is_ai_generated, color, created_at, updated_at
	`, blockID, userID, req.Title, req.Description, req.StartTime, req.EndTime, req.Status, req.Progress, req.QuestID, req.AreaID).Scan(
		&block.ID, &block.UserID, &block.DayPlanID, &block.QuestID, &block.AreaID,
		&block.Title, &block.Description, &block.StartTime, &block.EndTime,
		&block.BlockType, &block.Status, &block.Progress, &block.IsAIGenerated,
		&block.Color, &block.CreatedAt, &block.UpdatedAt,
	)

	if err != nil {
		http.Error(w, "Time block not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(block)
}

func (h *Handler) DeleteTimeBlock(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	blockID := chi.URLParam(r, "id")

	result, err := h.db.Exec(r.Context(), `
		DELETE FROM time_blocks WHERE id = $1 AND user_id = $2
	`, blockID, userID)

	if err != nil || result.RowsAffected() == 0 {
		http.Error(w, "Time block not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ==========================================
// TASK HANDLERS
// ==========================================

func (h *Handler) ListTasks(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	date := r.URL.Query().Get("date")
	timeBlockID := r.URL.Query().Get("timeBlockId")

	var tasks []Task
	var err error

	if timeBlockID != "" {
		tasks, err = h.getTasksForTimeBlock(r.Context(), userID, timeBlockID)
	} else if date != "" {
		tasks, err = h.getTasksForDay(r.Context(), userID, date, nil)
	} else {
		// Return today's tasks
		tasks, err = h.getTasksForDay(r.Context(), userID, time.Now().Format("2006-01-02"), nil)
	}

	if err != nil {
		http.Error(w, "Failed to fetch tasks", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tasks)
}

func (h *Handler) CreateTask(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	var req CreateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Log received data
	startStr := "nil"
	endStr := "nil"
	if req.ScheduledStart != nil {
		startStr = *req.ScheduledStart
	}
	if req.ScheduledEnd != nil {
		endStr = *req.ScheduledEnd
	}
	log.Printf("[CreateTask] Received: title=%s, date=%s, scheduledStart=%s, scheduledEnd=%s", req.Title, req.Date, startStr, endStr)

	priority := "medium"
	if req.Priority != nil {
		priority = *req.Priority
	}

	position := 0
	if req.Position != nil {
		position = *req.Position
	}

	timeBlock := "morning"
	if req.TimeBlock != nil {
		timeBlock = *req.TimeBlock
	}

	var task Task
	var scheduledStartStr, scheduledEndStr *string
	err := h.db.QueryRow(r.Context(), `
		INSERT INTO tasks (user_id, quest_id, area_id, title, description, date, scheduled_start, scheduled_end, time_block, position, estimated_minutes, priority, due_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7::time, $8::time, $9, $10, $11, $12, $13)
		RETURNING id, user_id, quest_id, area_id, title, description, date,
			TO_CHAR(scheduled_start, 'HH24:MI') as scheduled_start,
			TO_CHAR(scheduled_end, 'HH24:MI') as scheduled_end,
			time_block, position, estimated_minutes, actual_minutes, priority, status, due_at, completed_at, is_ai_generated, ai_notes, created_at, updated_at
	`, userID, req.QuestID, req.AreaID, req.Title, req.Description, req.Date, req.ScheduledStart, req.ScheduledEnd, timeBlock, position, req.EstimatedMinutes, priority, req.DueAt).Scan(
		&task.ID, &task.UserID, &task.QuestID, &task.AreaID,
		&task.Title, &task.Description, &task.Date, &scheduledStartStr, &scheduledEndStr,
		&task.TimeBlock, &task.Position, &task.EstimatedMinutes, &task.ActualMinutes,
		&task.Priority, &task.Status, &task.DueAt, &task.CompletedAt,
		&task.IsAIGenerated, &task.AINotes, &task.CreatedAt, &task.UpdatedAt,
	)

	if err != nil {
		log.Printf("[CreateTask] ERROR inserting task: %v", err)
		log.Printf("[CreateTask] Request data: userID=%s, title=%s, date=%s, timeBlock=%s", userID, req.Title, req.Date, timeBlock)
		http.Error(w, "Failed to create task: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("[CreateTask] SUCCESS: created task id=%s, title=%s", task.ID, task.Title)

	// Use times directly from SQL (already formatted as HH:mm)
	if scheduledStartStr != nil && *scheduledStartStr != "" {
		task.ScheduledStart = scheduledStartStr
	}
	if scheduledEndStr != nil && *scheduledEndStr != "" {
		task.ScheduledEnd = scheduledEndStr
	}

	// Sync to Google Calendar (async, don't block response)
	if h.googleCalSvc != nil {
		go func() {
			ctx := context.Background()
			err := h.googleCalSvc.SyncTaskToGoogleCalendar(ctx, userID, task.ID, task.Title, task.Description, task.Date, task.ScheduledStart, task.ScheduledEnd)
			if err != nil {
				log.Printf("[CreateTask] Google Calendar sync failed: %v", err)
			}
		}()
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(task)
}

func (h *Handler) UpdateTask(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	taskID := chi.URLParam(r, "id")

	var req UpdateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Handle status change to completed
	var completedAt *time.Time
	if req.Status != nil && *req.Status == "completed" {
		now := time.Now()
		completedAt = &now
	}

	var task Task
	var scheduledStartStr, scheduledEndStr *string
	err := h.db.QueryRow(r.Context(), `
		UPDATE tasks
		SET
			title = COALESCE($3, title),
			description = COALESCE($4, description),
			date = COALESCE($5, date),
			scheduled_start = COALESCE($6::time, scheduled_start),
			scheduled_end = COALESCE($7::time, scheduled_end),
			time_block = COALESCE($8, time_block),
			position = COALESCE($9, position),
			estimated_minutes = COALESCE($10, estimated_minutes),
			actual_minutes = COALESCE($11, actual_minutes),
			priority = COALESCE($12, priority),
			status = COALESCE($13, status),
			due_at = COALESCE($14, due_at),
			completed_at = COALESCE($15, completed_at),
			quest_id = COALESCE($16, quest_id),
			area_id = COALESCE($17, area_id),
			updated_at = now()
		WHERE id = $1 AND user_id = $2
		RETURNING id, user_id, quest_id, area_id, title, description, date,
			TO_CHAR(scheduled_start, 'HH24:MI') as scheduled_start,
			TO_CHAR(scheduled_end, 'HH24:MI') as scheduled_end,
			time_block, position, estimated_minutes, actual_minutes, priority, status, due_at, completed_at, is_ai_generated, ai_notes, created_at, updated_at
	`, taskID, userID, req.Title, req.Description, req.Date, req.ScheduledStart, req.ScheduledEnd, req.TimeBlock, req.Position, req.EstimatedMinutes, req.ActualMinutes, req.Priority, req.Status, req.DueAt, completedAt, req.QuestID, req.AreaID).Scan(
		&task.ID, &task.UserID, &task.QuestID, &task.AreaID,
		&task.Title, &task.Description, &task.Date, &scheduledStartStr, &scheduledEndStr,
		&task.TimeBlock, &task.Position, &task.EstimatedMinutes, &task.ActualMinutes,
		&task.Priority, &task.Status, &task.DueAt, &task.CompletedAt,
		&task.IsAIGenerated, &task.AINotes, &task.CreatedAt, &task.UpdatedAt,
	)

	if err != nil {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}

	// Use times directly from SQL (already formatted as HH:mm)
	if scheduledStartStr != nil && *scheduledStartStr != "" {
		task.ScheduledStart = scheduledStartStr
	}
	if scheduledEndStr != nil && *scheduledEndStr != "" {
		task.ScheduledEnd = scheduledEndStr
	}

	// Sync to Google Calendar (async, don't block response)
	if h.googleCalSvc != nil {
		go func() {
			ctx := context.Background()
			err := h.googleCalSvc.SyncTaskToGoogleCalendar(ctx, userID, task.ID, task.Title, task.Description, task.Date, task.ScheduledStart, task.ScheduledEnd)
			if err != nil {
				log.Printf("[UpdateTask] Google Calendar sync failed: %v", err)
			}
		}()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

func (h *Handler) CompleteTask(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	taskID := chi.URLParam(r, "id")

	var task Task
	var scheduledStartStr, scheduledEndStr *string
	err := h.db.QueryRow(r.Context(), `
		UPDATE tasks
		SET status = 'completed', completed_at = now(), updated_at = now()
		WHERE id = $1 AND user_id = $2
		RETURNING id, user_id, quest_id, area_id, title, description, date,
			TO_CHAR(scheduled_start, 'HH24:MI') as scheduled_start,
			TO_CHAR(scheduled_end, 'HH24:MI') as scheduled_end,
			time_block, position, estimated_minutes, actual_minutes, priority, status, due_at, completed_at, is_ai_generated, ai_notes, created_at, updated_at
	`, taskID, userID).Scan(
		&task.ID, &task.UserID, &task.QuestID, &task.AreaID,
		&task.Title, &task.Description, &task.Date, &scheduledStartStr, &scheduledEndStr,
		&task.TimeBlock, &task.Position, &task.EstimatedMinutes, &task.ActualMinutes,
		&task.Priority, &task.Status, &task.DueAt, &task.CompletedAt,
		&task.IsAIGenerated, &task.AINotes, &task.CreatedAt, &task.UpdatedAt,
	)

	if err != nil {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}

	// Use times directly from SQL (already formatted as HH:mm)
	if scheduledStartStr != nil && *scheduledStartStr != "" {
		task.ScheduledStart = scheduledStartStr
	}
	if scheduledEndStr != nil && *scheduledEndStr != "" {
		task.ScheduledEnd = scheduledEndStr
	}

	// Update quest progress if linked
	if task.QuestID != nil {
		h.updateQuestProgress(r.Context(), userID, *task.QuestID, task.ID, task.ActualMinutes)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

func (h *Handler) UncompleteTask(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	taskID := chi.URLParam(r, "id")

	var task Task
	var scheduledStartStr, scheduledEndStr *string
	err := h.db.QueryRow(r.Context(), `
		UPDATE tasks
		SET status = 'pending', completed_at = NULL, updated_at = now()
		WHERE id = $1 AND user_id = $2
		RETURNING id, user_id, quest_id, area_id, title, description, date,
			TO_CHAR(scheduled_start, 'HH24:MI') as scheduled_start,
			TO_CHAR(scheduled_end, 'HH24:MI') as scheduled_end,
			time_block, position, estimated_minutes, actual_minutes, priority, status, due_at, completed_at, is_ai_generated, ai_notes, created_at, updated_at
	`, taskID, userID).Scan(
		&task.ID, &task.UserID, &task.QuestID, &task.AreaID,
		&task.Title, &task.Description, &task.Date, &scheduledStartStr, &scheduledEndStr,
		&task.TimeBlock, &task.Position, &task.EstimatedMinutes, &task.ActualMinutes,
		&task.Priority, &task.Status, &task.DueAt, &task.CompletedAt,
		&task.IsAIGenerated, &task.AINotes, &task.CreatedAt, &task.UpdatedAt,
	)

	if err != nil {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}

	// Use times directly from SQL (already formatted as HH:mm)
	if scheduledStartStr != nil && *scheduledStartStr != "" {
		task.ScheduledStart = scheduledStartStr
	}
	if scheduledEndStr != nil && *scheduledEndStr != "" {
		task.ScheduledEnd = scheduledEndStr
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

func (h *Handler) DeleteTask(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	taskID := chi.URLParam(r, "id")

	// Get Google event ID before deleting
	var googleEventID *string
	h.db.QueryRow(r.Context(), `SELECT google_event_id FROM tasks WHERE id = $1 AND user_id = $2`, taskID, userID).Scan(&googleEventID)

	result, err := h.db.Exec(r.Context(), `
		DELETE FROM tasks WHERE id = $1 AND user_id = $2
	`, taskID, userID)

	if err != nil || result.RowsAffected() == 0 {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}

	// Delete from Google Calendar (async)
	if h.googleCalSvc != nil && googleEventID != nil && *googleEventID != "" {
		go func() {
			ctx := context.Background()
			err := h.googleCalSvc.DeleteGoogleCalendarEvent(ctx, userID, *googleEventID)
			if err != nil {
				log.Printf("[DeleteTask] Google Calendar delete failed: %v", err)
			}
		}()
	}

	w.WriteHeader(http.StatusNoContent)
}

// ==========================================
// HELPER METHODS
// ==========================================

func (h *Handler) getTimeBlocksForDay(ctx context.Context, userID, date string) ([]TimeBlock, error) {
	rows, err := h.db.Query(ctx, `
		SELECT
			tb.id, tb.user_id, tb.day_plan_id, tb.quest_id, tb.area_id,
			tb.title, tb.description, tb.start_time, tb.end_time,
			tb.block_type, tb.status, tb.progress, tb.is_ai_generated,
			tb.color, tb.created_at, tb.updated_at,
			q.title as quest_title, a.name as area_name, a.icon as area_icon
		FROM time_blocks tb
		LEFT JOIN quests q ON tb.quest_id = q.id
		LEFT JOIN areas a ON tb.area_id = a.id
		WHERE tb.user_id = $1 AND DATE(tb.start_time) = $2
		ORDER BY tb.start_time
	`, userID, date)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var blocks []TimeBlock
	for rows.Next() {
		var b TimeBlock
		err := rows.Scan(
			&b.ID, &b.UserID, &b.DayPlanID, &b.QuestID, &b.AreaID,
			&b.Title, &b.Description, &b.StartTime, &b.EndTime,
			&b.BlockType, &b.Status, &b.Progress, &b.IsAIGenerated,
			&b.Color, &b.CreatedAt, &b.UpdatedAt,
			&b.QuestTitle, &b.AreaName, &b.AreaIcon,
		)
		if err != nil {
			continue
		}
		blocks = append(blocks, b)
	}

	if blocks == nil {
		blocks = []TimeBlock{}
	}

	return blocks, nil
}

func (h *Handler) getTasksForDay(ctx context.Context, userID, date string, dayPlanID *string) ([]Task, error) {
	rows, err := h.db.Query(ctx, `
		SELECT
			t.id, t.user_id, t.quest_id, t.area_id,
			t.title, t.description, t.date,
			TO_CHAR(t.scheduled_start, 'HH24:MI') as scheduled_start,
			TO_CHAR(t.scheduled_end, 'HH24:MI') as scheduled_end,
			t.time_block, t.position, t.estimated_minutes, t.actual_minutes,
			t.priority, t.status, t.due_at, t.completed_at, t.is_ai_generated,
			t.ai_notes, t.created_at, t.updated_at,
			q.title as quest_title, a.name as area_name, a.icon as area_icon,
			(SELECT COUNT(*) FROM community_posts cp WHERE cp.task_id = t.id AND cp.is_hidden = false) as photos_count
		FROM tasks t
		LEFT JOIN quests q ON t.quest_id = q.id
		LEFT JOIN areas a ON t.area_id = a.id
		WHERE t.user_id = $1 AND t.date = $2
		ORDER BY t.scheduled_start NULLS LAST, t.position
	`, userID, date)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		var scheduledStart, scheduledEnd *string
		var timeBlock *string
		var photosCount int
		err := rows.Scan(
			&t.ID, &t.UserID, &t.QuestID, &t.AreaID,
			&t.Title, &t.Description, &t.Date, &scheduledStart, &scheduledEnd,
			&timeBlock, &t.Position, &t.EstimatedMinutes, &t.ActualMinutes,
			&t.Priority, &t.Status, &t.DueAt, &t.CompletedAt, &t.IsAIGenerated,
			&t.AINotes, &t.CreatedAt, &t.UpdatedAt,
			&t.QuestTitle, &t.AreaName, &t.AreaIcon, &photosCount,
		)
		if err != nil {
			continue
		}

		// Set photos count if > 0
		if photosCount > 0 {
			t.PhotosCount = &photosCount
		}

		if timeBlock != nil {
			t.TimeBlock = *timeBlock
		} else {
			t.TimeBlock = "morning"
		}

		if scheduledStart != nil && *scheduledStart != "" {
			t.ScheduledStart = scheduledStart
		} else {
			defaultStart := getDefaultStartTime(t.TimeBlock)
			t.ScheduledStart = &defaultStart
		}
		if scheduledEnd != nil && *scheduledEnd != "" {
			t.ScheduledEnd = scheduledEnd
		} else {
			defaultEnd := getDefaultEndTime(t.TimeBlock)
			t.ScheduledEnd = &defaultEnd
		}

		tasks = append(tasks, t)
	}

	if tasks == nil {
		tasks = []Task{}
	}

	return tasks, nil
}

// getTasksForTimeBlock is deprecated - time_blocks no longer exist
func (h *Handler) getTasksForTimeBlock(ctx context.Context, userID, timeBlockID string) ([]Task, error) {
	return []Task{}, nil
}

func (h *Handler) updateQuestProgress(ctx context.Context, userID, questID, taskID string, minutesSpent int) {
	// Log the progress
	h.db.Exec(ctx, `
		INSERT INTO quest_progress_log (user_id, quest_id, source_type, source_id, progress_value, minutes_spent)
		VALUES ($1, $2, 'task', $3, 1, $4)
	`, userID, questID, taskID, minutesSpent)

	// Update quest current_value
	h.db.Exec(ctx, `
		UPDATE quests
		SET current_value = current_value + 1
		WHERE id = $1 AND user_id = $2
	`, questID, userID)
}

// ==========================================
// WEEK VIEW ENDPOINT
// ==========================================

type WeekViewResponse struct {
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
	Tasks     []Task `json:"tasks"`
}

// GetWeekView returns all tasks for a week
// GET /calendar/week?startDate=2024-01-08
func (h *Handler) GetWeekView(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	startDateStr := r.URL.Query().Get("startDate")

	// Default to current week's Monday
	var startDate time.Time
	if startDateStr == "" {
		now := time.Now()
		// Find Monday of current week
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		startDate = now.AddDate(0, 0, -weekday+1)
	} else {
		var err error
		startDate, err = time.Parse("2006-01-02", startDateStr)
		if err != nil {
			http.Error(w, "Invalid startDate format", http.StatusBadRequest)
			return
		}
	}

	endDate := startDate.AddDate(0, 0, 6)
	startDateFmt := startDate.Format("2006-01-02")
	endDateFmt := endDate.Format("2006-01-02")

	// Get tasks for the week
	log.Printf("[GetWeekView] Fetching tasks for user=%s, start=%s, end=%s", userID, startDateFmt, endDateFmt)
	tasks, err := h.getTasksForWeek(r.Context(), userID, startDateFmt, endDateFmt)
	if err != nil {
		log.Printf("[GetWeekView] ERROR: %v", err)
		http.Error(w, "Failed to get tasks: "+err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("[GetWeekView] SUCCESS: found %d tasks", len(tasks))

	response := WeekViewResponse{
		StartDate: startDateFmt,
		EndDate:   endDateFmt,
		Tasks:     tasks,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *Handler) getTimeBlocksForWeek(ctx context.Context, userID, startDate, endDate string) ([]TimeBlock, error) {
	rows, err := h.db.Query(ctx, `
		SELECT
			tb.id, tb.user_id, tb.day_plan_id, tb.quest_id, tb.area_id,
			tb.title, tb.description, tb.start_time, tb.end_time,
			tb.block_type, tb.status, tb.progress, tb.is_ai_generated,
			tb.color, tb.created_at, tb.updated_at,
			q.title as quest_title, a.name as area_name, a.icon as area_icon
		FROM time_blocks tb
		LEFT JOIN quests q ON tb.quest_id = q.id
		LEFT JOIN areas a ON tb.area_id = a.id
		WHERE tb.user_id = $1 AND DATE(tb.start_time) BETWEEN $2 AND $3
		ORDER BY tb.start_time
	`, userID, startDate, endDate)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var blocks []TimeBlock
	for rows.Next() {
		var b TimeBlock
		err := rows.Scan(
			&b.ID, &b.UserID, &b.DayPlanID, &b.QuestID, &b.AreaID,
			&b.Title, &b.Description, &b.StartTime, &b.EndTime,
			&b.BlockType, &b.Status, &b.Progress, &b.IsAIGenerated,
			&b.Color, &b.CreatedAt, &b.UpdatedAt,
			&b.QuestTitle, &b.AreaName, &b.AreaIcon,
		)
		if err != nil {
			continue
		}
		blocks = append(blocks, b)
	}

	if blocks == nil {
		blocks = []TimeBlock{}
	}

	return blocks, nil
}

// getDefaultStartTime returns default start time based on time_block
func getDefaultStartTime(timeBlock string) string {
	switch timeBlock {
	case "morning":
		return "09:30"
	case "afternoon":
		return "14:00"
	case "evening":
		return "19:00"
	default:
		return "09:30" // Default to morning
	}
}

// getDefaultEndTime returns default end time based on time_block
func getDefaultEndTime(timeBlock string) string {
	switch timeBlock {
	case "morning":
		return "10:30"
	case "afternoon":
		return "15:00"
	case "evening":
		return "20:00"
	default:
		return "10:30" // Default to morning
	}
}

func (h *Handler) getTasksForWeek(ctx context.Context, userID, startDate, endDate string) ([]Task, error) {
	rows, err := h.db.Query(ctx, `
		SELECT
			t.id, t.user_id, t.quest_id, t.area_id,
			t.title, t.description, t.date,
			TO_CHAR(t.scheduled_start, 'HH24:MI') as scheduled_start,
			TO_CHAR(t.scheduled_end, 'HH24:MI') as scheduled_end,
			COALESCE(t.time_block, 'morning') as time_block,
			COALESCE(t.position, 0) as position,
			t.estimated_minutes,
			COALESCE(t.actual_minutes, 0) as actual_minutes,
			COALESCE(t.priority, 'medium') as priority,
			COALESCE(t.status, 'pending') as status,
			t.due_at, t.completed_at,
			COALESCE(t.is_ai_generated, false) as is_ai_generated,
			t.ai_notes, t.created_at, t.updated_at,
			q.title as quest_title, a.name as area_name, a.icon as area_icon
		FROM tasks t
		LEFT JOIN quests q ON t.quest_id = q.id
		LEFT JOIN areas a ON t.area_id = a.id
		WHERE t.user_id = $1 AND t.date BETWEEN $2 AND $3
		ORDER BY t.date, t.scheduled_start NULLS LAST, t.position
	`, userID, startDate, endDate)

	if err != nil {
		log.Printf("[getTasksForWeek] Query error: %v", err)
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		var scheduledStart, scheduledEnd *string
		var timeBlock string
		err := rows.Scan(
			&t.ID, &t.UserID, &t.QuestID, &t.AreaID,
			&t.Title, &t.Description, &t.Date, &scheduledStart, &scheduledEnd,
			&timeBlock, &t.Position, &t.EstimatedMinutes, &t.ActualMinutes,
			&t.Priority, &t.Status, &t.DueAt, &t.CompletedAt, &t.IsAIGenerated,
			&t.AINotes, &t.CreatedAt, &t.UpdatedAt,
			&t.QuestTitle, &t.AreaName, &t.AreaIcon,
		)
		if err != nil {
			log.Printf("[getTasksForWeek] Scan error: %v", err)
			continue
		}
		t.TimeBlock = timeBlock

		// Use string directly from SQL, or use default based on time_block
		if scheduledStart != nil && *scheduledStart != "" {
			t.ScheduledStart = scheduledStart
		} else {
			defaultStart := getDefaultStartTime(t.TimeBlock)
			t.ScheduledStart = &defaultStart
		}
		if scheduledEnd != nil && *scheduledEnd != "" {
			t.ScheduledEnd = scheduledEnd
		} else {
			defaultEnd := getDefaultEndTime(t.TimeBlock)
			t.ScheduledEnd = &defaultEnd
		}

		tasks = append(tasks, t)
	}

	if tasks == nil {
		tasks = []Task{}
	}

	return tasks, nil
}

// ==========================================
// RESCHEDULE DAILY GOAL (DRAG & DROP)
// ==========================================

type RescheduleTaskRequest struct {
	Date           string  `json:"date"`
	ScheduledStart *string `json:"scheduled_start,omitempty"` // HH:mm format
	ScheduledEnd   *string `json:"scheduled_end,omitempty"`   // HH:mm format
}

// RescheduleTask updates the date and scheduled time of a task (drag & drop)
// PATCH /calendar/tasks/{id}/reschedule
func (h *Handler) RescheduleTask(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	taskID := chi.URLParam(r, "id")

	var req RescheduleTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var task Task
	var scheduledStartStr, scheduledEndStr *string
	err := h.db.QueryRow(r.Context(), `
		UPDATE tasks
		SET
			date = COALESCE($3, date),
			scheduled_start = $4::time,
			scheduled_end = $5::time,
			updated_at = NOW()
		WHERE id = $1 AND user_id = $2
		RETURNING id, user_id, quest_id, area_id, title, description, date,
			TO_CHAR(scheduled_start, 'HH24:MI') as scheduled_start,
			TO_CHAR(scheduled_end, 'HH24:MI') as scheduled_end,
			time_block, position, estimated_minutes, actual_minutes, priority, status, due_at, completed_at, is_ai_generated, ai_notes, created_at, updated_at
	`, taskID, userID, req.Date, req.ScheduledStart, req.ScheduledEnd).Scan(
		&task.ID, &task.UserID, &task.QuestID, &task.AreaID,
		&task.Title, &task.Description, &task.Date, &scheduledStartStr, &scheduledEndStr,
		&task.TimeBlock, &task.Position, &task.EstimatedMinutes, &task.ActualMinutes,
		&task.Priority, &task.Status, &task.DueAt, &task.CompletedAt,
		&task.IsAIGenerated, &task.AINotes, &task.CreatedAt, &task.UpdatedAt,
	)

	if err != nil {
		http.Error(w, "Task not found or update failed", http.StatusNotFound)
		return
	}

	// Use times directly from SQL (already formatted as HH:mm)
	if scheduledStartStr != nil && *scheduledStartStr != "" {
		task.ScheduledStart = scheduledStartStr
	}
	if scheduledEndStr != nil && *scheduledEndStr != "" {
		task.ScheduledEnd = scheduledEndStr
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

// RescheduleGoal is deprecated - use RescheduleTask instead
// PATCH /calendar/goals/{id}/reschedule (kept for backwards compatibility)
func (h *Handler) RescheduleGoal(w http.ResponseWriter, r *http.Request) {
	// Redirect to RescheduleTask
	h.RescheduleTask(w, r)
}
