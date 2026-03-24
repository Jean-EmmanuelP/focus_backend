package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"firelevel-backend/internal/auth"
	"firelevel-backend/internal/backboard"

	"github.com/google/uuid"
)

// ==========================================
// New Backboard-powered request/response types
// These will replace the old Gemini types once the frontend migrates.
// ==========================================

type BackboardSendMessageRequest struct {
	Content       string                   `json:"content"`
	Source        string                   `json:"source,omitempty"` // "app", "web", "whatsapp"
	DeviceContext *backboard.DeviceContext  `json:"device_context,omitempty"`
}

type BackboardSendMessageResponse struct {
	Reply       string               `json:"reply"`
	MessageID   string               `json:"message_id"`
	SideEffects []backboard.SideEffect `json:"side_effects,omitempty"`
}

type BackboardHistoryResponse struct {
	Messages []BackboardHistoryMessage `json:"messages"`
}

type BackboardHistoryMessage struct {
	MessageID string `json:"message_id,omitempty"`
	Role      string `json:"role"` // "user" or "assistant"
	Content   string `json:"content"`
	CreatedAt string `json:"created_at,omitempty"`
}

// ==========================================
// Backboard-powered Handlers
// ==========================================

// SendMessageV2 handles POST /chat/v2/message — Backboard-powered chat.
// The frontend sends a message, the backend handles all Backboard communication
// and tool execution, and returns the AI reply + side effects.
func (h *Handler) SendMessageV2(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req BackboardSendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Content == "" {
		http.Error(w, "Content is required", http.StatusBadRequest)
		return
	}

	bbClient := h.getBackboardClient()
	if bbClient == nil {
		http.Error(w, "AI service not configured", http.StatusServiceUnavailable)
		return
	}

	ctx := r.Context()

	// 1. Ensure the user has a Backboard assistant
	assistantID, err := h.ensureBackboardAssistant(ctx, userID, bbClient)
	if err != nil {
		log.Printf("❌ ensureAssistant failed for user %s: %v", userID, err)
		http.Error(w, "Failed to setup AI assistant", http.StatusInternalServerError)
		return
	}

	// 2. Ensure the user has a thread
	threadID, err := h.ensureBackboardThread(ctx, userID, assistantID, bbClient)
	if err != nil {
		log.Printf("❌ ensureThread failed for user %s: %v", userID, err)
		http.Error(w, "Failed to setup conversation", http.StatusInternalServerError)
		return
	}

	// 3. Send the message to Backboard
	response, err := bbClient.SendMessage(ctx, threadID, req.Content)
	if err != nil {
		log.Printf("❌ SendMessage failed for user %s: %v", userID, err)
		http.Error(w, "AI service error", http.StatusBadGateway)
		return
	}

	// 4. Execute the tool call loop
	executor := backboard.NewExecutor(h.db, bbClient)
	reply, sideEffects, err := executor.RunToolLoop(ctx, userID, threadID, assistantID, response, req.DeviceContext)
	if err != nil {
		log.Printf("❌ Tool loop failed for user %s: %v", userID, err)
		// Return what we have
		reply = "Désolé, j'ai un souci technique. Tu peux réessayer ?"
	}

	// 5. Return the response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(BackboardSendMessageResponse{
		Reply:       reply,
		MessageID:   uuid.New().String(),
		SideEffects: sideEffects,
	})
}

// GetHistoryV2 handles GET /chat/v2/history — returns messages from Backboard thread.
func (h *Handler) GetHistoryV2(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	bbClient := h.getBackboardClient()
	if bbClient == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(BackboardHistoryResponse{Messages: []BackboardHistoryMessage{}})
		return
	}

	ctx := r.Context()

	// Get thread ID from user profile
	var threadID *string
	h.db.QueryRow(ctx, "SELECT backboard_thread_id FROM public.users WHERE id = $1", userID).Scan(&threadID)

	if threadID == nil || *threadID == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(BackboardHistoryResponse{Messages: []BackboardHistoryMessage{}})
		return
	}

	thread, err := bbClient.GetThread(ctx, *threadID)
	if err != nil {
		log.Printf("⚠️ GetThread failed for user %s: %v", userID, err)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(BackboardHistoryResponse{Messages: []BackboardHistoryMessage{}})
		return
	}

	var messages []BackboardHistoryMessage
	for _, msg := range thread.Messages {
		if msg.Content == nil || *msg.Content == "" {
			continue
		}
		// Only include user and assistant messages
		if msg.Role != "user" && msg.Role != "assistant" {
			continue
		}
		m := BackboardHistoryMessage{
			Role:    msg.Role,
			Content: *msg.Content,
		}
		if msg.MessageID != nil {
			m.MessageID = *msg.MessageID
		}
		if msg.CreatedAt != nil {
			m.CreatedAt = *msg.CreatedAt
		}
		messages = append(messages, m)
	}
	if messages == nil {
		messages = []BackboardHistoryMessage{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(BackboardHistoryResponse{Messages: messages})
}

// ClearHistoryV2 handles DELETE /chat/v2/history — deletes the Backboard thread.
func (h *Handler) ClearHistoryV2(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	bbClient := h.getBackboardClient()
	if bbClient == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	ctx := r.Context()

	var threadID *string
	h.db.QueryRow(ctx, "SELECT backboard_thread_id FROM public.users WHERE id = $1", userID).Scan(&threadID)

	if threadID != nil && *threadID != "" {
		if err := bbClient.DeleteThread(ctx, *threadID); err != nil {
			log.Printf("⚠️ DeleteThread failed: %v", err)
		}
	}

	// Clear from user profile
	h.db.Exec(ctx, "UPDATE public.users SET backboard_thread_id = NULL WHERE id = $1", userID)

	w.WriteHeader(http.StatusNoContent)
}

// SendVoiceMessageV2 handles POST /chat/v2/voice — transcribes then processes through Backboard.
func (h *Handler) SendVoiceMessageV2(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserContextKey).(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	h.ensureUserExists(r.Context(), userID)

	// Parse multipart form (max 10MB)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("audio")
	if err != nil {
		http.Error(w, "Audio file required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	audioData := make([]byte, header.Size)
	if _, err := file.Read(audioData); err != nil {
		http.Error(w, "Failed to read audio", http.StatusInternalServerError)
		return
	}

	// Transcribe using Gemini (keep existing transcription)
	transcript, err := h.transcribeAudio(r.Context(), audioData, header.Filename)
	if err != nil || transcript == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"reply":      "J'ai pas bien entendu, tu peux répéter ?",
			"transcript": "",
			"message_id": "",
		})
		return
	}

	// Process through Backboard (same as text message)
	bbClient := h.getBackboardClient()
	if bbClient == nil {
		http.Error(w, "AI service not configured", http.StatusServiceUnavailable)
		return
	}

	ctx := r.Context()
	assistantID, err := h.ensureBackboardAssistant(ctx, userID, bbClient)
	if err != nil {
		http.Error(w, "Failed to setup AI assistant", http.StatusInternalServerError)
		return
	}

	threadID, err := h.ensureBackboardThread(ctx, userID, assistantID, bbClient)
	if err != nil {
		http.Error(w, "Failed to setup conversation", http.StatusInternalServerError)
		return
	}

	response, err := bbClient.SendMessage(ctx, threadID, transcript)
	if err != nil {
		http.Error(w, "AI service error", http.StatusBadGateway)
		return
	}

	// Parse device context from form values
	var deviceCtx *backboard.DeviceContext
	if dcJSON := r.FormValue("device_context"); dcJSON != "" {
		deviceCtx = &backboard.DeviceContext{}
		json.Unmarshal([]byte(dcJSON), deviceCtx)
	}

	executor := backboard.NewExecutor(h.db, bbClient)
	reply, sideEffects, err := executor.RunToolLoop(ctx, userID, threadID, assistantID, response, deviceCtx)
	if err != nil {
		reply = "Désolé, j'ai un souci technique. Tu peux réessayer ?"
	}

	// Increment voice counter
	var updatedCount int
	h.db.QueryRow(ctx, `
		UPDATE users SET free_voice_messages_used = COALESCE(free_voice_messages_used, 0) + 1
		WHERE id = $1 RETURNING free_voice_messages_used
	`, userID).Scan(&updatedCount)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"reply":                    reply,
		"transcript":               transcript,
		"message_id":               uuid.New().String(),
		"side_effects":             sideEffects,
		"free_voice_messages_used": updatedCount,
	})
}

// ==========================================
// Backboard Lifecycle Helpers
// ==========================================

func (h *Handler) getBackboardClient() *backboard.Client {
	apiKey := bbAPIKey
	if apiKey == "" {
		return nil
	}
	return backboard.NewClient(apiKey)
}

// bbAPIKey is loaded once at init from env.
var bbAPIKey string

func init() {
	bbAPIKey = ""
}

// SetBackboardAPIKey sets the Backboard API key (called from main.go).
func SetBackboardAPIKey(key string) {
	bbAPIKey = key
}

// ensureBackboardAssistant ensures the user has a Backboard assistant, creating one if needed.
func (h *Handler) ensureBackboardAssistant(ctx context.Context, userID string, bbClient *backboard.Client) (string, error) {
	// Check DB for existing assistant ID
	var assistantID *string
	h.db.QueryRow(ctx, "SELECT backboard_assistant_id FROM public.users WHERE id = $1", userID).Scan(&assistantID)

	if assistantID != nil && *assistantID != "" {
		// Update the assistant with fresh date/time
		var companionName, timezone string
		var harshMode bool
		h.db.QueryRow(ctx, `
			SELECT COALESCE(companion_name, 'Kai'), COALESCE(timezone, 'Europe/Paris'), COALESCE(coach_harsh_mode, false)
			FROM public.users WHERE id = $1
		`, userID).Scan(&companionName, &timezone, &harshMode)

		config := backboard.BuildAssistantConfig(companionName, harshMode, timezone)
		if err := bbClient.UpdateAssistant(ctx, *assistantID, config); err != nil {
			log.Printf("⚠️ Failed to update assistant date for user %s: %v", userID, err)
			// Non-fatal: continue with existing assistant
		}
		return *assistantID, nil
	}

	// Create new assistant
	var companionName, timezone string
	var harshMode bool
	h.db.QueryRow(ctx, `
		SELECT COALESCE(companion_name, 'Kai'), COALESCE(timezone, 'Europe/Paris'), COALESCE(coach_harsh_mode, false)
		FROM public.users WHERE id = $1
	`, userID).Scan(&companionName, &timezone, &harshMode)

	config := backboard.BuildAssistantConfig(companionName, harshMode, timezone)
	newID, err := bbClient.CreateAssistant(ctx, config)
	if err != nil {
		return "", fmt.Errorf("create assistant: %w", err)
	}

	// Save to user profile
	h.db.Exec(ctx, "UPDATE public.users SET backboard_assistant_id = $1 WHERE id = $2", newID, userID)
	log.Printf("🤖 Created Backboard assistant %s for user %s", newID, userID)

	return newID, nil
}

// ensureBackboardThread ensures the user has a conversation thread, creating one if needed.
func (h *Handler) ensureBackboardThread(ctx context.Context, userID, assistantID string, bbClient *backboard.Client) (string, error) {
	// Check DB for existing thread ID
	var threadID *string
	h.db.QueryRow(ctx, "SELECT backboard_thread_id FROM public.users WHERE id = $1", userID).Scan(&threadID)

	if threadID != nil && *threadID != "" {
		return *threadID, nil
	}

	// Check Backboard for existing threads
	threads, err := bbClient.ListThreads(ctx, assistantID)
	if err == nil && len(threads) > 0 {
		tid := threads[0].ThreadID
		h.db.Exec(ctx, "UPDATE public.users SET backboard_thread_id = $1 WHERE id = $2", tid, userID)
		return tid, nil
	}

	// Create new thread
	newThreadID, err := bbClient.CreateThread(ctx, assistantID)
	if err != nil {
		return "", fmt.Errorf("create thread: %w", err)
	}

	h.db.Exec(ctx, "UPDATE public.users SET backboard_thread_id = $1 WHERE id = $2", newThreadID, userID)
	log.Printf("🧵 Created Backboard thread %s for user %s", newThreadID, userID)

	return newThreadID, nil
}
