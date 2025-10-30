package approval

import (
	"encoding/json"
	"testing"
	"time"

	"assisted-venue-approval/internal/drafts"
	"assisted-venue-approval/internal/models"
)

func TestAssembleAppliesDraftAndAISuggestions(t *testing.T) {
	venue := models.Venue{
		ID:             42,
		Name:           "Original Name",
		Location:       "123 Main St",
		AdditionalInfo: strPtr("Original description"),
		Lat:            floatPtr(40.0),
		Lng:            floatPtr(-73.0),
		Phone:          strPtr("111-222-3333"),
	}

	user := models.User{ID: 7, Username: "tester"}

	history := &models.ValidationHistory{
		ValidationScore:  95,
		ValidationStatus: "approved",
		ProcessedAt:      time.Now(),
	}
	aiPayload := map[string]any{
		"quality": map[string]any{
			"name":        "AI Suggested Name",
			"description": "AI description",
			"closed_days": "Closed on Mondays",
		},
	}
	bytes, err := json.Marshal(aiPayload)
	if err != nil {
		t.Fatalf("marshal suggestions: %v", err)
	}
	aiJSON := string(bytes)
	history.AIOutputData = &aiJSON

	draft := &drafts.VenueDraft{
		VenueID:  venue.ID,
		EditorID: 99,
		Fields: map[string]drafts.DraftField{
			"name": {
				Value:          "Editor Name",
				OriginalSource: "user",
			},
			"description": {
				Value:          "Editor description",
				OriginalSource: "user",
			},
			"hours_note": {
				Value:          "Editor note",
				OriginalSource: "user",
			},
		},
		UpdatedAt: time.Now(),
	}

	result, err := Assemble(MergeInput{
		Venue:         venue,
		User:          user,
		TrustScore:    0.5,
		LatestHistory: history,
		Draft:         draft,
	})
	if err != nil {
		t.Fatalf("Assemble returned error: %v", err)
	}

	if result == nil {
		t.Fatalf("expected merge result")
	}

	if result.Combined.Name != "Editor Name" {
		t.Fatalf("combined name = %q, expected draft override", result.Combined.Name)
	}

	if result.AISuggestions == nil || result.AISuggestions.DescriptionSuggestion != "AI description" {
		t.Fatalf("AI suggestions missing description")
	}

	if result.ApprovalFields == nil {
		t.Fatalf("expected approval fields")
	}

	if result.ApprovalFields.Name != "Editor Name" {
		t.Fatalf("approval fields name = %q, expected draft override", result.ApprovalFields.Name)
	}

	if result.ApprovalFields.HoursNote != "Editor note" {
		t.Fatalf("hours note = %q, expected editor note", result.ApprovalFields.HoursNote)
	}
}

func TestBuildApprovalDataSkipsUnchangedFields(t *testing.T) {
	venue := models.Venue{
		ID:             10,
		Name:           "Venue",
		Location:       "123 Road",
		AdditionalInfo: strPtr("Desc"),
		Phone:          strPtr("123"),
	}

	result := &MergeResult{
		Combined: models.CombinedInfo{
			Website: "https://example.com",
		},
		ApprovalFields: &models.ApprovalFieldData{
			Name:        "Venue",
			Address:     "123 Road",
			Description: "Desc",
			Phone:       "123",
			VegOnly:     0,
			OpenHours:   []string{"Monday: 24 hours"},
		},
	}

	data := BuildApprovalData(result, &venue, 1, "notes")
	if data == nil {
		t.Fatalf("expected approval data")
	}

	if data.Name != nil {
		t.Fatalf("expected name to remain nil because it matches original")
	}

	if data.OpenHours == nil {
		t.Fatalf("expected formatted hours to be set")
	}

	if data.Replacements == nil || data.Replacements.Replacement.OpenHours == nil {
		t.Fatalf("expected replacements to include open hours")
	}
}

func floatPtr(v float64) *float64 {
	return &v
}
