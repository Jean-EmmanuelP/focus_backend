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
// MEMORY SYSTEM (Inspired by Mira)
// With Vector Embeddings + Semantic Search
// ===========================================

type UserInfo struct {
	Name           string
	FocusToday     int
	FocusWeek      int
	TasksToday     int
	TasksCompleted int
}

// SemanticMemoryWithScore includes similarity score from vector search
type SemanticMemoryWithScore struct {
	SemanticMemory
	Similarity float64 `json:"similarity"`
}

func (h *Handler) getUserInfo(ctx context.Context, userID string) *UserInfo {
	info := &UserInfo{}

	// Get user name
	h.db.QueryRow(ctx, `
		SELECT COALESCE(pseudo, first_name, 'ami') FROM users WHERE id = $1
	`, userID).Scan(&info.Name)

	// Get focus minutes today
	h.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(duration_minutes), 0) FROM focus_sessions
		WHERE user_id = $1 AND DATE(started_at) = CURRENT_DATE AND status = 'completed'
	`, userID).Scan(&info.FocusToday)

	// Get focus minutes this week
	h.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(duration_minutes), 0) FROM focus_sessions
		WHERE user_id = $1 AND started_at >= DATE_TRUNC('week', CURRENT_DATE) AND status = 'completed'
	`, userID).Scan(&info.FocusWeek)

	// Get tasks today
	h.db.QueryRow(ctx, `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE status = 'completed')
		FROM tasks WHERE user_id = $1 AND date = CURRENT_DATE
	`, userID).Scan(&info.TasksToday, &info.TasksCompleted)

	return info
}

// generateEmbedding creates a 768-dim embedding using Gemini
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

	// Use text-embedding-004 model (768 dimensions)
	em := client.EmbeddingModel("text-embedding-004")
	res, err := em.EmbedContent(ctx, genai.Text(text))
	if err != nil {
		return nil, err
	}

	return res.Embedding.Values, nil
}

// vectorToString converts embedding to PostgreSQL vector format
func vectorToString(embedding []float32) string {
	parts := make([]string, len(embedding))
	for i, v := range embedding {
		parts[i] = fmt.Sprintf("%f", v)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// getRelevantMemories uses vector similarity search to find relevant memories
func (h *Handler) getRelevantMemories(ctx context.Context, userID, message string) []SemanticMemory {
	// Generate embedding for the query
	embedding, err := h.generateEmbedding(ctx, message)
	if err != nil {
		fmt.Printf("⚠️ Embedding error, falling back to recent memories: %v\n", err)
		return h.getRecentMemories(ctx, userID)
	}

	// Use the match_memories function for vector similarity search
	embeddingStr := vectorToString(embedding)
	rows, err := h.db.Query(ctx, `
		SELECT id, fact, category, mention_count, first_mentioned, last_mentioned, similarity
		FROM match_memories($1::vector(768), $2::uuid, 0.3, 10)
	`, embeddingStr, userID)
	if err != nil {
		fmt.Printf("⚠️ Vector search error, falling back to recent memories: %v\n", err)
		return h.getRecentMemories(ctx, userID)
	}
	defer rows.Close()

	var memories []SemanticMemory
	for rows.Next() {
		var m SemanticMemory
		var similarity float64
		if err := rows.Scan(&m.ID, &m.Fact, &m.Category, &m.MentionCount, &m.FirstMention, &m.LastMention, &similarity); err != nil {
			fmt.Printf("⚠️ Scan error: %v\n", err)
			continue
		}
		fmt.Printf("📝 Memory found (%.2f similarity): %s\n", similarity, m.Fact)
		memories = append(memories, m)
	}

	// If no vector matches, fall back to recent memories
	if len(memories) == 0 {
		return h.getRecentMemories(ctx, userID)
	}

	return memories
}

// getRecentMemories fallback when vector search fails or no embeddings exist
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

// extractAndSaveMemories extracts facts and stores with embeddings + semantic deduplication
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

	prompt := fmt.Sprintf(`Extrait les faits importants de ce message utilisateur.
Retourne un JSON array de faits. Catégories possibles: personal, work, goals, preferences, emotions.
Si aucun fait intéressant, retourne [].

Message: "%s"

Exemple de réponse:
[
  {"fact": "travaille dans la tech", "category": "work"},
  {"fact": "veut apprendre le piano", "category": "goals"}
]

Réponds UNIQUEMENT avec le JSON array:`, message)

	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil || len(resp.Candidates) == 0 {
		return
	}

	// Extract text
	responseText := ""
	for _, part := range resp.Candidates[0].Content.Parts {
		if text, ok := part.(genai.Text); ok {
			responseText += string(text)
		}
	}

	// Clean and parse
	responseText = strings.TrimSpace(responseText)
	responseText = strings.TrimPrefix(responseText, "```json")
	responseText = strings.TrimPrefix(responseText, "```")
	responseText = strings.TrimSuffix(responseText, "```")
	responseText = strings.TrimSpace(responseText)

	var facts []struct {
		Fact     string `json:"fact"`
		Category string `json:"category"`
	}

	if err := json.Unmarshal([]byte(responseText), &facts); err != nil {
		return
	}

	// Process each fact with semantic deduplication
	for _, f := range facts {
		if f.Fact == "" {
			continue
		}

		// Generate embedding for this fact
		embedding, err := h.generateEmbedding(ctx, f.Fact)
		if err != nil {
			fmt.Printf("⚠️ Failed to generate embedding for fact: %v\n", err)
			// Fallback: save without embedding
			h.db.Exec(ctx, `
				INSERT INTO chat_contexts (id, user_id, fact, category, mention_count, first_mentioned, last_mentioned)
				VALUES (gen_random_uuid(), $1, $2, $3, 1, NOW(), NOW())
				ON CONFLICT (user_id, fact) DO UPDATE SET
					mention_count = chat_contexts.mention_count + 1,
					last_mentioned = NOW()
			`, userID, f.Fact, f.Category)
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
			fmt.Printf("🔄 Semantic duplicate found (%.2f): '%s' ≈ '%s'\n", similarity, f.Fact, existingFact)
			h.db.Exec(ctx, `
				UPDATE chat_contexts
				SET mention_count = mention_count + 1, last_mentioned = NOW()
				WHERE id = $1
			`, existingID)
		} else {
			// New unique memory - insert with embedding
			fmt.Printf("✨ New memory: %s [%s]\n", f.Fact, f.Category)
			h.db.Exec(ctx, `
				INSERT INTO chat_contexts (id, user_id, fact, category, mention_count, first_mentioned, last_mentioned, embedding)
				VALUES (gen_random_uuid(), $1, $2, $3, 1, NOW(), NOW(), $4::vector(768))
			`, userID, f.Fact, f.Category, embeddingStr)
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
