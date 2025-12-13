package voice

import (
	"bytes"
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
)

type AIService struct {
	geminiAPIKey  string
	gradiumAPIKey string
	httpClient    *http.Client
}

func NewAIService() *AIService {
	return &AIService{
		geminiAPIKey:  os.Getenv("GEMINI_API_KEY"),
		gradiumAPIKey: os.Getenv("GRADIUM_API_KEY"),
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// ============================================
// Gemini AI - For intent extraction
// ============================================

type GeminiRequest struct {
	Contents         []GeminiContent        `json:"contents"`
	GenerationConfig GeminiGenerationConfig `json:"generationConfig,omitempty"`
}

type GeminiContent struct {
	Parts []GeminiPart `json:"parts"`
	Role  string       `json:"role,omitempty"`
}

type GeminiPart struct {
	Text string `json:"text"`
}

type GeminiGenerationConfig struct {
	Temperature     float64 `json:"temperature,omitempty"`
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
}

type GeminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error,omitempty"`
}

// ExtractIntentions - Analyse le texte utilisateur et extrait les intentions/goals
func (s *AIService) ExtractIntentions(userText, targetDate string, quests []Quest) (*IntentResponse, error) {
	systemPrompt := s.buildIntentSystemPrompt(targetDate, quests)

	return s.callGemini(systemPrompt, userText)
}

func (s *AIService) callGemini(systemPrompt, userText string) (*IntentResponse, error) {
	if s.geminiAPIKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY not configured")
	}

	// Combine system prompt and user text
	fullPrompt := systemPrompt + "\n\nUser input:\n" + userText

	reqBody := GeminiRequest{
		Contents: []GeminiContent{
			{
				Parts: []GeminiPart{{Text: fullPrompt}},
			},
		},
		GenerationConfig: GeminiGenerationConfig{
			Temperature:     0.2,
			MaxOutputTokens: 2048,
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Gemini API URL with API key
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1/models/gemini-2.0-flash:generateContent?key=%s", s.geminiAPIKey)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call Gemini API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp GeminiResponse
		json.NewDecoder(resp.Body).Decode(&errResp)
		if errResp.Error != nil {
			return nil, fmt.Errorf("Gemini API error: %s (code: %d)", errResp.Error.Message, errResp.Error.Code)
		}
		return nil, fmt.Errorf("Gemini API returned status %d", resp.StatusCode)
	}

	var geminiResp GeminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("no response from Gemini")
	}

	content := cleanJSONResponse(geminiResp.Candidates[0].Content.Parts[0].Text)

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

	// Get current hour to make smart suggestions
	currentHour := time.Now().Hour()

	return fmt.Sprintf(`Tu es Volta, un assistant vocal intelligent pour planifier la journée.
Date: %s
Heure actuelle: %02d:00
%s

## Ta mission
1. Comprendre ce que l'utilisateur veut faire
2. TOUJOURS proposer des horaires précis - même si l'utilisateur n'en donne pas
3. Retourner UNIQUEMENT un JSON valide

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
      "date": "YYYY-MM-DD",
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
  "tts_response": "string - résumé en français de ce que tu as planifié"
}

## Exemples

### Exemple 1: Avec horaires précis
User: "Ce matin je vais travailler de 8h à 10h sur du marketing"
→ Tu utilises les horaires donnés: 08:00 - 10:00

### Exemple 2: SANS horaires (tu dois proposer!)
User: "Je veux acheter des vêtements à ma meuf"
→ Tu proposes un créneau logique basé sur l'heure actuelle

{
  "intent_type": "ADD_GOAL",
  "goals": [
    {
      "title": "Shopping vêtements",
      "date": "%s",
      "priority": "medium",
      "time_block": "afternoon",
      "scheduled_start": "15:00",
      "scheduled_end": "17:00",
      "estimated_duration_minutes": 120,
      "status": "pending",
      "quest_id": null
    }
  ],
  "raw_user_text": "Je veux acheter des vêtements à ma meuf",
  "notes": "",
  "follow_up_question": null,
  "tts_response": "J'ai prévu Shopping vêtements de 15h à 17h. Tu peux modifier l'horaire si besoin."
}

### Exemple 3: Plusieurs tâches sans horaires
User: "Aujourd'hui je veux faire du sport et travailler sur mon projet"
→ Tu proposes des créneaux qui s'enchainent logiquement

{
  "intent_type": "ADD_GOAL",
  "goals": [
    {
      "title": "Sport",
      "date": "%s",
      "priority": "medium",
      "time_block": "morning",
      "scheduled_start": "10:00",
      "scheduled_end": "11:30",
      "estimated_duration_minutes": 90,
      "status": "pending",
      "quest_id": null
    },
    {
      "title": "Travail sur projet",
      "date": "%s",
      "priority": "medium",
      "time_block": "afternoon",
      "scheduled_start": "14:00",
      "scheduled_end": "16:00",
      "estimated_duration_minutes": 120,
      "status": "pending",
      "quest_id": null
    }
  ],
  "raw_user_text": "...",
  "notes": "",
  "follow_up_question": null,
  "tts_response": "J'ai planifié: Sport de 10h à 11h30, puis Travail sur projet de 14h à 16h."
}

IMPORTANT:
- Réponds TOUJOURS avec un JSON valide, sans markdown, sans texte autour
- Propose TOUJOURS des horaires même si l'utilisateur n'en donne pas
- Le tts_response doit résumer ce que tu as planifié avec les horaires`, targetDate, currentHour, questContext, currentHour, targetDate, targetDate, targetDate)
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
