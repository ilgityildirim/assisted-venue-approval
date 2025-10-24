package domain

import "time"

// VenueValidationAuditLog represents an audit record for venue approval/rejection
type VenueValidationAuditLog struct {
	ID        int64
	HistoryID int64
	AdminID   *int   // nullable - NULL for automated validations
	Status    string // "approved" or "rejected"
	Reason    *string
	CreatedAt time.Time
}

// NewAuditLog creates a new audit log entry
func NewAuditLog(historyID int64, adminID *int, status string, reason *string) *VenueValidationAuditLog {
	return &VenueValidationAuditLog{
		HistoryID: historyID,
		AdminID:   adminID,
		Status:    status,
		Reason:    reason,
		CreatedAt: time.Now(),
	}
}
