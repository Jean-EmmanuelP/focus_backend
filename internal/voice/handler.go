package voice

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"firelevel-backend/internal/auth"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// ===========================================
// VOICE - Pipecat Cloud Orchestration
// Calls Pipecat Cloud API to spawn bot + create Daily room,
// returns room URL + token to iOS.
// ===========================================

type Handler struct {
	jwtSecret string
}

func NewHandler(jwtSecret string) *Handler {
	return &Handler{jwtSecret: jwtSecret}
}

// --- Request/Response for iOS ---

type tokenRequest struct {
	Mode     string `json:"mode,omitempty"`
	Lang     string `json:"lang,omitempty"`
	Metadata string `json:"metadata,omitempty"`
}

type tokenResponse struct {
	RoomURL string `json:"room_url"`
	Token   string `json:"token"`
}

// --- Pipecat Cloud API types ---

type pipecatStartRequest struct {
	CreateDailyRoom bool           `json:"createDailyRoom"`
	Body            map[string]any `json:"body"`
}

type pipecatStartResponse struct {
	DailyRoom  string `json:"dailyRoom"`
	DailyToken string `json:"dailyToken"`
	SessionID  string `json:"sessionId"`
}

// GenerateDailyToken calls Pipecat Cloud to spawn a bot with a Daily room,
// and returns the room URL + user token to iOS.
// POST /voice/daily-token
func (h *Handler) GenerateDailyToken(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	pipecatAPIKey := os.Getenv("PIPECAT_CLOUD_API_KEY")
	pipecatAgent := os.Getenv("PIPECAT_AGENT_NAME")

	if pipecatAPIKey == "" {
		http.Error(w, "Pipecat Cloud not configured", http.StatusInternalServerError)
		return
	}
	if pipecatAgent == "" {
		pipecatAgent = "focus-voice"
	}

	var req tokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Parse metadata if present (backwards compat with iOS sending JSON metadata)
	mode := req.Mode
	lang := req.Lang
	if mode == "" || lang == "" {
		if req.Metadata != "" {
			var metaObj map[string]string
			if err := json.Unmarshal([]byte(req.Metadata), &metaObj); err == nil {
				if mode == "" {
					mode = metaObj["mode"]
				}
				if lang == "" {
					lang = metaObj["lang"]
				}
			}
		}
	}
	if mode == "" {
		mode = "voice_call"
	}
	if lang == "" {
		lang = "fr"
	}

	// 1. Generate agent auth token (JWT signed with our secret for bot→backend API calls)
	now := time.Now()
	agentClaims := jwt.RegisteredClaims{
		Subject:   userID,
		ExpiresAt: jwt.NewNumericDate(now.Add(1 * time.Hour)),
		IssuedAt:  jwt.NewNumericDate(now),
		ID:        uuid.New().String(),
	}
	agentJwt := jwt.NewWithClaims(jwt.SigningMethodHS256, agentClaims)
	agentTokenStr, err := agentJwt.SignedString([]byte(h.jwtSecret))
	if err != nil {
		http.Error(w, "Failed to generate agent token", http.StatusInternalServerError)
		return
	}

	// 2. Call Pipecat Cloud API to spawn bot + create Daily room
	pcReq := pipecatStartRequest{
		CreateDailyRoom: true,
		Body: map[string]any{
			"mode":       mode,
			"lang":       lang,
			"auth_token": agentTokenStr,
		},
	}
	pcBody, _ := json.Marshal(pcReq)

	apiURL := fmt.Sprintf("https://api.pipecat.daily.co/v1/public/%s/start", pipecatAgent)
	httpReq, _ := http.NewRequest("POST", apiURL, bytes.NewReader(pcBody))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+pipecatAPIKey)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to start Pipecat session: %v", err), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		http.Error(w, fmt.Sprintf("Pipecat Cloud API error: %d", resp.StatusCode), http.StatusBadGateway)
		return
	}

	var pcResp pipecatStartResponse
	if err := json.NewDecoder(resp.Body).Decode(&pcResp); err != nil {
		http.Error(w, "Failed to parse Pipecat response", http.StatusInternalServerError)
		return
	}

	// 3. Return room URL + token to iOS
	result := tokenResponse{
		RoomURL: pcResp.DailyRoom,
		Token:   pcResp.DailyToken,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
