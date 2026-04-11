package backboard

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Executor handles the Backboard tool call loop with direct DB access.
type Executor struct {
	db       *pgxpool.Pool
	bbClient *Client
}

// NewExecutor creates a tool executor with DB access and a Backboard client.
func NewExecutor(db *pgxpool.Pool, bbClient *Client) *Executor {
	return &Executor{db: db, bbClient: bbClient}
}

const maxToolCallRounds = 10

// RunToolLoop processes the initial Backboard response and handles tool calls until completion.
// Returns the final AI content and accumulated side effects.
func (e *Executor) RunToolLoop(
	ctx context.Context,
	userID string,
	threadID string,
	assistantID string,
	response *MessageResponse,
	deviceCtx *DeviceContext,
) (string, []SideEffect, error) {
	var allSideEffects []SideEffect

	for round := 0; round < maxToolCallRounds; round++ {
		status := ""
		if response.Status != nil {
			status = *response.Status
		}

		if status != "REQUIRES_ACTION" || len(response.ToolCalls) == 0 {
			break
		}

		runID := ""
		if response.RunID != nil {
			runID = *response.RunID
		}
		if runID == "" {
			break
		}

		var outputs []ToolOutput
		for _, tc := range response.ToolCalls {
			log.Printf("🔧 Tool call [round %d]: %s(%s)", round+1, tc.Function.Name, truncate(tc.Function.Arguments, 200))

			output, effects := e.executeToolCall(ctx, userID, assistantID, tc, deviceCtx)
			outputs = append(outputs, ToolOutput{
				ToolCallID: tc.ID,
				Output:     output,
			})
			allSideEffects = append(allSideEffects, effects...)
		}

		var err error
		response, err = e.bbClient.SubmitToolOutputs(ctx, threadID, runID, outputs)
		if err != nil {
			return "", allSideEffects, fmt.Errorf("submit tool outputs round %d: %w", round+1, err)
		}
	}

	content := "..."
	if response.Content != nil && *response.Content != "" {
		content = *response.Content
	}

	return content, allSideEffects, nil
}

// executeToolCall dispatches a single tool call and returns JSON output + side effects.
func (e *Executor) executeToolCall(
	ctx context.Context,
	userID string,
	assistantID string,
	tc ToolCall,
	deviceCtx *DeviceContext,
) (string, []SideEffect) {
	name := tc.Function.Name
	args := parseArgs(tc.Function.Arguments)

	switch name {

	// ==========================================
	// Context & DateTime
	// ==========================================

	case "get_user_context":
		result, err := e.getUserContext(ctx, userID, deviceCtx)
		if err != nil {
			return errorJSON(err), nil
		}
		return result, nil

	case "get_current_datetime":
		return e.getCurrentDatetime(ctx, userID), nil

	// ==========================================
	// Tasks
	// ==========================================

	case "get_today_tasks":
		result, err := e.getTasksForDate(ctx, userID, todayStr(ctx, userID, e.db))
		if err != nil {
			return errorJSON(err), nil
		}
		return result, nil

	case "get_tasks_for_date":
		dateStr := stringArg(args, "date", todayStr(ctx, userID, e.db))
		result, err := e.getTasksForDate(ctx, userID, dateStr)
		if err != nil {
			return errorJSON(err), nil
		}
		var effects []SideEffect
		if dateStr != todayStr(ctx, userID, e.db) {
			effects = append(effects, NewSideEffectWithData("queried_future_date", map[string]string{"date": dateStr}))
		}
		return result, effects

	case "create_task":
		result, err := e.createTask(ctx, userID, args)
		if err != nil {
			return errorJSON(err), nil
		}
		return result, []SideEffect{
			NewSideEffect("refresh_tasks"),
			NewSideEffect("calendar_needs_refresh"),
			NewSideEffectWithData("show_card", map[string]string{"card_type": "tasks"}),
		}

	case "complete_task":
		taskID := stringArg(args, "task_id", "")
		result, err := e.completeTask(ctx, userID, taskID)
		if err != nil {
			return errorJSON(err), nil
		}
		return result, []SideEffect{
			NewSideEffect("refresh_tasks"),
			NewSideEffect("calendar_needs_refresh"),
		}

	case "uncomplete_task":
		taskID := stringArg(args, "task_id", "")
		result, err := e.uncompleteTask(ctx, userID, taskID)
		if err != nil {
			return errorJSON(err), nil
		}
		return result, []SideEffect{
			NewSideEffect("refresh_tasks"),
			NewSideEffect("calendar_needs_refresh"),
		}

	case "update_task":
		result, err := e.updateTask(ctx, userID, args)
		if err != nil {
			return errorJSON(err), nil
		}
		return result, []SideEffect{
			NewSideEffect("refresh_tasks"),
			NewSideEffect("calendar_needs_refresh"),
		}

	case "delete_task":
		taskID := stringArg(args, "task_id", "")
		result, err := e.deleteTask(ctx, userID, taskID)
		if err != nil {
			return errorJSON(err), nil
		}
		return result, []SideEffect{
			NewSideEffect("refresh_tasks"),
			NewSideEffect("calendar_needs_refresh"),
		}

	// ==========================================
	// Routines
	// ==========================================

	case "get_rituals":
		result, err := e.getRituals(ctx, userID)
		if err != nil {
			return errorJSON(err), nil
		}
		return result, nil

	case "create_routine":
		result, err := e.createRoutine(ctx, userID, args)
		if err != nil {
			return errorJSON(err), nil
		}
		return result, []SideEffect{
			NewSideEffect("refresh_rituals"),
			NewSideEffectWithData("show_card", map[string]string{"card_type": "routines"}),
		}

	case "complete_routine":
		routineID := stringArg(args, "routine_id", "")
		result, err := e.completeRoutine(ctx, userID, routineID)
		if err != nil {
			return errorJSON(err), nil
		}
		return result, []SideEffect{NewSideEffect("refresh_rituals")}

	case "delete_routine":
		routineID := stringArg(args, "routine_id", "")
		result, err := e.deleteRoutine(ctx, userID, routineID)
		if err != nil {
			return errorJSON(err), nil
		}
		return result, []SideEffect{NewSideEffect("refresh_rituals")}

	// ==========================================
	// Reflections & Goals
	// ==========================================

	case "save_morning_checkin":
		result, err := e.saveMorningCheckin(ctx, userID, args)
		if err != nil {
			return errorJSON(err), nil
		}
		return result, []SideEffect{NewSideEffect("refresh_reflection")}

	case "save_evening_review":
		result, err := e.saveEveningReview(ctx, userID, args)
		if err != nil {
			return errorJSON(err), nil
		}
		return result, []SideEffect{NewSideEffect("refresh_reflection")}

	case "create_weekly_goals":
		result, err := e.createWeeklyGoals(ctx, userID, args)
		if err != nil {
			return errorJSON(err), nil
		}
		return result, []SideEffect{NewSideEffect("refresh_weekly_goals")}

	// ==========================================
	// Memory
	// ==========================================

	case "save_memory":
		content := stringArg(args, "content", "")
		category := stringArg(args, "category", "fact")
		memContent := fmt.Sprintf("[%s] %s", category, content)
		if err := e.bbClient.AddMemory(ctx, assistantID, memContent); err != nil {
			return errorJSON(err), nil
		}
		return toJSON(map[string]interface{}{"saved": true}), nil

	// ==========================================
	// Device-Side Tools (return side effects only)
	// ==========================================

	case "start_focus_session":
		duration := intArg(args, "duration_minutes", 0)
		taskID := stringArg(args, "task_id", "")
		taskTitle := stringArg(args, "task_title", "")
		return toJSON(map[string]interface{}{"started": true}), []SideEffect{
			NewSideEffectWithData("start_focus_session", map[string]interface{}{
				"duration_minutes": duration,
				"task_id":          taskID,
				"task_title":       taskTitle,
			}),
		}

	case "block_apps":
		duration := intArg(args, "duration_minutes", 0)
		durationLabel := "30 minutes"
		if duration > 0 {
			durationLabel = fmt.Sprintf("%d minutes", duration)
		}
		return toJSON(map[string]interface{}{
			"status":  "activation_requested",
			"message": fmt.Sprintf("Le blocage d'apps est en cours d'activation pour %s. Si l'utilisateur n'a pas encore configuré ses apps, l'app lui demandera de les sélectionner.", durationLabel),
		}), []SideEffect{
			NewSideEffectWithData("block_apps", map[string]interface{}{"duration_minutes": duration}),
		}

	case "unblock_apps":
		return toJSON(map[string]interface{}{"unblocked": true}), []SideEffect{NewSideEffect("unblock_apps")}

	case "show_force_unblock_card":
		return toJSON(map[string]interface{}{"card_shown": true}), []SideEffect{NewSideEffect("show_force_unblock_card")}

	case "show_card":
		cardType := stringArg(args, "card_type", "tasks")
		return toJSON(map[string]interface{}{"shown": true}), []SideEffect{
			NewSideEffectWithData("show_card", map[string]string{"card_type": cardType}),
		}

	case "save_favorite_video":
		url := stringArg(args, "url", "")
		title := stringArg(args, "title", "")
		return toJSON(map[string]interface{}{"saved": true}), []SideEffect{
			NewSideEffectWithData("save_favorite_video", map[string]string{"url": url, "title": title}),
		}

	case "get_favorite_video":
		// TODO: migrate to DB; for now return empty
		return toJSON(map[string]interface{}{"url": nil, "title": nil}), nil

	case "suggest_ritual_videos":
		category := stringArg(args, "category", "meditation")
		return toJSON(map[string]interface{}{"category": category, "shown": true}), []SideEffect{
			NewSideEffectWithData("show_video_suggestions", map[string]string{"category": category}),
		}

	case "set_morning_block":
		return toJSON(map[string]interface{}{"configured": true}), []SideEffect{
			NewSideEffectWithData("set_morning_block", args),
		}

	case "get_morning_block_status":
		// Use device context passed from frontend
		if deviceCtx != nil {
			return toJSON(map[string]interface{}{
				"enabled":      deviceCtx.MorningBlockEnabled,
				"start":        deviceCtx.MorningBlockStart,
				"end":          deviceCtx.MorningBlockEnd,
			}), nil
		}
		return toJSON(map[string]interface{}{"enabled": false}), nil

	// ==========================================
	// Planning
	// ==========================================

	case "get_planning_context":
		scope := stringArg(args, "scope", "today")
		result, err := e.getPlanningContext(ctx, userID, scope, deviceCtx)
		if err != nil {
			return errorJSON(err), nil
		}
		return result, nil

	case "create_tasks_batch":
		result, err := e.createTasksBatch(ctx, userID, args)
		if err != nil {
			return errorJSON(err), nil
		}
		return result, []SideEffect{
			NewSideEffect("refresh_tasks"),
			NewSideEffect("calendar_needs_refresh"),
			NewSideEffectWithData("show_card", map[string]string{"card_type": "planning"}),
		}

	case "start_morning_flow":
		result, err := e.getMorningFlowContext(ctx, userID, deviceCtx)
		if err != nil {
			return errorJSON(err), nil
		}
		return result, nil

	case "get_calendar_events":
		dateStr := stringArg(args, "date", todayStr(ctx, userID, e.db))
		result, err := e.getCalendarEvents(ctx, userID, dateStr)
		if err != nil {
			return errorJSON(err), nil
		}
		return result, nil

	case "schedule_calendar_blocking":
		result, err := e.scheduleCalendarBlocking(ctx, userID, args)
		if err != nil {
			return errorJSON(err), nil
		}
		return result, []SideEffect{NewSideEffect("refresh_calendar_events")}

	// ==========================================
	// Coaching Diagnostic
	// ==========================================

	case "save_productivity_challenges":
		result, err := e.saveProductivityChallenges(ctx, userID, args)
		if err != nil {
			return errorJSON(err), nil
		}
		return result, nil

	default:
		log.Printf("⚠️ Unknown tool: %s", name)
		return toJSON(map[string]string{"error": "unknown tool: " + name}), nil
	}
}

// ==========================================
// Helpers
// ==========================================

// DeviceContext holds device-only state sent by the frontend.
type DeviceContext struct {
	AppsBlocked          bool   `json:"apps_blocked"`
	AppBlockingAvailable bool   `json:"app_blocking_available"`
	MorningBlockEnabled  bool   `json:"morning_block_enabled"`
	MorningBlockStart    string `json:"morning_block_start"`
	MorningBlockEnd      string `json:"morning_block_end"`
}

func parseArgs(jsonStr string) map[string]interface{} {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &args); err != nil {
		return map[string]interface{}{}
	}
	return args
}

func stringArg(args map[string]interface{}, key, fallback string) string {
	if v, ok := args[key].(string); ok && v != "" {
		return v
	}
	return fallback
}

func stringArrayArg(args map[string]interface{}, key string) []string {
	if arr, ok := args[key].([]interface{}); ok {
		var result []string
		for _, v := range arr {
			if s, ok := v.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

func intArg(args map[string]interface{}, key string, fallback int) int {
	if v, ok := args[key].(float64); ok {
		return int(v)
	}
	return fallback
}

func toJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func errorJSON(err error) string {
	return toJSON(map[string]string{"error": err.Error()})
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// todayStr returns today's date in the user's timezone as YYYY-MM-DD.
func todayStr(ctx context.Context, userID string, db *pgxpool.Pool) string {
	// Try to get user timezone from DB
	var tz string
	err := db.QueryRow(ctx, "SELECT COALESCE(timezone, 'Europe/Paris') FROM public.users WHERE id = $1", userID).Scan(&tz)
	if err != nil {
		tz = "Europe/Paris"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	return time.Now().In(loc).Format("2006-01-02")
}
