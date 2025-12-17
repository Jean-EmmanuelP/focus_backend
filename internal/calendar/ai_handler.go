package calendar

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"firelevel-backend/internal/auth"

	"github.com/jackc/pgx/v5/pgxpool"
)

type AIHandler struct {
	db     *pgxpool.Pool
	apiKey string
}

func NewAIHandler(db *pgxpool.Pool) *AIHandler {
	return &AIHandler{
		db:     db,
		apiKey: os.Getenv("GEMINI_API_KEY"),
	}
}

// ==========================================
// AI REQUEST/RESPONSE TYPES
// ==========================================

type GenerateDayPlanRequest struct {
	IdealDayPrompt string `json:"idealDayPrompt"`
	Date           string `json:"date"`
}

type GenerateDayPlanResponse struct {
	DayPlan   DayPlan `json:"dayPlan"`
	Tasks     []Task  `json:"tasks"`
	AISummary string  `json:"aiSummary"`
}

type GenerateTasksRequest struct {
	TimeBlockID string `json:"timeBlockId"`
	Context     string `json:"context"`
	Count       int    `json:"count"`
}

type GeminiRequest struct {
	Contents []GeminiContent `json:"contents"`
}

type GeminiContent struct {
	Parts []GeminiPart `json:"parts"`
}

type GeminiPart struct {
	Text string `json:"text"`
}

type GeminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

// AI-parsed structures
type AIParsedDayPlan struct {
	Summary    string            `json:"summary"`
	TimeBlocks []AIParsedBlock   `json:"timeBlocks"`
}

type AIParsedBlock struct {
	Title       string          `json:"title"`
	StartTime   string          `json:"startTime"`
	EndTime     string          `json:"endTime"`
	Description string          `json:"description"`
	Category    string          `json:"category"`
	Tasks       []AIParsedTask  `json:"tasks"`
}

type AIParsedTask struct {
	Title            string `json:"title"`
	Description      string `json:"description"`
	EstimatedMinutes int    `json:"estimatedMinutes"`
	Priority         string `json:"priority"`
}

// ==========================================
// AI HANDLERS
// ==========================================

func (h *AIHandler) GenerateDayPlan(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	var req GenerateDayPlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Date == "" {
		req.Date = time.Now().Format("2006-01-02")
	}

	// Get user's quests (projects) for context
	quests, _ := h.getUserQuests(r.Context(), userID)

	// Get user's areas
	areas, _ := h.getUserAreas(r.Context(), userID)

	// Build the AI prompt
	prompt := h.buildDayPlanPrompt(req.IdealDayPrompt, req.Date, quests, areas)

	// Call Gemini API
	aiResponse, err := h.callGemini(prompt)
	if err != nil {
		http.Error(w, "Failed to generate plan: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Parse AI response
	parsedPlan, err := h.parseAIDayPlan(aiResponse)
	if err != nil {
		http.Error(w, "Failed to parse AI response: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Create day plan in database
	dayPlan, err := h.createDayPlanFromAI(r.Context(), userID, req.Date, req.IdealDayPrompt, parsedPlan, areas, quests)
	if err != nil {
		http.Error(w, "Failed to save plan: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Store AI conversation for context
	h.storeAIConversation(r.Context(), userID, dayPlan.DayPlan.ID, "day_planning", req.IdealDayPrompt, aiResponse)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dayPlan)
}

func (h *AIHandler) GenerateTasksForBlock(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	var req GenerateTasksRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Count == 0 {
		req.Count = 5
	}

	// Get the time block
	var block TimeBlock
	err := h.db.QueryRow(r.Context(), `
		SELECT id, title, description, start_time, end_time
		FROM time_blocks
		WHERE id = $1 AND user_id = $2
	`, req.TimeBlockID, userID).Scan(&block.ID, &block.Title, &block.Description, &block.StartTime, &block.EndTime)

	if err != nil {
		http.Error(w, "Time block not found", http.StatusNotFound)
		return
	}

	// Get user's quests (projects) for context
	quests, _ := h.getUserQuests(r.Context(), userID)

	// Build prompt for task generation
	prompt := h.buildTasksPrompt(block, req.Context, req.Count, quests)

	// Call Gemini
	aiResponse, err := h.callGemini(prompt)
	if err != nil {
		http.Error(w, "Failed to generate tasks: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Parse tasks from AI response
	tasks, err := h.parseAITasks(aiResponse)
	if err != nil {
		http.Error(w, "Failed to parse tasks: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Create tasks in database
	createdTasks, err := h.createTasksFromAI(r.Context(), userID, req.TimeBlockID, tasks)
	if err != nil {
		http.Error(w, "Failed to save tasks: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(createdTasks)
}

// ==========================================
// AI PROMPT BUILDERS
// ==========================================

func (h *AIHandler) buildDayPlanPrompt(idealDay, date string, quests []Quest, areas []Area) string {
	// Build quest context (quests = projects)
	var questContext strings.Builder
	for _, q := range quests {
		questContext.WriteString(fmt.Sprintf("- %s (objectif: %d/%d)\n", q.Title, q.CurrentValue, q.TargetValue))
	}

	// Build area context
	var areaContext strings.Builder
	for _, a := range areas {
		areaContext.WriteString(fmt.Sprintf("- %s (%s)\n", a.Name, a.Icon))
	}

	prompt := fmt.Sprintf(`Tu es un assistant de productivité expert. L'utilisateur décrit sa journée idéale et tu dois créer un plan structuré.

DATE: %s

DESCRIPTION DE LA JOURNÉE IDÉALE:
%s

PROJETS/QUÊTES EN COURS:
%s

CATÉGORIES (AREAS):
%s

INSTRUCTIONS:
1. Analyse la description et crée des blocs de temps réalistes
2. Pour chaque bloc, génère des tâches concrètes et actionnables
3. Si l'utilisateur mentionne un projet/quête, utilise-le pour créer les tâches
4. Chaque tâche doit avoir une durée estimée réaliste
5. Lie les tâches aux catégories appropriées

RÉPONDS UNIQUEMENT EN JSON avec ce format exact:
{
  "summary": "Résumé motivant de la journée en 1-2 phrases",
  "timeBlocks": [
    {
      "title": "Titre du bloc",
      "startTime": "HH:MM",
      "endTime": "HH:MM",
      "description": "Description courte",
      "category": "health|career|learning|relationships|creativity|finance",
      "tasks": [
        {
          "title": "Titre de la tâche",
          "description": "Description détaillée si nécessaire",
          "estimatedMinutes": 30,
          "priority": "low|medium|high|urgent"
        }
      ]
    }
  ]
}`, date, idealDay, questContext.String(), areaContext.String())

	return prompt
}

func (h *AIHandler) buildTasksPrompt(block TimeBlock, context string, count int, quests []Quest) string {
	duration := block.EndTime.Sub(block.StartTime).Minutes()

	// Build quest context (quests = projects)
	var questContext strings.Builder
	for _, q := range quests {
		questContext.WriteString(fmt.Sprintf("- %s (objectif: %d/%d)\n", q.Title, q.CurrentValue, q.TargetValue))
	}

	prompt := fmt.Sprintf(`Tu es un assistant de productivité. Génère %d tâches concrètes pour ce bloc de temps.

BLOC DE TEMPS:
- Titre: %s
- Description: %s
- Durée: %.0f minutes
- Contexte additionnel: %s

PROJETS/QUÊTES EN COURS:
%s

INSTRUCTIONS:
1. Génère exactement %d tâches actionnables
2. Chaque tâche doit être spécifique et réalisable
3. La somme des durées estimées doit approximer la durée du bloc
4. Les tâches doivent être de haute qualité pour minimiser la réflexion de l'utilisateur

RÉPONDS UNIQUEMENT EN JSON avec ce format exact:
{
  "tasks": [
    {
      "title": "Titre concis",
      "description": "Description détaillée avec étapes si nécessaire",
      "estimatedMinutes": 15,
      "priority": "medium"
    }
  ]
}`, count, block.Title, safeString(block.Description), duration, context, questContext.String(), count)

	return prompt
}

// ==========================================
// GEMINI API
// ==========================================

func (h *AIHandler) callGemini(prompt string) (string, error) {
	if h.apiKey == "" {
		return "", fmt.Errorf("GEMINI_API_KEY not configured")
	}

	// Models to try in order (primary, then fallbacks)
	models := []string{
		"gemini-2.0-flash",  // Primary - newest
		"gemini-1.5-flash",  // Fallback 1 - stable
		"gemini-1.5-pro",    // Fallback 2 - most capable
	}

	reqBody := GeminiRequest{
		Contents: []GeminiContent{
			{
				Parts: []GeminiPart{
					{Text: prompt},
				},
			},
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	var lastErr error
	for i, model := range models {
		// Try each model with retry
		for retry := 0; retry < 2; retry++ {
			url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, h.apiKey)

			resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonBody))
			if err != nil {
				lastErr = err
				break // Network error, try next model
			}

			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				lastErr = err
				break
			}

			if resp.StatusCode != http.StatusOK {
				lastErr = fmt.Errorf("Gemini API error (%s): %s", model, string(body))

				// Check if it's an overload error
				if resp.StatusCode == 503 || strings.Contains(string(body), "overloaded") || strings.Contains(string(body), "UNAVAILABLE") {
					fmt.Printf("[AI] Model %s overloaded (attempt %d), retrying...\n", model, retry+1)
					time.Sleep(time.Duration(500*(retry+1)) * time.Millisecond)
					continue
				}
				// Other error, try next model
				break
			}

			var geminiResp GeminiResponse
			if err := json.Unmarshal(body, &geminiResp); err != nil {
				lastErr = err
				break
			}

			if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
				lastErr = fmt.Errorf("empty response from %s", model)
				break
			}

			if i > 0 {
				fmt.Printf("[AI] Successfully used fallback model: %s\n", model)
			}
			return geminiResp.Candidates[0].Content.Parts[0].Text, nil
		}
	}

	return "", fmt.Errorf("all Gemini models failed: %w", lastErr)
}

// ==========================================
// AI RESPONSE PARSERS
// ==========================================

func (h *AIHandler) parseAIDayPlan(response string) (*AIParsedDayPlan, error) {
	// Clean up the response (remove markdown code blocks if present)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	var parsed AIParsedDayPlan
	if err := json.Unmarshal([]byte(response), &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %v, response: %s", err, response)
	}

	return &parsed, nil
}

func (h *AIHandler) parseAITasks(response string) ([]AIParsedTask, error) {
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	var parsed struct {
		Tasks []AIParsedTask `json:"tasks"`
	}
	if err := json.Unmarshal([]byte(response), &parsed); err != nil {
		return nil, err
	}

	return parsed.Tasks, nil
}

// ==========================================
// DATABASE OPERATIONS
// ==========================================

func (h *AIHandler) createDayPlanFromAI(ctx context.Context, userID, date, prompt string, parsed *AIParsedDayPlan, areas []Area, quests []Quest) (*GenerateDayPlanResponse, error) {
	// Create day plan
	var dayPlan DayPlan
	err := h.db.QueryRow(ctx, `
		INSERT INTO day_plans (user_id, date, ideal_day_prompt, ai_summary, status)
		VALUES ($1, $2, $3, $4, 'active')
		ON CONFLICT (user_id, date)
		DO UPDATE SET ideal_day_prompt = EXCLUDED.ideal_day_prompt, ai_summary = EXCLUDED.ai_summary, updated_at = now()
		RETURNING id, user_id, date, ideal_day_prompt, ai_summary, progress, status, created_at, updated_at
	`, userID, date, prompt, parsed.Summary).Scan(
		&dayPlan.ID, &dayPlan.UserID, &dayPlan.Date, &dayPlan.IdealDayPrompt,
		&dayPlan.AISummary, &dayPlan.Progress, &dayPlan.Status,
		&dayPlan.CreatedAt, &dayPlan.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Create area lookup map
	areaMap := make(map[string]string)
	for _, a := range areas {
		areaMap[strings.ToLower(a.Slug)] = a.ID
		areaMap[strings.ToLower(a.Name)] = a.ID
	}

	var allTasks []Task

	for _, block := range parsed.TimeBlocks {
		// Find area ID
		var areaID *string
		if id, ok := areaMap[strings.ToLower(block.Category)]; ok {
			areaID = &id
		}

		// Determine time_block based on start time
		timeBlockStr := "morning"
		startParts := strings.Split(block.StartTime, ":")
		if len(startParts) > 0 {
			var startHour int
			fmt.Sscanf(startParts[0], "%d", &startHour)
			if startHour >= 18 {
				timeBlockStr = "evening"
			} else if startHour >= 12 {
				timeBlockStr = "afternoon"
			}
		}

		// Create tasks directly (no time_blocks)
		for i, task := range block.Tasks {
			var t Task
			var scheduledStart, scheduledEnd *time.Time
			err := h.db.QueryRow(ctx, `
				INSERT INTO tasks (user_id, area_id, title, description, date, scheduled_start, scheduled_end, time_block, position, estimated_minutes, priority, is_ai_generated)
				VALUES ($1, $2, $3, $4, $5, $6::time, $7::time, $8, $9, $10, $11, true)
				RETURNING id, user_id, quest_id, area_id, title, description, date, scheduled_start, scheduled_end, time_block, position, estimated_minutes, actual_minutes, priority, status, due_at, completed_at, is_ai_generated, ai_notes, created_at, updated_at
			`, userID, areaID, task.Title, task.Description, date, block.StartTime, block.EndTime, timeBlockStr, i, task.EstimatedMinutes, task.Priority).Scan(
				&t.ID, &t.UserID, &t.QuestID, &t.AreaID,
				&t.Title, &t.Description, &t.Date, &scheduledStart, &scheduledEnd,
				&t.TimeBlock, &t.Position, &t.EstimatedMinutes, &t.ActualMinutes,
				&t.Priority, &t.Status, &t.DueAt, &t.CompletedAt,
				&t.IsAIGenerated, &t.AINotes, &t.CreatedAt, &t.UpdatedAt,
			)
			if err != nil {
				continue
			}

			// Convert times to HH:mm format
			if scheduledStart != nil {
				s := scheduledStart.Format("15:04")
				t.ScheduledStart = &s
			}
			if scheduledEnd != nil {
				s := scheduledEnd.Format("15:04")
				t.ScheduledEnd = &s
			}

			allTasks = append(allTasks, t)
		}
	}

	return &GenerateDayPlanResponse{
		DayPlan:   dayPlan,
		Tasks:     allTasks,
		AISummary: parsed.Summary,
	}, nil
}

func (h *AIHandler) createTasksFromAI(ctx context.Context, userID, date string, tasks []AIParsedTask) ([]Task, error) {
	var createdTasks []Task

	for i, task := range tasks {
		var t Task
		err := h.db.QueryRow(ctx, `
			INSERT INTO tasks (user_id, title, description, date, time_block, position, estimated_minutes, priority, is_ai_generated)
			VALUES ($1, $2, $3, $4, 'morning', $5, $6, $7, true)
			RETURNING id, user_id, quest_id, area_id, title, description, date, scheduled_start, scheduled_end, time_block, position, estimated_minutes, actual_minutes, priority, status, due_at, completed_at, is_ai_generated, ai_notes, created_at, updated_at
		`, userID, task.Title, task.Description, date, i, task.EstimatedMinutes, task.Priority).Scan(
			&t.ID, &t.UserID, &t.QuestID, &t.AreaID,
			&t.Title, &t.Description, &t.Date, &t.ScheduledStart, &t.ScheduledEnd,
			&t.TimeBlock, &t.Position, &t.EstimatedMinutes, &t.ActualMinutes,
			&t.Priority, &t.Status, &t.DueAt, &t.CompletedAt,
			&t.IsAIGenerated, &t.AINotes, &t.CreatedAt, &t.UpdatedAt,
		)
		if err != nil {
			continue
		}
		createdTasks = append(createdTasks, t)
	}

	return createdTasks, nil
}

func (h *AIHandler) storeAIConversation(ctx context.Context, userID, dayPlanID, convType, userMessage, aiResponse string) {
	h.db.Exec(ctx, `
		INSERT INTO ai_conversations (user_id, day_plan_id, conversation_type, user_message, ai_response, model_used)
		VALUES ($1, $2, $3, $4, $5, 'gemini-1.5-flash')
	`, userID, dayPlanID, convType, userMessage, aiResponse)
}

// ==========================================
// HELPER METHODS
// ==========================================

type Quest struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	CurrentValue int    `json:"currentValue"`
	TargetValue  int    `json:"targetValue"`
}

type Area struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
	Icon string `json:"icon"`
}

func (h *AIHandler) getUserQuests(ctx context.Context, userID string) ([]Quest, error) {
	rows, err := h.db.Query(ctx, `
		SELECT id, title, current_value, target_value
		FROM quests
		WHERE user_id = $1 AND status = 'active'
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var quests []Quest
	for rows.Next() {
		var q Quest
		rows.Scan(&q.ID, &q.Title, &q.CurrentValue, &q.TargetValue)
		quests = append(quests, q)
	}

	return quests, nil
}

func (h *AIHandler) getUserAreas(ctx context.Context, userID string) ([]Area, error) {
	rows, err := h.db.Query(ctx, `
		SELECT id, name, COALESCE(slug, ''), COALESCE(icon, '')
		FROM areas
		WHERE user_id = $1
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var areas []Area
	for rows.Next() {
		var a Area
		rows.Scan(&a.ID, &a.Name, &a.Slug, &a.Icon)
		areas = append(areas, a)
	}

	return areas, nil
}

func safeString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
