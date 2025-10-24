package domain

import (
	"encoding/json"
	"testing"

	"assisted-venue-approval/internal/models"
)

// Helper to create string pointer
func strPtr(s string) *string {
	return &s
}

// Helper to create float64 pointer
func float64Ptr(f float64) *float64 {
	return &f
}

func TestBuildVenueDataReplacements_AllFieldsChanged(t *testing.T) {
	// Setup original venue
	venue := &models.Venue{
		Name:           "Old Name",
		Location:       "Old Address",
		AdditionalInfo: strPtr("Old Description"),
		Lat:            float64Ptr(40.7128),
		Lng:            float64Ptr(-74.0060),
		Phone:          strPtr("+1234567890"),
		URL:            strPtr("http://old.com"),
		OpenHours:      strPtr(`{"monday":"9-5"}`),
		OpenHoursNote:  strPtr("Closed Sundays"),
	}

	// Setup approval data with all fields different
	approvalData := &ApprovalData{
		Name:          strPtr("New Name"),
		Address:       strPtr("New Address"),
		Description:   strPtr("New Description"),
		Lat:           float64Ptr(40.7589),
		Lng:           float64Ptr(-73.9851),
		Phone:         strPtr("+9876543210"),
		Website:       strPtr("http://new.com"),
		OpenHours:     strPtr(`{"monday":"10-6"}`),
		OpenHoursNote: strPtr("Open Daily"),
	}

	result := BuildVenueDataReplacements(venue, approvalData)

	if result == nil {
		t.Fatal("Expected non-nil result for changed fields")
	}

	if !result.HasReplacements() {
		t.Error("HasReplacements should return true")
	}

	// Verify original values
	if result.Original.Name == nil || *result.Original.Name != "Old Name" {
		t.Errorf("Original name mismatch: got %v, want 'Old Name'", result.Original.Name)
	}
	if result.Original.Address == nil || *result.Original.Address != "Old Address" {
		t.Errorf("Original address mismatch: got %v, want 'Old Address'", result.Original.Address)
	}
	if result.Original.Lat == nil || *result.Original.Lat != 40.7128 {
		t.Errorf("Original lat mismatch: got %v, want 40.7128", result.Original.Lat)
	}

	// Verify replacement values
	if result.Replacement.Name == nil || *result.Replacement.Name != "New Name" {
		t.Errorf("Replacement name mismatch: got %v, want 'New Name'", result.Replacement.Name)
	}
	if result.Replacement.Address == nil || *result.Replacement.Address != "New Address" {
		t.Errorf("Replacement address mismatch: got %v, want 'New Address'", result.Replacement.Address)
	}
	if result.Replacement.Lat == nil || *result.Replacement.Lat != 40.7589 {
		t.Errorf("Replacement lat mismatch: got %v, want 40.7589", result.Replacement.Lat)
	}
}

func TestBuildVenueDataReplacements_NoChanges(t *testing.T) {
	venue := &models.Venue{
		Name:     "Same Name",
		Location: "Same Address",
		Lat:      float64Ptr(40.7128),
		Lng:      float64Ptr(-74.0060),
	}

	// Approval data with same values
	approvalData := &ApprovalData{
		Name:    strPtr("Same Name"),
		Address: strPtr("Same Address"),
		Lat:     float64Ptr(40.7128),
		Lng:     float64Ptr(-74.0060),
	}

	result := BuildVenueDataReplacements(venue, approvalData)

	if result != nil {
		t.Errorf("Expected nil result for no changes, got: %+v", result)
	}
}

func TestBuildVenueDataReplacements_PartialChanges(t *testing.T) {
	venue := &models.Venue{
		Name:     "Old Name",
		Location: "Same Address",
		Phone:    strPtr("+1234567890"),
		Lat:      float64Ptr(40.7128),
	}

	// Only change name and lat
	approvalData := &ApprovalData{
		Name:    strPtr("New Name"),
		Address: strPtr("Same Address"), // No change
		Lat:     float64Ptr(40.7589),    // Changed
	}

	result := BuildVenueDataReplacements(venue, approvalData)

	if result == nil {
		t.Fatal("Expected non-nil result for partial changes")
	}

	// Name should be tracked (changed)
	if result.Original.Name == nil || *result.Original.Name != "Old Name" {
		t.Error("Expected original name to be tracked")
	}
	if result.Replacement.Name == nil || *result.Replacement.Name != "New Name" {
		t.Error("Expected replacement name to be tracked")
	}

	// Address should NOT be tracked (unchanged)
	if result.Original.Address != nil {
		t.Error("Expected original address to be nil (unchanged)")
	}
	if result.Replacement.Address != nil {
		t.Error("Expected replacement address to be nil (unchanged)")
	}

	// Lat should be tracked (changed)
	if result.Original.Lat == nil || *result.Original.Lat != 40.7128 {
		t.Error("Expected original lat to be tracked")
	}
	if result.Replacement.Lat == nil || *result.Replacement.Lat != 40.7589 {
		t.Error("Expected replacement lat to be tracked")
	}

	// Phone should NOT be tracked (not in approval data)
	if result.Original.Phone != nil {
		t.Error("Expected original phone to be nil (not changed)")
	}
}

func TestBuildVenueDataReplacements_NilAndEmptyHandling(t *testing.T) {
	venue := &models.Venue{
		Name:           "Name",
		Location:       "",         // Empty string
		AdditionalInfo: strPtr(""), // Empty pointer string
		Phone:          nil,
	}

	approvalData := &ApprovalData{
		Name:        strPtr("Name"),     // Same
		Address:     strPtr("  "),       // Whitespace only (treated as empty)
		Description: strPtr("New Desc"), // Replacing empty with value
		Phone:       strPtr("+123"),     // Replacing nil with value
	}

	result := BuildVenueDataReplacements(venue, approvalData)

	if result == nil {
		t.Fatal("Expected non-nil result for nil->value changes")
	}

	// Name should not be tracked (same)
	if result.Original.Name != nil {
		t.Error("Expected name not tracked (unchanged)")
	}

	// Address should not be tracked (both empty after normalization)
	if result.Original.Address != nil {
		t.Error("Expected address not tracked (both empty)")
	}

	// Description should be tracked (empty -> value)
	if result.Original.Description != nil && *result.Original.Description != "" {
		t.Error("Expected original description to be empty or nil")
	}
	if result.Replacement.Description == nil || *result.Replacement.Description != "New Desc" {
		t.Error("Expected replacement description to be tracked")
	}

	// Phone should be tracked (nil -> value)
	if result.Original.Phone != nil {
		t.Error("Expected original phone to be nil")
	}
	if result.Replacement.Phone == nil || *result.Replacement.Phone != "+123" {
		t.Error("Expected replacement phone to be tracked")
	}
}

func TestBuildVenueDataReplacements_NilInputs(t *testing.T) {
	result := BuildVenueDataReplacements(nil, &ApprovalData{})
	if result != nil {
		t.Error("Expected nil result for nil venue")
	}

	result = BuildVenueDataReplacements(&models.Venue{}, nil)
	if result != nil {
		t.Error("Expected nil result for nil approval data")
	}
}

func TestVenueDataReplacement_ToJSON(t *testing.T) {
	replacement := &VenueDataReplacement{
		Original: &VenueFieldData{
			Name:  strPtr("Old Name"),
			Lat:   float64Ptr(40.7128),
			Phone: strPtr("+123"),
		},
		Replacement: &VenueFieldData{
			Name:  strPtr("New Name"),
			Lat:   float64Ptr(40.7589),
			Phone: strPtr("+456"),
		},
	}

	jsonStr, err := replacement.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	// Parse JSON to verify structure
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	// Verify structure has "original" and "replacement" keys
	original, ok := parsed["original"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected 'original' key in JSON")
	}

	replacement_data, ok := parsed["replacement"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected 'replacement' key in JSON")
	}

	// Verify nested values
	if original["name"] != "Old Name" {
		t.Errorf("Expected original name 'Old Name', got: %v", original["name"])
	}
	if replacement_data["name"] != "New Name" {
		t.Errorf("Expected replacement name 'New Name', got: %v", replacement_data["name"])
	}
	if original["lat"] != 40.7128 {
		t.Errorf("Expected original lat 40.7128, got: %v", original["lat"])
	}
}

func TestVenueDataReplacement_ToJSON_Nil(t *testing.T) {
	var replacement *VenueDataReplacement
	jsonStr, err := replacement.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed for nil: %v", err)
	}
	if jsonStr != "{}" {
		t.Errorf("Expected '{}' for nil replacement, got: %s", jsonStr)
	}
}

func TestVenueDataReplacement_HasReplacements(t *testing.T) {
	// Nil replacement
	var nilRep *VenueDataReplacement
	if nilRep.HasReplacements() {
		t.Error("Nil replacement should not have replacements")
	}

	// Empty replacement
	emptyRep := &VenueDataReplacement{}
	if emptyRep.HasReplacements() {
		t.Error("Empty replacement should not have replacements")
	}

	// With only original
	onlyOrig := &VenueDataReplacement{
		Original: &VenueFieldData{Name: strPtr("test")},
	}
	if onlyOrig.HasReplacements() {
		t.Error("Replacement with only original should not have replacements")
	}

	// With only replacement
	onlyRepl := &VenueDataReplacement{
		Replacement: &VenueFieldData{Name: strPtr("test")},
	}
	if onlyRepl.HasReplacements() {
		t.Error("Replacement with only replacement should not have replacements")
	}

	// With both
	withBoth := &VenueDataReplacement{
		Original:    &VenueFieldData{Name: strPtr("old")},
		Replacement: &VenueFieldData{Name: strPtr("new")},
	}
	if !withBoth.HasReplacements() {
		t.Error("Replacement with both should have replacements")
	}
}

func TestBuildVenueDataReplacements_WhitespaceHandling(t *testing.T) {
	venue := &models.Venue{
		Name:     "  Old Name  ",
		Location: "Old Address",
	}

	// Approval data with trimmed values (should not be considered different)
	approvalData := &ApprovalData{
		Name:    strPtr("Old Name"), // Trimmed, should match
		Address: strPtr("New Addr"), // Actually different
	}

	result := BuildVenueDataReplacements(venue, approvalData)

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	// Name should NOT be tracked (same after trimming)
	if result.Original.Name != nil {
		t.Error("Expected name not tracked (same after trimming)")
	}

	// Address should be tracked (different)
	if result.Original.Address == nil || *result.Original.Address != "Old Address" {
		t.Error("Expected original address to be tracked")
	}
	if result.Replacement.Address == nil || *result.Replacement.Address != "New Addr" {
		t.Error("Expected replacement address to be tracked")
	}
}
