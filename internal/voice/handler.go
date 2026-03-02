package voice

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"firelevel-backend/internal/auth"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	lkauth "github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
)

// ===========================================
// VOICE — LiveKit Token + Agent Dispatch
// Creates a LiveKit room, dispatches the voice agent,
// and returns an access token to iOS.
// ===========================================

type Handler struct {
	jwtSecret string
}

func NewHandler(jwtSecret string) *Handler {
	return &Handler{jwtSecret: jwtSecret}
}

// --- Request/Response for iOS ---

type livekitTokenRequest struct {
	Mode string `json:"mode,omitempty"`
	Lang string `json:"lang,omitempty"`
}

type livekitTokenResponse struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

// GenerateLiveKitToken creates a LiveKit room, dispatches the voice agent,
// and returns a participant token for iOS.
// POST /voice/livekit-token
func (h *Handler) GenerateLiveKitToken(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	lkAPIKey := os.Getenv("LIVEKIT_API_KEY")
	lkAPISecret := os.Getenv("LIVEKIT_API_SECRET")
	lkURL := os.Getenv("LIVEKIT_URL")

	if lkAPIKey == "" || lkAPISecret == "" || lkURL == "" {
		http.Error(w, "LiveKit not configured", http.StatusInternalServerError)
		return
	}

	var req livekitTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	lang := req.Lang
	if lang == "" {
		lang = "fr"
	}
	mode := req.Mode
	if mode == "" {
		mode = "voice_call"
	}

	// Generate a unique room name
	roomName := fmt.Sprintf("volta-%s-%s", userID[:8], uuid.New().String()[:8])

	// Create an agent auth token (JWT) so the bot can call Focus API as this user
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

	// Build room metadata (the agent reads this on room join)
	metadata := map[string]string{
		"lang":       lang,
		"mode":       mode,
		"auth_token": agentTokenStr,
	}
	metadataJSON, _ := json.Marshal(metadata)

	// 1. Create the room explicitly with metadata
	roomClient := lksdk.NewRoomServiceClient(lkURL, lkAPIKey, lkAPISecret)
	_, err = roomClient.CreateRoom(context.Background(), &livekit.CreateRoomRequest{
		Name:            roomName,
		Metadata:        string(metadataJSON),
		EmptyTimeout:    60,  // close room after 60s if empty
		MaxParticipants: 2,   // user + agent
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create room: %v", err), http.StatusInternalServerError)
		return
	}

	// 2. Dispatch the voice agent to the room
	agentClient := lksdk.NewAgentDispatchServiceClient(lkURL, lkAPIKey, lkAPISecret)
	_, err = agentClient.CreateDispatch(context.Background(), &livekit.CreateAgentDispatchRequest{
		AgentName: "agent",
		Room:      roomName,
		Metadata:  string(metadataJSON),
	})
	if err != nil {
		// Log but don't fail — agent may auto-dispatch
		fmt.Printf("Warning: agent dispatch failed: %v\n", err)
	}

	// 3. Create LiveKit access token for the iOS participant
	at := lkauth.NewAccessToken(lkAPIKey, lkAPISecret)
	grant := &lkauth.VideoGrant{
		RoomJoin: true,
		Room:     roomName,
	}
	at.AddGrant(grant).
		SetIdentity(userID).
		SetName("user").
		SetValidFor(1 * time.Hour).
		SetMetadata(string(metadataJSON))

	token, err := at.ToJWT()
	if err != nil {
		http.Error(w, "Failed to generate LiveKit token", http.StatusInternalServerError)
		return
	}

	result := livekitTokenResponse{
		URL:   lkURL,
		Token: token,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
