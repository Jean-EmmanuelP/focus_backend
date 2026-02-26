package voice

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"firelevel-backend/internal/auth"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// ===========================================
// VOICE - LiveKit Token Generation
// Generates signed tokens for iOS <-> LiveKit Agent
// ===========================================

type Handler struct{}

func NewHandler() *Handler {
	return &Handler{}
}

// tokenRequest from iOS client
type tokenRequest struct {
	RoomName string `json:"room_name"`
	Metadata string `json:"metadata,omitempty"`
}

// tokenResponse sent back to iOS
type tokenResponse struct {
	Token string `json:"token"`
	URL   string `json:"url"`
}

// videoGrant encodes LiveKit room permissions
type videoGrant struct {
	RoomJoin       bool   `json:"roomJoin"`
	Room           string `json:"room"`
	CanPublish     bool   `json:"canPublish"`
	CanSubscribe   bool   `json:"canSubscribe"`
	CanPublishData bool   `json:"canPublishData"`
}

// livekitClaims extends jwt.RegisteredClaims with LiveKit-specific fields
type livekitClaims struct {
	jwt.RegisteredClaims
	Video    videoGrant `json:"video"`
	Metadata string     `json:"metadata,omitempty"`
}

// GenerateToken creates a LiveKit room token for the authenticated user.
// POST /voice/livekit-token
func (h *Handler) GenerateToken(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	apiKey := os.Getenv("LIVEKIT_API_KEY")
	apiSecret := os.Getenv("LIVEKIT_API_SECRET")
	livekitURL := os.Getenv("LIVEKIT_URL")

	if apiKey == "" || apiSecret == "" || livekitURL == "" {
		http.Error(w, "LiveKit not configured", http.StatusInternalServerError)
		return
	}

	var req tokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.RoomName == "" {
		http.Error(w, "room_name is required", http.StatusBadRequest)
		return
	}

	// Build participant metadata (mode + user context for the agent)
	participantMeta := req.Metadata

	now := time.Now()
	claims := livekitClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    apiKey,
			Subject:   userID,
			ID:        uuid.New().String(),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(1 * time.Hour)),
		},
		Video: videoGrant{
			RoomJoin:       true,
			Room:           req.RoomName,
			CanPublish:     true,
			CanSubscribe:   true,
			CanPublishData: true,
		},
		Metadata: participantMeta,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString([]byte(apiSecret))
	if err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	resp := tokenResponse{
		Token: signedToken,
		URL:   livekitURL,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
