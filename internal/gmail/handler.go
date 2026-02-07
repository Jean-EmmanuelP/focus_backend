package gmail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"firelevel-backend/internal/auth"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// ===========================================
// GMAIL INTEGRATION - AI Persona Building
// ===========================================
// Analyzes user's emails to build a rich persona
// for the AI companion (Kai) to better understand
// the user's life, interests, and communication style.
// ===========================================

// GmailConfig represents stored Gmail configuration
type GmailConfig struct {
	ID               string     `json:"id"`
	UserID           string     `json:"user_id"`
	IsConnected      bool       `json:"is_connected"`
	GoogleEmail      *string    `json:"google_email"`
	AccessToken      *string    `json:"-"` // Don't expose token
	RefreshToken     *string    `json:"-"` // Don't expose token
	TokenExpiry      *time.Time `json:"-"`
	LastAnalyzedAt   *time.Time `json:"last_analyzed_at"`
	PersonaGenerated bool       `json:"persona_generated"`
	MessagesAnalyzed int        `json:"messages_analyzed"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// SaveTokensRequest for saving OAuth tokens
type SaveTokensRequest struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	GoogleEmail  string `json:"google_email"`
}

// AnalysisResult returned after email analysis
type AnalysisResult struct {
	Success          bool         `json:"success"`
	MessagesAnalyzed int          `json:"messages_analyzed"`
	PersonaExtracted *PersonaData `json:"persona_extracted,omitempty"`
	Error            string       `json:"error,omitempty"`
}

// PersonaData extracted from email analysis
type PersonaData struct {
	Interests          []string `json:"interests,omitempty"`
	CommunicationStyle string   `json:"communication_style,omitempty"`
	ProfessionalContext string  `json:"professional_context,omitempty"`
	FrequentContacts   []string `json:"frequent_contacts,omitempty"`
	Topics             []string `json:"topics,omitempty"`
	WorkPlace          string   `json:"work_place,omitempty"`
	Role               string   `json:"role,omitempty"`
}

// Handler holds dependencies
type Handler struct {
	db          *pgxpool.Pool
	oauthConfig *oauth2.Config
}

// NewHandler creates a new Gmail handler
func NewHandler(db *pgxpool.Pool) *Handler {
	// OAuth2 config for Gmail
	config := &oauth2.Config{
		ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		Scopes: []string{
			gmail.GmailReadonlyScope,
			"https://www.googleapis.com/auth/userinfo.email",
		},
		Endpoint: google.Endpoint,
	}

	return &Handler{
		db:          db,
		oauthConfig: config,
	}
}

// GetConfig returns Gmail configuration
// GET /gmail/config
func (h *Handler) GetConfig(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	query := `
		SELECT id, is_connected, google_email, last_analyzed_at,
		       persona_generated, messages_analyzed
		FROM public.gmail_config
		WHERE user_id = $1
	`

	var config GmailConfig
	err := h.db.QueryRow(r.Context(), query, userID).Scan(
		&config.ID,
		&config.IsConnected,
		&config.GoogleEmail,
		&config.LastAnalyzedAt,
		&config.PersonaGenerated,
		&config.MessagesAnalyzed,
	)

	if err != nil {
		// Return default config if not found
		config = GmailConfig{
			IsConnected:      false,
			PersonaGenerated: false,
			MessagesAnalyzed: 0,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

// SaveTokens saves OAuth tokens after Google Sign-In
// POST /gmail/tokens
func (h *Handler) SaveTokens(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	var req SaveTokensRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Calculate token expiry
	expiry := time.Now().Add(time.Duration(req.ExpiresIn) * time.Second)

	// Upsert Gmail config
	query := `
		INSERT INTO public.gmail_config (
			user_id, is_connected, google_email, access_token,
			refresh_token, token_expiry, created_at, updated_at
		)
		VALUES ($1, true, $2, $3, $4, $5, NOW(), NOW())
		ON CONFLICT (user_id)
		DO UPDATE SET
			is_connected = true,
			google_email = EXCLUDED.google_email,
			access_token = EXCLUDED.access_token,
			refresh_token = COALESCE(EXCLUDED.refresh_token, gmail_config.refresh_token),
			token_expiry = EXCLUDED.token_expiry,
			updated_at = NOW()
		RETURNING id, is_connected, google_email, last_analyzed_at, persona_generated, messages_analyzed
	`

	var config GmailConfig
	err := h.db.QueryRow(r.Context(), query,
		userID, req.GoogleEmail, req.AccessToken, req.RefreshToken, expiry,
	).Scan(
		&config.ID,
		&config.IsConnected,
		&config.GoogleEmail,
		&config.LastAnalyzedAt,
		&config.PersonaGenerated,
		&config.MessagesAnalyzed,
	)

	if err != nil {
		fmt.Printf("❌ Gmail tokens save error: %v\n", err)
		http.Error(w, "Failed to save tokens", http.StatusInternalServerError)
		return
	}

	fmt.Printf("✅ Gmail connected for user %s: %s\n", userID, req.GoogleEmail)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

// Analyze fetches and analyzes emails to build persona
// POST /gmail/analyze
func (h *Handler) Analyze(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)
	fmt.Printf("📧 Starting Gmail analysis for user %s\n", userID)

	// Get stored tokens
	var accessToken, refreshToken string
	var tokenExpiry time.Time

	query := `
		SELECT access_token, refresh_token, token_expiry
		FROM public.gmail_config
		WHERE user_id = $1 AND is_connected = true
	`
	err := h.db.QueryRow(r.Context(), query, userID).Scan(
		&accessToken, &refreshToken, &tokenExpiry,
	)

	if err != nil {
		json.NewEncoder(w).Encode(AnalysisResult{
			Success: false,
			Error:   "Gmail not connected",
		})
		return
	}

	// Create OAuth token
	token := &oauth2.Token{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		Expiry:       tokenExpiry,
	}

	// Create Gmail client
	ctx := context.Background()
	client := h.oauthConfig.Client(ctx, token)

	gmailService, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		fmt.Printf("❌ Gmail service error: %v\n", err)
		json.NewEncoder(w).Encode(AnalysisResult{
			Success: false,
			Error:   "Failed to create Gmail service",
		})
		return
	}

	// Fetch recent emails (last 100 sent emails to understand communication style)
	fmt.Println("📧 Fetching sent emails...")
	sentMessages, err := h.fetchSentEmails(gmailService, 50)
	if err != nil {
		fmt.Printf("❌ Failed to fetch sent emails: %v\n", err)
	}

	// Fetch recent received emails for context
	fmt.Println("📧 Fetching received emails...")
	receivedMessages, err := h.fetchReceivedEmails(gmailService, 50)
	if err != nil {
		fmt.Printf("❌ Failed to fetch received emails: %v\n", err)
	}

	totalMessages := len(sentMessages) + len(receivedMessages)
	fmt.Printf("📧 Fetched %d total messages\n", totalMessages)

	// Analyze emails with AI to extract persona
	persona, err := h.analyzeWithAI(r.Context(), userID, sentMessages, receivedMessages)
	if err != nil {
		fmt.Printf("❌ AI analysis error: %v\n", err)
		// Still save the count even if AI fails
	}

	// Update database with analysis results
	updateQuery := `
		UPDATE public.gmail_config
		SET last_analyzed_at = NOW(),
		    messages_analyzed = $2,
		    persona_generated = $3,
		    updated_at = NOW()
		WHERE user_id = $1
	`
	h.db.Exec(r.Context(), updateQuery, userID, totalMessages, persona != nil)

	// Save persona to user knowledge if extracted
	if persona != nil {
		h.savePersonaToKnowledge(r.Context(), userID, persona)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(AnalysisResult{
		Success:          true,
		MessagesAnalyzed: totalMessages,
		PersonaExtracted: persona,
	})
}

// Disconnect removes Gmail connection
// DELETE /gmail/config
func (h *Handler) Disconnect(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(auth.UserContextKey).(string)

	query := `
		UPDATE public.gmail_config
		SET is_connected = false,
		    access_token = NULL,
		    refresh_token = NULL,
		    updated_at = NOW()
		WHERE user_id = $1
	`
	h.db.Exec(r.Context(), query, userID)

	w.WriteHeader(http.StatusNoContent)
}

// fetchSentEmails fetches sent emails
func (h *Handler) fetchSentEmails(service *gmail.Service, maxResults int64) ([]EmailData, error) {
	resp, err := service.Users.Messages.List("me").
		Q("in:sent").
		MaxResults(maxResults).
		Do()

	if err != nil {
		return nil, err
	}

	return h.fetchEmailDetails(service, resp.Messages)
}

// fetchReceivedEmails fetches received emails
func (h *Handler) fetchReceivedEmails(service *gmail.Service, maxResults int64) ([]EmailData, error) {
	resp, err := service.Users.Messages.List("me").
		Q("in:inbox -category:promotions -category:social").
		MaxResults(maxResults).
		Do()

	if err != nil {
		return nil, err
	}

	return h.fetchEmailDetails(service, resp.Messages)
}

// EmailData represents parsed email
type EmailData struct {
	Subject string
	From    string
	To      string
	Snippet string
	Body    string
	Date    time.Time
}

// fetchEmailDetails fetches full email content
func (h *Handler) fetchEmailDetails(service *gmail.Service, messages []*gmail.Message) ([]EmailData, error) {
	var emails []EmailData

	for _, msg := range messages {
		fullMsg, err := service.Users.Messages.Get("me", msg.Id).
			Format("full").
			Do()
		if err != nil {
			continue
		}

		email := EmailData{
			Snippet: fullMsg.Snippet,
		}

		// Extract headers
		for _, header := range fullMsg.Payload.Headers {
			switch header.Name {
			case "Subject":
				email.Subject = header.Value
			case "From":
				email.From = header.Value
			case "To":
				email.To = header.Value
			case "Date":
				// Parse date (simplified)
				email.Date = time.Now()
			}
		}

		// Extract body (simplified - just use snippet for now)
		email.Body = h.extractBody(fullMsg.Payload)
		if email.Body == "" {
			email.Body = fullMsg.Snippet
		}

		emails = append(emails, email)
	}

	return emails, nil
}

// extractBody extracts text body from email
func (h *Handler) extractBody(payload *gmail.MessagePart) string {
	if payload.MimeType == "text/plain" && payload.Body != nil && payload.Body.Data != "" {
		decoded, err := base64.URLEncoding.DecodeString(payload.Body.Data)
		if err == nil {
			return string(decoded)
		}
	}

	// Check parts
	for _, part := range payload.Parts {
		if body := h.extractBody(part); body != "" {
			return body
		}
	}

	return ""
}

// analyzeWithAI uses Gemini to analyze emails and extract persona
func (h *Handler) analyzeWithAI(ctx context.Context, userID string, sent, received []EmailData) (*PersonaData, error) {
	geminiKey := os.Getenv("GEMINI_API_KEY")
	if geminiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY not set")
	}

	// Build prompt with email samples
	var emailSamples strings.Builder
	emailSamples.WriteString("SENT EMAILS (to understand communication style):\n")
	for i, email := range sent {
		if i >= 20 { // Limit to 20 samples
			break
		}
		emailSamples.WriteString(fmt.Sprintf("- Subject: %s\n  Snippet: %s\n\n", email.Subject, truncate(email.Snippet, 200)))
	}

	emailSamples.WriteString("\nRECEIVED EMAILS (to understand context and relationships):\n")
	for i, email := range received {
		if i >= 20 {
			break
		}
		emailSamples.WriteString(fmt.Sprintf("- From: %s\n  Subject: %s\n  Snippet: %s\n\n", email.From, email.Subject, truncate(email.Snippet, 200)))
	}

	prompt := fmt.Sprintf(`Analyze these email samples to build a persona profile for an AI companion.
Extract the following information in JSON format:

%s

Respond ONLY with valid JSON in this exact format:
{
  "interests": ["list of interests/hobbies detected"],
  "communication_style": "brief description of how they communicate (formal/casual, verbose/concise, etc.)",
  "professional_context": "their job/industry if apparent",
  "frequent_contacts": ["names/types of people they interact with most"],
  "topics": ["recurring topics in their emails"],
  "work_place": "company or organization name if detected",
  "role": "their job role/title if detected"
}

If you can't detect something, use null for that field.`, emailSamples.String())

	// Call Gemini
	reqBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]string{
					{"text": prompt},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature":     0.3,
			"maxOutputTokens": 1000,
		},
	}

	jsonBody, _ := json.Marshal(reqBody)

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key=%s", geminiKey)
	resp, err := http.Post(url, "application/json", strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// Parse Gemini response
	var geminiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return nil, err
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("no response from Gemini")
	}

	// Parse the JSON from Gemini's response
	responseText := geminiResp.Candidates[0].Content.Parts[0].Text

	// Extract JSON from response (might be wrapped in markdown code blocks)
	jsonStart := strings.Index(responseText, "{")
	jsonEnd := strings.LastIndex(responseText, "}")
	if jsonStart >= 0 && jsonEnd > jsonStart {
		responseText = responseText[jsonStart : jsonEnd+1]
	}

	var persona PersonaData
	if err := json.Unmarshal([]byte(responseText), &persona); err != nil {
		fmt.Printf("❌ Failed to parse persona JSON: %v\nResponse: %s\n", err, responseText)
		return nil, err
	}

	return &persona, nil
}

// savePersonaToKnowledge saves extracted persona to chat context
func (h *Handler) savePersonaToKnowledge(ctx context.Context, userID string, persona *PersonaData) {
	// Save key facts to chat_contexts for Kai to use
	facts := []string{}

	if persona.ProfessionalContext != "" {
		facts = append(facts, fmt.Sprintf("Works in: %s", persona.ProfessionalContext))
	}
	if persona.WorkPlace != "" {
		facts = append(facts, fmt.Sprintf("Works at: %s", persona.WorkPlace))
	}
	if persona.Role != "" {
		facts = append(facts, fmt.Sprintf("Job role: %s", persona.Role))
	}
	if persona.CommunicationStyle != "" {
		facts = append(facts, fmt.Sprintf("Communication style: %s", persona.CommunicationStyle))
	}
	if len(persona.Interests) > 0 {
		facts = append(facts, fmt.Sprintf("Interests: %s", strings.Join(persona.Interests, ", ")))
	}
	if len(persona.Topics) > 0 {
		facts = append(facts, fmt.Sprintf("Common topics: %s", strings.Join(persona.Topics, ", ")))
	}

	// Insert facts into chat_contexts
	for _, fact := range facts {
		query := `
			INSERT INTO public.chat_contexts (user_id, fact, category, mention_count, first_mentioned, last_mentioned)
			VALUES ($1, $2, 'gmail_analysis', 1, NOW(), NOW())
			ON CONFLICT (user_id, fact) DO UPDATE SET
				mention_count = chat_contexts.mention_count + 1,
				last_mentioned = NOW()
		`
		h.db.Exec(ctx, query, userID, fact)
	}

	// Also update user profile with work info
	if persona.WorkPlace != "" || persona.Role != "" {
		var description string
		if persona.Role != "" && persona.WorkPlace != "" {
			description = fmt.Sprintf("%s at %s", persona.Role, persona.WorkPlace)
		} else if persona.WorkPlace != "" {
			description = fmt.Sprintf("Works at %s", persona.WorkPlace)
		} else {
			description = persona.Role
		}

		h.db.Exec(ctx, `UPDATE public.users SET description = $1 WHERE id = $2`, description, userID)
	}

	fmt.Printf("✅ Saved %d persona facts for user %s\n", len(facts), userID)
}

// Helper to truncate strings
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
