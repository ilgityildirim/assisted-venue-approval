package domain

import (
	"context"

	"assisted-venue-approval/internal/models"
)

// VenueRepository defines data access for venues and related views.
type VenueRepository interface {
	GetPendingVenuesWithUserCtx(ctx context.Context) ([]models.VenueWithUser, error)
	GetVenuesFilteredCtx(ctx context.Context, status string, search string, limit int, offset int) ([]models.VenueWithUser, int, error)
	GetVenueWithUserByIDCtx(ctx context.Context, venueID int64) (*models.VenueWithUser, error)
	GetSimilarVenuesCtx(ctx context.Context, venue models.Venue, limit int) ([]models.Venue, error)
	GetManualReviewVenuesCtx(ctx context.Context, search string, minScore int, trustedOnly bool, sort string, limit int, offset int) ([]models.VenueWithUser, []int, int, error)
	GetVenueStatisticsCtx(ctx context.Context) (*models.VenueStats, error)
	CountVenuesByPathCtx(ctx context.Context, path string, excludeVenueID int64) (int, error)
	FindDuplicateVenuesByNameAndLocation(ctx context.Context, name string, lat, lng float64, radiusMeters int, excludeVenueID int64) ([]models.Venue, error)

	UpdateVenueStatusCtx(ctx context.Context, venueID int64, active int, notes string, reviewer *string) error
	UpdateVenueActiveCtx(ctx context.Context, venueID int64, active int) error
	ApproveVenueWithDataReplacement(ctx context.Context, approvalData *ApprovalData) error
}

// ValidationRepository defines access for validation history and caches.
type ValidationRepository interface {
	SaveValidationResultCtx(ctx context.Context, result *models.ValidationResult) error
	SaveValidationResultWithGoogleDataCtx(ctx context.Context, result *models.ValidationResult, googleData *models.GooglePlaceData) error
	GetRecentValidationResultsCtx(ctx context.Context, limit int) ([]models.ValidationResult, error)
	GetVenueValidationHistoryCtx(ctx context.Context, venueID int64) ([]models.ValidationHistory, error)
	GetValidationHistoryPaginatedCtx(ctx context.Context, limit int, offset int) ([]models.ValidationHistory, int, error)
	GetCachedGooglePlaceDataCtx(ctx context.Context, venueID int64) (*models.GooglePlaceData, error)
	HasAnyValidationHistory(venueID int64) (bool, error)
	ValidateApprovalEligibility(venueID int64, threshold int) error
}

// UserRepository defines user-related data access. Not yet used by services here.
// TODO: Implement concrete queries when user operations are added.
type UserRepository interface {
	FindByID(ctx context.Context, id uint) (models.User, error)
	Save(ctx context.Context, u *models.User) error
	Delete(ctx context.Context, id uint) error
}

// Repository aggregates the repos commonly required by services.
type FeedbackRepository interface {
	CreateFeedbackCtx(ctx context.Context, f *models.EditorFeedback) error
	GetFeedbackByVenueCtx(ctx context.Context, venueID int64, limit int) ([]models.EditorFeedback, int, int, error)
	GetFeedbackStatsCtx(ctx context.Context, promptVersion *string) (*models.FeedbackStats, error)
}

// AuditLogRepository defines audit log data access for venue validations.
type AuditLogRepository interface {
	CreateAuditLogCtx(ctx context.Context, log *VenueValidationAuditLog) error
	GetAuditLogsByHistoryIDCtx(ctx context.Context, historyID int64) ([]VenueValidationAuditLog, error)
	GetAuditLogsByAdminIDCtx(ctx context.Context, adminID int, limit int, offset int) ([]VenueValidationAuditLog, int, error)
	GetAuditLogsByVenueIDCtx(ctx context.Context, venueID int64) ([]VenueValidationAuditLog, error)
}

// Repository aggregates the repos commonly required by services.
type Repository interface {
	VenueRepository
	ValidationRepository
	FeedbackRepository
	AuditLogRepository
}
