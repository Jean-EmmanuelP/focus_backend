package chat

import (
	"encoding/json"
	"testing"
)

// ============================================
// Tests de simulation de conversations
// Vérifie que le parsing JSON des réponses IA
// fonctionne correctement pour tous les scénarios
// ============================================

// aiResponseForTest mirrors the anonymous struct in processAIResponse
// so we can test JSON parsing independently
type aiResponseForTest struct {
	Reply       string `json:"reply"`
	FocusIntent *struct {
		Detected  bool   `json:"detected"`
		Title     string `json:"title"`
		StartTime string `json:"start_time"`
		EndTime   string `json:"end_time"`
		BlockApps bool   `json:"block_apps"`
	} `json:"focus_intent"`
	BlockNow       bool `json:"block_now"`
	UnblockNow     bool `json:"unblock_now"`
	CreateQuests   []struct {
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
	DeleteQuest     *struct {
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
	CreateWeeklyGoals  []string `json:"create_weekly_goals"`
	CompleteWeeklyGoal *struct {
		Content string `json:"content"`
	} `json:"complete_weekly_goal"`
	CreateTask         *ChatTaskInput `json:"create_task"`
	CreateJournalEntry *struct {
		Mood       string `json:"mood"`
		Transcript string `json:"transcript"`
	} `json:"create_journal_entry"`
	ShowCard *string `json:"show_card"`
}

// ============================================
// Simulation 1: "Quelles sont mes tâches ?"
// L'IA doit renvoyer show_card: "tasks"
// ============================================
func TestSimulation_ShowTasks(t *testing.T) {
	aiJSON := `{
		"reply": "Voici tes tâches du jour ! Tu as 3 choses à faire.",
		"show_card": "tasks"
	}`

	var resp aiResponseForTest
	if err := json.Unmarshal([]byte(aiJSON), &resp); err != nil {
		t.Fatalf("Failed to parse AI response: %v", err)
	}

	if resp.Reply != "Voici tes tâches du jour ! Tu as 3 choses à faire." {
		t.Errorf("Reply mismatch: got %q", resp.Reply)
	}
	if resp.ShowCard == nil || *resp.ShowCard != "tasks" {
		t.Errorf("ShowCard should be 'tasks', got %v", resp.ShowCard)
	}
	if resp.BlockNow {
		t.Error("BlockNow should be false")
	}

	// Verify response serialization
	response := &SendMessageResponse{Reply: resp.Reply, ShowCard: resp.ShowCard}
	out, _ := json.Marshal(response)
	var check map[string]interface{}
	json.Unmarshal(out, &check)

	if check["show_card"] != "tasks" {
		t.Errorf("Serialized show_card should be 'tasks', got %v", check["show_card"])
	}
}

// ============================================
// Simulation 2: "Montre-moi mes rituels"
// L'IA doit renvoyer show_card: "routines"
// ============================================
func TestSimulation_ShowRoutines(t *testing.T) {
	aiJSON := `{
		"reply": "Voici tes rituels du jour.",
		"show_card": "routines"
	}`

	var resp aiResponseForTest
	if err := json.Unmarshal([]byte(aiJSON), &resp); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if resp.ShowCard == nil || *resp.ShowCard != "routines" {
		t.Errorf("ShowCard should be 'routines', got %v", resp.ShowCard)
	}
}

// ============================================
// Simulation 3: "Bloque mes apps"
// L'IA doit renvoyer block_now: true
// ============================================
func TestSimulation_BlockApps(t *testing.T) {
	aiJSON := `{
		"reply": "C'est parti, je bloque tes apps. Reste concentré !",
		"block_now": true
	}`

	var resp aiResponseForTest
	if err := json.Unmarshal([]byte(aiJSON), &resp); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if !resp.BlockNow {
		t.Error("BlockNow should be true")
	}
	if resp.ShowCard != nil {
		t.Error("ShowCard should be nil for blocking action")
	}

	// Simulate building the response
	response := &SendMessageResponse{Reply: resp.Reply}
	if resp.BlockNow {
		response.Action = &ActionData{Type: "block_apps"}
	}

	if response.Action == nil || response.Action.Type != "block_apps" {
		t.Errorf("Action should be block_apps, got %v", response.Action)
	}
}

// ============================================
// Simulation 4: "Débloque mes apps"
// ============================================
func TestSimulation_UnblockApps(t *testing.T) {
	aiJSON := `{
		"reply": "OK je débloque. Fais une pause mais reviens vite.",
		"unblock_now": true
	}`

	var resp aiResponseForTest
	if err := json.Unmarshal([]byte(aiJSON), &resp); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if !resp.UnblockNow {
		t.Error("UnblockNow should be true")
	}

	response := &SendMessageResponse{Reply: resp.Reply}
	if resp.UnblockNow {
		response.Action = &ActionData{Type: "unblock_apps"}
	}

	if response.Action.Type != "unblock_apps" {
		t.Errorf("Action should be unblock_apps")
	}
}

// ============================================
// Simulation 5: "Crée une tâche : Aller à la salle"
// ============================================
func TestSimulation_CreateTask(t *testing.T) {
	aiJSON := `{
		"reply": "Tâche ajoutée : Aller à la salle à 18h.",
		"create_task": {
			"title": "Aller à la salle",
			"date": "2026-02-16",
			"time_block": "evening",
			"scheduled_start": "18:00",
			"scheduled_end": "19:30"
		}
	}`

	var resp aiResponseForTest
	if err := json.Unmarshal([]byte(aiJSON), &resp); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if resp.CreateTask == nil {
		t.Fatal("CreateTask should not be nil")
	}
	if resp.CreateTask.Title != "Aller à la salle" {
		t.Errorf("Task title mismatch: %q", resp.CreateTask.Title)
	}
	if resp.CreateTask.Date != "2026-02-16" {
		t.Errorf("Task date mismatch: %q", resp.CreateTask.Date)
	}
	if resp.CreateTask.TimeBlock != "evening" {
		t.Errorf("Task time_block mismatch: %q", resp.CreateTask.TimeBlock)
	}
	if resp.CreateTask.ScheduledStart != "18:00" {
		t.Errorf("ScheduledStart mismatch: %q", resp.CreateTask.ScheduledStart)
	}
	if resp.CreateTask.ScheduledEnd != "19:30" {
		t.Errorf("ScheduledEnd mismatch: %q", resp.CreateTask.ScheduledEnd)
	}
}

// ============================================
// Simulation 6: "J'ai fini ma tâche lecture"
// ============================================
func TestSimulation_CompleteTask(t *testing.T) {
	aiJSON := `{
		"reply": "Bien joué ! Tâche lecture complétée.",
		"complete_task": {"title": "Lecture"}
	}`

	var resp aiResponseForTest
	if err := json.Unmarshal([]byte(aiJSON), &resp); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if resp.CompleteTask == nil || resp.CompleteTask.Title != "Lecture" {
		t.Errorf("CompleteTask title should be 'Lecture'")
	}
}

// ============================================
// Simulation 7: "J'ai fait ma méditation et mon sport"
// ============================================
func TestSimulation_CompleteMultipleRoutines(t *testing.T) {
	aiJSON := `{
		"reply": "Super ! Méditation et sport validés pour aujourd'hui.",
		"complete_routines": ["Méditation", "Sport"]
	}`

	var resp aiResponseForTest
	if err := json.Unmarshal([]byte(aiJSON), &resp); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if len(resp.CompleteRoutines) != 2 {
		t.Fatalf("Expected 2 routines, got %d", len(resp.CompleteRoutines))
	}
	if resp.CompleteRoutines[0] != "Méditation" {
		t.Errorf("First routine should be 'Méditation', got %q", resp.CompleteRoutines[0])
	}
	if resp.CompleteRoutines[1] != "Sport" {
		t.Errorf("Second routine should be 'Sport', got %q", resp.CompleteRoutines[1])
	}
}

// ============================================
// Simulation 8: Morning check-in complet
// "J'ai bien dormi, mood 4, priorité: finir le projet"
// ============================================
func TestSimulation_MorningCheckin(t *testing.T) {
	aiJSON := `{
		"reply": "Excellente nuit ! Tu es prêt pour une grosse journée.",
		"morning_checkin": {
			"mood": 4,
			"sleep_quality": 5,
			"top_priority": "Finir le projet Focus",
			"intentions": ["Coder 2h", "Aller à la salle", "Lire 30 min"],
			"energy_level": 4
		}
	}`

	var resp aiResponseForTest
	if err := json.Unmarshal([]byte(aiJSON), &resp); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	mc := resp.MorningCheckin
	if mc == nil {
		t.Fatal("MorningCheckin should not be nil")
	}
	if mc.Mood != 4 {
		t.Errorf("Mood should be 4, got %d", mc.Mood)
	}
	if mc.SleepQuality != 5 {
		t.Errorf("SleepQuality should be 5, got %d", mc.SleepQuality)
	}
	if mc.TopPriority != "Finir le projet Focus" {
		t.Errorf("TopPriority mismatch: %q", mc.TopPriority)
	}
	if len(mc.Intentions) != 3 {
		t.Fatalf("Expected 3 intentions, got %d", len(mc.Intentions))
	}
	if mc.EnergyLevel != 4 {
		t.Errorf("EnergyLevel should be 4, got %d", mc.EnergyLevel)
	}
}

// ============================================
// Simulation 9: Evening check-in
// "Ma journée s'est bien passée, j'ai fini mon projet"
// ============================================
func TestSimulation_EveningCheckin(t *testing.T) {
	aiJSON := `{
		"reply": "Belle journée ! Repose-toi bien ce soir.",
		"evening_checkin": {
			"mood": 5,
			"biggest_win": "Fini le système de cards interactives",
			"blockers": "Aucun",
			"goal_for_tomorrow": "Tester avec de vrais utilisateurs",
			"grateful_for": "Mon équipe"
		}
	}`

	var resp aiResponseForTest
	if err := json.Unmarshal([]byte(aiJSON), &resp); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	ec := resp.EveningCheckin
	if ec == nil {
		t.Fatal("EveningCheckin should not be nil")
	}
	if ec.Mood != 5 {
		t.Errorf("Mood should be 5, got %d", ec.Mood)
	}
	if ec.BiggestWin != "Fini le système de cards interactives" {
		t.Errorf("BiggestWin mismatch: %q", ec.BiggestWin)
	}
	if ec.GratefulFor != "Mon équipe" {
		t.Errorf("GratefulFor mismatch: %q", ec.GratefulFor)
	}
}

// ============================================
// Simulation 10: Création de quests
// "Je veux lire 12 livres et courir 100km"
// ============================================
func TestSimulation_CreateQuests(t *testing.T) {
	aiJSON := `{
		"reply": "Deux nouvelles quêtes créées ! C'est ambitieux.",
		"create_quests": [
			{"title": "Lire 12 livres", "target_value": 12, "area": "Learning"},
			{"title": "Courir 100km", "target_value": 100, "area": "Health"}
		]
	}`

	var resp aiResponseForTest
	if err := json.Unmarshal([]byte(aiJSON), &resp); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if len(resp.CreateQuests) != 2 {
		t.Fatalf("Expected 2 quests, got %d", len(resp.CreateQuests))
	}
	if resp.CreateQuests[0].Title != "Lire 12 livres" {
		t.Errorf("Quest 1 title mismatch: %q", resp.CreateQuests[0].Title)
	}
	if resp.CreateQuests[0].TargetValue != 12 {
		t.Errorf("Quest 1 target should be 12, got %d", resp.CreateQuests[0].TargetValue)
	}
	if resp.CreateQuests[1].Area != "Health" {
		t.Errorf("Quest 2 area should be 'Health', got %q", resp.CreateQuests[1].Area)
	}
}

// ============================================
// Simulation 11: Création de routines
// "Ajoute méditation à 7h et lecture à 22h"
// ============================================
func TestSimulation_CreateRoutines(t *testing.T) {
	time1 := "07:00"
	time2 := "22:00"
	aiJSON := `{
		"reply": "Deux nouveaux rituels ajoutés !",
		"create_routines": [
			{"title": "Méditation", "frequency": "daily", "scheduled_time": "07:00"},
			{"title": "Lecture", "frequency": "daily", "scheduled_time": "22:00"}
		]
	}`

	var resp aiResponseForTest
	if err := json.Unmarshal([]byte(aiJSON), &resp); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if len(resp.CreateRoutines) != 2 {
		t.Fatalf("Expected 2 routines, got %d", len(resp.CreateRoutines))
	}
	if resp.CreateRoutines[0].Title != "Méditation" {
		t.Errorf("Routine 1 title mismatch: %q", resp.CreateRoutines[0].Title)
	}
	if resp.CreateRoutines[0].ScheduledTime == nil || *resp.CreateRoutines[0].ScheduledTime != time1 {
		t.Errorf("Routine 1 time should be %q", time1)
	}
	if resp.CreateRoutines[1].ScheduledTime == nil || *resp.CreateRoutines[1].ScheduledTime != time2 {
		t.Errorf("Routine 2 time should be %q", time2)
	}
}

// ============================================
// Simulation 12: Suppression de quest et routine
// ============================================
func TestSimulation_DeleteQuestAndRoutine(t *testing.T) {
	aiJSON := `{
		"reply": "Quest supprimée.",
		"delete_quest": {"title": "Courir 100km"}
	}`

	var resp aiResponseForTest
	if err := json.Unmarshal([]byte(aiJSON), &resp); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if resp.DeleteQuest == nil || resp.DeleteQuest.Title != "Courir 100km" {
		t.Error("DeleteQuest should have title 'Courir 100km'")
	}

	aiJSON2 := `{
		"reply": "Rituel supprimé.",
		"delete_routine": {"title": "Méditation"}
	}`

	var resp2 aiResponseForTest
	if err := json.Unmarshal([]byte(aiJSON2), &resp2); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if resp2.DeleteRoutine == nil || resp2.DeleteRoutine.Title != "Méditation" {
		t.Error("DeleteRoutine should have title 'Méditation'")
	}
}

// ============================================
// Simulation 13: Weekly goals
// "Mes objectifs de la semaine : coder, lire, sport"
// ============================================
func TestSimulation_WeeklyGoals(t *testing.T) {
	aiJSON := `{
		"reply": "Objectifs de la semaine définis !",
		"create_weekly_goals": ["Coder 20h", "Lire 2 livres", "Sport 4x"]
	}`

	var resp aiResponseForTest
	if err := json.Unmarshal([]byte(aiJSON), &resp); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if len(resp.CreateWeeklyGoals) != 3 {
		t.Fatalf("Expected 3 weekly goals, got %d", len(resp.CreateWeeklyGoals))
	}
	if resp.CreateWeeklyGoals[0] != "Coder 20h" {
		t.Errorf("Goal 1 mismatch: %q", resp.CreateWeeklyGoals[0])
	}
}

// ============================================
// Simulation 14: Complete weekly goal
// ============================================
func TestSimulation_CompleteWeeklyGoal(t *testing.T) {
	aiJSON := `{
		"reply": "Objectif complété, bien joué !",
		"complete_weekly_goal": {"content": "Coder 20h"}
	}`

	var resp aiResponseForTest
	if err := json.Unmarshal([]byte(aiJSON), &resp); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if resp.CompleteWeeklyGoal == nil || resp.CompleteWeeklyGoal.Content != "Coder 20h" {
		t.Error("CompleteWeeklyGoal content should be 'Coder 20h'")
	}
}

// ============================================
// Simulation 15: Journal entry
// "J'ai passé une bonne journée"
// ============================================
func TestSimulation_CreateJournalEntry(t *testing.T) {
	aiJSON := `{
		"reply": "Ton entrée journal est enregistrée.",
		"create_journal_entry": {
			"mood": "happy",
			"transcript": "J'ai passé une bonne journée, j'ai fini mon projet et je me sens motivé."
		}
	}`

	var resp aiResponseForTest
	if err := json.Unmarshal([]byte(aiJSON), &resp); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if resp.CreateJournalEntry == nil {
		t.Fatal("CreateJournalEntry should not be nil")
	}
	if resp.CreateJournalEntry.Mood != "happy" {
		t.Errorf("Mood should be 'happy', got %q", resp.CreateJournalEntry.Mood)
	}
	if resp.CreateJournalEntry.Transcript == "" {
		t.Error("Transcript should not be empty")
	}
}

// ============================================
// Simulation 16: Focus intent (session planifiée)
// "Je veux focus de 14h à 16h sur mon projet"
// ============================================
func TestSimulation_FocusIntent(t *testing.T) {
	aiJSON := `{
		"reply": "Session de focus prévue de 14h à 16h sur ton projet. Je bloque tes apps.",
		"focus_intent": {
			"detected": true,
			"title": "Projet Focus",
			"start_time": "14:00",
			"end_time": "16:00",
			"block_apps": true
		}
	}`

	var resp aiResponseForTest
	if err := json.Unmarshal([]byte(aiJSON), &resp); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	fi := resp.FocusIntent
	if fi == nil {
		t.Fatal("FocusIntent should not be nil")
	}
	if !fi.Detected {
		t.Error("FocusIntent.Detected should be true")
	}
	if fi.Title != "Projet Focus" {
		t.Errorf("Title mismatch: %q", fi.Title)
	}
	if fi.StartTime != "14:00" || fi.EndTime != "16:00" {
		t.Errorf("Time mismatch: %q - %q", fi.StartTime, fi.EndTime)
	}
	if !fi.BlockApps {
		t.Error("BlockApps should be true")
	}

	// Build response like handler does
	response := &SendMessageResponse{Reply: resp.Reply}
	if fi.Detected {
		response.Action = &ActionData{
			Type: "focus_scheduled",
			TaskData: &TaskData{
				Title:          fi.Title,
				ScheduledStart: fi.StartTime,
				ScheduledEnd:   fi.EndTime,
				BlockApps:      fi.BlockApps,
			},
		}
	}

	if response.Action.Type != "focus_scheduled" {
		t.Error("Action type should be 'focus_scheduled'")
	}
	if response.Action.TaskData.Title != "Projet Focus" {
		t.Error("TaskData title mismatch")
	}
}

// ============================================
// Simulation 17: Conversation simple sans action
// "Salut, comment tu vas ?"
// ============================================
func TestSimulation_SimpleReply(t *testing.T) {
	aiJSON := `{
		"reply": "Salut ! Je suis en forme. Prêt à t'aider. Qu'est-ce qu'on fait aujourd'hui ?"
	}`

	var resp aiResponseForTest
	if err := json.Unmarshal([]byte(aiJSON), &resp); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if resp.Reply == "" {
		t.Error("Reply should not be empty")
	}
	if resp.ShowCard != nil {
		t.Error("No card for simple reply")
	}
	if resp.BlockNow {
		t.Error("No blocking for simple reply")
	}
	if resp.CreateTask != nil {
		t.Error("No task for simple reply")
	}
	if resp.MorningCheckin != nil {
		t.Error("No checkin for simple reply")
	}
}

// ============================================
// Simulation 18: Actions multiples
// show_card + complete_routines dans la même réponse
// ============================================
func TestSimulation_ShowCardWithAction(t *testing.T) {
	aiJSON := `{
		"reply": "J'ai validé ta méditation. Voici tes rituels mis à jour.",
		"complete_routines": ["Méditation"],
		"show_card": "routines"
	}`

	var resp aiResponseForTest
	if err := json.Unmarshal([]byte(aiJSON), &resp); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if len(resp.CompleteRoutines) != 1 || resp.CompleteRoutines[0] != "Méditation" {
		t.Error("CompleteRoutines should have 'Méditation'")
	}
	if resp.ShowCard == nil || *resp.ShowCard != "routines" {
		t.Error("ShowCard should be 'routines'")
	}

	// Build response
	response := &SendMessageResponse{Reply: resp.Reply, ShowCard: resp.ShowCard}
	if len(resp.CompleteRoutines) > 0 {
		response.Action = &ActionData{Type: "routines_completed"}
	}

	if response.Action.Type != "routines_completed" {
		t.Error("Action should be routines_completed")
	}
	if *response.ShowCard != "routines" {
		t.Error("ShowCard should survive alongside action")
	}
}

// ============================================
// Simulation 19: Update quest progress
// "J'ai lu un livre"
// ============================================
func TestSimulation_UpdateQuestProgress(t *testing.T) {
	aiJSON := `{
		"reply": "Un de plus ! Tu en es à 5 sur 12.",
		"update_quest": {"title": "Lire 12 livres", "increment": 1}
	}`

	var resp aiResponseForTest
	if err := json.Unmarshal([]byte(aiJSON), &resp); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if resp.UpdateQuest == nil {
		t.Fatal("UpdateQuest should not be nil")
	}
	if resp.UpdateQuest.Title != "Lire 12 livres" {
		t.Errorf("Title mismatch: %q", resp.UpdateQuest.Title)
	}
	if resp.UpdateQuest.Increment != 1 {
		t.Errorf("Increment should be 1, got %d", resp.UpdateQuest.Increment)
	}
}

// ============================================
// Simulation 20: Réponse mal formée (fallback)
// L'IA renvoie du texte brut au lieu de JSON
// ============================================
func TestSimulation_MalformedResponse(t *testing.T) {
	rawText := "Désolé, j'ai eu un problème technique."

	var resp aiResponseForTest
	err := json.Unmarshal([]byte(rawText), &resp)

	// Should fail to parse — handler falls back to raw text
	if err == nil {
		t.Error("Should fail to parse non-JSON text")
	}

	// Simulate handler fallback
	response := &SendMessageResponse{Reply: rawText}
	if response.Reply != rawText {
		t.Error("Fallback should use raw text")
	}
	if response.Action != nil {
		t.Error("No action on fallback")
	}
	if response.ShowCard != nil {
		t.Error("No show_card on fallback")
	}
}

// ============================================
// Test response serialization
// Verify JSON output matches what iOS expects
// ============================================
func TestResponseSerialization(t *testing.T) {
	taskID := "abc-123"
	showCard := "tasks"

	response := SendMessageResponse{
		Reply:     "Tâche créée.",
		MessageID: "msg-456",
		Action: &ActionData{
			Type:   "task_created",
			TaskID: &taskID,
		},
		ShowCard: &showCard,
	}

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var check map[string]interface{}
	json.Unmarshal(data, &check)

	// Verify all fields iOS expects
	if check["reply"] != "Tâche créée." {
		t.Errorf("reply mismatch: %v", check["reply"])
	}
	if check["message_id"] != "msg-456" {
		t.Errorf("message_id mismatch: %v", check["message_id"])
	}
	if check["show_card"] != "tasks" {
		t.Errorf("show_card mismatch: %v", check["show_card"])
	}

	action := check["action"].(map[string]interface{})
	if action["type"] != "task_created" {
		t.Errorf("action.type mismatch: %v", action["type"])
	}
	if action["task_id"] != "abc-123" {
		t.Errorf("action.task_id mismatch: %v", action["task_id"])
	}
}

// ============================================
// Test response without optional fields
// Verify omitempty works correctly
// ============================================
func TestResponseOmitEmpty(t *testing.T) {
	response := SendMessageResponse{
		Reply:     "Salut !",
		MessageID: "msg-789",
	}

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var check map[string]interface{}
	json.Unmarshal(data, &check)

	if _, exists := check["show_card"]; exists {
		t.Error("show_card should be omitted when nil")
	}
	if _, exists := check["action"]; exists {
		t.Error("action should be omitted when nil")
	}
	if _, exists := check["tool"]; exists {
		t.Error("tool should be omitted when nil")
	}
}

// ============================================
// Test VoiceMessageResponse with show_card
// ============================================
func TestVoiceResponseWithShowCard(t *testing.T) {
	card := "tasks"
	response := VoiceMessageResponse{
		Reply:      "Voici tes tâches.",
		Transcript: "montre moi mes tâches",
		MessageID:  "msg-voice-1",
		Action:     nil,
		ShowCard:   &card,
	}

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var check map[string]interface{}
	json.Unmarshal(data, &check)

	if check["show_card"] != "tasks" {
		t.Errorf("show_card mismatch: %v", check["show_card"])
	}
	if check["transcript"] != "montre moi mes tâches" {
		t.Errorf("transcript mismatch: %v", check["transcript"])
	}
}

// ============================================
// Test ChatTaskInput serialization
// ============================================
func TestChatTaskInputParsing(t *testing.T) {
	aiJSON := `{
		"reply": "Tâche créée.",
		"create_task": {
			"title": "Appel avec le client",
			"date": "2026-02-17",
			"time_block": "morning",
			"scheduled_start": "10:00",
			"scheduled_end": "10:30"
		}
	}`

	var resp aiResponseForTest
	if err := json.Unmarshal([]byte(aiJSON), &resp); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	task := resp.CreateTask
	if task == nil {
		t.Fatal("CreateTask should not be nil")
	}
	if task.Title != "Appel avec le client" {
		t.Errorf("Title: %q", task.Title)
	}
	if task.Date != "2026-02-17" {
		t.Errorf("Date: %q", task.Date)
	}
	if task.TimeBlock != "morning" {
		t.Errorf("TimeBlock: %q", task.TimeBlock)
	}
}

// ============================================
// Test task without scheduled times (just date + block)
// ============================================
func TestChatTaskInputMinimal(t *testing.T) {
	aiJSON := `{
		"reply": "Tâche ajoutée pour demain.",
		"create_task": {
			"title": "Acheter du café",
			"date": "2026-02-17",
			"time_block": "morning"
		}
	}`

	var resp aiResponseForTest
	if err := json.Unmarshal([]byte(aiJSON), &resp); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if resp.CreateTask.ScheduledStart != "" {
		t.Error("ScheduledStart should be empty for minimal task")
	}
	if resp.CreateTask.ScheduledEnd != "" {
		t.Error("ScheduledEnd should be empty for minimal task")
	}
}
