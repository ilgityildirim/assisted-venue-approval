package domain

import "time"

// VenueValidationAuditLog represents an audit record for venue approval/rejection
type VenueValidationAuditLog struct {
	ID               int64
	VenueID          int64
	HistoryID        *int64 // nullable - can be NULL
	AdminID          *int   // nullable - NULL for automated validations
	Status           string // "approved" or "rejected"
	Reason           *string
	DataReplacements *string // JSON string tracking original vs replaced venue data
	CreatedAt        time.Time
}

// NewAuditLog creates a new audit log entry
func NewAuditLog(venueID int64, historyID *int64, adminID *int, status string, reason *string) *VenueValidationAuditLog {
	return &VenueValidationAuditLog{
		VenueID:   venueID,
		HistoryID: historyID,
		AdminID:   adminID,
		Status:    status,
		Reason:    reason,
		CreatedAt: time.Now(),
	}
}

// NewAuditLogWithReplacements creates a new audit log entry with data replacement tracking
func NewAuditLogWithReplacements(venueID int64, historyID *int64, adminID *int, status string, reason *string, dataReplacements *string) *VenueValidationAuditLog {
	return &VenueValidationAuditLog{
		VenueID:          venueID,
		HistoryID:        historyID,
		AdminID:          adminID,
		Status:           status,
		Reason:           reason,
		DataReplacements: dataReplacements,
		CreatedAt:        time.Now(),
	}
}
