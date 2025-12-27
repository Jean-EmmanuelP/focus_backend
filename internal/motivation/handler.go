package motivation

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"firelevel-backend/internal/auth"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Motivational phrases organized by language and context
var morningPhrases = map[string][]string{
	"en": {
		"Rise and shine! Today is full of possibilities. Plan your day and make it count.",
		"Good morning, champion! Your goals are waiting. Let's map out an incredible day.",
		"A new day, a fresh start. Take a moment to design your ideal day.",
		"Success starts with intention. What will you accomplish today?",
		"Every morning is a chance to be better. Plan wisely, execute boldly.",
		"The early bird catches the worm! Let's organize your day for maximum impact.",
		"Today is a gift. Unwrap it with purpose and a solid plan.",
		"Champions don't hit snooze on their dreams. Time to plan your victory!",
		"Your future self will thank you for planning today. Let's do this!",
		"Good morning! The world is waiting for what only you can offer.",
	},
	"fr": {
		"Debout champion ! Aujourd'hui regorge de possibilites. Planifie ta journee et fais-la compter.",
		"Bonjour ! Tes objectifs t'attendent. Concois une journee incroyable.",
		"Un nouveau jour, un nouveau depart. Prends un moment pour designer ta journee ideale.",
		"Le succes commence par l'intention. Qu'accompliras-tu aujourd'hui ?",
		"Chaque matin est une chance de progresser. Planifie avec sagesse, execute avec audace.",
		"L'avenir appartient a ceux qui se levent tot ! Organisons ta journee pour un impact maximal.",
		"Aujourd'hui est un cadeau. Ouvre-le avec intention et un plan solide.",
		"Les champions ne repoussent pas leurs reves. C'est l'heure de planifier ta victoire !",
		"Ton futur toi te remerciera d'avoir planifie aujourd'hui. On y va !",
		"Bonjour ! Le monde attend ce que toi seul peux offrir.",
	},
}

var taskReminderPhrases = map[string][]string{
	"en": {
		"Time to focus! '%s' is waiting for you.",
		"Reminder: '%s' is on your schedule. You've got this!",
		"Hey champion! Don't forget '%s' - it's time to shine.",
		"Your task '%s' is coming up. Stay on track!",
		"Focus time! '%s' deserves your attention now.",
		"Quick reminder: '%s' is scheduled. Make it happen!",
		"It's go time for '%s'! Show it what you're made of.",
		"'%s' is calling. Time to crush it!",
		"Heads up! '%s' starts soon. You're ready for this.",
		"Your commitment to '%s' matters. Time to deliver!",
	},
	"fr": {
		"C'est l'heure de te concentrer ! '%s' t'attend.",
		"Rappel : '%s' est au programme. Tu vas gerer !",
		"Hey champion ! N'oublie pas '%s' - c'est ton moment.",
		"Ta tache '%s' arrive bientot. Reste focus !",
		"C'est l'heure du focus ! '%s' merite ton attention.",
		"Petit rappel : '%s' est prevu. Fais-le !",
		"C'est parti pour '%s' ! Montre de quoi tu es capable.",
		"'%s' t'appelle. C'est l'heure de tout defoncer !",
		"Attention ! '%s' commence bientot. Tu es pret pour ca.",
		"Ton engagement envers '%s' compte. C'est l'heure de delivrer !",
	},
}

// Response types
type MorningPhraseResponse struct {
	Phrase   string `json:"phrase"`
	Language string `json:"language"`
	Type     string `json:"type"` // "morning" or "task_reminder"
}

type TaskReminderResponse struct {
	Phrase   string `json:"phrase"`
	TaskName string `json:"task_name"`
	Language string `json:"language"`
	Type     string `json:"type"`
}

// Handler holds dependencies
type Handler struct {
	db *pgxpool.Pool
}

// NewHandler creates a new Handler
func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
}

// GetMorningPhrase returns a random motivational phrase for the morning
// GET /motivation/morning?lang=fr
func (h *Handler) GetMorningPhrase(w http.ResponseWriter, r *http.Request) {
	// Get user ID from context (authenticated)
	_ = r.Context().Value(auth.UserContextKey).(string)

	// Get language from query param, default to "en"
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = "en"
	}

	// Fallback to English if language not supported
	phrases, ok := morningPhrases[lang]
	if !ok {
		phrases = morningPhrases["en"]
		lang = "en"
	}

	// Pick a random phrase
	rand.Seed(time.Now().UnixNano())
	phrase := phrases[rand.Intn(len(phrases))]

	response := MorningPhraseResponse{
		Phrase:   phrase,
		Language: lang,
		Type:     "morning",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetTaskReminderPhrase returns a random motivational phrase for a task reminder
// GET /motivation/task?lang=fr&task_name=Work%20on%20project
func (h *Handler) GetTaskReminderPhrase(w http.ResponseWriter, r *http.Request) {
	// Get user ID from context (authenticated)
	_ = r.Context().Value(auth.UserContextKey).(string)

	// Get language from query param, default to "en"
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = "en"
	}

	// Get task name
	taskName := r.URL.Query().Get("task_name")
	if taskName == "" {
		taskName = "your task"
	}

	// Fallback to English if language not supported
	phrases, ok := taskReminderPhrases[lang]
	if !ok {
		phrases = taskReminderPhrases["en"]
		lang = "en"
	}

	// Pick a random phrase and format with task name
	rand.Seed(time.Now().UnixNano())
	phraseTemplate := phrases[rand.Intn(len(phrases))]
	phrase := fmt.Sprintf(phraseTemplate, taskName)

	response := TaskReminderResponse{
		Phrase:   phrase,
		TaskName: taskName,
		Language: lang,
		Type:     "task_reminder",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetAllPhrases returns all available phrases (for admin/debug purposes)
// GET /motivation/all?lang=fr
func (h *Handler) GetAllPhrases(w http.ResponseWriter, r *http.Request) {
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = "en"
	}

	morning, ok := morningPhrases[lang]
	if !ok {
		morning = morningPhrases["en"]
	}

	taskReminder, ok := taskReminderPhrases[lang]
	if !ok {
		taskReminder = taskReminderPhrases["en"]
	}

	response := map[string]interface{}{
		"language":       lang,
		"morning":        morning,
		"task_reminders": taskReminder,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
