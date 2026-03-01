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
// VOICE - Daily + Pipecat Bot Orchestration
// Creates Daily room, spawns Pipecat bot, returns token to iOS
// ===========================================

type Handler struct {
	jwtSecret string
}

func NewHandler(jwtSecret string) *Handler {
	return &Handler{jwtSecret: jwtSecret}
}

// --- Daily API types ---

type dailyRoomRequest struct {
	Properties dailyRoomProperties `json:"properties"`
}

type dailyRoomProperties struct {
	MaxParticipants int  `json:"max_participants"`
	ExpInSeconds    int  `json:"exp"`
	EnableChat      bool `json:"enable_chat"`
}

type dailyRoomResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

type dailyTokenRequest struct {
	Properties dailyTokenProperties `json:"properties"`
}

type dailyTokenProperties struct {
	RoomName string `json:"room_name"`
	IsOwner  bool   `json:"is_owner"`
	ExpInSec int    `json:"exp"`
	UserName string `json:"user_name,omitempty"`
}

type dailyTokenResponse struct {
	Token string `json:"token"`
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

// --- Bot spawn request ---

type botStartRequest struct {
	RoomURL string         `json:"room_url"`
	Token   string         `json:"token"`
	Config  map[string]any `json:"config"`
}

// GenerateDailyToken creates a Daily room, spawns a Pipecat bot, and returns
// a user meeting token for the iOS client.
// POST /voice/daily-token
func (h *Handler) GenerateDailyToken(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	dailyAPIKey := os.Getenv("DAILY_API_KEY")
	pipecatBotURL := os.Getenv("PIPECAT_BOT_URL")

	if dailyAPIKey == "" {
		http.Error(w, "Daily not configured", http.StatusInternalServerError)
		return
	}
	if pipecatBotURL == "" {
		pipecatBotURL = "http://localhost:7860"
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

	// 1. Create Daily room
	roomReq := dailyRoomRequest{
		Properties: dailyRoomProperties{
			MaxParticipants: 2,
			ExpInSeconds:    3600,
			EnableChat:      true,
		},
	}
	roomBody, _ := json.Marshal(roomReq)

	httpReq, _ := http.NewRequest("POST", "https://api.daily.co/v1/rooms", bytes.NewReader(roomBody))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+dailyAPIKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create Daily room: %v", err), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		http.Error(w, fmt.Sprintf("Daily API error: %d", resp.StatusCode), http.StatusInternalServerError)
		return
	}

	var room dailyRoomResponse
	if err := json.NewDecoder(resp.Body).Decode(&room); err != nil {
		http.Error(w, "Failed to parse Daily room response", http.StatusInternalServerError)
		return
	}

	// 2. Generate agent auth token (JWT signed with our secret for bot→backend API calls)
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

	// 3. Create bot meeting token (for the Pipecat bot to join the room)
	botTokenReq := dailyTokenRequest{
		Properties: dailyTokenProperties{
			RoomName: room.Name,
			IsOwner:  true,
			ExpInSec: 3600,
			UserName: "Volta",
		},
	}
	botTokenBody, _ := json.Marshal(botTokenReq)

	httpReq2, _ := http.NewRequest("POST", "https://api.daily.co/v1/meeting-tokens", bytes.NewReader(botTokenBody))
	httpReq2.Header.Set("Content-Type", "application/json")
	httpReq2.Header.Set("Authorization", "Bearer "+dailyAPIKey)

	resp2, err := client.Do(httpReq2)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create bot token: %v", err), http.StatusInternalServerError)
		return
	}
	defer resp2.Body.Close()

	var botToken dailyTokenResponse
	if err := json.NewDecoder(resp2.Body).Decode(&botToken); err != nil {
		http.Error(w, "Failed to parse bot token response", http.StatusInternalServerError)
		return
	}

	// 4. Spawn the Pipecat bot
	botReq := botStartRequest{
		RoomURL: room.URL,
		Token:   botToken.Token,
		Config: map[string]any{
			"mode":       mode,
			"lang":       lang,
			"auth_token": agentTokenStr,
		},
	}
	botReqBody, _ := json.Marshal(botReq)

	httpReq3, _ := http.NewRequest("POST", pipecatBotURL+"/start_bot", bytes.NewReader(botReqBody))
	httpReq3.Header.Set("Content-Type", "application/json")

	resp3, err := client.Do(httpReq3)
	if err != nil {
		// Bot spawn failed but room is created — still return token so iOS can connect
		// (bot may join later or user gets an error)
		fmt.Printf("WARNING: Failed to spawn bot: %v\n", err)
	} else {
		resp3.Body.Close()
	}

	// 5. Create user meeting token
	userTokenReq := dailyTokenRequest{
		Properties: dailyTokenProperties{
			RoomName: room.Name,
			IsOwner:  false,
			ExpInSec: 3600,
			UserName: userID,
		},
	}
	userTokenBody, _ := json.Marshal(userTokenReq)

	httpReq4, _ := http.NewRequest("POST", "https://api.daily.co/v1/meeting-tokens", bytes.NewReader(userTokenBody))
	httpReq4.Header.Set("Content-Type", "application/json")
	httpReq4.Header.Set("Authorization", "Bearer "+dailyAPIKey)

	resp4, err := client.Do(httpReq4)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create user token: %v", err), http.StatusInternalServerError)
		return
	}
	defer resp4.Body.Close()

	var userToken dailyTokenResponse
	if err := json.NewDecoder(resp4.Body).Decode(&userToken); err != nil {
		http.Error(w, "Failed to parse user token response", http.StatusInternalServerError)
		return
	}

	// 6. Return room URL + user token to iOS
	result := tokenResponse{
		RoomURL: room.URL,
		Token:   userToken.Token,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
