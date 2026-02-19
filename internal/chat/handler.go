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
	"firelevel-backend/internal/streak"

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
	Content     string `json:"content"`
	Source      string `json:"source,omitempty"`       // "app" or "whatsapp"
	AppsBlocked bool   `json:"apps_blocked,omitempty"` // Whether apps are currently blocked on device
}

type ChatMessage struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	Content    string    `json:"content"`
	IsFromUser bool      `json:"is_from_user"`
	CreatedAt  time.Time `json:"created_at"`
	AudioURL   *string   `json:"audio_url,omitempty"` // Supabase Storage path for voice messages
}

type SendMessageResponse struct {
	Reply     string      `json:"reply"`
	Tool      *string     `json:"tool,omitempty"`
	MessageID string      `json:"message_id"`
	Action    *ActionData `json:"action,omitempty"`
	ShowCard  *string     `json:"show_card,omitempty"`
}

type VoiceMessageResponse struct {
	Reply      string      `json:"reply"`
	Transcript string      `json:"transcript"`
	MessageID  string      `json:"message_id"`
	Action     *ActionData `json:"action,omitempty"`
	ShowCard   *string     `json:"show_card,omitempty"`
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
// COACH PERSONA - Life Coach
// ===========================================

// kaiSystemPromptTemplate uses %s for the companion name
const kaiSystemPromptTemplate = `Tu es %s, un coach de vie personnel.

QUI TU ES:
- Un coach exigeant mais bienveillant — tu pousses l'utilisateur à se dépasser
- Tu connais ses objectifs, ses routines, ses tâches et sa progression
- Tu te souviens de tout ce qu'il te dit
- Tu le challenges quand il procrastine, tu célèbres quand il avance vraiment
- Tu es là aussi dans les moments difficiles — un bon coach sait écouter

COMMENT TU PARLES (c'est un CHAT, pas un email):
- Tutoiement toujours
- Réponses courtes et naturelles — 2-4 phrases max, comme un texto
- Direct, pas de blabla motivation LinkedIn
- Tu mentionnes ses vraies données quand c'est pertinent (tâches, routines, quests, streak)
- Un emoji max par message, seulement si naturel
- Tu finis souvent par une question ou une action concrète

PREMIER CONTACT (si le contexte montre "PREMIÈRE SÉANCE"):
C'est la toute première rencontre. Tu dois établir la relation et poser les fondations :
1. Accueille-le chaleureusement mais brièvement
2. Demande-lui ce qu'il veut améliorer dans sa vie (pas "aujourd'hui", dans sa VIE)
3. Quand il te donne ses objectifs, crée ses premières quests avec create_quests
4. Propose-lui des routines quotidiennes adaptées avec create_routines
5. Si l'utilisateur mentionne une tâche concrète à faire aujourd'hui, utilise IMMÉDIATEMENT create_task — ne te contente pas de répondre "ok"
6. Guide-le étape par étape — c'est OK de faire des messages un peu plus longs ici
Exemple de flow :
- "Bienvenue. Je suis ton coach. Dis-moi : c'est quoi le truc que tu veux vraiment changer dans ta vie ?"
- (il répond "être plus discipliné, lire plus, faire du sport")
- Tu crées les quests et routines, puis tu lui dis "C'est noté. Je t'ai créé tes objectifs. On commence par quoi aujourd'hui ?"
- (il dit "aujourd'hui je dois nettoyer mon appart") → Tu utilises create_task OBLIGATOIREMENT

UTILISATEUR ACTIF (si le contexte a des tâches/routines/quests):
- Le matin → orienter vers la planification, mentionner les tâches du jour
- L'après-midi → checker l'avancement, pousser si rien n'est fait
- Le soir → bilan, célébrer ou challenger
- Si des routines ne sont pas faites → les mentionner naturellement
- Si le streak est long → le valoriser
- Si des tâches sont en retard → demander ce qui bloque
- Si une quest avance bien → encourager à maintenir le rythme
- Si l'utilisateur dit "ça va pas" → écouter d'abord, coacher après

CRÉATION DE QUESTS:
Quand l'utilisateur exprime des objectifs ("je veux lire 12 livres", "perdre 5kg", "apprendre le piano"), tu peux en créer PLUSIEURS d'un coup :
{
  "reply": "C'est posé. 3 objectifs créés, on va les traquer ensemble 💪",
  "create_quests": [
    {"title": "Lire 12 livres", "target_value": 12, "area": "learning"},
    {"title": "Perdre 5kg", "target_value": 5, "area": "health"}
  ]
}
Areas possibles: health, learning, career, relationships, creativity, other

CRÉATION DE ROUTINES:
Quand l'utilisateur veut créer des habitudes, tu peux en créer PLUSIEURS d'un coup.
Si l'utilisateur mentionne une heure, ajoute "scheduled_time" au format "HH:MM".
{
  "reply": "3 routines ajoutées. On commence demain matin.",
  "create_routines": [
    {"title": "Méditation", "frequency": "daily", "scheduled_time": "07:00"},
    {"title": "Sport", "frequency": "daily", "scheduled_time": "18:00"},
    {"title": "Lire 30 min", "frequency": "daily"}
  ]
}

MISE À JOUR DE QUEST:
Quand l'utilisateur dit avoir progressé ("j'ai lu un chapitre", "j'ai perdu 1kg", "j'ai couru aujourd'hui"), incrémente la quest correspondante :
{
  "reply": "Noté ! T'es à 5/12 livres maintenant. Continue 📚",
  "update_quest": {
    "title": "Lire 12 livres",
    "increment": 1
  }
}

DÉTECTION DE FOCUS:
Si l'utilisateur mentionne vouloir travailler avec des horaires ("je dois bosser de 14h à 16h", "focus de 9h à 11h30"):
{
  "reply": "C'est noté, je bloque tes apps 🔒",
  "focus_intent": {
    "detected": true,
    "title": "Focus",
    "start_time": "HH:MM",
    "end_time": "HH:MM",
    "block_apps": true
  }
}

BLOCAGE D'APPS:
Tu peux bloquer les apps de l'utilisateur pour l'aider à se concentrer.
- Sur demande : "bloque mes apps", "bloque les apps pendant 2h", etc.
- De ta propre initiative : quand l'utilisateur a des tâches importantes et procrastine
Pour bloquer MAINTENANT (sans horaire spécifique):
{
  "reply": "C'est parti, je bloque tes apps 🔒",
  "block_now": true
}

DEMANDE DE DÉBLOCAGE:
- TOUJOURS demander la raison AVANT de débloquer
- Raison valable (urgence, appel, app nécessaire) → débloquer
- Raison faible (scroller, "juste 5 min", ennui) → refuser fermement
Pour débloquer :
{
  "reply": "OK, je débloque. Mais reviens vite 💪",
  "unblock_now": true
}

COMPLÉTION DE TÂCHES:
Quand l'utilisateur dit qu'il a fini une tâche ("j'ai fini X", "c'est fait", "done"):
{
  "reply": "Bien joué ! Une de moins 💪",
  "complete_task": {"title": "Nom de la tâche"}
}

COMPLÉTION DE ROUTINES:
Quand l'utilisateur dit qu'il a fait un rituel ("j'ai médité", "sport fait", "j'ai lu"):
{
  "reply": "Noté ✅",
  "complete_routines": ["Méditation", "Sport"]
}

SUPPRESSION:
Quand l'utilisateur veut supprimer un objectif ou rituel:
{
  "reply": "Supprimé.",
  "delete_quest": {"title": "Lire 12 livres"}
}
ou:
{
  "reply": "Supprimé.",
  "delete_routine": {"title": "Méditation"}
}

MORNING CHECK-IN:
Quand tu fais le check-in du matin avec l'utilisateur (humeur, sommeil, intentions), sauvegarde :
{
  "reply": "C'est noté, bonne journée !",
  "morning_checkin": {
    "mood": 4,
    "sleep_quality": 3,
    "top_priority": "Finir le projet X",
    "intentions": ["Finir le projet", "Faire du sport", "Lire 30 min"],
    "energy_level": 4
  }
}
mood, sleep_quality, energy_level: 1-5. intentions: array de strings.

EVENING CHECK-IN:
Quand tu fais la review du soir (bilan, victoire, bloqueurs, objectif demain), sauvegarde :
{
  "reply": "Belle journée. Repose-toi bien 🌙",
  "evening_checkin": {
    "mood": 4,
    "biggest_win": "J'ai fini le chapitre 3",
    "blockers": "Trop de réunions",
    "goal_for_tomorrow": "Finir le rapport",
    "grateful_for": "Ma santé"
  }
}
mood: 1-5. Les autres champs sont des strings.

OBJECTIFS DE LA SEMAINE:
Quand l'utilisateur définit ses objectifs hebdos:
{
  "reply": "Tes objectifs de la semaine sont posés 🎯",
  "create_weekly_goals": ["Finir le projet X", "Courir 3 fois", "Lire 2 chapitres"]
}

COMPLÉTION D'OBJECTIF HEBDO:
Quand l'utilisateur dit qu'un objectif hebdo est fait:
{
  "reply": "Coché ✅",
  "complete_weekly_goal": {"content": "Courir 3 fois"}
}

CRÉATION DE TÂCHE:
Quand l'utilisateur veut ajouter une tâche à son calendrier (pas focus, juste une tâche):
{
  "reply": "Ajouté à ton calendrier.",
  "create_task": {"title": "Rendez-vous dentiste", "date": "2025-01-15", "time_block": "afternoon", "scheduled_start": "14:00", "scheduled_end": "15:00"}
}
date: YYYY-MM-DD (utilise la date du CONTEXTE — "aujourd'hui" = Date, "demain" = Demain). time_block: morning/afternoon/evening. scheduled_start et scheduled_end: HH:MM (optionnels).
IMPORTANT: Tu DOIS TOUJOURS inclure "create_task" quand l'utilisateur mentionne une tâche. Si tu ne connais pas l'heure exacte, mets juste le titre et la date. Ne réponds JAMAIS "ajouté" sans le champ create_task.

ENTRÉE JOURNAL:
Quand l'utilisateur partage son humeur ou veut journaliser:
{
  "reply": "C'est noté dans ton journal.",
  "create_journal_entry": {"mood": "happy", "transcript": "Bonne journée, j'ai avancé sur mes projets"}
}
mood: happy, calm, neutral, sad, anxious, angry, grateful, motivated, tired, stressed

RÈGLES STRICTES:
- Réponses courtes (2-4 phrases), sauf pour le premier contact où tu peux être plus guidant
- JAMAIS de listes à puces dans tes réponses
- JAMAIS de "En tant qu'IA..." ou "N'hésite pas"
- Tu peux créer PLUSIEURS quests ou routines dans un même message
- TOUJOURS répondre en JSON valide — JAMAIS de texte brut
- Utilise les actions pour TOUTE modification de données — ne dis jamais "je t'ai noté ça" sans l'action correspondante
- CRITIQUE: Quand l'utilisateur demande d'ajouter/créer/mettre une tâche → TOUJOURS inclure "create_task" dans la réponse JSON. Ne JAMAIS répondre "ok" sans create_task.
- Quand le matin tu demandes comment il va, son sommeil, ses intentions → morning_checkin
- Quand le soir tu fais le bilan → evening_checkin
- Quand il dit avoir fait quelque chose → complete_routines ou complete_task

Format de réponse (inclure seulement les champs pertinents):
{
  "reply": "Ta réponse de coach",
  "focus_intent": null,
  "block_now": false,
  "unblock_now": false,
  "create_quests": [],
  "create_routines": [],
  "update_quest": null,
  "complete_task": null,
  "complete_routines": [],
  "delete_quest": null,
  "delete_routine": null,
  "morning_checkin": null,
  "evening_checkin": null,
  "create_weekly_goals": [],
  "complete_weekly_goal": null,
  "create_task": null,
  "create_journal_entry": null,
  "show_card": null
}

SHOW_CARD: Quand l'utilisateur demande de voir ses tâches, sa to-do list, ou ses rituels/routines du jour, ajoute "show_card" avec la valeur "tasks" ou "routines" pour afficher une liste interactive. Exemples: "montre mes tâches" → "show_card": "tasks", "quels sont mes rituels" → "show_card": "routines".`

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

	// Check if this is a greeting request (first message from coach)
	isGreeting := req.Content == "__greeting__"

	// Only save user message if it's NOT a greeting request
	if !isGreeting {
		userMsgID := uuid.New().String()
		h.db.Exec(r.Context(), `
			INSERT INTO chat_messages (id, user_id, content, is_from_user, message_type, source)
			VALUES ($1, $2, $3, true, 'text', $4)
		`, userMsgID, userID, req.Content, source)
	}

	// Replace greeting trigger with a prompt for the AI
	messageForAI := req.Content
	if isGreeting {
		messageForAI = "[SYSTEM: L'utilisateur vient d'ouvrir l'app pour la première fois. Envoie un message de bienvenue chaleureux et personnel. Présente-toi brièvement avec ton nom, et demande-lui comment tu peux l'aider aujourd'hui. Sois bref et engageant.]"
	}

	// Get user info BEFORE updating streak (so isFirstSession detection works)
	userInfo := h.getUserInfo(r.Context(), userID)
	userInfo.AppsBlocked = req.AppsBlocked

	// Update streak (user engaged today by sending a message)
	streak.UpdateUserStreak(r.Context(), h.db, userID)

	// Get relevant memories
	memories := h.getRelevantMemories(r.Context(), userID, messageForAI)

	// Get recent history (last 20 messages)
	history, _ := h.getRecentHistory(r.Context(), userID, 20)

	// Generate AI response
	response, err := h.generateResponse(r.Context(), userID, messageForAI, userInfo, memories, history)
	if err != nil {
		fmt.Printf("AI error: %v\n", err)
		response = &SendMessageResponse{
			Reply: "Désolé, j'ai un souci technique. Tu peux réessayer?",
		}
	}

	// Extract and save memories from user message (async, skip for greetings)
	if !isGreeting {
		go h.extractAndSaveMemories(context.Background(), userID, req.Content)
	}

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

	// Get audio_url (Supabase Storage path) if provided
	audioURL := r.FormValue("audio_url")
	if audioURL != "" {
		fmt.Printf("📎 Voice message audio_url: %s\n", audioURL)
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

	// Save user voice message (with audio_url if provided)
	userMsgID := uuid.New().String()
	h.db.Exec(r.Context(), `
		INSERT INTO chat_messages (id, user_id, content, is_from_user, message_type, source, audio_url)
		VALUES ($1, $2, $3, true, 'voice', $4, $5)
	`, userMsgID, userID, transcript, source, audioURL)

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
		ShowCard:   response.ShowCard,
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

	// Use Gemini 2.0 Flash for audio transcription
	model := client.GenerativeModel("gemini-2.0-flash")
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
	CompanionName  string
	FocusToday     int
	FocusWeek      int
	TasksToday     int
	TasksCompleted int
	CurrentStreak  int
	// Detailed data for coach context
	Tasks    []TaskSummary
	Routines []RoutineSummary
	Quests   []QuestSummary
	// Check-in status
	HasMorningCheckin bool
	HasEveningReview  bool
	// Last reflection
	LastReflectionWin     *string
	LastReflectionBlocker *string
	LastReflectionGoal    *string
	// Weekly goals
	WeeklyGoals []WeeklyGoalSummary
	// Latest mood
	LatestMood *string
	// Device state (from client)
	AppsBlocked bool
}

type TaskSummary struct {
	Title     string
	Status    string
	TimeBlock string
	StartTime *string
}

type RoutineSummary struct {
	Title       string
	IsCompleted bool
}

type QuestSummary struct {
	Title        string
	CurrentValue int
	TargetValue  int
	AreaName     *string
}

type WeeklyGoalSummary struct {
	Content     string
	IsCompleted bool
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

	// User profile + companion name + streak
	h.db.QueryRow(ctx, `
		SELECT COALESCE(pseudo, first_name, 'ami'),
		       COALESCE(companion_name, 'ton coach'),
		       COALESCE(current_streak, 0)
		FROM users WHERE id = $1
	`, userID).Scan(&info.Name, &info.CompanionName, &info.CurrentStreak)

	// Focus stats
	h.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(duration_minutes), 0) FROM focus_sessions
		WHERE user_id = $1 AND DATE(started_at) = CURRENT_DATE AND status = 'completed'
	`, userID).Scan(&info.FocusToday)

	h.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(duration_minutes), 0) FROM focus_sessions
		WHERE user_id = $1 AND started_at >= DATE_TRUNC('week', CURRENT_DATE) AND status = 'completed'
	`, userID).Scan(&info.FocusWeek)

	// Tasks count
	h.db.QueryRow(ctx, `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE status = 'completed')
		FROM tasks WHERE user_id = $1 AND date = CURRENT_DATE
	`, userID).Scan(&info.TasksToday, &info.TasksCompleted)

	// Today's tasks (detailed, max 10)
	taskRows, err := h.db.Query(ctx, `
		SELECT title, COALESCE(status, 'pending'), COALESCE(time_block, 'morning'),
		       CASE WHEN scheduled_start IS NOT NULL THEN to_char(scheduled_start, 'HH24:MI') END
		FROM tasks WHERE user_id = $1 AND date = CURRENT_DATE
		ORDER BY CASE time_block WHEN 'morning' THEN 1 WHEN 'afternoon' THEN 2 WHEN 'evening' THEN 3 ELSE 4 END,
		         scheduled_start NULLS LAST
		LIMIT 10
	`, userID)
	if err == nil {
		defer taskRows.Close()
		for taskRows.Next() {
			var t TaskSummary
			taskRows.Scan(&t.Title, &t.Status, &t.TimeBlock, &t.StartTime)
			info.Tasks = append(info.Tasks, t)
		}
	}

	// Routines with today's completion status
	routineRows, err := h.db.Query(ctx, `
		SELECT r.title,
		       EXISTS(
		           SELECT 1 FROM routine_completions rc
		           WHERE rc.routine_id = r.id AND rc.user_id = $1
		           AND DATE(rc.completed_at) = CURRENT_DATE
		       ) as is_completed
		FROM routines r
		WHERE r.user_id = $1
		ORDER BY r.created_at
		LIMIT 10
	`, userID)
	if err == nil {
		defer routineRows.Close()
		for routineRows.Next() {
			var r RoutineSummary
			routineRows.Scan(&r.Title, &r.IsCompleted)
			info.Routines = append(info.Routines, r)
		}
	}

	// Active quests with progress
	questRows, err := h.db.Query(ctx, `
		SELECT q.title, q.current_value, q.target_value, a.name
		FROM quests q
		LEFT JOIN areas a ON a.id = q.area_id
		WHERE q.user_id = $1 AND q.status = 'active'
		ORDER BY q.created_at
		LIMIT 8
	`, userID)
	if err == nil {
		defer questRows.Close()
		for questRows.Next() {
			var q QuestSummary
			questRows.Scan(&q.Title, &q.CurrentValue, &q.TargetValue, &q.AreaName)
			info.Quests = append(info.Quests, q)
		}
	}

	// Morning check-in status
	var morningCount int
	h.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM daily_reflections
		WHERE user_id = $1 AND date = CURRENT_DATE AND reflection_type = 'morning'
	`, userID).Scan(&morningCount)
	info.HasMorningCheckin = morningCount > 0

	// Evening review status
	var eveningCount int
	h.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM daily_reflections
		WHERE user_id = $1 AND date = CURRENT_DATE AND reflection_type = 'evening'
	`, userID).Scan(&eveningCount)
	info.HasEveningReview = eveningCount > 0

	// Last reflection (yesterday or today)
	h.db.QueryRow(ctx, `
		SELECT biggest_win, challenges, goal_for_tomorrow
		FROM daily_reflections
		WHERE user_id = $1
		ORDER BY date DESC LIMIT 1
	`, userID).Scan(&info.LastReflectionWin, &info.LastReflectionBlocker, &info.LastReflectionGoal)

	// Weekly goals for current week
	weeklyRows, err := h.db.Query(ctx, `
		SELECT wgi.content, wgi.is_completed
		FROM weekly_goal_items wgi
		JOIN weekly_goals wg ON wg.id = wgi.weekly_goal_id
		WHERE wg.user_id = $1
		AND wg.week_start_date = DATE_TRUNC('week', CURRENT_DATE)::date
		ORDER BY wgi.position
		LIMIT 5
	`, userID)
	if err == nil {
		defer weeklyRows.Close()
		for weeklyRows.Next() {
			var g WeeklyGoalSummary
			weeklyRows.Scan(&g.Content, &g.IsCompleted)
			info.WeeklyGoals = append(info.WeeklyGoals, g)
		}
	}

	// Latest mood from journal
	h.db.QueryRow(ctx, `
		SELECT mood FROM journal_entries
		WHERE user_id = $1 AND mood IS NOT NULL
		ORDER BY entry_date DESC LIMIT 1
	`, userID).Scan(&info.LatestMood)

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

	model := client.GenerativeModel("gemini-2.0-flash")
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

	model := client.GenerativeModel("gemini-2.0-flash")
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
		SELECT id, user_id, content, is_from_user, created_at, audio_url
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
		rows.Scan(&m.ID, &m.UserID, &m.Content, &m.IsFromUser, &m.CreatedAt, &m.AudioURL)
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

	model := client.GenerativeModel("gemini-2.0-flash")
	model.SetTemperature(0.8)
	model.SetMaxOutputTokens(1000)
	model.ResponseMIMEType = "application/json"

	// Build rich coach context
	streakStr := ""
	if userInfo.CurrentStreak > 0 {
		jourStr := "jour"
		if userInfo.CurrentStreak > 1 {
			jourStr = "jours"
		}
		streakStr = fmt.Sprintf("\n- Streak: %d %s 🔥", userInfo.CurrentStreak, jourStr)
	}

	appsBlockedStr := ""
	if userInfo.AppsBlocked {
		appsBlockedStr = "\n- Apps: BLOQUÉES 🔒"
	}

	now := time.Now()
	tomorrow := now.AddDate(0, 0, 1)
	contextStr := fmt.Sprintf(`
CONTEXTE:
- Utilisateur: %s%s
- Date: %s
- Demain: %s
- Focus aujourd'hui: %d minutes
- Focus cette semaine: %d minutes
- Tâches: %d/%d complétées aujourd'hui
- Heure: %s%s
`, userInfo.Name, streakStr, now.Format("2006-01-02"), tomorrow.Format("2006-01-02"),
		userInfo.FocusToday, userInfo.FocusWeek,
		userInfo.TasksCompleted, userInfo.TasksToday,
		now.Format("15:04"), appsBlockedStr)

	// Detect first session (new user with no data)
	isFirstSession := len(userInfo.Tasks) == 0 && len(userInfo.Routines) == 0 && len(userInfo.Quests) == 0 && userInfo.CurrentStreak == 0
	if isFirstSession {
		contextStr += "\n⭐ PREMIÈRE SÉANCE — C'est un nouvel utilisateur, pas de données existantes. Guide-le pour créer ses premiers objectifs et routines.\n"
	}

	// Add today's tasks
	if len(userInfo.Tasks) > 0 {
		contextStr += "\nTÂCHES AUJOURD'HUI:\n"
		for _, t := range userInfo.Tasks {
			icon := "⬜"
			if t.Status == "completed" {
				icon = "✅"
			} else if t.Status == "in_progress" {
				icon = "⏳"
			}
			timeStr := ""
			if t.StartTime != nil {
				timeStr = fmt.Sprintf(" (%s)", *t.StartTime)
			}
			contextStr += fmt.Sprintf("%s %s%s\n", icon, t.Title, timeStr)
		}
	}

	// Add routines
	if len(userInfo.Routines) > 0 {
		completedCount := 0
		contextStr += "\nROUTINES AUJOURD'HUI:\n"
		for _, r := range userInfo.Routines {
			icon := "⬜"
			if r.IsCompleted {
				icon = "✅"
				completedCount++
			}
			contextStr += fmt.Sprintf("%s %s\n", icon, r.Title)
		}
		contextStr += fmt.Sprintf("→ %d/%d complétées\n", completedCount, len(userInfo.Routines))
	}

	// Add active quests
	if len(userInfo.Quests) > 0 {
		contextStr += "\nQUESTS ACTIVES:\n"
		for _, q := range userInfo.Quests {
			pct := 0
			if q.TargetValue > 0 {
				pct = (q.CurrentValue * 100) / q.TargetValue
			}
			areaStr := ""
			if q.AreaName != nil {
				areaStr = fmt.Sprintf(" [%s]", *q.AreaName)
			}
			contextStr += fmt.Sprintf("- \"%s\" → %d/%d (%d%%)%s\n", q.Title, q.CurrentValue, q.TargetValue, pct, areaStr)
		}
	}

	// Add weekly goals
	if len(userInfo.WeeklyGoals) > 0 {
		contextStr += "\nOBJECTIFS DE LA SEMAINE:\n"
		for _, g := range userInfo.WeeklyGoals {
			icon := "⬜"
			if g.IsCompleted {
				icon = "✅"
			}
			contextStr += fmt.Sprintf("%s %s\n", icon, g.Content)
		}
	}

	// Add last reflection
	if userInfo.LastReflectionGoal != nil && *userInfo.LastReflectionGoal != "" {
		contextStr += fmt.Sprintf("\nDERNIER OBJECTIF FIXÉ: %s\n", *userInfo.LastReflectionGoal)
	}
	if userInfo.LastReflectionBlocker != nil && *userInfo.LastReflectionBlocker != "" {
		contextStr += fmt.Sprintf("DERNIER BLOCAGE: %s\n", *userInfo.LastReflectionBlocker)
	}

	// Add mood
	if userInfo.LatestMood != nil {
		contextStr += fmt.Sprintf("\nDERNIÈRE HUMEUR: %s\n", *userInfo.LatestMood)
	}

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

	// Build system prompt with dynamic companion name
	systemPrompt := fmt.Sprintf(kaiSystemPromptTemplate, userInfo.CompanionName)

	prompt := fmt.Sprintf(`%s
%s
%s
MESSAGE: %s

Réponds en JSON:`, systemPrompt, contextStr, historyStr, message)

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

	fmt.Printf("🤖 Raw AI response: %s\n", responseText)

	var aiResp struct {
		Reply       string `json:"reply"`
		FocusIntent *struct {
			Detected  bool   `json:"detected"`
			Title     string `json:"title"`
			StartTime string `json:"start_time"`
			EndTime   string `json:"end_time"`
			BlockApps bool   `json:"block_apps"`
		} `json:"focus_intent"`
		BlockNow      bool `json:"block_now"`
		UnblockNow    bool `json:"unblock_now"`
		CreateQuests  []struct {
			Title       string `json:"title"`
			TargetValue int    `json:"target_value"`
			Area        string `json:"area"`
		} `json:"create_quests"`
		CreateRoutines []struct {
			Title         string  `json:"title"`
			Frequency     string  `json:"frequency"`
			ScheduledTime *string `json:"scheduled_time"`
		} `json:"create_routines"`
		UpdateQuest *struct {
			Title     string `json:"title"`
			Increment int    `json:"increment"`
		} `json:"update_quest"`
		CompleteTask *struct {
			Title string `json:"title"`
		} `json:"complete_task"`
		CompleteRoutines []string `json:"complete_routines"`
		DeleteQuest *struct {
			Title string `json:"title"`
		} `json:"delete_quest"`
		DeleteRoutine *struct {
			Title string `json:"title"`
		} `json:"delete_routine"`
		MorningCheckin *struct {
			Mood         int      `json:"mood"`
			SleepQuality int      `json:"sleep_quality"`
			TopPriority  string   `json:"top_priority"`
			Intentions   []string `json:"intentions"`
			EnergyLevel  int      `json:"energy_level"`
		} `json:"morning_checkin"`
		EveningCheckin *struct {
			Mood            int    `json:"mood"`
			BiggestWin      string `json:"biggest_win"`
			Blockers        string `json:"blockers"`
			GoalForTomorrow string `json:"goal_for_tomorrow"`
			GratefulFor     string `json:"grateful_for"`
		} `json:"evening_checkin"`
		CreateWeeklyGoals   []string `json:"create_weekly_goals"`
		CompleteWeeklyGoal *struct {
			Content string `json:"content"`
		} `json:"complete_weekly_goal"`
		CreateTask *ChatTaskInput `json:"create_task"`
		CreateJournalEntry *struct {
			Mood       string `json:"mood"`
			Transcript string `json:"transcript"`
		} `json:"create_journal_entry"`
		ShowCard *string `json:"show_card"`
	}

	if err := json.Unmarshal([]byte(responseText), &aiResp); err != nil {
		fmt.Printf("⚠️ JSON parse failed: %v — using raw text as reply\n", err)
		return &SendMessageResponse{Reply: responseText}, nil
	}

	response := &SendMessageResponse{Reply: aiResp.Reply, ShowCard: aiResp.ShowCard}

	// Handle focus intent (scheduled blocking with times)
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

	// Handle immediate app blocking
	if aiResp.BlockNow {
		response.Action = &ActionData{
			Type: "block_apps",
		}
	}

	// Handle app unblocking
	if aiResp.UnblockNow {
		response.Action = &ActionData{
			Type: "unblock_apps",
		}
	}

	// Handle quest creation (multiple)
	if len(aiResp.CreateQuests) > 0 {
		createdCount := 0
		for _, q := range aiResp.CreateQuests {
			if q.Title == "" {
				continue
			}
			_, err := h.createQuestFromChat(ctx, userID, q.Title, q.TargetValue, q.Area)
			if err != nil {
				fmt.Printf("⚠️ Failed to create quest '%s': %v\n", q.Title, err)
			} else {
				createdCount++
			}
		}
		if createdCount > 0 {
			response.Action = &ActionData{Type: "quests_created"}
		}
	}

	// Handle routine creation (multiple)
	if len(aiResp.CreateRoutines) > 0 {
		createdCount := 0
		for _, r := range aiResp.CreateRoutines {
			if r.Title == "" {
				continue
			}
			_, err := h.createRoutineFromChat(ctx, userID, r.Title, r.Frequency, r.ScheduledTime)
			if err != nil {
				fmt.Printf("⚠️ Failed to create routine '%s': %v\n", r.Title, err)
			} else {
				createdCount++
			}
		}
		if createdCount > 0 {
			response.Action = &ActionData{Type: "routines_created"}
		}
	}

	// Handle quest update (increment progress)
	if aiResp.UpdateQuest != nil && aiResp.UpdateQuest.Title != "" {
		err := h.updateQuestProgress(ctx, userID, aiResp.UpdateQuest.Title, aiResp.UpdateQuest.Increment)
		if err != nil {
			fmt.Printf("⚠️ Failed to update quest '%s': %v\n", aiResp.UpdateQuest.Title, err)
		} else {
			response.Action = &ActionData{Type: "quest_updated"}
		}
	}

	// Handle task completion
	if aiResp.CompleteTask != nil && aiResp.CompleteTask.Title != "" {
		err := h.completeTaskByTitle(ctx, userID, aiResp.CompleteTask.Title)
		if err != nil {
			fmt.Printf("⚠️ Failed to complete task '%s': %v\n", aiResp.CompleteTask.Title, err)
		} else {
			response.Action = &ActionData{Type: "task_completed"}
		}
	}

	// Handle routine completion (multiple)
	if len(aiResp.CompleteRoutines) > 0 {
		completedCount := 0
		for _, title := range aiResp.CompleteRoutines {
			if title == "" {
				continue
			}
			err := h.completeRoutineByTitle(ctx, userID, title)
			if err != nil {
				fmt.Printf("⚠️ Failed to complete routine '%s': %v\n", title, err)
			} else {
				completedCount++
			}
		}
		if completedCount > 0 {
			response.Action = &ActionData{Type: "routines_completed"}
		}
	}

	// Handle quest deletion
	if aiResp.DeleteQuest != nil && aiResp.DeleteQuest.Title != "" {
		err := h.deleteQuestByTitle(ctx, userID, aiResp.DeleteQuest.Title)
		if err != nil {
			fmt.Printf("⚠️ Failed to delete quest '%s': %v\n", aiResp.DeleteQuest.Title, err)
		} else {
			response.Action = &ActionData{Type: "quest_deleted"}
		}
	}

	// Handle routine deletion
	if aiResp.DeleteRoutine != nil && aiResp.DeleteRoutine.Title != "" {
		err := h.deleteRoutineByTitle(ctx, userID, aiResp.DeleteRoutine.Title)
		if err != nil {
			fmt.Printf("⚠️ Failed to delete routine '%s': %v\n", aiResp.DeleteRoutine.Title, err)
		} else {
			response.Action = &ActionData{Type: "routine_deleted"}
		}
	}

	// Handle morning check-in
	if aiResp.MorningCheckin != nil && aiResp.MorningCheckin.Mood > 0 {
		err := h.saveMorningCheckin(ctx, userID, aiResp.MorningCheckin.Mood, aiResp.MorningCheckin.SleepQuality, aiResp.MorningCheckin.TopPriority, aiResp.MorningCheckin.Intentions, aiResp.MorningCheckin.EnergyLevel)
		if err != nil {
			fmt.Printf("⚠️ Failed to save morning check-in: %v\n", err)
		} else {
			response.Action = &ActionData{Type: "morning_checkin_saved"}
		}
	}

	// Handle evening check-in
	if aiResp.EveningCheckin != nil && aiResp.EveningCheckin.Mood > 0 {
		err := h.saveEveningCheckin(ctx, userID, aiResp.EveningCheckin.Mood, aiResp.EveningCheckin.BiggestWin, aiResp.EveningCheckin.Blockers, aiResp.EveningCheckin.GoalForTomorrow, aiResp.EveningCheckin.GratefulFor)
		if err != nil {
			fmt.Printf("⚠️ Failed to save evening check-in: %v\n", err)
		} else {
			response.Action = &ActionData{Type: "evening_checkin_saved"}
		}
	}

	// Handle weekly goals creation
	if len(aiResp.CreateWeeklyGoals) > 0 {
		err := h.createWeeklyGoals(ctx, userID, aiResp.CreateWeeklyGoals)
		if err != nil {
			fmt.Printf("⚠️ Failed to create weekly goals: %v\n", err)
		} else {
			response.Action = &ActionData{Type: "weekly_goals_created"}
		}
	}

	// Handle weekly goal completion
	if aiResp.CompleteWeeklyGoal != nil && aiResp.CompleteWeeklyGoal.Content != "" {
		err := h.completeWeeklyGoal(ctx, userID, aiResp.CompleteWeeklyGoal.Content)
		if err != nil {
			fmt.Printf("⚠️ Failed to complete weekly goal '%s': %v\n", aiResp.CompleteWeeklyGoal.Content, err)
		} else {
			response.Action = &ActionData{Type: "weekly_goal_completed"}
		}
	}

	// Handle task creation (calendar task, not focus)
	if aiResp.CreateTask != nil && aiResp.CreateTask.Title != "" {
		taskID, err := h.createCalendarTask(ctx, userID, aiResp.CreateTask)
		if err != nil {
			fmt.Printf("⚠️ Failed to create task '%s': %v\n", aiResp.CreateTask.Title, err)
		} else {
			response.Action = &ActionData{Type: "task_created", TaskID: &taskID}
		}
	}

	// Handle journal entry creation
	if aiResp.CreateJournalEntry != nil && aiResp.CreateJournalEntry.Transcript != "" {
		err := h.createJournalEntry(ctx, userID, aiResp.CreateJournalEntry.Mood, aiResp.CreateJournalEntry.Transcript)
		if err != nil {
			fmt.Printf("⚠️ Failed to create journal entry: %v\n", err)
		} else {
			response.Action = &ActionData{Type: "journal_entry_created"}
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
			$6, 'high', true, 'Créé par le coach', true
		)
		RETURNING id
	`, userID, taskData.Title, taskData.Date, taskData.ScheduledStart, taskData.ScheduledEnd, timeBlock).Scan(&taskID)

	return taskID, err
}

// ===========================================
// QUEST CREATION (from coach chat)
// ===========================================

func (h *Handler) createQuestFromChat(ctx context.Context, userID, title string, targetValue int, areaSlug string) (string, error) {
	if targetValue <= 0 {
		targetValue = 1
	}
	if areaSlug == "" {
		areaSlug = "other"
	}

	// Find or create the user's area matching the slug
	areaID, err := h.findOrCreateArea(ctx, userID, areaSlug)
	if err != nil {
		return "", fmt.Errorf("failed to find/create area: %w", err)
	}

	var questID string
	err = h.db.QueryRow(ctx, `
		INSERT INTO quests (user_id, area_id, title, target_value, current_value, status)
		VALUES ($1, $2, $3, $4, 0, 'active')
		RETURNING id
	`, userID, areaID, title, targetValue).Scan(&questID)

	if err != nil {
		return "", err
	}

	fmt.Printf("✅ Quest created from chat: %s (target: %d, area: %s)\n", title, targetValue, areaSlug)
	return questID, nil
}

// ===========================================
// ROUTINE CREATION (from coach chat)
// ===========================================

func (h *Handler) createRoutineFromChat(ctx context.Context, userID, title, frequency string, scheduledTime *string) (string, error) {
	if frequency == "" {
		frequency = "daily"
	}

	// Routines require an area_id (NOT NULL) — use "other" as default
	areaID, err := h.findOrCreateArea(ctx, userID, "other")
	if err != nil {
		return "", fmt.Errorf("failed to find/create area for routine: %w", err)
	}

	var routineID string
	err = h.db.QueryRow(ctx, `
		INSERT INTO routines (user_id, area_id, title, frequency, scheduled_time)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id, title) DO UPDATE SET frequency = EXCLUDED.frequency, scheduled_time = EXCLUDED.scheduled_time
		RETURNING id
	`, userID, areaID, title, frequency, scheduledTime).Scan(&routineID)

	if err != nil {
		return "", err
	}

	timeStr := ""
	if scheduledTime != nil {
		timeStr = fmt.Sprintf(" à %s", *scheduledTime)
	}
	fmt.Printf("✅ Routine created from chat: %s (%s%s)\n", title, frequency, timeStr)
	return routineID, nil
}

// ===========================================
// AREA HELPER (find or create)
// ===========================================

var areaDefaults = map[string]struct {
	Name string
	Icon string
}{
	"health":        {Name: "Santé", Icon: "heart"},
	"learning":      {Name: "Apprentissage", Icon: "book"},
	"career":        {Name: "Carrière", Icon: "briefcase"},
	"relationships": {Name: "Relations", Icon: "person.2"},
	"creativity":    {Name: "Créativité", Icon: "paintbrush"},
	"other":         {Name: "Autre", Icon: "star"},
}

func (h *Handler) findOrCreateArea(ctx context.Context, userID, slug string) (string, error) {
	// Try to find existing area by slug
	var areaID string
	err := h.db.QueryRow(ctx, `
		SELECT id FROM areas WHERE user_id = $1 AND slug = $2
	`, userID, slug).Scan(&areaID)

	if err == nil {
		return areaID, nil
	}

	// Not found — create it with defaults
	defaults, ok := areaDefaults[slug]
	if !ok {
		defaults = areaDefaults["other"]
		slug = "other"
	}

	err = h.db.QueryRow(ctx, `
		INSERT INTO areas (user_id, name, slug, icon)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, slug) DO UPDATE SET name = EXCLUDED.name
		RETURNING id
	`, userID, defaults.Name, slug, defaults.Icon).Scan(&areaID)

	return areaID, err
}

// ===========================================
// QUEST PROGRESS UPDATE (from coach chat)
// ===========================================

func (h *Handler) updateQuestProgress(ctx context.Context, userID, questTitle string, increment int) error {
	if increment <= 0 {
		increment = 1
	}

	// Find the best matching active quest (exact match first, then fuzzy, LIMIT 1)
	result, err := h.db.Exec(ctx, `
		UPDATE quests SET
			current_value = LEAST(current_value + $3, target_value),
			status = CASE
				WHEN current_value + $3 >= target_value THEN 'completed'
				ELSE status
			END
		WHERE id = (
			SELECT id FROM quests
			WHERE user_id = $1 AND status = 'active'
			AND LOWER(title) LIKE '%' || LOWER($2) || '%'
			ORDER BY
				CASE WHEN LOWER(title) = LOWER($2) THEN 0 ELSE 1 END,
				created_at DESC
			LIMIT 1
		)
	`, userID, questTitle, increment)

	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("no active quest matching '%s'", questTitle)
	}

	fmt.Printf("✅ Quest progress updated: '%s' +%d\n", questTitle, increment)
	return nil
}

// ===========================================
// TASK COMPLETION (from coach chat)
// ===========================================

func (h *Handler) completeTaskByTitle(ctx context.Context, userID, title string) error {
	result, err := h.db.Exec(ctx, `
		UPDATE tasks SET status = 'completed', completed_at = NOW()
		WHERE id = (
			SELECT id FROM tasks
			WHERE user_id = $1 AND date = CURRENT_DATE AND status != 'completed'
			AND LOWER(title) LIKE '%' || LOWER($2) || '%'
			ORDER BY
				CASE WHEN LOWER(title) = LOWER($2) THEN 0 ELSE 1 END,
				created_at DESC
			LIMIT 1
		)
	`, userID, title)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("no pending task matching '%s'", title)
	}
	fmt.Printf("✅ Task completed: '%s'\n", title)
	return nil
}

// ===========================================
// ROUTINE COMPLETION (from coach chat)
// ===========================================

func (h *Handler) completeRoutineByTitle(ctx context.Context, userID, title string) error {
	// Find the routine by fuzzy title match
	var routineID string
	err := h.db.QueryRow(ctx, `
		SELECT id FROM routines
		WHERE user_id = $1
		AND LOWER(title) LIKE '%' || LOWER($2) || '%'
		ORDER BY
			CASE WHEN LOWER(title) = LOWER($2) THEN 0 ELSE 1 END,
			created_at DESC
		LIMIT 1
	`, userID, title).Scan(&routineID)
	if err != nil {
		return fmt.Errorf("no routine matching '%s'", title)
	}

	// Check if already completed today
	var exists bool
	h.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM routine_completions
			WHERE user_id = $1 AND routine_id = $2 AND DATE(completed_at) = CURRENT_DATE
		)
	`, userID, routineID).Scan(&exists)
	if exists {
		return nil // Already done today, idempotent
	}

	_, err = h.db.Exec(ctx, `
		INSERT INTO routine_completions (id, user_id, routine_id, completed_at)
		VALUES ($1, $2, $3, NOW())
	`, uuid.New().String(), userID, routineID)
	if err != nil {
		return err
	}
	fmt.Printf("✅ Routine completed: '%s'\n", title)
	return nil
}

// ===========================================
// QUEST DELETION (from coach chat)
// ===========================================

func (h *Handler) deleteQuestByTitle(ctx context.Context, userID, title string) error {
	result, err := h.db.Exec(ctx, `
		DELETE FROM quests
		WHERE id = (
			SELECT id FROM quests
			WHERE user_id = $1
			AND LOWER(title) LIKE '%' || LOWER($2) || '%'
			ORDER BY
				CASE WHEN LOWER(title) = LOWER($2) THEN 0 ELSE 1 END,
				created_at DESC
			LIMIT 1
		)
	`, userID, title)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("no quest matching '%s'", title)
	}
	fmt.Printf("✅ Quest deleted: '%s'\n", title)
	return nil
}

// ===========================================
// ROUTINE DELETION (from coach chat)
// ===========================================

func (h *Handler) deleteRoutineByTitle(ctx context.Context, userID, title string) error {
	result, err := h.db.Exec(ctx, `
		DELETE FROM routines
		WHERE id = (
			SELECT id FROM routines
			WHERE user_id = $1
			AND LOWER(title) LIKE '%' || LOWER($2) || '%'
			ORDER BY
				CASE WHEN LOWER(title) = LOWER($2) THEN 0 ELSE 1 END,
				created_at DESC
			LIMIT 1
		)
	`, userID, title)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("no routine matching '%s'", title)
	}
	fmt.Printf("✅ Routine deleted: '%s'\n", title)
	return nil
}

// ===========================================
// MORNING CHECK-IN (from coach chat)
// ===========================================

func (h *Handler) saveMorningCheckin(ctx context.Context, userID string, mood, sleepQuality int, topPriority string, intentions []string, energyLevel int) error {
	if mood < 1 {
		mood = 3
	}
	if sleepQuality < 1 {
		sleepQuality = 3
	}
	if energyLevel < 1 {
		energyLevel = 3
	}

	intentionsJSON, _ := json.Marshal(intentions)

	_, err := h.db.Exec(ctx, `
		INSERT INTO morning_checkins (id, user_id, date, morning_mood, sleep_quality, top_priority, intentions, energy_level)
		VALUES ($1, $2, CURRENT_DATE, $3, $4, $5, $6::jsonb, $7)
		ON CONFLICT (user_id, date) DO UPDATE SET
			morning_mood = EXCLUDED.morning_mood,
			sleep_quality = EXCLUDED.sleep_quality,
			top_priority = EXCLUDED.top_priority,
			intentions = EXCLUDED.intentions,
			energy_level = EXCLUDED.energy_level,
			updated_at = NOW()
	`, uuid.New().String(), userID, mood, sleepQuality, topPriority, string(intentionsJSON), energyLevel)
	if err != nil {
		return err
	}
	fmt.Printf("✅ Morning check-in saved (mood: %d, sleep: %d)\n", mood, sleepQuality)
	return nil
}

// ===========================================
// EVENING CHECK-IN (from coach chat)
// ===========================================

func (h *Handler) saveEveningCheckin(ctx context.Context, userID string, mood int, biggestWin, blockers, goalForTomorrow, gratefulFor string) error {
	if mood < 1 {
		mood = 3
	}

	// Get today's stats for the evening check-in
	var ritualsCompleted, tasksCompleted, focusMinutes int
	h.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM routine_completions
		WHERE user_id = $1 AND DATE(completed_at) = CURRENT_DATE
	`, userID).Scan(&ritualsCompleted)
	h.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM tasks
		WHERE user_id = $1 AND date = CURRENT_DATE AND status = 'completed'
	`, userID).Scan(&tasksCompleted)
	h.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(duration_minutes), 0) FROM focus_sessions
		WHERE user_id = $1 AND DATE(started_at) = CURRENT_DATE AND status = 'completed'
	`, userID).Scan(&focusMinutes)

	_, err := h.db.Exec(ctx, `
		INSERT INTO evening_checkins (id, user_id, date, evening_mood, biggest_win, blockers, rituals_completed, tasks_completed, focus_minutes, goal_for_tomorrow, grateful_for)
		VALUES ($1, $2, CURRENT_DATE, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (user_id, date) DO UPDATE SET
			evening_mood = EXCLUDED.evening_mood,
			biggest_win = EXCLUDED.biggest_win,
			blockers = EXCLUDED.blockers,
			rituals_completed = EXCLUDED.rituals_completed,
			tasks_completed = EXCLUDED.tasks_completed,
			focus_minutes = EXCLUDED.focus_minutes,
			goal_for_tomorrow = EXCLUDED.goal_for_tomorrow,
			grateful_for = EXCLUDED.grateful_for,
			updated_at = NOW()
	`, uuid.New().String(), userID, mood, biggestWin, blockers, ritualsCompleted, tasksCompleted, focusMinutes, goalForTomorrow, gratefulFor)
	if err != nil {
		return err
	}
	fmt.Printf("✅ Evening check-in saved (mood: %d, rituals: %d, tasks: %d, focus: %dm)\n", mood, ritualsCompleted, tasksCompleted, focusMinutes)
	return nil
}

// ===========================================
// WEEKLY GOALS (from coach chat)
// ===========================================

func (h *Handler) createWeeklyGoals(ctx context.Context, userID string, goals []string) error {
	// Get Monday of current week
	now := time.Now()
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	monday := now.AddDate(0, 0, -(weekday - 1))
	weekStart := monday.Format("2006-01-02")

	// Create or get weekly_goals record
	var weeklyGoalID string
	err := h.db.QueryRow(ctx, `
		INSERT INTO weekly_goals (id, user_id, week_start_date)
		VALUES ($1, $2, $3::date)
		ON CONFLICT (user_id, week_start_date) DO UPDATE SET updated_at = NOW()
		RETURNING id
	`, uuid.New().String(), userID, weekStart).Scan(&weeklyGoalID)
	if err != nil {
		return fmt.Errorf("failed to create weekly_goals: %w", err)
	}

	// Delete existing items for this week (replace all)
	h.db.Exec(ctx, `DELETE FROM weekly_goal_items WHERE weekly_goal_id = $1`, weeklyGoalID)

	// Insert new goals
	for i, goal := range goals {
		if goal == "" {
			continue
		}
		_, err := h.db.Exec(ctx, `
			INSERT INTO weekly_goal_items (id, weekly_goal_id, content, position, is_completed)
			VALUES ($1, $2, $3, $4, false)
		`, uuid.New().String(), weeklyGoalID, goal, i)
		if err != nil {
			fmt.Printf("⚠️ Failed to create weekly goal item '%s': %v\n", goal, err)
		}
	}

	fmt.Printf("✅ Weekly goals created: %d items\n", len(goals))
	return nil
}

func (h *Handler) completeWeeklyGoal(ctx context.Context, userID, content string) error {
	// Get Monday of current week
	now := time.Now()
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	monday := now.AddDate(0, 0, -(weekday - 1))
	weekStart := monday.Format("2006-01-02")

	result, err := h.db.Exec(ctx, `
		UPDATE weekly_goal_items SET is_completed = true
		WHERE id = (
			SELECT wgi.id FROM weekly_goal_items wgi
			JOIN weekly_goals wg ON wg.id = wgi.weekly_goal_id
			WHERE wg.user_id = $1 AND wg.week_start_date = $2::date
			AND LOWER(wgi.content) LIKE '%' || LOWER($3) || '%'
			AND wgi.is_completed = false
			ORDER BY
				CASE WHEN LOWER(wgi.content) = LOWER($3) THEN 0 ELSE 1 END
			LIMIT 1
		)
	`, userID, weekStart, content)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("no weekly goal matching '%s'", content)
	}
	fmt.Printf("✅ Weekly goal completed: '%s'\n", content)
	return nil
}

// ===========================================
// CALENDAR TASK CREATION (from coach chat)
// ===========================================

// ChatTaskInput is used for task creation from chat
type ChatTaskInput struct {
	Title          string `json:"title"`
	Date           string `json:"date"`
	TimeBlock      string `json:"time_block"`
	ScheduledStart string `json:"scheduled_start"`
	ScheduledEnd   string `json:"scheduled_end"`
}

func (h *Handler) createCalendarTask(ctx context.Context, userID string, task *ChatTaskInput) (string, error) {
	date := task.Date
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	timeBlock := task.TimeBlock
	if timeBlock == "" {
		timeBlock = "morning"
	}

	var taskID string
	var err error

	if task.ScheduledStart != "" && task.ScheduledEnd != "" {
		err = h.db.QueryRow(ctx, `
			INSERT INTO tasks (user_id, title, date, time_block, scheduled_start, scheduled_end, priority, is_ai_generated, ai_notes)
			VALUES ($1, $2, $3, $4, $5::time, $6::time, 'medium', true, 'Créé par le coach')
			RETURNING id
		`, userID, task.Title, date, timeBlock, task.ScheduledStart, task.ScheduledEnd).Scan(&taskID)
	} else {
		err = h.db.QueryRow(ctx, `
			INSERT INTO tasks (user_id, title, date, time_block, priority, is_ai_generated, ai_notes)
			VALUES ($1, $2, $3, $4, 'medium', true, 'Créé par le coach')
			RETURNING id
		`, userID, task.Title, date, timeBlock).Scan(&taskID)
	}

	if err != nil {
		return "", err
	}
	fmt.Printf("✅ Calendar task created: '%s' on %s\n", task.Title, date)
	return taskID, nil
}

// ===========================================
// JOURNAL ENTRY (from coach chat)
// ===========================================

func (h *Handler) createJournalEntry(ctx context.Context, userID, mood, transcript string) error {
	if mood == "" {
		mood = "neutral"
	}

	// Try to add 'text' to allowed media_types (safe to run multiple times)
	h.db.Exec(ctx, `
		ALTER TABLE journal_entries DROP CONSTRAINT IF EXISTS journal_entries_media_type_check
	`)
	h.db.Exec(ctx, `
		ALTER TABLE journal_entries ADD CONSTRAINT journal_entries_media_type_check
		CHECK (media_type IN ('audio', 'video', 'text'))
	`)

	_, err := h.db.Exec(ctx, `
		INSERT INTO journal_entries (id, user_id, media_type, transcript, mood, entry_date)
		VALUES ($1, $2, 'text', $3, $4, CURRENT_DATE)
	`, uuid.New().String(), userID, transcript, mood)
	if err != nil {
		return err
	}
	fmt.Printf("✅ Journal entry created (mood: %s)\n", mood)
	return nil
}

// ===========================================
// STREAK UPDATE
// Streak logic moved to internal/streak package (shared across handlers)
