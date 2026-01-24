package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"firelevel-backend/internal/auth"

	"github.com/go-chi/chi/v5"
	"github.com/google/generative-ai-go/genai"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/api/option"
)

type Handler struct {
	db *pgxpool.Pool
}

func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/chat/message", h.SendMessage)
	r.Get("/chat/history", h.GetHistory)
	r.Delete("/chat/history", h.ClearHistory)
}

// ============================================
// Request/Response Types
// ============================================

type SendMessageRequest struct {
	Content      string        `json:"content"`
	MessageType  string        `json:"message_type,omitempty"`   // text, voice
	VoiceURL     string        `json:"voice_url,omitempty"`      // For voice messages
	Context      *ChatContext  `json:"context,omitempty"`        // User context for AI
}

type ChatContext struct {
	UserName             string `json:"user_name"`
	CurrentStreak        int    `json:"current_streak"`
	TodayTasksCount      int    `json:"today_tasks_count"`
	TodayTasksCompleted  int    `json:"today_tasks_completed"`
	TodayRitualsCount    int    `json:"today_rituals_count"`
	TodayRitualsCompleted int   `json:"today_rituals_completed"`
	WeeklyGoalsCount     int    `json:"weekly_goals_count"`
	WeeklyGoalsCompleted int    `json:"weekly_goals_completed"`
	FocusMinutesToday    int    `json:"focus_minutes_today"`
	FocusMinutesWeek     int    `json:"focus_minutes_week"`
	TimeOfDay            string `json:"time_of_day"` // morning, afternoon, evening, night
	DayOfWeek            string `json:"day_of_week"`
	CurrentMood          *int   `json:"current_mood,omitempty"` // 1-5
}

type ChatMessage struct {
	ID              string     `json:"id"`
	UserID          string     `json:"user_id"`
	Content         string     `json:"content"`
	IsFromUser      bool       `json:"is_from_user"`
	MessageType     string     `json:"message_type"`
	VoiceURL        *string    `json:"voice_url,omitempty"`
	VoiceTranscript *string    `json:"voice_transcript,omitempty"`
	ToolAction      *string    `json:"tool_action,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

type SendMessageResponse struct {
	Reply      string       `json:"reply"`
	Tool       *string      `json:"tool,omitempty"` // planDay, weeklyGoals, dailyReflection, startFocus, viewStats, logMood
	MessageID  string       `json:"message_id"`
	Action     *ActionData  `json:"action,omitempty"` // Action taken by AI (task created, etc.)
}

// ActionData represents an action taken by Kai
type ActionData struct {
	Type     string      `json:"type"`               // "task_created", "focus_scheduled"
	TaskID   *string     `json:"task_id,omitempty"`
	TaskData *TaskData   `json:"task,omitempty"`
}

type TaskData struct {
	Title          string  `json:"title"`
	Date           string  `json:"date"`
	ScheduledStart string  `json:"scheduled_start"`
	ScheduledEnd   string  `json:"scheduled_end"`
	BlockApps      bool    `json:"block_apps"`
}

// FocusIntent represents a detected focus request
type FocusIntent struct {
	Detected       bool
	Title          string
	StartTime      string // HH:mm
	EndTime        string // HH:mm
	Date           string // YYYY-MM-DD
	BlockApps      bool
	DurationMins   int
}

// Coach persona
const coachSystemPrompt = `Tu es Kai, un ami proche qui aide avec la productivité.

PERSONNALITÉ:
- Tu tutoies toujours l'utilisateur
- Tu es empathique et authentique - comme un vrai ami
- Tu écoutes avant de conseiller
- Tu célèbres les victoires sincèrement
- Tu parles en français naturellement

STYLE DE RÉPONSE:
- Maximum 2-3 phrases par réponse
- Ton naturel, pas de formules corporate
- 1 emoji max si ça aide

CAPACITÉ SPÉCIALE - DÉTECTION DE FOCUS:
Si l'utilisateur mentionne vouloir travailler/se concentrer avec des horaires (ex: "je dois bosser de 14h à 16h", "je veux focus de 9h à 11h30"), tu DOIS retourner:
{
  "reply": "C'est noté! Je bloque tes apps de [heure] à [heure]. 🔒",
  "focus_intent": {
    "detected": true,
    "title": "Focus - [description courte]",
    "start_time": "HH:MM",
    "end_time": "HH:MM",
    "block_apps": true
  }
}

Exemples de détection:
- "je dois bosser de 14h à 16h" → start_time: "14:00", end_time: "16:00"
- "focus de 9h30 à 11h" → start_time: "09:30", end_time: "11:00"
- "je veux travailler pendant 2h" → start_time: maintenant, end_time: +2h
- "coupe mes apps stp" sans horaires → demande les horaires

Réponds UNIQUEMENT en JSON:
{
  "reply": "Ta réponse",
  "tool": null,
  "focus_intent": null ou { "detected": true, "title": "...", "start_time": "HH:MM", "end_time": "HH:MM", "block_apps": true }
}`

// ============================================
// Handlers
// ============================================

func (h *Handler) SendMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Content == "" {
		http.Error(w, "Content is required", http.StatusBadRequest)
		return
	}

	if req.MessageType == "" {
		req.MessageType = "text"
	}

	// Save user message (source = 'app' for iOS app)
	userMsgID := uuid.New().String()
	_, err := h.db.Exec(r.Context(), `
		INSERT INTO chat_messages (id, user_id, content, is_from_user, message_type, voice_url, voice_transcript, source)
		VALUES ($1, $2, $3, true, $4, $5, $6, 'app')
	`, userMsgID, userID, req.Content, req.MessageType, nilIfEmpty(req.VoiceURL), nilIfEmpty(req.Content))

	if err != nil {
		fmt.Printf("Failed to save user message: %v\n", err)
	}

	// Get recent conversation history for context
	history, err := h.getRecentHistory(r.Context(), userID, 10)
	if err != nil {
		fmt.Printf("Failed to get history: %v\n", err)
	}

	// Generate AI response
	response, err := h.generateAIResponse(r.Context(), req.Content, req.Context, history)
	if err != nil {
		fmt.Printf("AI error: %v\n", err)
		response = &SendMessageResponse{
			Reply: "Je rencontre un problème technique. Réessaie dans un moment.",
		}
	}

	// If focus intent detected, create the task automatically
	if response.Action != nil && response.Action.Type == "focus_scheduled" && response.Action.TaskData != nil {
		taskID, err := h.createFocusTask(r.Context(), userID, response.Action.TaskData)
		if err != nil {
			fmt.Printf("Failed to create focus task: %v\n", err)
		} else {
			response.Action.TaskID = &taskID
			response.Action.Type = "task_created"
			fmt.Printf("✅ Created focus task %s for user %s: %s (%s - %s)\n",
				taskID, userID, response.Action.TaskData.Title,
				response.Action.TaskData.ScheduledStart, response.Action.TaskData.ScheduledEnd)
		}
	}

	// Save AI response (source = 'app' for iOS app)
	aiMsgID := uuid.New().String()
	_, err = h.db.Exec(r.Context(), `
		INSERT INTO chat_messages (id, user_id, content, is_from_user, message_type, tool_action, source)
		VALUES ($1, $2, $3, false, 'text', $4, 'app')
	`, aiMsgID, userID, response.Reply, response.Tool)

	if err != nil {
		fmt.Printf("Failed to save AI message: %v\n", err)
	}

	response.MessageID = aiMsgID

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *Handler) GetHistory(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get limit from query, default 50
	limit := 50

	messages, err := h.getRecentHistory(r.Context(), userID, limit)
	if err != nil {
		http.Error(w, "Failed to get history", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

func (h *Handler) ClearHistory(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	_, err := h.db.Exec(r.Context(), `DELETE FROM chat_messages WHERE user_id = $1`, userID)
	if err != nil {
		http.Error(w, "Failed to clear history", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ============================================
// Helper Methods
// ============================================

func (h *Handler) getRecentHistory(ctx context.Context, userID string, limit int) ([]ChatMessage, error) {
	// Fetch messages from both app and WhatsApp sources (unified history)
	query := `
		SELECT id, user_id, content, is_from_user, message_type,
		       voice_url, voice_transcript, tool_action, created_at
		FROM chat_messages
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`

	rows, err := h.db.Query(ctx, query, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []ChatMessage
	for rows.Next() {
		var m ChatMessage
		err := rows.Scan(
			&m.ID, &m.UserID, &m.Content, &m.IsFromUser, &m.MessageType,
			&m.VoiceURL, &m.VoiceTranscript, &m.ToolAction, &m.CreatedAt,
		)
		if err != nil {
			continue
		}
		messages = append(messages, m)
	}

	// Reverse to get chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

func (h *Handler) generateAIResponse(ctx context.Context, userMessage string, userContext *ChatContext, history []ChatMessage) (*SendMessageResponse, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY not set")
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}
	defer client.Close()

	model := client.GenerativeModel("gemini-1.5-flash")
	model.SetTemperature(0.7)
	model.SetMaxOutputTokens(500)

	// Build context string
	contextStr := ""
	if userContext != nil {
		contextStr = fmt.Sprintf(`
CONTEXTE DE L'UTILISATEUR:
- Nom: %s
- Streak actuel: %d jours
- Tâches aujourd'hui: %d/%d complétées
- Rituels aujourd'hui: %d/%d complétés
- Objectifs semaine: %d/%d complétés
- Focus aujourd'hui: %d minutes
- Focus cette semaine: %d minutes
- Moment: %s (%s)
`,
			userContext.UserName,
			userContext.CurrentStreak,
			userContext.TodayTasksCompleted, userContext.TodayTasksCount,
			userContext.TodayRitualsCompleted, userContext.TodayRitualsCount,
			userContext.WeeklyGoalsCompleted, userContext.WeeklyGoalsCount,
			userContext.FocusMinutesToday,
			userContext.FocusMinutesWeek,
			userContext.TimeOfDay, userContext.DayOfWeek,
		)
	}

	// Build conversation history
	historyStr := ""
	if len(history) > 0 {
		historyStr = "\nHISTORIQUE RÉCENT:\n"
		for _, m := range history {
			if m.IsFromUser {
				historyStr += fmt.Sprintf("Utilisateur: %s\n", m.Content)
			} else {
				historyStr += fmt.Sprintf("Kai: %s\n", m.Content)
			}
		}
	}

	prompt := fmt.Sprintf(`%s
%s
%s
MESSAGE DE L'UTILISATEUR: %s

Réponds en JSON:`, coachSystemPrompt, contextStr, historyStr, userMessage)

	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return nil, fmt.Errorf("failed to generate content: %w", err)
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("empty response from Gemini")
	}

	// Extract text from response
	responseText := ""
	for _, part := range resp.Candidates[0].Content.Parts {
		if text, ok := part.(genai.Text); ok {
			responseText += string(text)
		}
	}

	// Parse JSON response
	responseText = strings.TrimSpace(responseText)
	// Remove markdown code blocks if present
	responseText = strings.TrimPrefix(responseText, "```json")
	responseText = strings.TrimPrefix(responseText, "```")
	responseText = strings.TrimSuffix(responseText, "```")
	responseText = strings.TrimSpace(responseText)

	var aiResp struct {
		Reply       string  `json:"reply"`
		Tool        *string `json:"tool"`
		FocusIntent *struct {
			Detected  bool   `json:"detected"`
			Title     string `json:"title"`
			StartTime string `json:"start_time"`
			EndTime   string `json:"end_time"`
			BlockApps bool   `json:"block_apps"`
		} `json:"focus_intent"`
	}

	if err := json.Unmarshal([]byte(responseText), &aiResp); err != nil {
		// If JSON parsing fails, use raw text as reply
		return &SendMessageResponse{
			Reply: responseText,
		}, nil
	}

	response := &SendMessageResponse{
		Reply: aiResp.Reply,
		Tool:  aiResp.Tool,
	}

	// If focus intent detected, prepare action data for iOS
	if aiResp.FocusIntent != nil && aiResp.FocusIntent.Detected {
		response.Action = &ActionData{
			Type: "focus_scheduled",
			TaskData: &TaskData{
				Title:          aiResp.FocusIntent.Title,
				Date:           time.Now().Format("2006-01-02"),
				ScheduledStart: aiResp.FocusIntent.StartTime,
				ScheduledEnd:   aiResp.FocusIntent.EndTime,
				BlockApps:      aiResp.FocusIntent.BlockApps,
			},
		}
	}

	return response, nil
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// createFocusTask creates a task with blockApps enabled
func (h *Handler) createFocusTask(ctx context.Context, userID string, taskData *TaskData) (string, error) {
	// Determine time block based on start time
	timeBlock := "morning"
	if taskData.ScheduledStart != "" {
		hour := 0
		fmt.Sscanf(taskData.ScheduledStart, "%d:", &hour)
		if hour >= 12 && hour < 18 {
			timeBlock = "afternoon"
		} else if hour >= 18 {
			timeBlock = "evening"
		}
	}

	var taskID string
	err := h.db.QueryRow(ctx, `
		INSERT INTO tasks (
			user_id, title, date, scheduled_start, scheduled_end,
			time_block, priority, is_ai_generated, ai_notes, block_apps
		) VALUES (
			$1, $2, $3, $4::time, $5::time,
			$6, 'high', true, 'Créé automatiquement par Kai', true
		)
		RETURNING id
	`, userID, taskData.Title, taskData.Date, taskData.ScheduledStart, taskData.ScheduledEnd, timeBlock).Scan(&taskID)

	if err != nil {
		return "", fmt.Errorf("failed to create focus task: %w", err)
	}

	return taskID, nil
}
