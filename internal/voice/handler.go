package voice

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"firelevel-backend/internal/auth"
	"firelevel-backend/internal/database"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type Handler struct {
	db        *database.DB
	aiService *AIService
}

func NewHandler(db *database.DB) *Handler {
	return &Handler{
		db:        db,
		aiService: NewAIService(),
	}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/voice/process", h.ProcessVoiceIntent)
	r.Post("/assistant/voice", h.VoiceAssistant) // New endpoint with Gradium TTS
	r.Get("/voice/intentions", h.GetIntentLogs)
	r.Get("/daily-goals", h.GetDailyGoals)
	r.Get("/daily-goals/{date}", h.GetDailyGoalsByDate)
	r.Post("/daily-goals", h.CreateDailyGoal)
	r.Patch("/daily-goals/{id}", h.UpdateDailyGoal)
	r.Delete("/daily-goals/{id}", h.DeleteDailyGoal)
	r.Post("/daily-goals/{id}/complete", h.CompleteDailyGoal)
	r.Get("/daily-goals/{id}/subtasks", h.GetGoalSubtasks)
	r.Post("/calendar/schedule-goals", h.ScheduleGoalsToCalendar)
}

// ============================================
// Request/Response Types
// ============================================

type ProcessVoiceRequest struct {
	Text string `json:"text"`
	Date string `json:"date,omitempty"` // Optional, defaults to today
}

type IntentResponse struct {
	IntentType       string       `json:"intent_type"`
	Goals            []GoalFromAI `json:"goals"`
	RawUserText      string       `json:"raw_user_text"`
	Notes            string       `json:"notes"`
	FollowUpQuestion *string      `json:"follow_up_question"`
}

type GoalFromAI struct {
	Title     string `json:"title"`
	Date      string `json:"date"`
	Priority  string `json:"priority"`
	TimeBlock string `json:"time_block"`
	Status    string `json:"status"`
}

type ProcessVoiceResponse struct {
	IntentLog    IntentLog    `json:"intent_log"`
	Goals        []DailyGoal  `json:"goals"`
	TimeBlocks   []TimeBlock  `json:"time_blocks,omitempty"`
	Message      string       `json:"message"`
	TTSResponse  string       `json:"tts_response"`
}

type IntentLog struct {
	ID               string          `json:"id"`
	UserID           string          `json:"user_id"`
	RawUserText      string          `json:"raw_user_text"`
	IntentType       string          `json:"intent_type"`
	AIResponse       json.RawMessage `json:"ai_response"`
	Notes            *string         `json:"notes"`
	FollowUpQuestion *string         `json:"follow_up_question"`
	ProcessedAt      *time.Time      `json:"processed_at"`
	CreatedAt        time.Time       `json:"created_at"`
}

type DailyGoal struct {
	ID             string     `json:"id"`
	UserID         string     `json:"user_id"`
	IntentLogID    *string    `json:"intent_log_id"`
	QuestID        *string    `json:"quest_id"`
	QuestTitle     *string    `json:"quest_title,omitempty"`
	DayPlanID      *string    `json:"day_plan_id"`
	Title          string     `json:"title"`
	Description    *string    `json:"description"`
	Date           string     `json:"date"`
	Priority       string     `json:"priority"`
	TimeBlock      string     `json:"time_block"`
	ScheduledStart *string    `json:"scheduled_start"`
	ScheduledEnd   *string    `json:"scheduled_end"`
	Status         string     `json:"status"`
	IsAIScheduled  bool       `json:"is_ai_scheduled"`
	Subtasks       []Subtask  `json:"subtasks,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type Subtask struct {
	ID               string    `json:"id"`
	DailyGoalID      string    `json:"daily_goal_id"`
	TaskID           *string   `json:"task_id"`
	Title            string    `json:"title"`
	EstimatedMinutes *int      `json:"estimated_minutes"`
	ScheduledStart   *string   `json:"scheduled_start"`
	ScheduledEnd     *string   `json:"scheduled_end"`
	Position         int       `json:"position"`
	Status           string    `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
}

type TimeBlock struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description *string   `json:"description"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	Status      string    `json:"status"`
	BlockType   string    `json:"block_type"`
	QuestID     *string   `json:"quest_id"`
	DailyGoalID *string   `json:"daily_goal_id"`
	Source      string    `json:"source"`
}

type CreateDailyGoalRequest struct {
	Title       string  `json:"title"`
	Description *string `json:"description"`
	Date        string  `json:"date"`
	Priority    string  `json:"priority"`
	TimeBlock   string  `json:"time_block"`
	QuestID     *string `json:"quest_id"`
}

type UpdateDailyGoalRequest struct {
	Title          *string `json:"title"`
	Description    *string `json:"description"`
	Priority       *string `json:"priority"`
	TimeBlock      *string `json:"time_block"`
	Status         *string `json:"status"`
	ScheduledStart *string `json:"scheduled_start"`
	ScheduledEnd   *string `json:"scheduled_end"`
	QuestID        *string `json:"quest_id"`
}

type ScheduleGoalsRequest struct {
	Date    string   `json:"date"`
	GoalIDs []string `json:"goal_ids,omitempty"` // If empty, schedule all pending goals for the date
}

// Voice Assistant Request/Response (with Gradium TTS)
type VoiceAssistantRequest struct {
	UserID      string `json:"user_id,omitempty"` // Optional, extracted from auth
	Text        string `json:"text"`
	Date        string `json:"date,omitempty"` // Optional, defaults to today
	VoiceID     string `json:"voice_id,omitempty"` // Gradium voice ID, default: b35yykvVppLXyw_l
	AudioFormat string `json:"audio_format,omitempty"` // wav, mp3 - default: wav
}

type VoiceAssistantResponse struct {
	IntentLog    IntentLog   `json:"intent_log"`
	Goals        []DailyGoal `json:"goals"`
	TimeBlocks   []TimeBlock `json:"time_blocks,omitempty"`
	ReplyText    string      `json:"reply_text"`
	AudioFormat  string      `json:"audio_format"`
	AudioBase64  string      `json:"audio_base64,omitempty"`
}

// ============================================
// Handlers
// ============================================

func (h *Handler) ProcessVoiceIntent(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req ProcessVoiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Text == "" {
		http.Error(w, "Text is required", http.StatusBadRequest)
		return
	}

	// Default date to today
	targetDate := req.Date
	if targetDate == "" {
		targetDate = time.Now().Format("2006-01-02")
	}

	// Step 1: Get user's quests for context
	quests, err := h.getUserQuests(userID)
	if err != nil {
		http.Error(w, "Failed to get user quests", http.StatusInternalServerError)
		return
	}

	// Step 2: Call Perplexity AI to extract intentions
	intentResponse, err := h.aiService.ExtractIntentions(req.Text, targetDate, quests)
	if err != nil {
		http.Error(w, "Failed to process voice input: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Step 3: Save intent log
	intentLog, err := h.saveIntentLog(userID, req.Text, intentResponse)
	if err != nil {
		http.Error(w, "Failed to save intent log", http.StatusInternalServerError)
		return
	}

	// Step 4: Process based on intent type
	var goals []DailyGoal
	var timeBlocks []TimeBlock
	var message, ttsResponse string

	switch intentResponse.IntentType {
	case "ADD_GOAL":
		// Create daily goals from AI response
		goals, err = h.createGoalsFromIntent(userID, intentLog.ID, intentResponse, quests)
		if err != nil {
			http.Error(w, "Failed to create goals", http.StatusInternalServerError)
			return
		}

		// Step 5: Use Grok to intelligently schedule goals into time blocks
		if len(goals) > 0 {
			timeBlocks, err = h.scheduleGoalsWithGrok(userID, targetDate, goals)
			if err != nil {
				// Non-fatal - goals created but not scheduled
				message = "Goals created but scheduling failed"
			} else {
				message = "Goals created and scheduled"
			}
		}

		ttsResponse = h.generateTTSResponse(intentResponse, goals)

	case "UPDATE_GOAL":
		message = "Goal update processed"
		ttsResponse = "J'ai mis à jour tes objectifs."

	case "LIST_GOALS":
		goals, err = h.getDailyGoalsByDate(userID, targetDate)
		if err != nil {
			http.Error(w, "Failed to get goals", http.StatusInternalServerError)
			return
		}
		message = "Here are your goals"
		ttsResponse = h.generateListTTSResponse(goals)

	case "ASK_QUESTION":
		message = "Question received"
		ttsResponse = "Je peux t'aider à organiser ta journée. Dis-moi ce que tu veux accomplir aujourd'hui."

	default:
		message = "I didn't understand that"
		ttsResponse = "Je n'ai pas bien compris. Peux-tu reformuler tes objectifs pour aujourd'hui ?"
	}

	// Update intent log as processed
	h.markIntentProcessed(intentLog.ID)

	response := ProcessVoiceResponse{
		IntentLog:   *intentLog,
		Goals:       goals,
		TimeBlocks:  timeBlocks,
		Message:     message,
		TTSResponse: ttsResponse,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// VoiceAssistant - Complete voice assistant endpoint with Perplexity + Gradium TTS
func (h *Handler) VoiceAssistant(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req VoiceAssistantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Text == "" {
		http.Error(w, "Text is required", http.StatusBadRequest)
		return
	}

	// Default values
	targetDate := req.Date
	if targetDate == "" {
		targetDate = time.Now().Format("2006-01-02")
	}
	voiceID := req.VoiceID
	if voiceID == "" {
		voiceID = "b35yykvVppLXyw_l" // Default French voice
	}
	audioFormat := req.AudioFormat
	if audioFormat == "" {
		audioFormat = "wav"
	}

	// Step 1: Get user's quests for context
	quests, err := h.getUserQuests(userID)
	if err != nil {
		http.Error(w, "Failed to get user quests", http.StatusInternalServerError)
		return
	}

	// Step 2: Call Perplexity AI to extract intentions
	intentResponse, err := h.aiService.ExtractIntentions(req.Text, targetDate, quests)
	if err != nil {
		http.Error(w, "Failed to process voice input: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Step 3: Save intent log
	intentLog, err := h.saveIntentLog(userID, req.Text, intentResponse)
	if err != nil {
		http.Error(w, "Failed to save intent log", http.StatusInternalServerError)
		return
	}

	// Step 4: Process based on intent type
	var goals []DailyGoal
	var timeBlocks []TimeBlock
	var replyText string

	switch intentResponse.IntentType {
	case "ADD_GOAL":
		goals, err = h.createGoalsFromIntent(userID, intentLog.ID, intentResponse, quests)
		if err != nil {
			http.Error(w, "Failed to create goals", http.StatusInternalServerError)
			return
		}

		// Schedule goals with Perplexity (since we removed Grok)
		if len(goals) > 0 {
			timeBlocks, _ = h.scheduleGoalsWithGrok(userID, targetDate, goals)
		}

		replyText = h.generateTTSResponse(intentResponse, goals)

	case "UPDATE_GOAL":
		replyText = "J'ai mis à jour tes objectifs."

	case "LIST_GOALS":
		goals, err = h.getDailyGoalsByDate(userID, targetDate)
		if err != nil {
			http.Error(w, "Failed to get goals", http.StatusInternalServerError)
			return
		}
		replyText = h.generateListTTSResponse(goals)

	case "ASK_QUESTION":
		replyText = "Je peux t'aider à organiser ta journée. Dis-moi ce que tu veux accomplir aujourd'hui."

	case "NEED_CLARIFICATION":
		if intentResponse.FollowUpQuestion != nil {
			replyText = *intentResponse.FollowUpQuestion
		} else {
			replyText = "Peux-tu me donner plus de détails sur ce que tu veux accomplir ?"
		}

	default:
		replyText = "Je n'ai pas bien compris. Peux-tu reformuler tes objectifs pour aujourd'hui ?"
	}

	// Update intent log as processed
	h.markIntentProcessed(intentLog.ID)

	// Step 5: Generate TTS audio with Gradium
	audioBase64, err := h.aiService.GenerateTTS(replyText, voiceID, audioFormat)
	if err != nil {
		// Non-fatal error - return response without audio
		fmt.Printf("TTS generation failed: %v\n", err)
	}

	response := VoiceAssistantResponse{
		IntentLog:   *intentLog,
		Goals:       goals,
		TimeBlocks:  timeBlocks,
		ReplyText:   replyText,
		AudioFormat: audioFormat,
		AudioBase64: audioBase64,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *Handler) GetIntentLogs(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	query := `
		SELECT id, user_id, raw_user_text, intent_type, ai_response, notes,
		       follow_up_question, processed_at, created_at
		FROM intent_logs
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT 50
	`

	rows, err := h.db.Pool.Query(r.Context(), query, userID)
	if err != nil {
		http.Error(w, "Failed to get intent logs", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var logs []IntentLog
	for rows.Next() {
		var log IntentLog
		err := rows.Scan(
			&log.ID, &log.UserID, &log.RawUserText, &log.IntentType,
			&log.AIResponse, &log.Notes, &log.FollowUpQuestion,
			&log.ProcessedAt, &log.CreatedAt,
		)
		if err != nil {
			continue
		}
		logs = append(logs, log)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}

func (h *Handler) GetDailyGoals(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get date from query params, default to today
	date := r.URL.Query().Get("date")
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	goals, err := h.getDailyGoalsByDate(userID, date)
	if err != nil {
		http.Error(w, "Failed to get daily goals", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(goals)
}

func (h *Handler) GetDailyGoalsByDate(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	date := chi.URLParam(r, "date")
	goals, err := h.getDailyGoalsByDate(userID, date)
	if err != nil {
		http.Error(w, "Failed to get daily goals", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(goals)
}

func (h *Handler) CreateDailyGoal(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req CreateDailyGoalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	id := uuid.New().String()
	query := `
		INSERT INTO daily_goals (id, user_id, title, description, date, priority, time_block, quest_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, user_id, title, description, date, priority, time_block,
		          scheduled_start, scheduled_end, status, is_ai_scheduled, quest_id, created_at, updated_at
	`

	var goal DailyGoal
	var scheduledStart, scheduledEnd *time.Time
	err := h.db.Pool.QueryRow(r.Context(), query,
		id, userID, req.Title, req.Description, req.Date, req.Priority, req.TimeBlock, req.QuestID,
	).Scan(
		&goal.ID, &goal.UserID, &goal.Title, &goal.Description, &goal.Date,
		&goal.Priority, &goal.TimeBlock, &scheduledStart, &scheduledEnd,
		&goal.Status, &goal.IsAIScheduled, &goal.QuestID, &goal.CreatedAt, &goal.UpdatedAt,
	)

	if err != nil {
		http.Error(w, "Failed to create goal: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if scheduledStart != nil {
		s := scheduledStart.Format("15:04")
		goal.ScheduledStart = &s
	}
	if scheduledEnd != nil {
		e := scheduledEnd.Format("15:04")
		goal.ScheduledEnd = &e
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(goal)
}

func (h *Handler) UpdateDailyGoal(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	goalID := chi.URLParam(r, "id")

	var req UpdateDailyGoalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	query := `
		UPDATE daily_goals
		SET title = COALESCE($3, title),
		    description = COALESCE($4, description),
		    priority = COALESCE($5, priority),
		    time_block = COALESCE($6, time_block),
		    status = COALESCE($7, status),
		    scheduled_start = COALESCE($8::time, scheduled_start),
		    scheduled_end = COALESCE($9::time, scheduled_end),
		    quest_id = COALESCE($10, quest_id),
		    updated_at = NOW()
		WHERE id = $1 AND user_id = $2
		RETURNING id, user_id, title, description, date, priority, time_block,
		          scheduled_start, scheduled_end, status, is_ai_scheduled, quest_id, created_at, updated_at
	`

	var goal DailyGoal
	var scheduledStart, scheduledEnd *time.Time
	err := h.db.Pool.QueryRow(r.Context(), query,
		goalID, userID, req.Title, req.Description, req.Priority, req.TimeBlock,
		req.Status, req.ScheduledStart, req.ScheduledEnd, req.QuestID,
	).Scan(
		&goal.ID, &goal.UserID, &goal.Title, &goal.Description, &goal.Date,
		&goal.Priority, &goal.TimeBlock, &scheduledStart, &scheduledEnd,
		&goal.Status, &goal.IsAIScheduled, &goal.QuestID, &goal.CreatedAt, &goal.UpdatedAt,
	)

	if err != nil {
		http.Error(w, "Failed to update goal", http.StatusInternalServerError)
		return
	}

	if scheduledStart != nil {
		s := scheduledStart.Format("15:04")
		goal.ScheduledStart = &s
	}
	if scheduledEnd != nil {
		e := scheduledEnd.Format("15:04")
		goal.ScheduledEnd = &e
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(goal)
}

func (h *Handler) DeleteDailyGoal(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	goalID := chi.URLParam(r, "id")

	_, err := h.db.Pool.Exec(r.Context(),
		"DELETE FROM daily_goals WHERE id = $1 AND user_id = $2",
		goalID, userID,
	)

	if err != nil {
		http.Error(w, "Failed to delete goal", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) CompleteDailyGoal(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	goalID := chi.URLParam(r, "id")

	query := `
		UPDATE daily_goals
		SET status = 'done', updated_at = NOW()
		WHERE id = $1 AND user_id = $2
		RETURNING id, user_id, title, description, date, priority, time_block,
		          scheduled_start, scheduled_end, status, is_ai_scheduled, quest_id, created_at, updated_at
	`

	var goal DailyGoal
	var scheduledStart, scheduledEnd *time.Time
	err := h.db.Pool.QueryRow(r.Context(), query, goalID, userID).Scan(
		&goal.ID, &goal.UserID, &goal.Title, &goal.Description, &goal.Date,
		&goal.Priority, &goal.TimeBlock, &scheduledStart, &scheduledEnd,
		&goal.Status, &goal.IsAIScheduled, &goal.QuestID, &goal.CreatedAt, &goal.UpdatedAt,
	)

	if err != nil {
		http.Error(w, "Failed to complete goal", http.StatusInternalServerError)
		return
	}

	if scheduledStart != nil {
		s := scheduledStart.Format("15:04")
		goal.ScheduledStart = &s
	}
	if scheduledEnd != nil {
		e := scheduledEnd.Format("15:04")
		goal.ScheduledEnd = &e
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(goal)
}

func (h *Handler) GetGoalSubtasks(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	goalID := chi.URLParam(r, "id")

	query := `
		SELECT gs.id, gs.daily_goal_id, gs.task_id, gs.title, gs.estimated_minutes,
		       gs.scheduled_start, gs.scheduled_end, gs.position, gs.status, gs.created_at
		FROM goal_subtasks gs
		JOIN daily_goals dg ON dg.id = gs.daily_goal_id
		WHERE gs.daily_goal_id = $1 AND dg.user_id = $2
		ORDER BY gs.position
	`

	rows, err := h.db.Pool.Query(r.Context(), query, goalID, userID)
	if err != nil {
		http.Error(w, "Failed to get subtasks", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var subtasks []Subtask
	for rows.Next() {
		var s Subtask
		var scheduledStart, scheduledEnd *time.Time
		err := rows.Scan(
			&s.ID, &s.DailyGoalID, &s.TaskID, &s.Title, &s.EstimatedMinutes,
			&scheduledStart, &scheduledEnd, &s.Position, &s.Status, &s.CreatedAt,
		)
		if err != nil {
			continue
		}
		if scheduledStart != nil {
			str := scheduledStart.Format("15:04")
			s.ScheduledStart = &str
		}
		if scheduledEnd != nil {
			str := scheduledEnd.Format("15:04")
			s.ScheduledEnd = &str
		}
		subtasks = append(subtasks, s)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(subtasks)
}

func (h *Handler) ScheduleGoalsToCalendar(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req ScheduleGoalsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Date == "" {
		req.Date = time.Now().Format("2006-01-02")
	}

	// Get goals to schedule
	var goals []DailyGoal
	var err error

	if len(req.GoalIDs) > 0 {
		goals, err = h.getGoalsByIDs(userID, req.GoalIDs)
	} else {
		goals, err = h.getPendingGoalsForDate(userID, req.Date)
	}

	if err != nil {
		http.Error(w, "Failed to get goals", http.StatusInternalServerError)
		return
	}

	if len(goals) == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message":     "No goals to schedule",
			"time_blocks": []TimeBlock{},
		})
		return
	}

	// Use Grok to schedule
	timeBlocks, err := h.scheduleGoalsWithGrok(userID, req.Date, goals)
	if err != nil {
		http.Error(w, "Failed to schedule goals: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":     "Goals scheduled successfully",
		"time_blocks": timeBlocks,
	})
}

// ============================================
// Helper Methods
// ============================================

func (h *Handler) getUserQuests(userID string) ([]Quest, error) {
	query := `
		SELECT id, title, description, status
		FROM quests
		WHERE user_id = $1 AND status = 'active'
	`

	rows, err := h.db.Pool.Query(context.Background(), query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var quests []Quest
	for rows.Next() {
		var q Quest
		if err := rows.Scan(&q.ID, &q.Title, &q.Description, &q.Status); err != nil {
			continue
		}
		quests = append(quests, q)
	}

	return quests, nil
}

func (h *Handler) saveIntentLog(userID, rawText string, intent *IntentResponse) (*IntentLog, error) {
	id := uuid.New().String()
	aiResponseJSON, _ := json.Marshal(intent)

	query := `
		INSERT INTO intent_logs (id, user_id, raw_user_text, intent_type, ai_response, notes, follow_up_question)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, user_id, raw_user_text, intent_type, ai_response, notes, follow_up_question, processed_at, created_at
	`

	var log IntentLog
	err := h.db.Pool.QueryRow(context.Background(), query,
		id, userID, rawText, intent.IntentType, aiResponseJSON, intent.Notes, intent.FollowUpQuestion,
	).Scan(
		&log.ID, &log.UserID, &log.RawUserText, &log.IntentType,
		&log.AIResponse, &log.Notes, &log.FollowUpQuestion, &log.ProcessedAt, &log.CreatedAt,
	)

	if err != nil {
		return nil, err
	}

	return &log, nil
}

func (h *Handler) markIntentProcessed(intentID string) {
	h.db.Pool.Exec(context.Background(),
		"UPDATE intent_logs SET processed_at = NOW() WHERE id = $1",
		intentID,
	)
}

func (h *Handler) createGoalsFromIntent(userID, intentLogID string, intent *IntentResponse, quests []Quest) ([]DailyGoal, error) {
	var goals []DailyGoal

	for _, g := range intent.Goals {
		id := uuid.New().String()

		// Try to match goal to a quest
		questID := h.matchGoalToQuest(g.Title, quests)

		query := `
			INSERT INTO daily_goals (id, user_id, intent_log_id, quest_id, title, date, priority, time_block, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING id, user_id, intent_log_id, quest_id, title, date, priority, time_block,
			          scheduled_start, scheduled_end, status, is_ai_scheduled, created_at, updated_at
		`

		var goal DailyGoal
		var scheduledStart, scheduledEnd *time.Time
		err := h.db.Pool.QueryRow(context.Background(), query,
			id, userID, intentLogID, questID, g.Title, g.Date, g.Priority, g.TimeBlock, g.Status,
		).Scan(
			&goal.ID, &goal.UserID, &goal.IntentLogID, &goal.QuestID, &goal.Title,
			&goal.Date, &goal.Priority, &goal.TimeBlock, &scheduledStart, &scheduledEnd,
			&goal.Status, &goal.IsAIScheduled, &goal.CreatedAt, &goal.UpdatedAt,
		)

		if err != nil {
			continue
		}

		if scheduledStart != nil {
			s := scheduledStart.Format("15:04")
			goal.ScheduledStart = &s
		}
		if scheduledEnd != nil {
			e := scheduledEnd.Format("15:04")
			goal.ScheduledEnd = &e
		}

		// Get quest title if matched
		if questID != nil {
			for _, q := range quests {
				if q.ID == *questID {
					goal.QuestTitle = &q.Title
					break
				}
			}
		}

		goals = append(goals, goal)
	}

	return goals, nil
}

func (h *Handler) matchGoalToQuest(goalTitle string, quests []Quest) *string {
	// Simple keyword matching - could be enhanced with AI
	goalLower := strings.ToLower(goalTitle)

	for _, quest := range quests {
		questLower := strings.ToLower(quest.Title)
		descLower := ""
		if quest.Description != nil {
			descLower = strings.ToLower(*quest.Description)
		}

		// Check if goal contains quest title words
		questWords := strings.Fields(questLower)
		matchCount := 0
		for _, word := range questWords {
			if len(word) > 3 && strings.Contains(goalLower, word) {
				matchCount++
			}
		}

		if matchCount >= 1 || strings.Contains(goalLower, questLower) || strings.Contains(descLower, goalLower) {
			return &quest.ID
		}
	}

	return nil
}

func (h *Handler) getDailyGoalsByDate(userID, date string) ([]DailyGoal, error) {
	query := `
		SELECT dg.id, dg.user_id, dg.intent_log_id, dg.quest_id, q.title as quest_title,
		       dg.day_plan_id, dg.title, dg.description, dg.date, dg.priority, dg.time_block,
		       dg.scheduled_start, dg.scheduled_end, dg.status, dg.is_ai_scheduled,
		       dg.created_at, dg.updated_at
		FROM daily_goals dg
		LEFT JOIN quests q ON q.id = dg.quest_id
		WHERE dg.user_id = $1 AND dg.date = $2
		ORDER BY
			CASE dg.time_block
				WHEN 'morning' THEN 1
				WHEN 'afternoon' THEN 2
				WHEN 'evening' THEN 3
				ELSE 4
			END,
			dg.scheduled_start NULLS LAST,
			dg.priority DESC
	`

	rows, err := h.db.Pool.Query(context.Background(), query, userID, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var goals []DailyGoal
	for rows.Next() {
		var g DailyGoal
		var scheduledStart, scheduledEnd *time.Time
		err := rows.Scan(
			&g.ID, &g.UserID, &g.IntentLogID, &g.QuestID, &g.QuestTitle,
			&g.DayPlanID, &g.Title, &g.Description, &g.Date, &g.Priority, &g.TimeBlock,
			&scheduledStart, &scheduledEnd, &g.Status, &g.IsAIScheduled,
			&g.CreatedAt, &g.UpdatedAt,
		)
		if err != nil {
			continue
		}

		if scheduledStart != nil {
			s := scheduledStart.Format("15:04")
			g.ScheduledStart = &s
		}
		if scheduledEnd != nil {
			e := scheduledEnd.Format("15:04")
			g.ScheduledEnd = &e
		}

		goals = append(goals, g)
	}

	return goals, nil
}

func (h *Handler) getGoalsByIDs(userID string, goalIDs []string) ([]DailyGoal, error) {
	query := `
		SELECT dg.id, dg.user_id, dg.intent_log_id, dg.quest_id, q.title as quest_title,
		       dg.day_plan_id, dg.title, dg.description, dg.date, dg.priority, dg.time_block,
		       dg.scheduled_start, dg.scheduled_end, dg.status, dg.is_ai_scheduled,
		       dg.created_at, dg.updated_at
		FROM daily_goals dg
		LEFT JOIN quests q ON q.id = dg.quest_id
		WHERE dg.user_id = $1 AND dg.id = ANY($2)
	`

	rows, err := h.db.Pool.Query(context.Background(), query, userID, goalIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var goals []DailyGoal
	for rows.Next() {
		var g DailyGoal
		var scheduledStart, scheduledEnd *time.Time
		err := rows.Scan(
			&g.ID, &g.UserID, &g.IntentLogID, &g.QuestID, &g.QuestTitle,
			&g.DayPlanID, &g.Title, &g.Description, &g.Date, &g.Priority, &g.TimeBlock,
			&scheduledStart, &scheduledEnd, &g.Status, &g.IsAIScheduled,
			&g.CreatedAt, &g.UpdatedAt,
		)
		if err != nil {
			continue
		}

		if scheduledStart != nil {
			s := scheduledStart.Format("15:04")
			g.ScheduledStart = &s
		}
		if scheduledEnd != nil {
			e := scheduledEnd.Format("15:04")
			g.ScheduledEnd = &e
		}

		goals = append(goals, g)
	}

	return goals, nil
}

func (h *Handler) getPendingGoalsForDate(userID, date string) ([]DailyGoal, error) {
	query := `
		SELECT dg.id, dg.user_id, dg.intent_log_id, dg.quest_id, q.title as quest_title,
		       dg.day_plan_id, dg.title, dg.description, dg.date, dg.priority, dg.time_block,
		       dg.scheduled_start, dg.scheduled_end, dg.status, dg.is_ai_scheduled,
		       dg.created_at, dg.updated_at
		FROM daily_goals dg
		LEFT JOIN quests q ON q.id = dg.quest_id
		WHERE dg.user_id = $1 AND dg.date = $2 AND dg.status = 'pending' AND dg.scheduled_start IS NULL
	`

	rows, err := h.db.Pool.Query(context.Background(), query, userID, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var goals []DailyGoal
	for rows.Next() {
		var g DailyGoal
		var scheduledStart, scheduledEnd *time.Time
		err := rows.Scan(
			&g.ID, &g.UserID, &g.IntentLogID, &g.QuestID, &g.QuestTitle,
			&g.DayPlanID, &g.Title, &g.Description, &g.Date, &g.Priority, &g.TimeBlock,
			&scheduledStart, &scheduledEnd, &g.Status, &g.IsAIScheduled,
			&g.CreatedAt, &g.UpdatedAt,
		)
		if err != nil {
			continue
		}

		goals = append(goals, g)
	}

	return goals, nil
}

func (h *Handler) generateTTSResponse(intent *IntentResponse, goals []DailyGoal) string {
	if len(goals) == 0 {
		return "Je n'ai pas pu créer d'objectifs à partir de ta demande."
	}

	response := "J'ai ajouté "
	if len(goals) == 1 {
		response += "un objectif : " + goals[0].Title
	} else {
		response += fmt.Sprintf("%d objectifs : ", len(goals))
		for i, g := range goals {
			if i > 0 {
				if i == len(goals)-1 {
					response += " et "
				} else {
					response += ", "
				}
			}
			response += g.Title
		}
	}

	return response + "."
}

func (h *Handler) generateListTTSResponse(goals []DailyGoal) string {
	if len(goals) == 0 {
		return "Tu n'as pas encore d'objectifs pour aujourd'hui. Dis-moi ce que tu veux accomplir !"
	}

	response := fmt.Sprintf("Tu as %d objectif", len(goals))
	if len(goals) > 1 {
		response += "s"
	}
	response += " pour aujourd'hui : "

	for i, g := range goals {
		if i > 0 {
			if i == len(goals)-1 {
				response += " et "
			} else {
				response += ", "
			}
		}
		response += g.Title
	}

	return response + "."
}

type Quest struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Description *string `json:"description"`
	Status      string  `json:"status"`
}
