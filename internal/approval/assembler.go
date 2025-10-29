package approval

import (
	"encoding/json"
	"fmt"
	"strings"

	"assisted-venue-approval/internal/domain"
	"assisted-venue-approval/internal/drafts"
	"assisted-venue-approval/internal/models"
)

// MergeInput captures the data sources that contribute to a venue's final state.
type MergeInput struct {
	Venue         models.Venue
	User          models.User
	TrustScore    float64
	GoogleData    *models.GooglePlaceData
	LatestHistory *models.ValidationHistory
	Draft         *drafts.VenueDraft
}

// MergeResult returns the merged view used by both the UI and persistence layers.
type MergeResult struct {
	Combined       models.CombinedInfo
	ApprovalFields *models.ApprovalFieldData
	AISuggestions  *models.AISuggestions
	DraftApplied   bool
}

// Assemble merges venue, Google, AI, and editor inputs into a cohesive representation.
// It returns the rendered CombinedInfo for display plus the ApprovalFieldData that should
// be transformed into domain.ApprovalData before persisting.
func Assemble(input MergeInput) (*MergeResult, error) {
	venue := input.Venue

	googleData := input.GoogleData
	if googleData == nil && input.LatestHistory != nil && input.LatestHistory.GooglePlaceData != nil {
		googleData = input.LatestHistory.GooglePlaceData
	}
	if googleData != nil {
		venue.GoogleData = googleData
	}

	draftMap := convertDraftForMerge(input.Draft)

	combined, err := models.GetCombinedVenueInfo(venue, input.User, input.TrustScore)
	if err != nil {
		return nil, fmt.Errorf("build combined venue info: %w", err)
	}
	if draftMap != nil {
		models.ApplyEditorDrafts(&combined, draftMap)
	}

	aiSuggestions := parseAISuggestions(input.LatestHistory)

	approvalFields, err := models.GetApprovalFieldData(venue, input.User, input.TrustScore, aiSuggestions, draftMap)
	if err != nil {
		return nil, fmt.Errorf("build approval field data: %w", err)
	}

	return &MergeResult{
		Combined:       combined,
		ApprovalFields: approvalFields,
		AISuggestions:  aiSuggestions,
		DraftApplied:   draftMap != nil && len(draftMap) > 0,
	}, nil
}

// BuildApprovalData converts MergeResult output into domain.ApprovalData suitable for persistence.
// Notes and adminID must be provided by the caller because they depend on request context.
func BuildApprovalData(result *MergeResult, venue *models.Venue, adminID int, notes string) *domain.ApprovalData {
	if result == nil || venue == nil {
		return nil
	}

	data := domain.NewApprovalData(venue.ID, adminID, notes)
	fields := result.ApprovalFields

	if fields != nil {
		if name := strings.TrimSpace(fields.Name); name != "" && strings.TrimSpace(venue.Name) != name {
			data.Name = strPtr(name)
		}
		if address := strings.TrimSpace(fields.Address); address != "" && strings.TrimSpace(venue.Location) != address {
			data.Address = strPtr(address)
		}
		if desc := strings.TrimSpace(fields.Description); desc != "" && differsPtr(venue.AdditionalInfo, desc) {
			data.Description = strPtr(desc)
		}
		if fields.Lat != nil && differsFloat64(venue.Lat, *fields.Lat) {
			data.Lat = fields.Lat
		}
		if fields.Lng != nil && differsFloat64(venue.Lng, *fields.Lng) {
			data.Lng = fields.Lng
		}
		if phone := strings.TrimSpace(fields.Phone); phone != "" && differsPtr(venue.Phone, phone) {
			data.Phone = strPtr(phone)
		}
		if website := strings.TrimSpace(result.Combined.Website); website != "" && differsPtr(venue.URL, website) {
			data.Website = strPtr(website)
		}

		if formattedHours, err := FormatOpenHoursFromCombined(fields.OpenHours); err == nil && formattedHours != "" {
			data.OpenHours = &formattedHours
		}
		if note := strings.TrimSpace(fields.HoursNote); note != "" && differsPtr(venue.OpenHoursNote, note) {
			data.OpenHoursNote = strPtr(note)
		}
	}

	// Extract classification fields from Combined Info
	if venueType := strings.TrimSpace(result.Combined.VenueType); venueType != "" {
		entryType := venueTypeToInt(venueType)
		if entryType != venue.EntryType {
			data.EntryType = &entryType
		}
	}

	if path := strings.TrimSpace(result.Combined.Path); path != "" && differsPtr(venue.Path, path) {
		data.Path = strPtr(path)
	}

	if veganStatus := strings.TrimSpace(result.Combined.VeganStatus); veganStatus != "" {
		vegan, vegonly := veganStatusToFlags(veganStatus)
		if vegan != venue.Vegan {
			data.Vegan = &vegan
		}
		if vegonly != venue.VegOnly {
			data.VegOnly = &vegonly
		}
	}

	if category := strings.TrimSpace(result.Combined.Category); category != "" {
		categoryID := categoryToInt(category)
		if categoryID != venue.Category {
			data.Category = &categoryID
		}
	}

	data.Replacements = domain.BuildVenueDataReplacements(venue, data)
	return data
}

func convertDraftForMerge(draft *drafts.VenueDraft) map[string]interface{} {
	if draft == nil || len(draft.Fields) == 0 {
		return nil
	}

	out := make(map[string]interface{}, len(draft.Fields))
	for field, value := range draft.Fields {
		out[field] = map[string]interface{}{
			"value":           value.Value,
			"original_source": value.OriginalSource,
		}
	}
	return out
}

func parseAISuggestions(history *models.ValidationHistory) *models.AISuggestions {
	if history == nil || history.AIOutputData == nil {
		return &models.AISuggestions{}
	}

	rawJSON := strings.TrimSpace(*history.AIOutputData)
	if rawJSON == "" {
		return &models.AISuggestions{}
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(rawJSON), &payload); err != nil {
		return &models.AISuggestions{}
	}

	suggestions := &models.AISuggestions{}
	quality, ok := payload["quality"].(map[string]interface{})
	if !ok {
		return suggestions
	}

	if name, ok := quality["name"].(string); ok {
		suggestions.NameSuggestion = strings.TrimSpace(name)
	}
	if desc, ok := quality["description"].(string); ok {
		suggestions.DescriptionSuggestion = strings.TrimSpace(desc)
	}
	if closed, ok := quality["closed_days"].(string); ok {
		suggestions.ClosedDays = strings.TrimSpace(closed)
	}

	return suggestions
}

func strPtr(v string) *string {
	value := v
	return &value
}

func differsPtr(original *string, candidate string) bool {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return false
	}
	if original == nil {
		return true
	}
	return strings.TrimSpace(*original) != candidate
}

func differsFloat64(original *float64, candidate float64) bool {
	if original == nil {
		return true
	}
	return *original != candidate
}

// venueTypeToInt converts venue type string to database integer
func venueTypeToInt(venueType string) int {
	switch strings.ToLower(strings.TrimSpace(venueType)) {
	case "restaurant":
		return 1
	case "store":
		return 2
	default:
		return 1 // default to restaurant
	}
}

// veganStatusToFlags converts vegan status string to vegan and vegonly flags
// Returns (vegan, vegonly) as two separate integers
func veganStatusToFlags(status string) (int, int) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "vegan":
		return 1, 1 // vegan=1, vegonly=1
	case "vegetarian":
		return 0, 1 // vegan=0, vegonly=1
	case "veg-options":
		return 0, 0 // vegan=0, vegonly=0
	default:
		return 0, 0 // default to veg-options
	}
}

// categoryToInt converts category label to database ID
func categoryToInt(categoryLabel string) int {
	// Category mapping from combined.go
	categories := map[string]int{
		"Health Store":    1,
		"Veg Store":       2,
		"Bakery":          3,
		"B&B":             4,
		"Delivery":        5,
		"Catering":        6,
		"Organization":    7,
		"Farmer's Market": 8,
		"Food Truck":      10,
		"Market Vendor":   11,
		"Ice Cream":       12,
		"Juice Bar":       13,
		"Professional":    14,
		"Coffee & Tea":    15,
		"Spa":             16,
		"Other":           99,
	}

	if id, ok := categories[categoryLabel]; ok {
		return id
	}
	return 0 // default to 0 (Generic Restaurant)
}
