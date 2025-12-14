package voice

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"google.golang.org/genai"
)

type AIService struct {
	geminiAPIKey  string
	gradiumAPIKey string
	httpClient    *http.Client
	genaiClient   *genai.Client
}

func NewAIService() *AIService {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  os.Getenv("GEMINI_API_KEY"),
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		// Log error but don't fail - will check client in callGemini
		fmt.Printf("Warning: Failed to create genai client: %v\n", err)
	}

	return &AIService{
		geminiAPIKey:  os.Getenv("GEMINI_API_KEY"),
		gradiumAPIKey: os.Getenv("GRADIUM_API_KEY"),
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		genaiClient: client,
	}
}

// ============================================
// Gemini AI - For intent extraction (using official SDK)
// ============================================

// ExtractIntentions - Analyse le texte utilisateur et extrait les intentions/goals
func (s *AIService) ExtractIntentions(userText, targetDate string, quests []Quest) (*IntentResponse, error) {
	systemPrompt := s.buildIntentSystemPrompt(targetDate, quests)

	return s.callGemini(systemPrompt, userText)
}

func (s *AIService) callGemini(systemPrompt, userText string) (*IntentResponse, error) {
	if s.genaiClient == nil {
		return nil, fmt.Errorf("Gemini client not initialized - check GEMINI_API_KEY")
	}

	ctx := context.Background()

	// Combine system prompt and user text
	fullPrompt := systemPrompt + "\n\nUser input:\n" + userText

	// Use gemini-2.5-flash model via official SDK
	result, err := s.genaiClient.Models.GenerateContent(
		ctx,
		"gemini-2.5-flash",
		genai.Text(fullPrompt),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("Gemini API error: %w", err)
	}

	// Extract text from response
	responseText := result.Text()
	if responseText == "" {
		return nil, fmt.Errorf("no response from Gemini")
	}

	content := cleanJSONResponse(responseText)

	var intentResp IntentResponse
	if err := json.Unmarshal([]byte(content), &intentResp); err != nil {
		return nil, fmt.Errorf("failed to parse intent response: %w (content: %s)", err, content)
	}

	return &intentResp, nil
}

func cleanJSONResponse(content string) string {
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	return strings.TrimSpace(content)
}

func (s *AIService) buildIntentSystemPrompt(targetDate string, quests []Quest) string {
	questContext := ""
	if len(quests) > 0 {
		questContext = "\n\nQuêtes actives de l'utilisateur (associe les objectifs si pertinent):\n"
		for _, q := range quests {
			questContext += fmt.Sprintf("- ID: %s | Titre: %s\n", q.ID, q.Title)
		}
	}

	// Get current time info for smart suggestions
	now := time.Now()
	currentHour := now.Hour()
	weekday := now.Weekday()

	// Calculate dates for relative date references
	// Map French weekday names to Go weekdays
	weekdayNames := map[time.Weekday]string{
		time.Monday:    "lundi",
		time.Tuesday:   "mardi",
		time.Wednesday: "mercredi",
		time.Thursday:  "jeudi",
		time.Friday:    "vendredi",
		time.Saturday:  "samedi",
		time.Sunday:    "dimanche",
	}

	// Calculate next occurrence of each weekday
	weekdayDates := ""
	for i := 0; i < 7; i++ {
		targetDay := time.Weekday((int(weekday) + i) % 7)
		daysUntil := i
		if daysUntil == 0 && targetDay != weekday {
			daysUntil = 7
		}
		futureDate := now.AddDate(0, 0, daysUntil)
		weekdayDates += fmt.Sprintf("- %s → %s\n", weekdayNames[targetDay], futureDate.Format("2006-01-02"))
	}

	tomorrow := now.AddDate(0, 0, 1).Format("2006-01-02")
	afterTomorrow := now.AddDate(0, 0, 2).Format("2006-01-02")

	return fmt.Sprintf(`Tu es Volta, un assistant vocal intelligent pour planifier la journée.
Date d'aujourd'hui: %s
Jour actuel: %s
Heure actuelle: %02d:00
%s

## DATES RELATIVES - TRÈS IMPORTANT

Tu DOIS convertir les références temporelles relatives en dates YYYY-MM-DD:
- "aujourd'hui" → %s
- "demain" → %s
- "après-demain" → %s
- "dans X jours" → calcule la date
- Jours de la semaine → utilise le PROCHAIN occurence:
%s
### Exemples de conversion:
- "mercredi prochain" ou "mercredi" → utilise la date du prochain mercredi
- "ce weekend" → utilise le prochain samedi
- "la semaine prochaine" → ajoute 7 jours

## Ta mission
1. Comprendre ce que l'utilisateur veut faire
2. Identifier la DATE correcte (aujourd'hui ou une autre date mentionnée)
3. TOUJOURS proposer des horaires précis - même si l'utilisateur n'en donne pas
4. Retourner UNIQUEMENT un JSON valide

## RÈGLE IMPORTANTE: Proposer des horaires intelligemment

Si l'utilisateur ne précise PAS d'horaire, tu DOIS en proposer un logique:
- Regarde l'heure actuelle et propose un créneau réaliste
- Si il est %02dh, propose de commencer dans 30min-1h
- Estime la durée selon le type de tâche
- Enchaine les tâches logiquement

### Estimation automatique des durées:
- Shopping/courses → 90-120 min
- Sport/salle → 60-90 min
- Travail sur projet → 120 min
- Emails/admin → 30 min
- Appel téléphonique → 20-30 min
- Réunion → 60 min
- Repas → 60 min
- TikTok/contenu → 90 min
- Marketing → 120 min
- Rendez-vous → 60 min

## Schéma JSON de sortie
{
  "intent_type": "ADD_GOAL",
  "goals": [
    {
      "title": "string - titre court et clair",
      "date": "YYYY-MM-DD (utilise la date relative correcte!)",
      "priority": "low | medium | high",
      "time_block": "morning | afternoon | evening",
      "scheduled_start": "HH:MM",
      "scheduled_end": "HH:MM",
      "estimated_duration_minutes": number,
      "status": "pending",
      "quest_id": null
    }
  ],
  "raw_user_text": "string - texte original",
  "notes": "string - notes optionnelles",
  "follow_up_question": null,
  "tts_response": "string - résumé en français incluant le JOUR (ex: 'mercredi' ou 'demain')"
}

## Exemples

### Exemple 1: Avec date future
User: "Mercredi j'ai un appel à 14h"
→ Utilise la date du prochain mercredi

{
  "intent_type": "ADD_GOAL",
  "goals": [
    {
      "title": "Appel téléphonique",
      "date": "2024-01-17",
      "priority": "medium",
      "time_block": "afternoon",
      "scheduled_start": "14:00",
      "scheduled_end": "14:30",
      "estimated_duration_minutes": 30,
      "status": "pending",
      "quest_id": null
    }
  ],
  "tts_response": "J'ai ajouté ton appel pour mercredi à 14h."
}

### Exemple 2: Demain
User: "Demain je dois aller faire les courses"
→ Utilise la date de demain (%s)

{
  "intent_type": "ADD_GOAL",
  "goals": [
    {
      "title": "Courses",
      "date": "%s",
      "priority": "medium",
      "time_block": "morning",
      "scheduled_start": "10:00",
      "scheduled_end": "11:30",
      "estimated_duration_minutes": 90,
      "status": "pending",
      "quest_id": null
    }
  ],
  "tts_response": "J'ai prévu les courses pour demain de 10h à 11h30."
}

### Exemple 3: Plusieurs tâches, dates différentes
User: "Aujourd'hui sport et vendredi réunion avec l'équipe"
→ Une tâche pour aujourd'hui, une pour vendredi

{
  "intent_type": "ADD_GOAL",
  "goals": [
    {
      "title": "Sport",
      "date": "%s",
      "priority": "medium",
      "time_block": "afternoon",
      "scheduled_start": "15:00",
      "scheduled_end": "16:30",
      "estimated_duration_minutes": 90,
      "status": "pending",
      "quest_id": null
    },
    {
      "title": "Réunion équipe",
      "date": "2024-01-19",
      "priority": "high",
      "time_block": "morning",
      "scheduled_start": "10:00",
      "scheduled_end": "11:00",
      "estimated_duration_minutes": 60,
      "status": "pending",
      "quest_id": null
    }
  ],
  "tts_response": "J'ai planifié Sport pour aujourd'hui de 15h à 16h30, et Réunion équipe pour vendredi de 10h à 11h."
}

IMPORTANT:
- Réponds TOUJOURS avec un JSON valide, sans markdown, sans texte autour
- Propose TOUJOURS des horaires même si l'utilisateur n'en donne pas
- Convertis TOUJOURS les dates relatives (demain, mercredi, etc.) en format YYYY-MM-DD
- Le tts_response doit mentionner le JOUR en français (pas juste la date)`, targetDate, weekdayNames[weekday], currentHour, questContext, targetDate, tomorrow, afterTomorrow, weekdayDates, currentHour, tomorrow, tomorrow, targetDate)
}

// ============================================
// Types pour le scheduling
// ============================================

type ScheduledBlock struct {
	GoalID          string `json:"goal_id"`
	Title           string `json:"title"`
	StartTime       string `json:"start_time"`
	EndTime         string `json:"end_time"`
	DurationMinutes int    `json:"duration_minutes"`
	BlockType       string `json:"block_type"`
	Priority        string `json:"priority"`
	Reasoning       string `json:"reasoning"`
}

type ScheduleResponse struct {
	Blocks    []ScheduledBlock `json:"blocks"`
	Summary   string           `json:"summary"`
	Conflicts []string         `json:"conflicts,omitempty"`
}

// ============================================
// Helper methods on Handler
// ============================================

func (h *Handler) getExistingTimeBlocks(userID, date string) ([]TimeBlock, error) {
	query := `
		SELECT id, title, description, start_time, end_time, status, block_type,
		       quest_id, daily_goal_id, COALESCE(source, 'manual') as source
		FROM time_blocks
		WHERE user_id = $1 AND DATE(start_time) = $2
		ORDER BY start_time
	`

	rows, err := h.db.Query(context.Background(), query, userID, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var blocks []TimeBlock
	for rows.Next() {
		var b TimeBlock
		err := rows.Scan(
			&b.ID, &b.Title, &b.Description, &b.StartTime, &b.EndTime,
			&b.Status, &b.BlockType, &b.QuestID, &b.DailyGoalID, &b.Source,
		)
		if err != nil {
			continue
		}
		blocks = append(blocks, b)
	}

	return blocks, nil
}

func (h *Handler) createTimeBlockFromSchedule(userID, date string, block ScheduledBlock) (*TimeBlock, error) {
	id := uuid.New().String()

	startTime, _ := time.Parse("2006-01-02 15:04", date+" "+block.StartTime)
	endTime, _ := time.Parse("2006-01-02 15:04", date+" "+block.EndTime)

	query := `
		INSERT INTO time_blocks (id, user_id, title, start_time, end_time, block_type, status, daily_goal_id, source, is_ai_generated)
		VALUES ($1, $2, $3, $4, $5, $6, 'scheduled', $7, 'ai_scheduled', true)
		RETURNING id, title, description, start_time, end_time, status, block_type, quest_id, daily_goal_id, source
	`

	var tb TimeBlock
	err := h.db.QueryRow(context.Background(), query,
		id, userID, block.Title, startTime, endTime, block.BlockType, block.GoalID,
	).Scan(
		&tb.ID, &tb.Title, &tb.Description, &tb.StartTime, &tb.EndTime,
		&tb.Status, &tb.BlockType, &tb.QuestID, &tb.DailyGoalID, &tb.Source,
	)

	if err != nil {
		return nil, err
	}

	return &tb, nil
}

func (h *Handler) updateGoalSchedule(goalID, startTime, endTime string) {
	query := `
		UPDATE daily_goals
		SET scheduled_start = $2::time, scheduled_end = $3::time, is_ai_scheduled = true, updated_at = NOW()
		WHERE id = $1
	`
	h.db.Exec(context.Background(), query, goalID, startTime, endTime)
}

// ============================================
// Gradium TTS - Text-to-Speech via WebSocket
// ============================================

// GradiumTTSSetupMessage is sent first to configure the TTS session
type GradiumTTSSetupMessage struct {
	Type         string `json:"type"`
	ModelName    string `json:"model_name"`
	VoiceID      string `json:"voice_id"`
	OutputFormat string `json:"output_format"`
}

// GradiumTTSTextMessage is sent to request text-to-speech conversion
type GradiumTTSTextMessage struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// GradiumTTSEndOfStream signals end of text input
type GradiumTTSEndOfStream struct {
	Type string `json:"type"`
}

// GradiumTTSResponse is received from the server
type GradiumTTSResponse struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id,omitempty"`
	Audio     string `json:"audio,omitempty"`
	Message   string `json:"message,omitempty"`
	Code      int    `json:"code,omitempty"`
}

// GenerateTTS generates speech audio from text using Gradium WebSocket API
// Returns base64 encoded audio data
func (s *AIService) GenerateTTS(text, voiceID, audioFormat string) (string, error) {
	if s.gradiumAPIKey == "" {
		return "", fmt.Errorf("GRADIUM_API_KEY not configured")
	}

	// Use Europe endpoint (user is in France/Switzerland)
	wsURL := "wss://eu.api.gradium.ai/api/speech/tts"

	// Create WebSocket dialer with custom headers
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	// Set API key in header
	headers := http.Header{}
	headers.Set("x-api-key", s.gradiumAPIKey)

	// Connect to WebSocket
	conn, _, err := dialer.Dial(wsURL, headers)
	if err != nil {
		return "", fmt.Errorf("failed to connect to Gradium WebSocket: %w", err)
	}
	defer conn.Close()

	// Send setup message first
	setupMsg := GradiumTTSSetupMessage{
		Type:         "setup",
		ModelName:    "default",
		VoiceID:      voiceID,
		OutputFormat: audioFormat,
	}

	if err := conn.WriteJSON(setupMsg); err != nil {
		return "", fmt.Errorf("failed to send setup message: %w", err)
	}

	// Wait for ready response
	var readyResp GradiumTTSResponse
	if err := conn.ReadJSON(&readyResp); err != nil {
		return "", fmt.Errorf("failed to read ready response: %w", err)
	}

	if readyResp.Type == "error" {
		return "", fmt.Errorf("Gradium error: %s (code: %d)", readyResp.Message, readyResp.Code)
	}

	if readyResp.Type != "ready" {
		return "", fmt.Errorf("expected ready message, got: %s", readyResp.Type)
	}

	// Send text message
	textMsg := GradiumTTSTextMessage{
		Type: "text",
		Text: text,
	}

	if err := conn.WriteJSON(textMsg); err != nil {
		return "", fmt.Errorf("failed to send text message: %w", err)
	}

	// Send end of stream
	eosMsg := GradiumTTSEndOfStream{
		Type: "end_of_stream",
	}

	if err := conn.WriteJSON(eosMsg); err != nil {
		return "", fmt.Errorf("failed to send end_of_stream: %w", err)
	}

	// Collect all audio chunks
	var audioChunks []string

	for {
		var resp GradiumTTSResponse
		if err := conn.ReadJSON(&resp); err != nil {
			// Connection closed or error
			break
		}

		switch resp.Type {
		case "audio":
			audioChunks = append(audioChunks, resp.Audio)
		case "end_of_stream":
			// All audio received
			goto done
		case "error":
			return "", fmt.Errorf("Gradium TTS error: %s (code: %d)", resp.Message, resp.Code)
		}
	}

done:
	if len(audioChunks) == 0 {
		return "", fmt.Errorf("no audio received from Gradium")
	}

	// Decode all base64 chunks, combine, then re-encode
	var combinedAudio []byte
	for _, chunk := range audioChunks {
		decoded, err := base64.StdEncoding.DecodeString(chunk)
		if err != nil {
			// Try using the chunk as-is if not base64
			combinedAudio = append(combinedAudio, []byte(chunk)...)
			continue
		}
		combinedAudio = append(combinedAudio, decoded...)
	}

	return base64.StdEncoding.EncodeToString(combinedAudio), nil
}
