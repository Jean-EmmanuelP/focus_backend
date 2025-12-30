package telegram

import (
	"encoding/json"
	"log"
	"net/http"
)

// WebhookHandler handles incoming webhooks from Supabase triggers
type WebhookHandler struct {
	service *Service
}

// NewWebhookHandler creates a new webhook handler
func NewWebhookHandler() *WebhookHandler {
	return &WebhookHandler{
		service: Get(),
	}
}

// UserCreatedPayload represents the Supabase trigger payload for new users
type UserCreatedPayload struct {
	Type   string `json:"type"`
	Table  string `json:"table"`
	Schema string `json:"schema"`
	Record struct {
		ID        string  `json:"id"`
		Email     *string `json:"email"`
		Pseudo    *string `json:"pseudo"`
		FirstName *string `json:"first_name"`
		CreatedAt string  `json:"created_at"`
	} `json:"record"`
	OldRecord interface{} `json:"old_record"`
}

// HandleUserCreated handles the webhook when a new user is created
// POST /webhooks/user-created
func (h *WebhookHandler) HandleUserCreated(w http.ResponseWriter, r *http.Request) {
	var payload UserCreatedPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Printf("❌ Webhook decode error: %v", err)
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	// Get user display name
	userName := "New User"
	if payload.Record.Pseudo != nil && *payload.Record.Pseudo != "" {
		userName = *payload.Record.Pseudo
	} else if payload.Record.FirstName != nil && *payload.Record.FirstName != "" {
		userName = *payload.Record.FirstName
	}

	// Get email
	userEmail := ""
	if payload.Record.Email != nil {
		userEmail = *payload.Record.Email
	}

	// Send notification
	h.service.Send(Event{
		Type:      EventUserSignup,
		UserID:    payload.Record.ID,
		UserName:  userName,
		UserEmail: userEmail,
	})

	log.Printf("✅ New user webhook processed: %s (%s)", userName, payload.Record.ID)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}
