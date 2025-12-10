package calendar

import (
	"encoding/json"
	"net/http"
	"time"

	"firelevel-backend/internal/auth"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	db *pgxpool.Pool
}

func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
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
	TimeBlockID      *string    `json:"timeBlockId,omitempty"`
	QuestID          *string    `json:"questId,omitempty"`
	AreaID           *string    `json:"areaId,omitempty"`
	DayPlanID        *string    `json:"dayPlanId,omitempty"`
	Title            string     `json:"title"`
	Description      *string    `json:"description,omitempty"`
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
}

type Project struct {
	ID              string                 `json:"id"`
	UserID          string                 `json:"userId"`
	QuestID         *string                `json:"questId,omitempty"`
	AreaID          *string                `json:"areaId,omitempty"`
	Name            string                 `json:"name"`
	Description     *string                `json:"description,omitempty"`
	Components      []string               `json:"components"`
	Keywords        []string               `json:"keywords"`
	TimeAllocations map[string]int         `json:"timeAllocations"`
	Status          string                 `json:"status"`
	CreatedAt       time.Time              `json:"createdAt"`
	UpdatedAt       time.Time              `json:"updatedAt"`
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
	TimeBlockID      *string    `json:"timeBlockId,omitempty"`
	QuestID          *string    `json:"questId,omitempty"`
	AreaID           *string    `json:"areaId,omitempty"`
	DayPlanID        *string    `json:"dayPlanId,omitempty"`
	Title            string     `json:"title"`
	Description      *string    `json:"description,omitempty"`
	Position         *int       `json:"position,omitempty"`
	EstimatedMinutes *int       `json:"estimatedMinutes,omitempty"`
	Priority         *string    `json:"priority,omitempty"`
	DueAt            *time.Time `json:"dueAt,omitempty"`
}

type UpdateTaskRequest struct {
	Title            *string    `json:"title,omitempty"`
	Description      *string    `json:"description,omitempty"`
	Position         *int       `json:"position,omitempty"`
	EstimatedMinutes *int       `json:"estimatedMinutes,omitempty"`
	ActualMinutes    *int       `json:"actualMinutes,omitempty"`
	Priority         *string    `json:"priority,omitempty"`
	Status           *string    `json:"status,omitempty"`
	DueAt            *time.Time `json:"dueAt,omitempty"`
	TimeBlockID      *string    `json:"timeBlockId,omitempty"`
	QuestID          *string    `json:"questId,omitempty"`
	AreaID           *string    `json:"areaId,omitempty"`
}

type CreateProjectRequest struct {
	QuestID         *string            `json:"questId,omitempty"`
	AreaID          *string            `json:"areaId,omitempty"`
	Name            string             `json:"name"`
	Description     *string            `json:"description,omitempty"`
	Components      []string           `json:"components"`
	Keywords        []string           `json:"keywords"`
	TimeAllocations map[string]int     `json:"timeAllocations"`
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

	priority := "medium"
	if req.Priority != nil {
		priority = *req.Priority
	}

	position := 0
	if req.Position != nil {
		position = *req.Position
	}

	var task Task
	err := h.db.QueryRow(r.Context(), `
		INSERT INTO tasks (user_id, time_block_id, quest_id, area_id, day_plan_id, title, description, position, estimated_minutes, priority, due_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, user_id, time_block_id, quest_id, area_id, day_plan_id, title, description, position, estimated_minutes, actual_minutes, priority, status, due_at, completed_at, is_ai_generated, ai_notes, created_at, updated_at
	`, userID, req.TimeBlockID, req.QuestID, req.AreaID, req.DayPlanID, req.Title, req.Description, position, req.EstimatedMinutes, priority, req.DueAt).Scan(
		&task.ID, &task.UserID, &task.TimeBlockID, &task.QuestID, &task.AreaID,
		&task.DayPlanID, &task.Title, &task.Description, &task.Position,
		&task.EstimatedMinutes, &task.ActualMinutes, &task.Priority, &task.Status,
		&task.DueAt, &task.CompletedAt, &task.IsAIGenerated, &task.AINotes,
		&task.CreatedAt, &task.UpdatedAt,
	)

	if err != nil {
		http.Error(w, "Failed to create task: "+err.Error(), http.StatusInternalServerError)
		return
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
	err := h.db.QueryRow(r.Context(), `
		UPDATE tasks
		SET
			title = COALESCE($3, title),
			description = COALESCE($4, description),
			position = COALESCE($5, position),
			estimated_minutes = COALESCE($6, estimated_minutes),
			actual_minutes = COALESCE($7, actual_minutes),
			priority = COALESCE($8, priority),
			status = COALESCE($9, status),
			due_at = COALESCE($10, due_at),
			completed_at = COALESCE($11, completed_at),
			time_block_id = COALESCE($12, time_block_id),
			quest_id = COALESCE($13, quest_id),
			area_id = COALESCE($14, area_id),
			updated_at = now()
		WHERE id = $1 AND user_id = $2
		RETURNING id, user_id, time_block_id, quest_id, area_id, day_plan_id, title, description, position, estimated_minutes, actual_minutes, priority, status, due_at, completed_at, is_ai_generated, ai_notes, created_at, updated_at
	`, taskID, userID, req.Title, req.Description, req.Position, req.EstimatedMinutes, req.ActualMinutes, req.Priority, req.Status, req.DueAt, completedAt, req.TimeBlockID, req.QuestID, req.AreaID).Scan(
		&task.ID, &task.UserID, &task.TimeBlockID, &task.QuestID, &task.AreaID,
		&task.DayPlanID, &task.Title, &task.Description, &task.Position,
		&task.EstimatedMinutes, &task.ActualMinutes, &task.Priority, &task.Status,
		&task.DueAt, &task.CompletedAt, &task.IsAIGenerated, &task.AINotes,
		&task.CreatedAt, &task.UpdatedAt,
	)

	if err != nil {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

func (h *Handler) CompleteTask(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	taskID := chi.URLParam(r, "id")

	var task Task
	err := h.db.QueryRow(r.Context(), `
		UPDATE tasks
		SET status = 'completed', completed_at = now(), updated_at = now()
		WHERE id = $1 AND user_id = $2
		RETURNING id, user_id, time_block_id, quest_id, area_id, day_plan_id, title, description, position, estimated_minutes, actual_minutes, priority, status, due_at, completed_at, is_ai_generated, ai_notes, created_at, updated_at
	`, taskID, userID).Scan(
		&task.ID, &task.UserID, &task.TimeBlockID, &task.QuestID, &task.AreaID,
		&task.DayPlanID, &task.Title, &task.Description, &task.Position,
		&task.EstimatedMinutes, &task.ActualMinutes, &task.Priority, &task.Status,
		&task.DueAt, &task.CompletedAt, &task.IsAIGenerated, &task.AINotes,
		&task.CreatedAt, &task.UpdatedAt,
	)

	if err != nil {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
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
	err := h.db.QueryRow(r.Context(), `
		UPDATE tasks
		SET status = 'pending', completed_at = NULL, updated_at = now()
		WHERE id = $1 AND user_id = $2
		RETURNING id, user_id, time_block_id, quest_id, area_id, day_plan_id, title, description, position, estimated_minutes, actual_minutes, priority, status, due_at, completed_at, is_ai_generated, ai_notes, created_at, updated_at
	`, taskID, userID).Scan(
		&task.ID, &task.UserID, &task.TimeBlockID, &task.QuestID, &task.AreaID,
		&task.DayPlanID, &task.Title, &task.Description, &task.Position,
		&task.EstimatedMinutes, &task.ActualMinutes, &task.Priority, &task.Status,
		&task.DueAt, &task.CompletedAt, &task.IsAIGenerated, &task.AINotes,
		&task.CreatedAt, &task.UpdatedAt,
	)

	if err != nil {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

func (h *Handler) DeleteTask(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	taskID := chi.URLParam(r, "id")

	result, err := h.db.Exec(r.Context(), `
		DELETE FROM tasks WHERE id = $1 AND user_id = $2
	`, taskID, userID)

	if err != nil || result.RowsAffected() == 0 {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ==========================================
// PROJECT HANDLERS
// ==========================================

func (h *Handler) ListProjects(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	rows, err := h.db.Query(r.Context(), `
		SELECT id, user_id, quest_id, area_id, name, description, components, keywords, time_allocations, status, created_at, updated_at
		FROM projects
		WHERE user_id = $1 AND status = 'active'
		ORDER BY name
	`, userID)

	if err != nil {
		http.Error(w, "Failed to fetch projects", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var p Project
		var componentsJSON, keywordsJSON, timeAllocJSON []byte

		err := rows.Scan(
			&p.ID, &p.UserID, &p.QuestID, &p.AreaID,
			&p.Name, &p.Description, &componentsJSON, &keywordsJSON,
			&timeAllocJSON, &p.Status, &p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			continue
		}

		json.Unmarshal(componentsJSON, &p.Components)
		json.Unmarshal(keywordsJSON, &p.Keywords)
		json.Unmarshal(timeAllocJSON, &p.TimeAllocations)

		if p.Components == nil {
			p.Components = []string{}
		}
		if p.Keywords == nil {
			p.Keywords = []string{}
		}
		if p.TimeAllocations == nil {
			p.TimeAllocations = map[string]int{}
		}

		projects = append(projects, p)
	}

	if projects == nil {
		projects = []Project{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(projects)
}

func (h *Handler) CreateProject(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	var req CreateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	componentsJSON, _ := json.Marshal(req.Components)
	keywordsJSON, _ := json.Marshal(req.Keywords)
	timeAllocJSON, _ := json.Marshal(req.TimeAllocations)

	var p Project
	var componentsOut, keywordsOut, timeAllocOut []byte

	err := h.db.QueryRow(r.Context(), `
		INSERT INTO projects (user_id, quest_id, area_id, name, description, components, keywords, time_allocations)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, user_id, quest_id, area_id, name, description, components, keywords, time_allocations, status, created_at, updated_at
	`, userID, req.QuestID, req.AreaID, req.Name, req.Description, componentsJSON, keywordsJSON, timeAllocJSON).Scan(
		&p.ID, &p.UserID, &p.QuestID, &p.AreaID,
		&p.Name, &p.Description, &componentsOut, &keywordsOut,
		&timeAllocOut, &p.Status, &p.CreatedAt, &p.UpdatedAt,
	)

	if err != nil {
		http.Error(w, "Failed to create project: "+err.Error(), http.StatusInternalServerError)
		return
	}

	json.Unmarshal(componentsOut, &p.Components)
	json.Unmarshal(keywordsOut, &p.Keywords)
	json.Unmarshal(timeAllocOut, &p.TimeAllocations)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(p)
}

func (h *Handler) DeleteProject(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	projectID := chi.URLParam(r, "id")

	result, err := h.db.Exec(r.Context(), `
		UPDATE projects SET status = 'archived', updated_at = now()
		WHERE id = $1 AND user_id = $2
	`, projectID, userID)

	if err != nil || result.RowsAffected() == 0 {
		http.Error(w, "Project not found", http.StatusNotFound)
		return
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
	query := `
		SELECT
			t.id, t.user_id, t.time_block_id, t.quest_id, t.area_id, t.day_plan_id,
			t.title, t.description, t.position, t.estimated_minutes, t.actual_minutes,
			t.priority, t.status, t.due_at, t.completed_at, t.is_ai_generated,
			t.ai_notes, t.created_at, t.updated_at,
			q.title as quest_title, a.name as area_name, a.icon as area_icon
		FROM tasks t
		LEFT JOIN quests q ON t.quest_id = q.id
		LEFT JOIN areas a ON t.area_id = a.id
		WHERE t.user_id = $1 AND DATE(t.created_at) = $2
		ORDER BY t.position, t.created_at
	`

	rows, err := h.db.Query(ctx, query, userID, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		err := rows.Scan(
			&t.ID, &t.UserID, &t.TimeBlockID, &t.QuestID, &t.AreaID, &t.DayPlanID,
			&t.Title, &t.Description, &t.Position, &t.EstimatedMinutes, &t.ActualMinutes,
			&t.Priority, &t.Status, &t.DueAt, &t.CompletedAt, &t.IsAIGenerated,
			&t.AINotes, &t.CreatedAt, &t.UpdatedAt,
			&t.QuestTitle, &t.AreaName, &t.AreaIcon,
		)
		if err != nil {
			continue
		}
		tasks = append(tasks, t)
	}

	if tasks == nil {
		tasks = []Task{}
	}

	return tasks, nil
}

func (h *Handler) getTasksForTimeBlock(ctx context.Context, userID, timeBlockID string) ([]Task, error) {
	rows, err := h.db.Query(ctx, `
		SELECT
			t.id, t.user_id, t.time_block_id, t.quest_id, t.area_id, t.day_plan_id,
			t.title, t.description, t.position, t.estimated_minutes, t.actual_minutes,
			t.priority, t.status, t.due_at, t.completed_at, t.is_ai_generated,
			t.ai_notes, t.created_at, t.updated_at,
			q.title as quest_title, a.name as area_name, a.icon as area_icon
		FROM tasks t
		LEFT JOIN quests q ON t.quest_id = q.id
		LEFT JOIN areas a ON t.area_id = a.id
		WHERE t.user_id = $1 AND t.time_block_id = $2
		ORDER BY t.position
	`, userID, timeBlockID)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		err := rows.Scan(
			&t.ID, &t.UserID, &t.TimeBlockID, &t.QuestID, &t.AreaID, &t.DayPlanID,
			&t.Title, &t.Description, &t.Position, &t.EstimatedMinutes, &t.ActualMinutes,
			&t.Priority, &t.Status, &t.DueAt, &t.CompletedAt, &t.IsAIGenerated,
			&t.AINotes, &t.CreatedAt, &t.UpdatedAt,
			&t.QuestTitle, &t.AreaName, &t.AreaIcon,
		)
		if err != nil {
			continue
		}
		tasks = append(tasks, t)
	}

	if tasks == nil {
		tasks = []Task{}
	}

	return tasks, nil
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
