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
	// No suggested path available in quality review context
	combinedInfo, err := models.GetCombinedVenueInfo(venue, user, trustLevel, "")
	if err != nil {
		return nil, fmt.Errorf("failed to get combined venue info: %w", err)
	}

	// Build prompts using combined data
	systemPrompt := qr.buildSystemPrompt()
	userPrompt := qr.buildUserPrompt(combinedInfo, category)

	// Call OpenAI API (use gpt-4o-mini for better instruction following)
	resp, err := qr.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: openai.GPT4oMini,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: userPrompt},
		},
		Temperature:    0.0,
		MaxTokens:      500,
		ResponseFormat: &openai.ChatCompletionResponseFormat{Type: openai.ChatCompletionResponseFormatTypeJSONObject},
	})

	if err != nil {
		return nil, err
	}

	// Parse response
	qs, err := qr.parseResponse(resp.Choices[0].Message.Content)
	if err != nil {
		return nil, err
	}

	qs.Description = applyServesMeatGuard(
		qs.Description,
		category,
		combinedInfo.Category,
		combinedInfo.VenueType,
		combinedInfo.VeganStatus,
	)

	return qs, nil
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
			"VenuePath":   combinedInfo.Path,
			"Lat":         combinedInfo.Lat,
			"Lng":         combinedInfo.Lng,
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

func applyServesMeatGuard(desc, primaryCategory, secondaryCategory, venueType, veganStatus string) string {
	trimmedLeading := strings.TrimLeft(desc, " \t\n\r")

	if !strings.HasPrefix(trimmedLeading, "Serves meat") {
		return desc
	}

	if servesMeatPrefixAllowed(primaryCategory, secondaryCategory, venueType, veganStatus) {
		return desc
	}

	periodIdx := strings.Index(trimmedLeading, ".")
	if periodIdx == -1 {
		// Cannot safely remove prefix without a full sentence delimiter
		return desc
	}

	remainder := strings.TrimSpace(trimmedLeading[periodIdx+1:])
	leading := desc[:len(desc)-len(trimmedLeading)]

	if remainder == "" {
		return strings.TrimSpace(leading)
	}
	if leading == "" {
		return remainder
	}
	return leading + remainder
}

func servesMeatPrefixAllowed(primaryCategory, secondaryCategory, venueType, veganStatus string) bool {
	vs := strings.ToLower(strings.TrimSpace(veganStatus))
	if vs == "vegan" || vs == "vegetarian" {
		return false
	}

	allowed := map[string]struct{}{
		"restaurant": {},
		"food truck": {},
	}

	normalizedPrimary := normalizeCategory(primaryCategory)
	if normalizedPrimary != "" {
		_, ok := allowed[normalizedPrimary]
		return ok
	}

	normalizedSecondary := normalizeCategory(secondaryCategory)
	if normalizedSecondary != "" {
		_, ok := allowed[normalizedSecondary]
		return ok
	}

	normalizedVenueType := normalizeCategory(venueType)
	_, ok := allowed[normalizedVenueType]
	return ok
}

func normalizeCategory(input string) string {
	lowered := strings.ToLower(strings.TrimSpace(input))
	if lowered == "" {
		return ""
	}
	if idx := strings.Index(lowered, "("); idx != -1 {
		lowered = strings.TrimSpace(lowered[:idx])
	}
	replacer := strings.NewReplacer("/", " ", "-", " ", "&", "and")
	lowered = replacer.Replace(lowered)
	fields := strings.Fields(lowered)
	return strings.Join(fields, " ")
}

const fallbackQualitySystemPrompt = `You are a content editor for HappyCow venue listings.
Rewrite description to follow ALL guidelines. Fix name format if needed.
Output JSON: {"description": "...", "name": "..."}`
