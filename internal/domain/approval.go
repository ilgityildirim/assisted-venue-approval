package domain

import (
	"encoding/json"
	"fmt"
	"strings"

	"assisted-venue-approval/internal/models"
)

// VenueFieldData represents venue field values (used for original or replacement data)
type VenueFieldData struct {
	Name          *string  `json:"name,omitempty"`
	Address       *string  `json:"address,omitempty"`
	Description   *string  `json:"description,omitempty"`
	Lat           *float64 `json:"lat,omitempty"`
	Lng           *float64 `json:"lng,omitempty"`
	Phone         *string  `json:"phone,omitempty"`
	Website       *string  `json:"website,omitempty"`
	OpenHours     *string  `json:"openhours,omitempty"`
	OpenHoursNote *string  `json:"openhours_note,omitempty"`
}

// VenueDataReplacement tracks original vs replaced values for audit purposes
// Contains two nested objects: original venue data and replacement data
type VenueDataReplacement struct {
	Original    *VenueFieldData `json:"original,omitempty"`
	Replacement *VenueFieldData `json:"replacement,omitempty"`
}

// ToJSON serializes the replacement data to JSON string for audit log storage
func (vdr *VenueDataReplacement) ToJSON() (string, error) {
	if vdr == nil {
		return "{}", nil
	}

	jsonBytes, err := json.Marshal(vdr)
	if err != nil {
		return "", fmt.Errorf("failed to marshal venue data replacement: %w", err)
	}

	return string(jsonBytes), nil
}

// HasReplacements returns true if any field was replaced
func (vdr *VenueDataReplacement) HasReplacements() bool {
	if vdr == nil {
		return false
	}

	return vdr.Original != nil && vdr.Replacement != nil
}

// ApprovalData contains all data needed to approve a venue with data replacement
type ApprovalData struct {
	VenueID      int64                 // Venue ID to approve
	AdminID      int                   // Admin performing the approval
	Notes        string                // Approval notes
	Replacements *VenueDataReplacement // Data replacements to apply

	// New field values to write to database (after replacement logic)
	Name          *string  // Final venue name
	Address       *string  // Final address (location)
	Description   *string  // Final description (additionalinfo)
	Lat           *float64 // Final latitude
	Lng           *float64 // Final longitude
	Phone         *string  // Final phone number
	Website       *string  // Final website URL
	OpenHours     *string  // Final open hours JSON
	OpenHoursNote *string  // Final hours note (closed days)
}

// NewApprovalData creates approval data with only the fields that need updating
func NewApprovalData(venueID int64, adminID int, notes string) *ApprovalData {
	return &ApprovalData{
		VenueID:      venueID,
		AdminID:      adminID,
		Notes:        notes,
		Replacements: &VenueDataReplacement{},
	}
}

// BuildVenueDataReplacements compares original venue data with approval changes
// and returns a VenueDataReplacement object tracking what changed.
// Returns nil if no fields were modified.
func BuildVenueDataReplacements(venue *models.Venue, approvalData *ApprovalData) *VenueDataReplacement {
	if venue == nil || approvalData == nil {
		return nil
	}

	original := &VenueFieldData{}
	replacement := &VenueFieldData{}
	hasChanges := false

	// Helper to normalize strings for comparison (trim whitespace, treat empty as nil)
	normalizeStr := func(s *string) *string {
		if s == nil {
			return nil
		}
		trimmed := strings.TrimSpace(*s)
		if trimmed == "" {
			return nil
		}
		return &trimmed
	}

	// Helper to check if strings are different (handles nil and empty)
	strDiffers := func(orig, new *string) bool {
		normOrig := normalizeStr(orig)
		normNew := normalizeStr(new)
		if normOrig == nil && normNew == nil {
			return false
		}
		if normOrig == nil || normNew == nil {
			return true
		}
		return *normOrig != *normNew
	}

	// Name: venue.Name (string) vs approvalData.Name (*string)
	if approvalData.Name != nil {
		origName := venue.Name
		if strDiffers(&origName, approvalData.Name) {
			original.Name = &origName
			replacement.Name = approvalData.Name
			hasChanges = true
		}
	}

	// Address: venue.Location (string) vs approvalData.Address (*string)
	if approvalData.Address != nil {
		origAddr := venue.Location
		if strDiffers(&origAddr, approvalData.Address) {
			original.Address = &origAddr
			replacement.Address = approvalData.Address
			hasChanges = true
		}
	}

	// Description: venue.AdditionalInfo (*string) vs approvalData.Description (*string)
	if approvalData.Description != nil && strDiffers(venue.AdditionalInfo, approvalData.Description) {
		original.Description = venue.AdditionalInfo
		replacement.Description = approvalData.Description
		hasChanges = true
	}

	// Lat: venue.Lat (*float64) vs approvalData.Lat (*float64)
	if approvalData.Lat != nil {
		if venue.Lat == nil || *venue.Lat != *approvalData.Lat {
			original.Lat = venue.Lat
			replacement.Lat = approvalData.Lat
			hasChanges = true
		}
	}

	// Lng: venue.Lng (*float64) vs approvalData.Lng (*float64)
	if approvalData.Lng != nil {
		if venue.Lng == nil || *venue.Lng != *approvalData.Lng {
			original.Lng = venue.Lng
			replacement.Lng = approvalData.Lng
			hasChanges = true
		}
	}

	// Phone: venue.Phone (*string) vs approvalData.Phone (*string)
	if approvalData.Phone != nil && strDiffers(venue.Phone, approvalData.Phone) {
		original.Phone = venue.Phone
		replacement.Phone = approvalData.Phone
		hasChanges = true
	}

	// Website: venue.URL (*string) vs approvalData.Website (*string)
	if approvalData.Website != nil && strDiffers(venue.URL, approvalData.Website) {
		original.Website = venue.URL
		replacement.Website = approvalData.Website
		hasChanges = true
	}

	// OpenHours: venue.OpenHours (*string) vs approvalData.OpenHours (*string)
	if approvalData.OpenHours != nil && strDiffers(venue.OpenHours, approvalData.OpenHours) {
		original.OpenHours = venue.OpenHours
		replacement.OpenHours = approvalData.OpenHours
		hasChanges = true
	}

	// OpenHoursNote: venue.OpenHoursNote (*string) vs approvalData.OpenHoursNote (*string)
	if approvalData.OpenHoursNote != nil && strDiffers(venue.OpenHoursNote, approvalData.OpenHoursNote) {
		original.OpenHoursNote = venue.OpenHoursNote
		replacement.OpenHoursNote = approvalData.OpenHoursNote
		hasChanges = true
	}

	// Return nil if no changes detected
	if !hasChanges {
		return nil
	}

	return &VenueDataReplacement{
		Original:    original,
		Replacement: replacement,
	}
}
