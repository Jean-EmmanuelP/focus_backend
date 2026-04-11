package backboard

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// ==========================================
// Context & DateTime Tools
// ==========================================

func (e *Executor) getUserContext(ctx context.Context, userID string, deviceCtx *DeviceContext) (string, error) {
	// Fetch user info
	var userName, companionName, userLanguage string
	var satisfactionScore int
	err := e.db.QueryRow(ctx, `
		SELECT COALESCE(pseudo, first_name, ''),
		       COALESCE(companion_name, 'Kai'),
		       COALESCE(language, 'fr'),
		       COALESCE(satisfaction_score, 45)
		FROM public.users WHERE id = $1
	`, userID).Scan(&userName, &companionName, &userLanguage, &satisfactionScore)
	if err != nil {
		return "", fmt.Errorf("get user: %w", err)
	}

	today := todayStr(ctx, userID, e.db)

	// Count tasks
	var tasksTotal, tasksCompleted int
	e.db.QueryRow(ctx, "SELECT COUNT(*) FROM tasks WHERE user_id = $1 AND date = $2", userID, today).Scan(&tasksTotal)
	e.db.QueryRow(ctx, "SELECT COUNT(*) FROM tasks WHERE user_id = $1 AND date = $2 AND status = 'completed'", userID, today).Scan(&tasksCompleted)

	// Count rituals
	var ritualsTotal, ritualsCompleted int
	e.db.QueryRow(ctx, "SELECT COUNT(*) FROM routines WHERE user_id = $1", userID).Scan(&ritualsTotal)
	e.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM routine_completions
		WHERE user_id = $1 AND completion_date = $2
	`, userID, today).Scan(&ritualsCompleted)

	// Focus minutes today
	var focusMinutes int
	e.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(duration_minutes), 0) FROM focus_sessions
		WHERE user_id = $1 AND DATE(started_at) = $2
	`, userID, today).Scan(&focusMinutes)

	// Time of day
	now := time.Now()
	hour := now.Hour()
	timeOfDay := "night"
	switch {
	case hour >= 5 && hour < 12:
		timeOfDay = "morning"
	case hour >= 12 && hour < 18:
		timeOfDay = "afternoon"
	case hour >= 18 && hour < 22:
		timeOfDay = "evening"
	}

	// Check-in status
	var morningCheckinDone, eveningReviewDone bool
	e.db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM daily_reflections WHERE user_id = $1 AND date = $2 AND mood IS NOT NULL)
	`, userID, today).Scan(&morningCheckinDone)
	e.db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM daily_reflections WHERE user_id = $1 AND date = $2 AND biggest_win IS NOT NULL)
	`, userID, today).Scan(&eveningReviewDone)

	// Streak
	var currentStreak int
	e.db.QueryRow(ctx, "SELECT COALESCE(current_streak, 0) FROM public.users WHERE id = $1", userID).Scan(&currentStreak)

	// Days since last message
	var daysSinceLastMessage int
	err = e.db.QueryRow(ctx, `
		SELECT COALESCE(
			(CURRENT_DATE - last_active_date)::int,
			-1
		) FROM public.users WHERE id = $1
	`, userID).Scan(&daysSinceLastMessage)
	if err != nil {
		daysSinceLastMessage = -1
	}

	appsBlocked := false
	morningBlockEnabled := false
	morningBlockStart := "06:00"
	morningBlockEnd := "09:00"
	if deviceCtx != nil {
		appsBlocked = deviceCtx.AppsBlocked
		morningBlockEnabled = deviceCtx.MorningBlockEnabled
		morningBlockStart = deviceCtx.MorningBlockStart
		morningBlockEnd = deviceCtx.MorningBlockEnd
	}

	// Fetch user's productivity challenges from onboarding diagnostic
	var productivityChallenges []string
	var onboardingResponses json.RawMessage
	err = e.db.QueryRow(ctx, `
		SELECT COALESCE(responses, '{}'::jsonb) FROM public.user_onboarding WHERE user_id = $1
	`, userID).Scan(&onboardingResponses)
	if err == nil && len(onboardingResponses) > 0 {
		var respMap map[string]interface{}
		if json.Unmarshal(onboardingResponses, &respMap) == nil {
			if challenges, ok := respMap["productivity_challenges"]; ok {
				if arr, ok := challenges.([]interface{}); ok {
					for _, c := range arr {
						if s, ok := c.(string); ok {
							productivityChallenges = append(productivityChallenges, s)
						}
					}
				}
			}
		}
	}

	result := map[string]interface{}{
		"user_name":             userName,
		"companion_name":        companionName,
		"today_date":            today,
		"tasks_today":           tasksTotal,
		"tasks_completed":       tasksCompleted,
		"rituals_today":         ritualsTotal,
		"rituals_completed":     ritualsCompleted,
		"focus_minutes_today":   focusMinutes,
		"time_of_day":           timeOfDay,
		"apps_blocked":          appsBlocked,
		"satisfaction_score":     satisfactionScore,
		"morning_checkin_done":  morningCheckinDone,
		"evening_review_done":  eveningReviewDone,
		"current_streak":        currentStreak,
		"all_tasks_completed":   tasksTotal > 0 && tasksCompleted == tasksTotal,
		"all_rituals_completed": ritualsTotal > 0 && ritualsCompleted == ritualsTotal,
		"user_language":         userLanguage,
		"morning_block_enabled":    morningBlockEnabled,
		"morning_block_start":      morningBlockStart,
		"morning_block_end":        morningBlockEnd,
		"days_since_last_message":  daysSinceLastMessage,
	}

	if len(productivityChallenges) > 0 {
		result["productivity_challenges"] = productivityChallenges
	}

	return toJSON(result), nil
}

// ==========================================
// Coaching Diagnostic
// ==========================================

func (e *Executor) saveProductivityChallenges(ctx context.Context, userID string, args map[string]interface{}) (string, error) {
	challenges := stringArrayArg(args, "challenges")
	if len(challenges) == 0 {
		return errorJSON(fmt.Errorf("no challenges provided")), nil
	}

	// Cap at 5
	if len(challenges) > 5 {
		challenges = challenges[:5]
	}

	challengesJSON, _ := json.Marshal(map[string]interface{}{
		"productivity_challenges": challenges,
	})

	// Merge into user_onboarding.responses JSONB
	query := `
		INSERT INTO public.user_onboarding (user_id, responses, current_step, created_at, updated_at)
		VALUES ($1, $2::jsonb, 0, NOW(), NOW())
		ON CONFLICT (user_id)
		DO UPDATE SET
			responses = COALESCE(user_onboarding.responses, '{}'::jsonb) || $2::jsonb,
			updated_at = NOW()
	`
	_, err := e.db.Exec(ctx, query, userID, string(challengesJSON))
	if err != nil {
		return "", fmt.Errorf("save challenges: %w", err)
	}

	log.Printf("Saved %d productivity challenges for user %s: %v", len(challenges), userID, challenges)

	return toJSON(map[string]interface{}{
		"saved":      true,
		"challenges": challenges,
		"count":      len(challenges),
	}), nil
}

func (e *Executor) getCurrentDatetime(ctx context.Context, userID string) string {
	tz := "Europe/Paris"
	e.db.QueryRow(ctx, "SELECT COALESCE(timezone, 'Europe/Paris') FROM public.users WHERE id = $1", userID).Scan(&tz)
	loc, _ := time.LoadLocation(tz)
	if loc == nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)

	return toJSON(map[string]interface{}{
		"date":     fmt.Sprintf("%s %d %s %d", frenchWeekday(now.Weekday()), now.Day(), frenchMonth(now.Month()), now.Year()),
		"time":     fmt.Sprintf("%02d:%02d", now.Hour(), now.Minute()),
		"iso_date": now.Format("2006-01-02"),
	})
}

// ==========================================
// Task Tools (direct SQL)
// ==========================================

func (e *Executor) getTasksForDate(ctx context.Context, userID, dateStr string) (string, error) {
	rows, err := e.db.Query(ctx, `
		SELECT id, title, COALESCE(status, 'pending'), COALESCE(time_block, ''), COALESCE(priority, 'medium'), date
		FROM tasks WHERE user_id = $1 AND date = $2
		ORDER BY position, created_at
	`, userID, dateStr)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var tasks []map[string]interface{}
	for rows.Next() {
		var id, title, status, timeBlock, priority, date string
		if err := rows.Scan(&id, &title, &status, &timeBlock, &priority, &date); err != nil {
			continue
		}
		tasks = append(tasks, map[string]interface{}{
			"id":         id,
			"title":      title,
			"status":     status,
			"time_block": timeBlock,
			"priority":   priority,
			"date":       date,
		})
	}
	if tasks == nil {
		tasks = []map[string]interface{}{}
	}

	return toJSON(map[string]interface{}{"date": dateStr, "tasks": tasks}), nil
}

func (e *Executor) createTask(ctx context.Context, userID string, args map[string]interface{}) (string, error) {
	title := stringArg(args, "title", "Nouvelle tâche")
	dateStr := stringArg(args, "date", todayStr(ctx, userID, e.db))
	priority := stringArg(args, "priority", "medium")
	timeBlock := stringArg(args, "time_block", "morning")

	var taskID string
	err := e.db.QueryRow(ctx, `
		INSERT INTO tasks (user_id, title, date, priority, time_block, is_ai_generated)
		VALUES ($1, $2, $3, $4, $5, true)
		RETURNING id
	`, userID, title, dateStr, priority, timeBlock).Scan(&taskID)
	if err != nil {
		return "", fmt.Errorf("create task: %w", err)
	}

	return toJSON(map[string]interface{}{"created": true, "task_id": taskID, "title": title}), nil
}

func (e *Executor) completeTask(ctx context.Context, userID, taskID string) (string, error) {
	if taskID == "" {
		return "", fmt.Errorf("task_id is required")
	}

	tag, err := e.db.Exec(ctx, `
		UPDATE tasks SET status = 'completed', completed_at = now(), updated_at = now()
		WHERE id = $1 AND user_id = $2
	`, taskID, userID)
	if err != nil {
		return "", fmt.Errorf("complete task: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return "", fmt.Errorf("task not found: %s", taskID)
	}

	return toJSON(map[string]interface{}{"completed": true, "task_id": taskID}), nil
}

func (e *Executor) uncompleteTask(ctx context.Context, userID, taskID string) (string, error) {
	if taskID == "" {
		return "", fmt.Errorf("task_id is required")
	}

	tag, err := e.db.Exec(ctx, `
		UPDATE tasks SET status = 'pending', completed_at = NULL, updated_at = now()
		WHERE id = $1 AND user_id = $2
	`, taskID, userID)
	if err != nil {
		return "", fmt.Errorf("uncomplete task: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return "", fmt.Errorf("task not found: %s", taskID)
	}

	return toJSON(map[string]interface{}{"uncompleted": true, "task_id": taskID}), nil
}

func (e *Executor) updateTask(ctx context.Context, userID string, args map[string]interface{}) (string, error) {
	taskID := stringArg(args, "task_id", "")
	if taskID == "" {
		return "", fmt.Errorf("task_id is required")
	}

	// Build dynamic SET clause
	sets := []string{}
	sqlArgs := []interface{}{}
	argIdx := 1

	if v, ok := args["title"].(string); ok && v != "" {
		sets = append(sets, fmt.Sprintf("title = $%d", argIdx))
		sqlArgs = append(sqlArgs, v)
		argIdx++
	}
	if v, ok := args["date"].(string); ok && v != "" {
		sets = append(sets, fmt.Sprintf("date = $%d", argIdx))
		sqlArgs = append(sqlArgs, v)
		argIdx++
	}
	if v, ok := args["priority"].(string); ok && v != "" {
		sets = append(sets, fmt.Sprintf("priority = $%d", argIdx))
		sqlArgs = append(sqlArgs, v)
		argIdx++
	}
	if v, ok := args["time_block"].(string); ok && v != "" {
		sets = append(sets, fmt.Sprintf("time_block = $%d", argIdx))
		sqlArgs = append(sqlArgs, v)
		argIdx++
	}

	if len(sets) == 0 {
		return toJSON(map[string]interface{}{"updated": false, "reason": "no fields to update"}), nil
	}

	sets = append(sets, "updated_at = now()")

	query := fmt.Sprintf("UPDATE tasks SET %s WHERE id = $%d AND user_id = $%d",
		joinStrings(sets, ", "), argIdx, argIdx+1)
	sqlArgs = append(sqlArgs, taskID, userID)

	tag, err := e.db.Exec(ctx, query, sqlArgs...)
	if err != nil {
		return "", fmt.Errorf("update task: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return "", fmt.Errorf("task not found: %s", taskID)
	}

	return toJSON(map[string]interface{}{"updated": true, "task_id": taskID}), nil
}

func (e *Executor) deleteTask(ctx context.Context, userID, taskID string) (string, error) {
	if taskID == "" {
		return "", fmt.Errorf("task_id is required")
	}

	tag, err := e.db.Exec(ctx, "DELETE FROM tasks WHERE id = $1 AND user_id = $2", taskID, userID)
	if err != nil {
		return "", fmt.Errorf("delete task: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return "", fmt.Errorf("task not found: %s", taskID)
	}

	return toJSON(map[string]interface{}{"deleted": true, "task_id": taskID}), nil
}

// ==========================================
// Routine Tools
// ==========================================

func (e *Executor) getRituals(ctx context.Context, userID string) (string, error) {
	today := todayStr(ctx, userID, e.db)
	rows, err := e.db.Query(ctx, `
		SELECT r.id, r.title, COALESCE(r.icon, '✨'),
		       EXISTS(SELECT 1 FROM routine_completions rc WHERE rc.routine_id = r.id AND rc.user_id = $1 AND rc.completion_date = $2) as is_completed
		FROM routines r
		WHERE r.user_id = $1
		ORDER BY r.created_at
	`, userID, today)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var rituals []map[string]interface{}
	for rows.Next() {
		var id, title, icon string
		var isCompleted bool
		if err := rows.Scan(&id, &title, &icon, &isCompleted); err != nil {
			continue
		}
		rituals = append(rituals, map[string]interface{}{
			"id":           id,
			"title":        title,
			"icon":         icon,
			"is_completed": isCompleted,
		})
	}
	if rituals == nil {
		rituals = []map[string]interface{}{}
	}

	return toJSON(map[string]interface{}{"rituals": rituals}), nil
}

func (e *Executor) createRoutine(ctx context.Context, userID string, args map[string]interface{}) (string, error) {
	title := stringArg(args, "title", "Nouveau rituel")
	icon := stringArg(args, "icon", "star")
	frequency := stringArg(args, "frequency", "daily")
	scheduledTime := stringArg(args, "scheduled_time", "")

	// Get first area as default
	var areaID string
	e.db.QueryRow(ctx, "SELECT id FROM areas WHERE user_id = $1 LIMIT 1", userID).Scan(&areaID)

	var routineID string
	var err error
	if scheduledTime != "" {
		err = e.db.QueryRow(ctx, `
			INSERT INTO routines (user_id, area_id, title, frequency, icon, scheduled_time)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING id
		`, userID, areaID, title, frequency, icon, scheduledTime).Scan(&routineID)
	} else {
		err = e.db.QueryRow(ctx, `
			INSERT INTO routines (user_id, area_id, title, frequency, icon)
			VALUES ($1, $2, $3, $4, $5) RETURNING id
		`, userID, areaID, title, frequency, icon).Scan(&routineID)
	}
	if err != nil {
		return "", fmt.Errorf("create routine: %w", err)
	}

	return toJSON(map[string]interface{}{"created": true, "routine_id": routineID}), nil
}

func (e *Executor) completeRoutine(ctx context.Context, userID, routineID string) (string, error) {
	today := todayStr(ctx, userID, e.db)
	_, err := e.db.Exec(ctx, `
		INSERT INTO routine_completions (user_id, routine_id, completion_date)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, routine_id, completion_date) DO NOTHING
	`, userID, routineID, today)
	if err != nil {
		return "", fmt.Errorf("complete routine: %w", err)
	}

	return toJSON(map[string]interface{}{"completed": true, "routine_id": routineID}), nil
}

func (e *Executor) deleteRoutine(ctx context.Context, userID, routineID string) (string, error) {
	_, err := e.db.Exec(ctx, "DELETE FROM routines WHERE id = $1 AND user_id = $2", routineID, userID)
	if err != nil {
		return "", fmt.Errorf("delete routine: %w", err)
	}
	return toJSON(map[string]interface{}{"deleted": true, "routine_id": routineID}), nil
}

// ==========================================
// Reflections & Goals
// ==========================================

func (e *Executor) saveMorningCheckin(ctx context.Context, userID string, args map[string]interface{}) (string, error) {
	today := todayStr(ctx, userID, e.db)
	mood := intArg(args, "mood", 3)
	sleepQuality := intArg(args, "sleep_quality", 3)
	intentions := stringArg(args, "intentions", "")

	_, err := e.db.Exec(ctx, `
		INSERT INTO daily_reflections (user_id, date, mood, sleep_quality, intentions)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id, date) DO UPDATE SET mood = $3, sleep_quality = $4, intentions = $5
	`, userID, today, mood, sleepQuality, intentions)
	if err != nil {
		return "", fmt.Errorf("save morning checkin: %w", err)
	}

	return toJSON(map[string]interface{}{"saved": true}), nil
}

func (e *Executor) saveEveningReview(ctx context.Context, userID string, args map[string]interface{}) (string, error) {
	today := todayStr(ctx, userID, e.db)
	biggestWin := stringArg(args, "biggest_win", "")
	blockers := stringArg(args, "blockers", "")
	tomorrowGoal := stringArg(args, "tomorrow_goal", "")

	_, err := e.db.Exec(ctx, `
		INSERT INTO daily_reflections (user_id, date, biggest_win, blockers, tomorrow_goal)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id, date) DO UPDATE SET biggest_win = $3, blockers = $4, tomorrow_goal = $5
	`, userID, today, biggestWin, blockers, tomorrowGoal)
	if err != nil {
		return "", fmt.Errorf("save evening review: %w", err)
	}

	return toJSON(map[string]interface{}{"saved": true}), nil
}

func (e *Executor) createWeeklyGoals(ctx context.Context, userID string, args map[string]interface{}) (string, error) {
	goalsRaw, ok := args["goals"].([]interface{})
	if !ok || len(goalsRaw) == 0 {
		return "", fmt.Errorf("goals array is required")
	}

	// Get current week start (Monday)
	now := time.Now()
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	monday := now.AddDate(0, 0, -(weekday - 1))
	weekStart := monday.Format("2006-01-02")

	// Upsert weekly goals
	var goalSetID string
	err := e.db.QueryRow(ctx, `
		INSERT INTO weekly_goals (user_id, week_start)
		VALUES ($1, $2)
		ON CONFLICT (user_id, week_start) DO UPDATE SET updated_at = now()
		RETURNING id
	`, userID, weekStart).Scan(&goalSetID)
	if err != nil {
		return "", fmt.Errorf("create weekly goals: %w", err)
	}

	// Delete old items and insert new
	e.db.Exec(ctx, "DELETE FROM weekly_goal_items WHERE weekly_goal_id = $1", goalSetID)
	for _, g := range goalsRaw {
		if content, ok := g.(string); ok {
			e.db.Exec(ctx, `
				INSERT INTO weekly_goal_items (weekly_goal_id, content) VALUES ($1, $2)
			`, goalSetID, content)
		}
	}

	return toJSON(map[string]interface{}{"created": true, "count": len(goalsRaw)}), nil
}

// ==========================================
// Morning Flow (composite)
// ==========================================

func (e *Executor) getMorningFlowContext(ctx context.Context, userID string, deviceCtx *DeviceContext) (string, error) {
	userCtx, err := e.getUserContext(ctx, userID, deviceCtx)
	if err != nil {
		return "", err
	}

	tasksResult, err := e.getTasksForDate(ctx, userID, todayStr(ctx, userID, e.db))
	if err != nil {
		return "", err
	}

	ritualsResult, err := e.getRituals(ctx, userID)
	if err != nil {
		return "", err
	}

	// Combine all into one response
	var userCtxMap, tasksMap, ritualsMap map[string]interface{}
	json.Unmarshal([]byte(userCtx), &userCtxMap)
	json.Unmarshal([]byte(tasksResult), &tasksMap)
	json.Unmarshal([]byte(ritualsResult), &ritualsMap)

	combined := userCtxMap
	combined["tasks"] = tasksMap["tasks"]
	combined["rituals"] = ritualsMap["rituals"]

	return toJSON(combined), nil
}

// ==========================================
// Calendar Events
// ==========================================

func (e *Executor) getCalendarEvents(ctx context.Context, userID, dateStr string) (string, error) {
	rows, err := e.db.Query(ctx, `
		SELECT id, title, COALESCE(start_time, ''), COALESCE(end_time, ''), COALESCE(block_apps, false)
		FROM calendar_events
		WHERE user_id = $1 AND date = $2
		ORDER BY start_time
	`, userID, dateStr)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var events []map[string]interface{}
	for rows.Next() {
		var id, title, startTime, endTime string
		var blockApps bool
		if err := rows.Scan(&id, &title, &startTime, &endTime, &blockApps); err != nil {
			continue
		}
		events = append(events, map[string]interface{}{
			"id":         id,
			"title":      title,
			"start_time": startTime,
			"end_time":   endTime,
			"block_apps": blockApps,
		})
	}
	if events == nil {
		events = []map[string]interface{}{}
	}

	return toJSON(map[string]interface{}{"date": dateStr, "events": events}), nil
}

func (e *Executor) scheduleCalendarBlocking(ctx context.Context, userID string, args map[string]interface{}) (string, error) {
	eventIDs, ok := args["event_ids"].([]interface{})
	if !ok {
		return "", fmt.Errorf("event_ids is required")
	}
	enabled := true
	if v, ok := args["enabled"].(bool); ok {
		enabled = v
	}

	for _, eid := range eventIDs {
		if id, ok := eid.(string); ok {
			e.db.Exec(ctx, `
				UPDATE calendar_events SET block_apps = $1 WHERE id = $2 AND user_id = $3
			`, enabled, id, userID)
		}
	}

	return toJSON(map[string]interface{}{"updated": true, "count": len(eventIDs)}), nil
}

// ==========================================
// Planning Tools
// ==========================================

func (e *Executor) getPlanningContext(ctx context.Context, userID, scope string, deviceCtx *DeviceContext) (string, error) {
	today := todayStr(ctx, userID, e.db)

	// Determine date range based on scope
	loc := time.UTC
	var tz string
	e.db.QueryRow(ctx, "SELECT COALESCE(timezone, 'Europe/Paris') FROM public.users WHERE id = $1", userID).Scan(&tz)
	if l, err := time.LoadLocation(tz); err == nil {
		loc = l
	}
	now := time.Now().In(loc)

	var dates []string
	switch scope {
	case "tomorrow":
		d := now.AddDate(0, 0, 1)
		dates = []string{d.Format("2006-01-02")}
	case "2days":
		dates = []string{today, now.AddDate(0, 0, 1).Format("2006-01-02")}
	case "week":
		for i := 0; i < 7; i++ {
			d := now.AddDate(0, 0, i)
			dates = append(dates, d.Format("2006-01-02"))
		}
	default: // "today"
		dates = []string{today}
	}

	// Fetch tasks + calendar events for each date
	var days []map[string]interface{}
	for _, dateStr := range dates {
		d, _ := time.Parse("2006-01-02", dateStr)
		dayName := frenchWeekday(d.Weekday())

		tasksJSON, _ := e.getTasksForDate(ctx, userID, dateStr)
		var tasksResult map[string]interface{}
		json.Unmarshal([]byte(tasksJSON), &tasksResult)
		tasks := tasksResult["tasks"]

		eventsJSON, _ := e.getCalendarEvents(ctx, userID, dateStr)
		var eventsResult map[string]interface{}
		json.Unmarshal([]byte(eventsJSON), &eventsResult)
		events := eventsResult["events"]

		days = append(days, map[string]interface{}{
			"date":            dateStr,
			"day_name":        dayName,
			"tasks":           tasks,
			"calendar_events": events,
		})
	}

	// Fetch rituals
	ritualsJSON, _ := e.getRituals(ctx, userID)
	var ritualsResult map[string]interface{}
	json.Unmarshal([]byte(ritualsJSON), &ritualsResult)

	// Basic user context
	userCtxJSON, _ := e.getUserContext(ctx, userID, deviceCtx)
	var userCtx map[string]interface{}
	json.Unmarshal([]byte(userCtxJSON), &userCtx)

	result := map[string]interface{}{
		"scope":        scope,
		"days":         days,
		"rituals":      ritualsResult,
		"user_context": userCtx,
	}

	return toJSON(result), nil
}

func (e *Executor) createTasksBatch(ctx context.Context, userID string, args map[string]interface{}) (string, error) {
	tasksRaw, ok := args["tasks"].([]interface{})
	if !ok || len(tasksRaw) == 0 {
		return "", fmt.Errorf("tasks array is required and must not be empty")
	}

	today := todayStr(ctx, userID, e.db)
	var created []map[string]interface{}

	for _, t := range tasksRaw {
		taskMap, ok := t.(map[string]interface{})
		if !ok {
			continue
		}

		title := ""
		if v, ok := taskMap["title"].(string); ok {
			title = v
		}
		if title == "" {
			continue
		}

		dateStr := today
		if v, ok := taskMap["date"].(string); ok && v != "" {
			dateStr = v
		}
		priority := "medium"
		if v, ok := taskMap["priority"].(string); ok && v != "" {
			priority = v
		}
		timeBlock := "morning"
		if v, ok := taskMap["time_block"].(string); ok && v != "" {
			timeBlock = v
		}
		estimatedMinutes := 0
		if v, ok := taskMap["estimated_minutes"].(float64); ok {
			estimatedMinutes = int(v)
		}

		var taskID string
		err := e.db.QueryRow(ctx, `
			INSERT INTO tasks (user_id, title, date, priority, time_block, estimated_minutes, is_ai_generated)
			VALUES ($1, $2, $3, $4, $5, NULLIF($6, 0), true)
			RETURNING id
		`, userID, title, dateStr, priority, timeBlock, estimatedMinutes).Scan(&taskID)
		if err != nil {
			log.Printf("create_tasks_batch: failed to create task '%s': %v", title, err)
			continue
		}

		created = append(created, map[string]interface{}{
			"id":         taskID,
			"title":      title,
			"date":       dateStr,
			"time_block": timeBlock,
			"priority":   priority,
		})
	}

	return toJSON(map[string]interface{}{
		"created": len(created),
		"tasks":   created,
	}), nil
}

// ==========================================
// String Helpers
// ==========================================

func joinStrings(strs []string, sep string) string {
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}
