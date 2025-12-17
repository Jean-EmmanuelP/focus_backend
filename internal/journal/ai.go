package journal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"google.golang.org/genai"
)

type AIService struct {
	geminiAPIKey string
	genaiClient  *genai.Client
}

func NewAIService() *AIService {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  os.Getenv("GEMINI_API_KEY"),
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		fmt.Printf("Warning: Failed to create genai client for journal: %v\n", err)
	}

	return &AIService{
		geminiAPIKey: os.Getenv("GEMINI_API_KEY"),
		genaiClient:  client,
	}
}

// JournalAnalysis contains AI-extracted information from journal entry
type JournalAnalysis struct {
	Transcript string   `json:"transcript"`
	Summary    string   `json:"summary"`
	Title      string   `json:"title"`
	Mood       string   `json:"mood"`
	MoodScore  int      `json:"mood_score"`
	Tags       []string `json:"tags"`
}

// BilanEntryData is data from entries used for bilan generation
type BilanEntryData struct {
	Transcript *string `json:"transcript"`
	Summary    *string `json:"summary"`
	Mood       *string `json:"mood"`
	MoodScore  *int    `json:"mood_score"`
	Date       string  `json:"date"`
}

// BilanData is the AI-generated bilan content
type BilanData struct {
	Summary        string   `json:"summary"`
	Wins           []string `json:"wins"`
	Improvements   []string `json:"improvements"`
	MoodTrend      string   `json:"mood_trend"`
	AvgMoodScore   float64  `json:"avg_mood_score"`
	SuggestedGoals []string `json:"suggested_goals,omitempty"` // For monthly only
}

// TranscriptResult contains only the transcription from STT
type TranscriptResult struct {
	Transcript string `json:"transcript"`
}

// TranscribeAudio does Speech-to-Text ONLY (no analysis)
// This is called immediately when user uploads audio/video
func (s *AIService) TranscribeAudio(mediaData []byte, mediaType, contentType string) (*TranscriptResult, error) {
	if s.genaiClient == nil {
		return nil, fmt.Errorf("Gemini client not initialized")
	}

	ctx := context.Background()

	mimeType := contentType
	if mimeType == "" {
		if mediaType == "audio" {
			mimeType = "audio/mp4"
		} else {
			mimeType = "video/mp4"
		}
	}

	base64Media := base64.StdEncoding.EncodeToString(mediaData)

	// Simple transcription prompt - just STT, no analysis
	prompt := fmt.Sprintf(`Tu es un service de transcription. L'utilisateur t'envoie un enregistrement audio/video.

Ton UNIQUE travail: Transcrire le contenu en texte (transcription complete et fidele).

Reponds UNIQUEMENT avec un JSON valide:
{
  "transcript": "transcription complete du texte..."
}

IMPORTANT:
- Transcris TOUT le contenu audio
- Ne resume pas, ne modifie pas
- Reponds uniquement avec le JSON, sans markdown ni texte autour.

[Audio data - %d bytes, MIME: %s]
%s`, len(mediaData), mimeType, base64Media[:min(1000, len(base64Media))]+"...")

	models := []string{
		"gemini-2.0-flash",
		"gemini-1.5-flash",
	}

	var lastErr error
	for _, model := range models {
		result, err := s.genaiClient.Models.GenerateContent(
			ctx,
			model,
			genai.Text(prompt),
			nil,
		)

		if err != nil {
			lastErr = err
			errStr := err.Error()
			if strings.Contains(errStr, "503") || strings.Contains(errStr, "overloaded") {
				time.Sleep(500 * time.Millisecond)
				continue
			}
			fmt.Printf("[JournalAI] STT Model %s error: %v\n", model, err)
			continue
		}

		responseText := result.Text()
		if responseText == "" {
			lastErr = fmt.Errorf("no response from %s", model)
			continue
		}

		content := cleanJSONResponse(responseText)

		var result2 TranscriptResult
		if err := json.Unmarshal([]byte(content), &result2); err != nil {
			lastErr = fmt.Errorf("failed to parse STT response: %w (content: %s)", err, content)
			continue
		}

		return &result2, nil
	}

	return nil, fmt.Errorf("all Gemini models failed for STT: %w", lastErr)
}

// EntryAnalysis contains AI analysis results (generated monthly, not at upload time)
type EntryAnalysis struct {
	Summary   string   `json:"summary"`
	Title     string   `json:"title"`
	Mood      string   `json:"mood"`
	MoodScore int      `json:"mood_score"`
	Tags      []string `json:"tags"`
}

// AnalyzeEntry analyzes a single journal entry from its transcript
// Called during monthly batch processing
func (s *AIService) AnalyzeEntry(transcript string) (*EntryAnalysis, error) {
	if s.genaiClient == nil {
		return nil, fmt.Errorf("Gemini client not initialized")
	}

	ctx := context.Background()

	prompt := fmt.Sprintf(`Tu es un assistant d'analyse de journal personnel. Voici la transcription d'une reflexion quotidienne:

"%s"

Analyse cette reflexion et fournis:
1. Un titre court et accrocheur (max 50 caracteres)
2. Un resume en 3-5 points cles (bullet points)
3. L'humeur generale (great, good, neutral, low, bad)
4. Un score d'humeur de 1 a 10
5. 2-5 tags thematiques parmi: productivite, sante, relations, travail, croissance_personnelle, famille, argent, creativite, sport, apprentissage

Reponds UNIQUEMENT avec un JSON valide:
{
  "title": "Titre court et inspirant",
  "summary": "- Point cle 1\n- Point cle 2\n- Point cle 3",
  "mood": "good",
  "mood_score": 7,
  "tags": ["productivite", "sante"]
}

IMPORTANT: Reponds uniquement avec le JSON, sans markdown ni texte autour.`, transcript)

	models := []string{
		"gemini-2.0-flash",
		"gemini-1.5-flash",
	}

	var lastErr error
	for _, model := range models {
		result, err := s.genaiClient.Models.GenerateContent(
			ctx,
			model,
			genai.Text(prompt),
			nil,
		)

		if err != nil {
			lastErr = err
			continue
		}

		responseText := result.Text()
		if responseText == "" {
			lastErr = fmt.Errorf("no response from %s", model)
			continue
		}

		content := cleanJSONResponse(responseText)

		var analysis EntryAnalysis
		if err := json.Unmarshal([]byte(content), &analysis); err != nil {
			lastErr = fmt.Errorf("failed to parse analysis response: %w", err)
			continue
		}

		return &analysis, nil
	}

	return nil, fmt.Errorf("all Gemini models failed for analysis: %w", lastErr)
}

// AnalyzeJournalEntry transcribes and analyzes an audio/video journal entry (DEPRECATED - use TranscribeAudio + AnalyzeEntry)
func (s *AIService) AnalyzeJournalEntry(mediaData []byte, mediaType, contentType string) (*JournalAnalysis, error) {
	// First transcribe
	transcriptResult, err := s.TranscribeAudio(mediaData, mediaType, contentType)
	if err != nil {
		return nil, err
	}

	// Then analyze
	analysis, err := s.AnalyzeEntry(transcriptResult.Transcript)
	if err != nil {
		// Return at least the transcript
		return &JournalAnalysis{
			Transcript: transcriptResult.Transcript,
		}, nil
	}

	return &JournalAnalysis{
		Transcript: transcriptResult.Transcript,
		Summary:    analysis.Summary,
		Title:      analysis.Title,
		Mood:       analysis.Mood,
		MoodScore:  analysis.MoodScore,
		Tags:       analysis.Tags,
	}, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// GenerateBilan creates a weekly or monthly summary from journal entries
func (s *AIService) GenerateBilan(entries []BilanEntryData, bilanType string) (*BilanData, error) {
	if s.genaiClient == nil {
		return nil, fmt.Errorf("Gemini client not initialized")
	}

	ctx := context.Background()

	// Build entries context
	entriesText := ""
	var totalScore int
	var scoreCount int

	for _, e := range entries {
		entriesText += fmt.Sprintf("\n## %s\n", e.Date)
		if e.Summary != nil {
			entriesText += fmt.Sprintf("Resume: %s\n", *e.Summary)
		}
		if e.Transcript != nil && len(*e.Transcript) > 0 {
			// Include first 500 chars of transcript
			transcript := *e.Transcript
			if len(transcript) > 500 {
				transcript = transcript[:500] + "..."
			}
			entriesText += fmt.Sprintf("Transcription: %s\n", transcript)
		}
		if e.Mood != nil {
			entriesText += fmt.Sprintf("Humeur: %s", *e.Mood)
			if e.MoodScore != nil {
				entriesText += fmt.Sprintf(" (%d/10)", *e.MoodScore)
				totalScore += *e.MoodScore
				scoreCount++
			}
			entriesText += "\n"
		}
	}

	avgScore := float64(0)
	if scoreCount > 0 {
		avgScore = float64(totalScore) / float64(scoreCount)
	}

	periodLabel := "semaine"
	if bilanType == "monthly" {
		periodLabel = "mois"
	}

	prompt := fmt.Sprintf(`Tu es un coach de vie qui analyse les reflexions journalieres d'un utilisateur.

Voici les entrees de journal pour cette %s:
%s

Analyse ces reflexions et genere un bilan %s. Reponds UNIQUEMENT avec un JSON valide:

{
  "summary": "Resume global de la %s en 2-3 phrases. Capture l'essence et le progres.",
  "wins": ["Victoire 1", "Victoire 2", "Victoire 3"],
  "improvements": ["Point d'amelioration 1", "Point d'amelioration 2"],
  "mood_trend": "improving|stable|declining",
  "avg_mood_score": %.1f%s
}

Pour mood_trend:
- "improving" si l'humeur generale s'ameliore
- "stable" si elle reste constante
- "declining" si elle diminue

IMPORTANT: Reponds uniquement avec le JSON, sans markdown ni texte autour.`, periodLabel, entriesText, bilanType, periodLabel, avgScore, getSuggestedGoalsField(bilanType))

	models := []string{
		"gemini-2.0-flash",
		"gemini-1.5-flash",
	}

	var lastErr error
	for _, model := range models {
		result, err := s.genaiClient.Models.GenerateContent(
			ctx,
			model,
			genai.Text(prompt),
			nil,
		)

		if err != nil {
			lastErr = err
			continue
		}

		responseText := result.Text()
		if responseText == "" {
			lastErr = fmt.Errorf("no response from %s", model)
			continue
		}

		content := cleanJSONResponse(responseText)

		var bilanData BilanData
		if err := json.Unmarshal([]byte(content), &bilanData); err != nil {
			lastErr = fmt.Errorf("failed to parse bilan response: %w", err)
			continue
		}

		// Override with calculated avg if present
		if avgScore > 0 {
			bilanData.AvgMoodScore = avgScore
		}

		return &bilanData, nil
	}

	return nil, fmt.Errorf("all Gemini models failed for bilan: %w", lastErr)
}

func getSuggestedGoalsField(bilanType string) string {
	if bilanType == "monthly" {
		return `,
  "suggested_goals": ["Objectif suggere 1 pour le mois prochain", "Objectif suggere 2"]`
	}
	return ""
}

func cleanJSONResponse(content string) string {
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	return strings.TrimSpace(content)
}
