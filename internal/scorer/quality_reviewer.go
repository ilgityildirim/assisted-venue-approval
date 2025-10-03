package scorer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"

	"assisted-venue-approval/internal/models"
	"assisted-venue-approval/internal/prompts"
)

type QualityReviewer struct {
	client                *openai.Client
	pm                    *prompts.Manager
	descriptionGuidelines string
	nameGuidelines        string
	timeout               time.Duration
}

func NewQualityReviewer(apiKey string, pm *prompts.Manager, timeout time.Duration) *QualityReviewer {
	// Load guidelines
	descGuides, nameGuides, err := prompts.LoadGuidelines(
		"config/description_guidelines.md",
		"config/name_translation_guidelines.md",
	)
	if err != nil {
		fmt.Printf("quality: failed to load guidelines: %v\n", err)
	}

	return &QualityReviewer{
		client:                openai.NewClient(apiKey),
		pm:                    pm,
		descriptionGuidelines: descGuides,
		nameGuidelines:        nameGuides,
		timeout:               timeout,
	}
}

func (qr *QualityReviewer) ReviewQuality(ctx context.Context, venue models.Venue, category string) (*models.QualitySuggestions, error) {
	ctx, cancel := context.WithTimeout(ctx, qr.timeout)
	defer cancel()

	// Build prompts
	systemPrompt := qr.buildSystemPrompt()
	userPrompt := qr.buildUserPrompt(venue, category)

	// Call OpenAI API (use gpt-3.5-turbo for cost efficiency)
	resp, err := qr.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: openai.GPT3Dot5Turbo,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: userPrompt},
		},
		Temperature:    0.3,
		ResponseFormat: &openai.ChatCompletionResponseFormat{Type: openai.ChatCompletionResponseFormatTypeJSONObject},
	})

	if err != nil {
		return nil, err
	}

	// Parse response
	return qr.parseResponse(resp.Choices[0].Message.Content)
}

func (qr *QualityReviewer) buildSystemPrompt() string {
	if qr.pm != nil {
		data := map[string]any{
			"DescriptionGuidelines": qr.descriptionGuidelines,
			"NameGuidelines":        qr.nameGuidelines,
		}
		if out, err := qr.pm.Render("quality_system", data); err == nil {
			return out
		}
	}
	return fallbackQualitySystemPrompt
}

func (qr *QualityReviewer) buildUserPrompt(venue models.Venue, category string) string {
	description := ""
	if venue.AdditionalInfo != nil {
		description = *venue.AdditionalInfo
	}

	if qr.pm != nil {
		data := map[string]any{
			"VenueName":   venue.Name,
			"Description": description,
			"Category":    category,
		}
		if out, err := qr.pm.Render("quality_user", data); err == nil {
			return out
		}
	}
	return fmt.Sprintf("Assess quality of:\nName: %s\nDescription: %s\nCategory: %s", venue.Name, description, category)
}

func (qr *QualityReviewer) parseResponse(response string) (*models.QualitySuggestions, error) {
	// Clean response (remove markdown code blocks if present)
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	var qs models.QualitySuggestions
	if err := json.Unmarshal([]byte(response), &qs); err != nil {
		return nil, fmt.Errorf("failed to parse quality suggestions response: %w", err)
	}
	return &qs, nil
}

const fallbackQualitySystemPrompt = `You are a content editor for HappyCow venue listings.
Rewrite description to follow ALL guidelines. Fix name format if needed.
Output JSON: {"description": "...", "name": "..."}`
