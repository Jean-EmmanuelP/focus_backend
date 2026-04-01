package quests

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"firelevel-backend/internal/auth"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	db *pgxpool.Pool
}

func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
}

type Quest struct {
	ID           string `json:"id"`
	AreaID       string `json:"area_id"`
	Title        string `json:"title"`
	Status       string `json:"status"`
	CurrentValue int    `json:"current_value"`
	TargetValue  int    `json:"target_value"`
	Term         string `json:"term"` // short, medium, long
}

type createQuestRequest struct {
	Title       string `json:"title"`
	Area        string `json:"area,omitempty"`
	TargetValue int    `json:"target_value,omitempty"`
	TargetDate  string `json:"target_date,omitempty"`
	Term        string `json:"term,omitempty"` // short, medium, long
}

// areaDefaults maps area slugs to display names and icons.
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

// List returns the user's quests (active by default).
// GET /quests
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	rows, err := h.db.Query(r.Context(), `
		SELECT id, area_id, title, status, current_value, target_value, COALESCE(term, 'short')
		FROM quests
		WHERE user_id = $1 AND status = 'active'
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		log.Printf("quests.List error: %v", err)
		http.Error(w, "Failed to list quests", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	quests := []Quest{}
	for rows.Next() {
		var q Quest
		if err := rows.Scan(&q.ID, &q.AreaID, &q.Title, &q.Status, &q.CurrentValue, &q.TargetValue, &q.Term); err != nil {
			log.Printf("quests.List scan error: %v", err)
			continue
		}
		quests = append(quests, q)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(quests)
}

// Create creates a new quest for the user.
// POST /quests
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	var req createQuestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.Title) == "" {
		http.Error(w, "Title is required", http.StatusBadRequest)
		return
	}

	if req.TargetValue <= 0 {
		req.TargetValue = 1
	}

	areaSlug := req.Area
	if areaSlug == "" {
		areaSlug = "other"
	}

	// Find or create the area
	areaID, err := h.findOrCreateArea(r, userID, areaSlug)
	if err != nil {
		log.Printf("quests.Create area error: %v", err)
		http.Error(w, "Failed to resolve area", http.StatusInternalServerError)
		return
	}

	term := req.Term
	if term == "" {
		term = "short"
	}
	if term != "short" && term != "medium" && term != "long" {
		term = "short"
	}

	var q Quest
	err = h.db.QueryRow(r.Context(), `
		INSERT INTO quests (user_id, area_id, title, target_value, current_value, status, term)
		VALUES ($1, $2, $3, $4, 0, 'active', $5)
		RETURNING id, area_id, title, status, current_value, target_value, term
	`, userID, areaID, req.Title, req.TargetValue, term).Scan(
		&q.ID, &q.AreaID, &q.Title, &q.Status, &q.CurrentValue, &q.TargetValue, &q.Term,
	)
	if err != nil {
		log.Printf("quests.Create insert error: %v", err)
		http.Error(w, "Failed to create quest", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(q)
}

func (h *Handler) findOrCreateArea(r *http.Request, userID, slug string) (string, error) {
	var areaID string
	err := h.db.QueryRow(r.Context(), `
		SELECT id FROM areas WHERE user_id = $1 AND slug = $2
	`, userID, slug).Scan(&areaID)
	if err == nil {
		return areaID, nil
	}

	defaults, ok := areaDefaults[slug]
	if !ok {
		defaults = areaDefaults["other"]
		slug = "other"
	}

	err = h.db.QueryRow(r.Context(), `
		INSERT INTO areas (user_id, name, slug, icon)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, slug) DO UPDATE SET name = EXCLUDED.name
		RETURNING id
	`, userID, defaults.Name, slug, defaults.Icon).Scan(&areaID)
	if err != nil {
		return "", fmt.Errorf("insert area: %w", err)
	}
	return areaID, nil
}
