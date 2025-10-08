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

func (qr *QualityReviewer) ReviewQuality(ctx context.Context, venue models.Venue, user models.User, category string, trustLevel float64) (*models.QualitySuggestions, error) {
	ctx, cancel := context.WithTimeout(ctx, qr.timeout)
	defer cancel()

	// Get combined venue info for quality review
	combinedInfo, err := models.GetCombinedVenueInfo(venue, user, trustLevel)
	if err != nil {
		return nil, fmt.Errorf("failed to get combined venue info: %w", err)
	}

	// Build prompts using combined data
	systemPrompt := qr.buildSystemPrompt()
	userPrompt := qr.buildUserPrompt(combinedInfo, category)

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

func (qr *QualityReviewer) buildUserPrompt(combinedInfo models.CombinedInfo, category string) string {
	// Format hours for display
	hoursStr := "Not available"
	if len(combinedInfo.Hours) > 0 {
		hoursStr = strings.Join(combinedInfo.Hours, "; ")
	}

	if qr.pm != nil {
		data := map[string]any{
			"VenueName":   combinedInfo.Name,
			"Description": combinedInfo.Description,
			"Category":    category,
			"VeganStatus": combinedInfo.VeganStatus,
			"Hours":       hoursStr,
		}
		if out, err := qr.pm.Render("quality_user", data); err == nil {
			return out
		}
	}
	return fmt.Sprintf("Assess quality of:\nName: %s\nDescription: %s\nCategory: %s\nVeganStatus: %s", combinedInfo.Name, combinedInfo.Description, category, combinedInfo.VeganStatus)
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
