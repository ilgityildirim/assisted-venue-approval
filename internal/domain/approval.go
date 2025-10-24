package domain

import (
	"encoding/json"
	"fmt"
)

// VenueDataReplacement tracks original vs replaced values for audit purposes
// Only fields that were actually replaced should be non-nil
type VenueDataReplacement struct {
	// Name replacement (AI suggestion)
	OriginalName *string `json:"original_name,omitempty"`
	ReplacedName *string `json:"replaced_name,omitempty"`

	// Address replacement (Google data with user fallback)
	OriginalAddress *string `json:"original_address,omitempty"`
	ReplacedAddress *string `json:"replaced_address,omitempty"`

	// Description replacement (AI suggestion)
	OriginalDescription *string `json:"original_description,omitempty"`
	ReplacedDescription *string `json:"replaced_description,omitempty"`

	// Coordinates replacement (Google data)
	OriginalLat *float64 `json:"original_lat,omitempty"`
	ReplacedLat *float64 `json:"replaced_lat,omitempty"`
	OriginalLng *float64 `json:"original_lng,omitempty"`
	ReplacedLng *float64 `json:"replaced_lng,omitempty"`

	// Phone replacement (Google data with user fallback)
	OriginalPhone *string `json:"original_phone,omitempty"`
	ReplacedPhone *string `json:"replaced_phone,omitempty"`

	// Open hours replacement (Google data formatted to JSON)
	OriginalOpenHours *string `json:"original_openhours,omitempty"`
	ReplacedOpenHours *string `json:"replaced_openhours,omitempty"`

	// Open hours note replacement (AI closed days suggestion)
	OriginalOpenHoursNote *string `json:"original_openhours_note,omitempty"`
	ReplacedOpenHoursNote *string `json:"replaced_openhours_note,omitempty"`
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

	return vdr.OriginalName != nil ||
		vdr.OriginalAddress != nil ||
		vdr.OriginalDescription != nil ||
		vdr.OriginalLat != nil ||
		vdr.OriginalPhone != nil ||
		vdr.OriginalOpenHours != nil ||
		vdr.OriginalOpenHoursNote != nil
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
