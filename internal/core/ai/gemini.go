package ai

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"github.com/ledaian41/verix-stock/internal/modules/article"
	"google.golang.org/api/option"
)

type Synthesizer struct {
	client *genai.Client
	model  *genai.GenerativeModel
}

func NewSynthesizer(ctx context.Context) (*Synthesizer, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY not set")
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, err
	}

	model := client.GenerativeModel("gemini-1.5-flash")
	// Set temperature low for consistent summarization
	model.SetTemperature(0.2)

	return &Synthesizer{
		client: client,
		model:  model,
	}, nil
}

func (s *Synthesizer) Close() {
	s.client.Close()
}

type SynthesisResult struct {
	Summary        string
	SentimentScore float64
}

func (s *Synthesizer) Synthesize(ctx context.Context, ticker string, drafts []article.DraftArticle) (*SynthesisResult, error) {
	if len(drafts) == 0 {
		return nil, fmt.Errorf("no articles to synthesize")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Analyze and synthesize the following news articles for stock ticker %s into a concise summary.\n\n", ticker))
	sb.WriteString("Requirements:\n")
	sb.WriteString("1. Provide a 3-5 bullet point summary of the key market-moving information.\n")
	sb.WriteString("2. Provide a sentiment score between -1.0 (Very Negative) and 1.0 (Very Positive).\n")
	sb.WriteString("3. Response format: [SUMMARY]\n<bulltets>\n[SENTIMENT]\n<score>\n\n")

	for i, d := range drafts {
		sb.WriteString(fmt.Sprintf("Article %d: %s\n", i+1, d.Title))
		sb.WriteString(fmt.Sprintf("Content: %s\n\n", d.FullContent))
	}

	resp, err := s.model.GenerateContent(ctx, genai.Text(sb.String()))
	if err != nil {
		return nil, err
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("empty AI response")
	}

	fullText := ""
	for _, part := range resp.Candidates[0].Content.Parts {
		if t, ok := part.(genai.Text); ok {
			fullText += string(t)
		}
	}

	return parseAIResponse(fullText), nil
}

func parseAIResponse(text string) *SynthesisResult {
	result := &SynthesisResult{
		Summary:        "Summary extraction failed",
		SentimentScore: 0.0,
	}

	parts := strings.Split(text, "[SENTIMENT]")
	if len(parts) >= 2 {
		scoreStr := strings.TrimSpace(parts[1])
		fmt.Sscanf(scoreStr, "%f", &result.SentimentScore)
		
		summaryPart := strings.TrimPrefix(parts[0], "[SUMMARY]")
		result.Summary = strings.TrimSpace(summaryPart)
	} else {
		// Fallback if formatting is weird
		result.Summary = text
	}

	return result
}
