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
	Reply      string  `json:"reply"`
	Tool       *string `json:"tool,omitempty"` // planDay, weeklyGoals, dailyReflection, startFocus, viewStats, logMood
	MessageID  string  `json:"message_id"`
}

// Coach persona
const coachSystemPrompt = `Tu es Kai, un coach personnel exigeant mais bienveillant.

PERSONNALITÉ:
- Tu tutoies toujours l'utilisateur
- Tu es direct et concis - pas de blabla
- Tu pousses à l'action, pas aux excuses
- Tu célèbres les victoires, même petites
- Tu utilises un langage motivant mais jamais condescendant
- Tu parles en français

STYLE DE RÉPONSE:
- Maximum 2-3 phrases par réponse
- Pas d'emojis excessifs (1-2 max si nécessaire)
- Pose des questions orientées action
- Propose des solutions concrètes

OUTILS DISPONIBLES:
Tu peux suggérer ces outils quand c'est pertinent (retourne le nom dans le champ "tool"):
- planDay: Planifier la journée (matin idéalement)
- weeklyGoals: Définir les objectifs de la semaine
- dailyReflection: Réflexion de fin de journée
- startFocus: Lancer une session de focus
- viewStats: Voir les statistiques
- logMood: Logger son humeur

CONTEXTE UTILISATEUR (fourni avec chaque message):
- Nom, streak actuel, tâches du jour, rituels, objectifs hebdo, minutes focus

Réponds UNIQUEMENT en JSON avec ce format:
{
  "reply": "Ta réponse ici",
  "tool": null ou "nomDuOutil"
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

	// Save user message
	userMsgID := uuid.New().String()
	_, err := h.db.Exec(r.Context(), `
		INSERT INTO chat_messages (id, user_id, content, is_from_user, message_type, voice_url, voice_transcript)
		VALUES ($1, $2, $3, true, $4, $5, $6)
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

	// Save AI response
	aiMsgID := uuid.New().String()
	_, err = h.db.Exec(r.Context(), `
		INSERT INTO chat_messages (id, user_id, content, is_from_user, message_type, tool_action)
		VALUES ($1, $2, $3, false, 'text', $4)
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
		Reply string  `json:"reply"`
		Tool  *string `json:"tool"`
	}

	if err := json.Unmarshal([]byte(responseText), &aiResp); err != nil {
		// If JSON parsing fails, use raw text as reply
		return &SendMessageResponse{
			Reply: responseText,
		}, nil
	}

	return &SendMessageResponse{
		Reply: aiResp.Reply,
		Tool:  aiResp.Tool,
	}, nil
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
