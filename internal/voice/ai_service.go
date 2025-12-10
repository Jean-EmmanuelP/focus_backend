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
	perplexityAPIKey string
	gradiumAPIKey    string
	httpClient       *http.Client
}

func NewAIService() *AIService {
	return &AIService{
		perplexityAPIKey: os.Getenv("PERPLEXITY_API_KEY"),
		gradiumAPIKey:    os.Getenv("GRADIUM_API_KEY"),
		httpClient: &http.Client{
			Timeout: 60 * time.Second, // Increased for TTS generation
		},
	}
}

// ============================================
// Perplexity AI - Single LLM for everything
// ============================================

type PerplexityRequest struct {
	Model    string          `json:"model"`
	Messages []PerplexityMsg `json:"messages"`
}

type PerplexityMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type PerplexityResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// ExtractIntentions - Premier appel pour extraire les intentions
func (s *AIService) ExtractIntentions(userText, targetDate string, quests []Quest) (*IntentResponse, error) {
	systemPrompt := s.buildIntentSystemPrompt(targetDate, quests)

	return s.callPerplexity(systemPrompt, userText)
}

// ContinueConversation - Pour les tours suivants de conversation
func (s *AIService) ContinueConversation(conversationHistory []PerplexityMsg, userText, targetDate string, quests []Quest) (*IntentResponse, error) {
	systemPrompt := s.buildIntentSystemPrompt(targetDate, quests)

	// Ajouter le nouveau message utilisateur à l'historique
	messages := []PerplexityMsg{{Role: "system", Content: systemPrompt}}
	messages = append(messages, conversationHistory...)
	messages = append(messages, PerplexityMsg{Role: "user", Content: userText})

	return s.callPerplexityWithHistory(messages)
}

// ScheduleGoals - Demander à Perplexity de planifier intelligemment les objectifs
func (s *AIService) ScheduleGoals(goals []DailyGoal, existingBlocks []TimeBlock, date string) (*ScheduleResponse, error) {
	systemPrompt := s.buildSchedulingPrompt(existingBlocks, date)
	userPrompt := s.buildGoalsPrompt(goals)

	reqBody := PerplexityRequest{
		Model: "sonar-pro",
		Messages: []PerplexityMsg{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", "https://api.perplexity.ai/chat/completions", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+s.perplexityAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Perplexity API returned status %d", resp.StatusCode)
	}

	var perplexityResp PerplexityResponse
	if err := json.NewDecoder(resp.Body).Decode(&perplexityResp); err != nil {
		return nil, err
	}

	if len(perplexityResp.Choices) == 0 {
		return nil, fmt.Errorf("no response from Perplexity")
	}

	content := cleanJSONResponse(perplexityResp.Choices[0].Message.Content)

	var scheduleResp ScheduleResponse
	if err := json.Unmarshal([]byte(content), &scheduleResp); err != nil {
		return nil, fmt.Errorf("failed to parse schedule: %w", err)
	}

	return &scheduleResp, nil
}

func (s *AIService) callPerplexity(systemPrompt, userText string) (*IntentResponse, error) {
	reqBody := PerplexityRequest{
		Model: "sonar-pro",
		Messages: []PerplexityMsg{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userText},
		},
	}

	return s.executePerplexityRequest(reqBody)
}

func (s *AIService) callPerplexityWithHistory(messages []PerplexityMsg) (*IntentResponse, error) {
	reqBody := PerplexityRequest{
		Model:    "sonar-pro",
		Messages: messages,
	}

	return s.executePerplexityRequest(reqBody)
}

func (s *AIService) executePerplexityRequest(reqBody PerplexityRequest) (*IntentResponse, error) {
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.perplexity.ai/chat/completions", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+s.perplexityAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call Perplexity API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Perplexity API returned status %d", resp.StatusCode)
	}

	var perplexityResp PerplexityResponse
	if err := json.NewDecoder(resp.Body).Decode(&perplexityResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(perplexityResp.Choices) == 0 {
		return nil, fmt.Errorf("no response from Perplexity")
	}

	content := cleanJSONResponse(perplexityResp.Choices[0].Message.Content)

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
			questContext += fmt.Sprintf("- ID: %s | Titre: %s", q.ID, q.Title)
			if q.Description != nil {
				questContext += fmt.Sprintf(" | Description: %s", *q.Description)
			}
			questContext += "\n"
		}
	}

	return fmt.Sprintf(`Tu es un assistant vocal intelligent pour planifier la journée. Date: %s
%s

## Ta mission
1. Comprendre les intentions de l'utilisateur (voix → texte)
2. Extraire et structurer ses objectifs avec des horaires précis
3. Si l'heure n'est pas précisée, DEMANDER poliment à quelle heure
4. Retourner UNIQUEMENT un JSON valide

## Schéma JSON de sortie
{
  "intent_type": "ADD_GOAL | UPDATE_GOAL | LIST_GOALS | ASK_QUESTION | NEED_CLARIFICATION | OTHER",
  "goals": [
    {
      "title": "string",
      "date": "YYYY-MM-DD",
      "priority": "low | medium | high",
      "time_block": "morning | afternoon | evening | none",
      "scheduled_start": "HH:MM | null",
      "scheduled_end": "HH:MM | null",
      "estimated_duration_minutes": number | null,
      "status": "pending",
      "quest_id": "uuid | null"
    }
  ],
  "raw_user_text": "string",
  "notes": "string",
  "follow_up_question": "string | null",
  "tts_response": "string"
}

## Règles de planification automatique

### Quand l'utilisateur donne une heure:
- "à 9h" → scheduled_start: "09:00"
- "vers 14h30" → scheduled_start: "14:30"
- "de 10h à 12h" → scheduled_start: "10:00", scheduled_end: "12:00"

### Quand l'utilisateur ne donne PAS d'heure:
- Mets time_block approprié (morning/afternoon/evening/none)
- scheduled_start: null
- intent_type: "NEED_CLARIFICATION"
- follow_up_question: Question naturelle pour demander l'heure
- tts_response: Version vocale de la question

### Estimation automatique des durées:
- Sport/salle → 60-90 min
- Travail sur projet → 120 min
- Emails/admin → 30 min
- Appel téléphonique → 20 min
- Méditation → 15 min
- Repas → 45-60 min

### Priorités:
- "important", "absolument", "faut vraiment" → high
- "si j'ai le temps", "optionnel" → low
- Par défaut → medium

### Matching des quêtes:
- Si un objectif correspond à une quête active, mets l'ID dans quest_id
- Sois intelligent (ex: "bosser mon app" → quête contenant "app" ou "développement")

## Exemples de réponses

### Exemple 1 - Objectif avec heure précise
User: "Je veux aller à la salle à 18h"
{
  "intent_type": "ADD_GOAL",
  "goals": [{
    "title": "Aller à la salle de sport",
    "date": "%s",
    "priority": "medium",
    "time_block": "evening",
    "scheduled_start": "18:00",
    "scheduled_end": "19:30",
    "estimated_duration_minutes": 90,
    "status": "pending",
    "quest_id": null
  }],
  "raw_user_text": "Je veux aller à la salle à 18h",
  "notes": "",
  "follow_up_question": null,
  "tts_response": "C'est noté ! Salle de sport à 18h pour environ 1h30."
}

### Exemple 2 - Objectif SANS heure (demander clarification)
User: "Faut que je bosse sur mon projet ce matin"
{
  "intent_type": "NEED_CLARIFICATION",
  "goals": [{
    "title": "Travailler sur mon projet",
    "date": "%s",
    "priority": "high",
    "time_block": "morning",
    "scheduled_start": null,
    "scheduled_end": null,
    "estimated_duration_minutes": 120,
    "status": "pending",
    "quest_id": null
  }],
  "raw_user_text": "Faut que je bosse sur mon projet ce matin",
  "notes": "Heure de début non précisée",
  "follow_up_question": "À quelle heure tu veux commencer à travailler sur ton projet ? Et tu penses y passer combien de temps ?",
  "tts_response": "OK pour bosser sur ton projet ce matin ! À quelle heure tu veux commencer ?"
}

### Exemple 3 - Réponse à une clarification (conversation multi-tour)
User: "À 9h, pendant 2 heures"
{
  "intent_type": "ADD_GOAL",
  "goals": [{
    "title": "Travailler sur mon projet",
    "date": "%s",
    "priority": "high",
    "time_block": "morning",
    "scheduled_start": "09:00",
    "scheduled_end": "11:00",
    "estimated_duration_minutes": 120,
    "status": "pending",
    "quest_id": null
  }],
  "raw_user_text": "À 9h, pendant 2 heures",
  "notes": "Confirmation suite à clarification",
  "follow_up_question": null,
  "tts_response": "Parfait ! Travail sur ton projet de 9h à 11h. C'est ajouté à ta journée !"
}

IMPORTANT: Réponds TOUJOURS avec un JSON valide, sans markdown, sans texte autour.`, targetDate, questContext, targetDate, targetDate, targetDate)
}

func (s *AIService) buildSchedulingPrompt(existingBlocks []TimeBlock, date string) string {
	existingContext := ""
	if len(existingBlocks) > 0 {
		existingContext = "\n\nBlocs déjà planifiés (éviter les conflits):\n"
		for _, b := range existingBlocks {
			existingContext += fmt.Sprintf("- %s: %s - %s\n",
				b.Title,
				b.StartTime.Format("15:04"),
				b.EndTime.Format("15:04"),
			)
		}
	}

	return fmt.Sprintf(`Tu dois planifier des objectifs dans la journée de manière optimale.
Date: %s
%s

## Règles de planification:
- Travail cognitif intense → Matin 8h-12h
- Sport → 7h-8h ou 17h-19h
- Admin/emails → 13h-14h
- Créativité → 15h-17h
- Pause déjeuner: 12h-13h30 (ne pas planifier)
- 15min minimum entre les blocs
- Priorité haute = créneaux optimaux

## Format de réponse JSON:
{
  "blocks": [
    {
      "goal_id": "uuid",
      "title": "string",
      "start_time": "HH:MM",
      "end_time": "HH:MM",
      "duration_minutes": number,
      "block_type": "focus | meeting | exercise | admin | creative | personal",
      "priority": "low | medium | high",
      "reasoning": "string"
    }
  ],
  "summary": "Résumé de la journée",
  "conflicts": []
}

Réponds UNIQUEMENT avec le JSON.`, date, existingContext)
}

func (s *AIService) buildGoalsPrompt(goals []DailyGoal) string {
	prompt := "Objectifs à planifier:\n\n"

	for _, g := range goals {
		prompt += fmt.Sprintf("ID: %s\n", g.ID)
		prompt += fmt.Sprintf("Titre: %s\n", g.Title)
		prompt += fmt.Sprintf("Priorité: %s\n", g.Priority)
		prompt += fmt.Sprintf("Période: %s\n", g.TimeBlock)
		if g.ScheduledStart != nil {
			prompt += fmt.Sprintf("Heure demandée: %s\n", *g.ScheduledStart)
		}
		prompt += "\n"
	}

	return prompt
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

func (h *Handler) scheduleGoalsWithPerplexity(userID, date string, goals []DailyGoal) ([]TimeBlock, error) {
	existingBlocks, err := h.getExistingTimeBlocks(userID, date)
	if err != nil {
		return nil, err
	}

	schedule, err := h.aiService.ScheduleGoals(goals, existingBlocks, date)
	if err != nil {
		return nil, err
	}

	var timeBlocks []TimeBlock
	for _, block := range schedule.Blocks {
		tb, err := h.createTimeBlockFromSchedule(userID, date, block)
		if err != nil {
			continue
		}
		timeBlocks = append(timeBlocks, *tb)
		h.updateGoalSchedule(block.GoalID, block.StartTime, block.EndTime)
	}

	return timeBlocks, nil
}

func (h *Handler) getExistingTimeBlocks(userID, date string) ([]TimeBlock, error) {
	query := `
		SELECT id, title, description, start_time, end_time, status, block_type,
		       quest_id, daily_goal_id, COALESCE(source, 'manual') as source
		FROM time_blocks
		WHERE user_id = $1 AND DATE(start_time) = $2
		ORDER BY start_time
	`

	rows, err := h.db.Pool.Query(context.Background(), query, userID, date)
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
	err := h.db.Pool.QueryRow(context.Background(), query,
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
	h.db.Pool.Exec(context.Background(), query, goalID, startTime, endTime)
}

// Alias for backwards compatibility - scheduleGoalsWithGrok now uses Perplexity
func (h *Handler) scheduleGoalsWithGrok(userID, date string, goals []DailyGoal) ([]TimeBlock, error) {
	return h.scheduleGoalsWithPerplexity(userID, date, goals)
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
