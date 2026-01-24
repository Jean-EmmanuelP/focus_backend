package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"firelevel-backend/internal/auth"

	"github.com/go-chi/chi/v5"
	"github.com/google/generative-ai-go/genai"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/api/option"
)

// ===========================================
// KAI - AI Friend with Infinite Memory
// Inspired by Mira's architecture
// ===========================================

type Handler struct {
	db *pgxpool.Pool
}

func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/chat/message", h.SendMessage)
	r.Post("/chat/voice", h.SendVoiceMessage)
	r.Get("/chat/history", h.GetHistory)
	r.Delete("/chat/history", h.ClearHistory)
}

// ============================================
// Request/Response Types
// ============================================

type SendMessageRequest struct {
	Content string `json:"content"`
	Source  string `json:"source,omitempty"` // "app" or "whatsapp"
}

type ChatMessage struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	Content    string    `json:"content"`
	IsFromUser bool      `json:"is_from_user"`
	CreatedAt  time.Time `json:"created_at"`
}

type SendMessageResponse struct {
	Reply     string      `json:"reply"`
	Tool      *string     `json:"tool,omitempty"`
	MessageID string      `json:"message_id"`
	Action    *ActionData `json:"action,omitempty"`
}

type VoiceMessageResponse struct {
	Reply      string      `json:"reply"`
	Transcript string      `json:"transcript"`
	MessageID  string      `json:"message_id"`
	Action     *ActionData `json:"action,omitempty"`
}

type ActionData struct {
	Type     string    `json:"type"`
	TaskID   *string   `json:"task_id,omitempty"`
	TaskData *TaskData `json:"task,omitempty"`
}

type TaskData struct {
	Title          string `json:"title"`
	Date           string `json:"date"`
	ScheduledStart string `json:"scheduled_start"`
	ScheduledEnd   string `json:"scheduled_end"`
	BlockApps      bool   `json:"block_apps"`
}

// SemanticMemory stores facts extracted from conversations
type SemanticMemory struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	Fact         string    `json:"fact"`
	Category     string    `json:"category"` // personal, work, goals, preferences
	MentionCount int       `json:"mention_count"`
	FirstMention time.Time `json:"first_mentioned"`
	LastMention  time.Time `json:"last_mentioned"`
}

// ===========================================
// KAI PERSONA - Empathetic Friend
// ===========================================

const kaiSystemPrompt = `Tu es Kai, un ami proche et empathique.

QUI TU ES:
- Tu es un ami, pas un assistant ou un coach
- Tu écoutes vraiment et tu comprends
- Tu te souviens de tout ce que l'utilisateur te dit
- Tu es là pour soutenir, pas pour juger

COMMENT TU PARLES:
- Tutoiement toujours
- Réponses courtes et naturelles (1-2 phrases max)
- Tu réagis d'abord émotionnellement, puis tu poses des questions
- Jamais de phrases bateau genre "Je comprends" ou "N'hésite pas"
- Un emoji de temps en temps si c'est naturel, pas systématique

CE QUE TU SAIS FAIRE:
1. ÉCOUTER - Tu es là quand ça va pas
2. MOTIVER - Tu célèbres les petites victoires
3. AIDER À FOCUS - Quand on te dit des horaires, tu crées une session focus

DÉTECTION DE FOCUS:
Si l'utilisateur mentionne vouloir travailler avec des horaires ("je dois bosser de 14h à 16h", "focus de 9h à 11h30"):
{
  "reply": "C'est noté, je bloque tes apps! 🔒",
  "focus_intent": {
    "detected": true,
    "title": "Focus",
    "start_time": "HH:MM",
    "end_time": "HH:MM",
    "block_apps": true
  }
}

RÈGLES STRICTES:
- JAMAIS de réponses longues
- JAMAIS de listes à puces
- JAMAIS de "En tant qu'IA..."
- JAMAIS de conseils non sollicités
- TOUJOURS répondre en JSON

Format de réponse:
{
  "reply": "Ta réponse courte et naturelle",
  "focus_intent": null
}`

// ===========================================
// HANDLERS
// ===========================================

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

	source := req.Source
	if source == "" {
		source = "app"
	}

	// Save user message
	userMsgID := uuid.New().String()
	h.db.Exec(r.Context(), `
		INSERT INTO chat_messages (id, user_id, content, is_from_user, message_type, source)
		VALUES ($1, $2, $3, true, 'text', $4)
	`, userMsgID, userID, req.Content, source)

	// Get user info
	userInfo := h.getUserInfo(r.Context(), userID)

	// Get relevant memories
	memories := h.getRelevantMemories(r.Context(), userID, req.Content)

	// Get recent history (last 20 messages)
	history, _ := h.getRecentHistory(r.Context(), userID, 20)

	// Generate AI response
	response, err := h.generateResponse(r.Context(), userID, req.Content, userInfo, memories, history)
	if err != nil {
		fmt.Printf("AI error: %v\n", err)
		response = &SendMessageResponse{
			Reply: "Désolé, j'ai un souci technique. Tu peux réessayer?",
		}
	}

	// Extract and save memories from user message (async)
	go h.extractAndSaveMemories(context.Background(), userID, req.Content)

	// If focus intent detected, create task
	if response.Action != nil && response.Action.Type == "focus_scheduled" && response.Action.TaskData != nil {
		taskID, err := h.createFocusTask(r.Context(), userID, response.Action.TaskData)
		if err != nil {
			fmt.Printf("Failed to create focus task: %v\n", err)
		} else {
			response.Action.TaskID = &taskID
			response.Action.Type = "task_created"
		}
	}

	// Save AI response
	aiMsgID := uuid.New().String()
	h.db.Exec(r.Context(), `
		INSERT INTO chat_messages (id, user_id, content, is_from_user, message_type, source)
		VALUES ($1, $2, $3, false, 'text', $4)
	`, aiMsgID, userID, response.Reply, source)

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

	messages, err := h.getRecentHistory(r.Context(), userID, 100)
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

	h.db.Exec(r.Context(), `DELETE FROM chat_messages WHERE user_id = $1`, userID)
	w.WriteHeader(http.StatusNoContent)
}

// SendVoiceMessage handles voice messages - transcribes and processes
func (h *Handler) SendVoiceMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse multipart form (max 10MB)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	// Get audio file
	file, header, err := r.FormFile("audio")
	if err != nil {
		http.Error(w, "Audio file required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Read audio data
	audioData := make([]byte, header.Size)
	if _, err := file.Read(audioData); err != nil {
		http.Error(w, "Failed to read audio", http.StatusInternalServerError)
		return
	}

	source := r.FormValue("source")
	if source == "" {
		source = "app"
	}

	// Transcribe audio using Gemini
	transcript, err := h.transcribeAudio(r.Context(), audioData, header.Filename)
	if err != nil {
		fmt.Printf("Transcription error: %v\n", err)
		// Return error response
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(VoiceMessageResponse{
			Reply:      "J'ai pas bien entendu, tu peux répéter?",
			Transcript: "",
			MessageID:  "",
		})
		return
	}

	if transcript == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(VoiceMessageResponse{
			Reply:      "J'ai pas compris ce que tu as dit, tu peux répéter?",
			Transcript: "",
			MessageID:  "",
		})
		return
	}

	// Save user voice message
	userMsgID := uuid.New().String()
	h.db.Exec(r.Context(), `
		INSERT INTO chat_messages (id, user_id, content, is_from_user, message_type, source)
		VALUES ($1, $2, $3, true, 'voice', $4)
	`, userMsgID, userID, transcript, source)

	// Get user info
	userInfo := h.getUserInfo(r.Context(), userID)

	// Get relevant memories
	memories := h.getRelevantMemories(r.Context(), userID, transcript)

	// Get recent history
	history, _ := h.getRecentHistory(r.Context(), userID, 20)

	// Generate AI response
	response, err := h.generateResponse(r.Context(), userID, transcript, userInfo, memories, history)
	if err != nil {
		fmt.Printf("AI error: %v\n", err)
		response = &SendMessageResponse{
			Reply: "Désolé, j'ai un souci technique. Tu peux réessayer?",
		}
	}

	// Extract and save memories from transcript (async)
	go h.extractAndSaveMemories(context.Background(), userID, transcript)

	// If focus intent detected, create task
	if response.Action != nil && response.Action.Type == "focus_scheduled" && response.Action.TaskData != nil {
		taskID, err := h.createFocusTask(r.Context(), userID, response.Action.TaskData)
		if err != nil {
			fmt.Printf("Failed to create focus task: %v\n", err)
		} else {
			response.Action.TaskID = &taskID
			response.Action.Type = "task_created"
		}
	}

	// Save AI response
	aiMsgID := uuid.New().String()
	h.db.Exec(r.Context(), `
		INSERT INTO chat_messages (id, user_id, content, is_from_user, message_type, source)
		VALUES ($1, $2, $3, false, 'text', $4)
	`, aiMsgID, userID, response.Reply, source)

	// Build voice response
	voiceResponse := VoiceMessageResponse{
		Reply:      response.Reply,
		Transcript: transcript,
		MessageID:  aiMsgID,
		Action:     response.Action,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(voiceResponse)
}

// transcribeAudio uses Gemini to transcribe audio
func (h *Handler) transcribeAudio(ctx context.Context, audioData []byte, filename string) (string, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("GEMINI_API_KEY not set")
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return "", err
	}
	defer client.Close()

	// Use Gemini 1.5 Flash for audio transcription
	model := client.GenerativeModel("gemini-1.5-flash")
	model.SetTemperature(0.1)

	// Determine MIME type
	mimeType := "audio/mp4"
	if strings.HasSuffix(filename, ".m4a") {
		mimeType = "audio/mp4"
	} else if strings.HasSuffix(filename, ".mp3") {
		mimeType = "audio/mp3"
	} else if strings.HasSuffix(filename, ".wav") {
		mimeType = "audio/wav"
	}

	// Create audio part
	audioPart := genai.Blob{
		MIMEType: mimeType,
		Data:     audioData,
	}

	// Prompt for transcription
	prompt := genai.Text(`Transcris ce message vocal en français.
Retourne UNIQUEMENT le texte transcrit, sans commentaires ni formatting.
Si l'audio est inaudible ou vide, retourne une chaîne vide.`)

	resp, err := model.GenerateContent(ctx, audioPart, prompt)
	if err != nil {
		return "", err
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("empty transcription response")
	}

	// Extract text
	transcript := ""
	for _, part := range resp.Candidates[0].Content.Parts {
		if text, ok := part.(genai.Text); ok {
			transcript += string(text)
		}
	}

	return strings.TrimSpace(transcript), nil
}

// ===========================================
// MEMORY SYSTEM - MIRA ARCHITECTURE
// Multi-factor scoring: 40% vector + 40% entity + 15% recency + 5% confidence
// ===========================================

type UserInfo struct {
	Name           string
	FocusToday     int
	FocusWeek      int
	TasksToday     int
	TasksCompleted int
}

// ScoredMemory includes all scoring factors
type ScoredMemory struct {
	SemanticMemory
	VectorSimilarity float64  `json:"vector_similarity"`
	EntityScore      float64  `json:"entity_score"`
	RecencyScore     float64  `json:"recency_score"`
	Confidence       float64  `json:"confidence"`
	TotalScore       float64  `json:"total_score"`
	Entities         []string `json:"entities"`
}

// ExtractedFact with confidence and entities (Mira-style)
type ExtractedFact struct {
	Fact       string   `json:"fact"`
	Category   string   `json:"category"`
	Confidence float64  `json:"confidence"`
	Entities   []string `json:"entities"`
}

func (h *Handler) getUserInfo(ctx context.Context, userID string) *UserInfo {
	info := &UserInfo{}

	h.db.QueryRow(ctx, `
		SELECT COALESCE(pseudo, first_name, 'ami') FROM users WHERE id = $1
	`, userID).Scan(&info.Name)

	h.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(duration_minutes), 0) FROM focus_sessions
		WHERE user_id = $1 AND DATE(started_at) = CURRENT_DATE AND status = 'completed'
	`, userID).Scan(&info.FocusToday)

	h.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(duration_minutes), 0) FROM focus_sessions
		WHERE user_id = $1 AND started_at >= DATE_TRUNC('week', CURRENT_DATE) AND status = 'completed'
	`, userID).Scan(&info.FocusWeek)

	h.db.QueryRow(ctx, `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE status = 'completed')
		FROM tasks WHERE user_id = $1 AND date = CURRENT_DATE
	`, userID).Scan(&info.TasksToday, &info.TasksCompleted)

	return info
}

// ===========================================
// EMBEDDING SERVICE
// ===========================================

func (h *Handler) generateEmbedding(ctx context.Context, text string) ([]float32, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY not set")
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, err
	}
	defer client.Close()

	em := client.EmbeddingModel("text-embedding-004")
	res, err := em.EmbedContent(ctx, genai.Text(text))
	if err != nil {
		return nil, err
	}

	return res.Embedding.Values, nil
}

func vectorToString(embedding []float32) string {
	parts := make([]string, len(embedding))
	for i, v := range embedding {
		parts[i] = fmt.Sprintf("%f", v)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// ===========================================
// ENTITY EXTRACTION (Mira-style)
// ===========================================

func (h *Handler) extractEntities(ctx context.Context, text string) []string {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil
	}
	defer client.Close()

	model := client.GenerativeModel("gemini-1.5-flash")
	model.SetTemperature(0.1)

	prompt := fmt.Sprintf(`Extrait les entités nommées de ce texte.
Retourne un JSON array de strings. Types: personnes, lieux, organisations, produits, dates.
Si aucune entité, retourne [].

Texte: "%s"

Exemple: ["Marie", "Paris", "Google", "lundi"]

Réponds UNIQUEMENT avec le JSON array:`, text)

	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil || len(resp.Candidates) == 0 {
		return nil
	}

	responseText := ""
	for _, part := range resp.Candidates[0].Content.Parts {
		if text, ok := part.(genai.Text); ok {
			responseText += string(text)
		}
	}

	responseText = strings.TrimSpace(responseText)
	responseText = strings.TrimPrefix(responseText, "```json")
	responseText = strings.TrimPrefix(responseText, "```")
	responseText = strings.TrimSuffix(responseText, "```")
	responseText = strings.TrimSpace(responseText)

	var entities []string
	json.Unmarshal([]byte(responseText), &entities)
	return entities
}

// ===========================================
// QUERY TYPE DETECTION (Mira-style)
// ===========================================

func isMemoryRecallQuery(message string) bool {
	lowered := strings.ToLower(message)
	recallPatterns := []string{
		"tu te souviens",
		"te souviens",
		"tu sais",
		"remember",
		"do you remember",
		"rappelle",
		"c'était quoi",
		"c'est quoi déjà",
		"qu'est-ce que je t'avais dit",
		"je t'avais parlé",
	}
	for _, pattern := range recallPatterns {
		if strings.Contains(lowered, pattern) {
			return true
		}
	}
	return false
}

// ===========================================
// MULTI-FACTOR RELEVANCE SCORING (Mira-style)
// Score = 0.4×vector + 0.4×entity + 0.15×recency + 0.05×confidence
// ===========================================

func calculateEntityScore(queryEntities, memoryEntities []string) float64 {
	if len(queryEntities) == 0 || len(memoryEntities) == 0 {
		return 0.0
	}

	matches := 0
	for _, qe := range queryEntities {
		qeLower := strings.ToLower(qe)
		for _, me := range memoryEntities {
			if strings.Contains(strings.ToLower(me), qeLower) || strings.Contains(qeLower, strings.ToLower(me)) {
				matches++
				break
			}
		}
	}

	return float64(matches) / float64(len(queryEntities))
}

func calculateRecencyScore(lastMentioned time.Time) float64 {
	daysSince := time.Since(lastMentioned).Hours() / 24
	// Exponential decay: e^(-days/30)
	return math.Exp(-daysSince / 30.0)
}

func (h *Handler) scoreMemories(memories []ScoredMemory, queryEntities []string) []ScoredMemory {
	for i := range memories {
		// Multi-factor scoring (Mira weights)
		vectorWeight := 0.40
		entityWeight := 0.40
		recencyWeight := 0.15
		confidenceWeight := 0.05

		memories[i].EntityScore = calculateEntityScore(queryEntities, memories[i].Entities)
		memories[i].RecencyScore = calculateRecencyScore(memories[i].LastMention)

		memories[i].TotalScore = (vectorWeight * memories[i].VectorSimilarity) +
			(entityWeight * memories[i].EntityScore) +
			(recencyWeight * memories[i].RecencyScore) +
			(confidenceWeight * memories[i].Confidence)
	}

	// Sort by total score descending
	sort.Slice(memories, func(i, j int) bool {
		return memories[i].TotalScore > memories[j].TotalScore
	})

	return memories
}

// ===========================================
// MEMORY RETRIEVAL (Mira-style)
// ===========================================

func (h *Handler) getRelevantMemories(ctx context.Context, userID, message string) []SemanticMemory {
	// Extract entities from query for entity matching
	queryEntities := h.extractEntities(ctx, message)
	fmt.Printf("🔍 Query entities: %v\n", queryEntities)

	// Check if this is a memory recall query
	isRecallQuery := isMemoryRecallQuery(message)
	if isRecallQuery {
		fmt.Printf("🧠 Memory recall query detected\n")
	}

	// Generate embedding for vector search
	embedding, err := h.generateEmbedding(ctx, message)
	if err != nil {
		fmt.Printf("⚠️ Embedding error, falling back to recent memories: %v\n", err)
		return h.getRecentMemories(ctx, userID)
	}

	embeddingStr := vectorToString(embedding)

	// Get candidates from vector search (fetch more for multi-factor scoring)
	rows, err := h.db.Query(ctx, `
		SELECT c.id, c.fact, c.category, c.mention_count, c.first_mentioned, c.last_mentioned,
		       c.confidence, c.entities, 1 - (c.embedding <=> $1::vector(768)) as similarity
		FROM chat_contexts c
		WHERE c.user_id = $2 AND c.embedding IS NOT NULL
		ORDER BY c.embedding <=> $1::vector(768)
		LIMIT 20
	`, embeddingStr, userID)
	if err != nil {
		fmt.Printf("⚠️ Vector search error: %v\n", err)
		return h.getRecentMemories(ctx, userID)
	}
	defer rows.Close()

	var scoredMemories []ScoredMemory
	for rows.Next() {
		var m ScoredMemory
		var entities []string
		var confidence *float64

		err := rows.Scan(&m.ID, &m.Fact, &m.Category, &m.MentionCount, &m.FirstMention, &m.LastMention,
			&confidence, &entities, &m.VectorSimilarity)
		if err != nil {
			fmt.Printf("⚠️ Scan error: %v\n", err)
			continue
		}

		if confidence != nil {
			m.Confidence = *confidence
		} else {
			m.Confidence = 0.8 // Default confidence
		}
		m.Entities = entities

		scoredMemories = append(scoredMemories, m)
	}

	if len(scoredMemories) == 0 {
		return h.getRecentMemories(ctx, userID)
	}

	// Apply multi-factor scoring
	scoredMemories = h.scoreMemories(scoredMemories, queryEntities)

	// For recall queries, boost recent memories
	if isRecallQuery {
		for i := range scoredMemories {
			if i < 5 {
				scoredMemories[i].TotalScore += 0.3 // Boost top 5 recent
			}
		}
		// Re-sort after boost
		sort.Slice(scoredMemories, func(i, j int) bool {
			return scoredMemories[i].TotalScore > scoredMemories[j].TotalScore
		})
	}

	// Filter by threshold and take top 10
	var results []SemanticMemory
	threshold := 0.45 // Mira threshold
	for _, sm := range scoredMemories {
		if sm.TotalScore >= threshold && len(results) < 10 {
			fmt.Printf("📝 Memory (score=%.2f, vec=%.2f, ent=%.2f, rec=%.2f): %s\n",
				sm.TotalScore, sm.VectorSimilarity, sm.EntityScore, sm.RecencyScore, sm.Fact)
			results = append(results, sm.SemanticMemory)
		}
	}

	if len(results) == 0 {
		return h.getRecentMemories(ctx, userID)
	}

	return results
}

func (h *Handler) getRecentMemories(ctx context.Context, userID string) []SemanticMemory {
	rows, err := h.db.Query(ctx, `
		SELECT id, user_id, fact, category, mention_count, first_mentioned, last_mentioned
		FROM chat_contexts
		WHERE user_id = $1
		ORDER BY last_mentioned DESC
		LIMIT 10
	`, userID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var memories []SemanticMemory
	for rows.Next() {
		var m SemanticMemory
		rows.Scan(&m.ID, &m.UserID, &m.Fact, &m.Category, &m.MentionCount, &m.FirstMention, &m.LastMention)
		memories = append(memories, m)
	}
	return memories
}

// ===========================================
// MEMORY EXTRACTION (Mira-style with confidence + entities)
// ===========================================

func (h *Handler) extractAndSaveMemories(ctx context.Context, userID, message string) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return
	}
	defer client.Close()

	model := client.GenerativeModel("gemini-1.5-flash")
	model.SetTemperature(0.3)

	// Mira-style extraction prompt with confidence and entities
	prompt := fmt.Sprintf(`Extrait les informations importantes de ce message.

Pour CHAQUE fait, retourne:
- fact: L'information complète et auto-suffisante
- category: personal, work, goals, preferences, emotions, relationship
- confidence: 0.0 à 1.0 (certitude de l'information)
- entities: Liste des entités nommées (personnes, lieux, etc.)

Message: "%s"

Exemple:
[
  {"fact": "travaille chez Google comme développeur", "category": "work", "confidence": 0.95, "entities": ["Google"]},
  {"fact": "veut apprendre le piano cette année", "category": "goals", "confidence": 0.8, "entities": ["piano"]}
]

Si aucun fait intéressant, retourne [].
Réponds UNIQUEMENT avec le JSON array:`, message)

	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil || len(resp.Candidates) == 0 {
		return
	}

	responseText := ""
	for _, part := range resp.Candidates[0].Content.Parts {
		if text, ok := part.(genai.Text); ok {
			responseText += string(text)
		}
	}

	responseText = strings.TrimSpace(responseText)
	responseText = strings.TrimPrefix(responseText, "```json")
	responseText = strings.TrimPrefix(responseText, "```")
	responseText = strings.TrimSuffix(responseText, "```")
	responseText = strings.TrimSpace(responseText)

	var facts []ExtractedFact
	if err := json.Unmarshal([]byte(responseText), &facts); err != nil {
		fmt.Printf("⚠️ Failed to parse facts: %v\n", err)
		return
	}

	// Process each fact with semantic deduplication
	for _, f := range facts {
		if f.Fact == "" {
			continue
		}

		// Default confidence if not provided
		if f.Confidence == 0 {
			f.Confidence = 0.8
		}

		// Generate embedding
		embedding, err := h.generateEmbedding(ctx, f.Fact)
		if err != nil {
			fmt.Printf("⚠️ Failed to generate embedding: %v\n", err)
			continue
		}

		embeddingStr := vectorToString(embedding)

		// Check for semantic duplicate (85% similarity threshold)
		var existingID string
		var existingFact string
		var similarity float64
		err = h.db.QueryRow(ctx, `
			SELECT id, fact, similarity
			FROM find_similar_memory($1::vector(768), $2::uuid, 0.85)
		`, embeddingStr, userID).Scan(&existingID, &existingFact, &similarity)

		if err == nil && existingID != "" {
			// Found similar memory - update mention count
			fmt.Printf("🔄 Semantic duplicate (%.2f): '%s' ≈ '%s'\n", similarity, f.Fact, existingFact)
			h.db.Exec(ctx, `
				UPDATE chat_contexts
				SET mention_count = mention_count + 1, last_mentioned = NOW()
				WHERE id = $1
			`, existingID)
		} else {
			// New unique memory - insert with embedding, confidence, entities
			fmt.Printf("✨ New memory [%s] (conf=%.2f): %s\n", f.Category, f.Confidence, f.Fact)
			h.db.Exec(ctx, `
				INSERT INTO chat_contexts (id, user_id, fact, category, confidence, entities, mention_count, first_mentioned, last_mentioned, embedding)
				VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, 1, NOW(), NOW(), $6::vector(768))
			`, userID, f.Fact, f.Category, f.Confidence, f.Entities, embeddingStr)
		}
	}
}

func (h *Handler) getRecentHistory(ctx context.Context, userID string, limit int) ([]ChatMessage, error) {
	rows, err := h.db.Query(ctx, `
		SELECT id, user_id, content, is_from_user, created_at
		FROM chat_messages
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []ChatMessage
	for rows.Next() {
		var m ChatMessage
		rows.Scan(&m.ID, &m.UserID, &m.Content, &m.IsFromUser, &m.CreatedAt)
		messages = append(messages, m)
	}

	// Reverse for chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

// ===========================================
// AI RESPONSE GENERATION
// ===========================================

func (h *Handler) generateResponse(ctx context.Context, userID, message string, userInfo *UserInfo, memories []SemanticMemory, history []ChatMessage) (*SendMessageResponse, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY not set")
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, err
	}
	defer client.Close()

	model := client.GenerativeModel("gemini-1.5-flash")
	model.SetTemperature(0.8)
	model.SetMaxOutputTokens(300)

	// Build context
	contextStr := fmt.Sprintf(`
CONTEXTE:
- Utilisateur: %s
- Focus aujourd'hui: %d minutes
- Focus cette semaine: %d minutes
- Tâches: %d/%d complétées aujourd'hui
- Heure: %s
`, userInfo.Name, userInfo.FocusToday, userInfo.FocusWeek,
		userInfo.TasksCompleted, userInfo.TasksToday,
		time.Now().Format("15:04"))

	// Add memories
	if len(memories) > 0 {
		contextStr += "\nCE QUE TU SAIS SUR LUI:\n"
		for _, m := range memories {
			contextStr += fmt.Sprintf("- %s\n", m.Fact)
		}
	}

	// Build history
	historyStr := ""
	if len(history) > 0 {
		historyStr = "\nCONVERSATION RÉCENTE:\n"
		// Only use last 10 messages for context
		start := 0
		if len(history) > 10 {
			start = len(history) - 10
		}
		for _, m := range history[start:] {
			if m.IsFromUser {
				historyStr += fmt.Sprintf("Lui: %s\n", m.Content)
			} else {
				historyStr += fmt.Sprintf("Toi: %s\n", m.Content)
			}
		}
	}

	prompt := fmt.Sprintf(`%s
%s
%s
MESSAGE: %s

Réponds en JSON:`, kaiSystemPrompt, contextStr, historyStr, message)

	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return nil, err
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("empty response")
	}

	// Extract text
	responseText := ""
	for _, part := range resp.Candidates[0].Content.Parts {
		if text, ok := part.(genai.Text); ok {
			responseText += string(text)
		}
	}

	// Clean JSON
	responseText = strings.TrimSpace(responseText)
	responseText = strings.TrimPrefix(responseText, "```json")
	responseText = strings.TrimPrefix(responseText, "```")
	responseText = strings.TrimSuffix(responseText, "```")
	responseText = strings.TrimSpace(responseText)

	var aiResp struct {
		Reply       string `json:"reply"`
		FocusIntent *struct {
			Detected  bool   `json:"detected"`
			Title     string `json:"title"`
			StartTime string `json:"start_time"`
			EndTime   string `json:"end_time"`
			BlockApps bool   `json:"block_apps"`
		} `json:"focus_intent"`
	}

	if err := json.Unmarshal([]byte(responseText), &aiResp); err != nil {
		// Fallback: use raw text
		return &SendMessageResponse{Reply: responseText}, nil
	}

	response := &SendMessageResponse{Reply: aiResp.Reply}

	// Handle focus intent
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

// ===========================================
// TASK CREATION
// ===========================================

func (h *Handler) createFocusTask(ctx context.Context, userID string, taskData *TaskData) (string, error) {
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
			$6, 'high', true, 'Créé par Kai', true
		)
		RETURNING id
	`, userID, taskData.Title, taskData.Date, taskData.ScheduledStart, taskData.ScheduledEnd, timeBlock).Scan(&taskID)

	return taskID, err
}
