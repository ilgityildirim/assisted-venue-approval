package repository

import (
	"context"

	"assisted-venue-approval/internal/domain"
	"assisted-venue-approval/internal/domain/specs"
	"assisted-venue-approval/internal/models"
	"assisted-venue-approval/pkg/database"
)

// SQLRepository is a thin adapter over pkg/database.DB to satisfy domain repositories.
// It keeps business logic decoupled from the SQL layer.
type SQLRepository struct {
	db *database.DB
}

func NewSQLRepository(db *database.DB) *SQLRepository {
	return &SQLRepository{db: db}
}

// Ensure interface compliance at compile time
var _ domain.Repository = (*SQLRepository)(nil)

// VenueRepository methods
func (r *SQLRepository) GetPendingVenuesWithUserCtx(ctx context.Context) ([]models.VenueWithUser, error) {
	return r.db.GetPendingVenuesWithUserCtx(ctx)
}

func (r *SQLRepository) GetVenuesFilteredCtx(ctx context.Context, status string, search string, limit int, offset int) ([]models.VenueWithUser, int, error) {
	return r.db.GetVenuesFilteredCtx(ctx, status, search, limit, offset)
}

func (r *SQLRepository) GetVenueWithUserByIDCtx(ctx context.Context, venueID int64) (*models.VenueWithUser, error) {
	return r.db.GetVenueWithUserByIDCtx(ctx, venueID)
}

func (r *SQLRepository) GetSimilarVenuesCtx(ctx context.Context, venue models.Venue, limit int) ([]models.Venue, error) {
	return r.db.GetSimilarVenuesCtx(ctx, venue, limit)
}

func (r *SQLRepository) GetManualReviewVenuesCtx(ctx context.Context, search string, minScore int, sort string, limit int, offset int) ([]models.VenueWithUser, []int, int, error) {
	return r.db.GetManualReviewVenuesCtx(ctx, search, minScore, sort, limit, offset)
}

func (r *SQLRepository) GetVenueStatisticsCtx(ctx context.Context) (*models.VenueStats, error) {
	return r.db.GetVenueStatisticsCtx(ctx)
}

func (r *SQLRepository) UpdateVenueStatusCtx(ctx context.Context, venueID int64, active int, notes string, reviewer *string) error {
	return r.db.UpdateVenueStatusCtx(ctx, venueID, active, notes, reviewer)
}

func (r *SQLRepository) UpdateVenueActiveCtx(ctx context.Context, venueID int64, active int) error {
	return r.db.UpdateVenueActiveCtx(ctx, venueID, active)
}

func (r *SQLRepository) CountVenuesByPathCtx(ctx context.Context, path string, excludeVenueID int64) (int, error) {
	return r.db.CountVenuesByPathCtx(ctx, path, excludeVenueID)
}

func (r *SQLRepository) FindDuplicateVenuesByNameAndLocation(ctx context.Context, name string, lat, lng float64, radiusMeters int, excludeVenueID int64) ([]models.Venue, error) {
	return r.db.FindDuplicateVenuesByNameAndLocation(ctx, name, lat, lng, radiusMeters, excludeVenueID)
}

func (r *SQLRepository) ApproveVenueWithDataReplacement(ctx context.Context, approvalData *domain.ApprovalData) error {
	return r.db.ApproveVenueWithDataReplacementCtx(ctx, approvalData)
}

// ValidationRepository methods
func (r *SQLRepository) SaveValidationResultCtx(ctx context.Context, result *models.ValidationResult) error {
	return r.db.SaveValidationResultCtx(ctx, result)
}

func (r *SQLRepository) SaveValidationResultWithGoogleDataCtx(ctx context.Context, result *models.ValidationResult, googleData *models.GooglePlaceData) error {
	return r.db.SaveValidationResultWithGoogleDataCtx(ctx, result, googleData)
}

func (r *SQLRepository) GetRecentValidationResultsCtx(ctx context.Context, limit int) ([]models.ValidationResult, error) {
	return r.db.GetRecentValidationResultsCtx(ctx, limit)
}

func (r *SQLRepository) GetVenueValidationHistoryCtx(ctx context.Context, venueID int64) ([]models.ValidationHistory, error) {
	return r.db.GetVenueValidationHistoryCtx(ctx, venueID)
}

func (r *SQLRepository) GetValidationHistoryPaginatedCtx(ctx context.Context, limit int, offset int) ([]models.ValidationHistory, int, error) {
	return r.db.GetValidationHistoryPaginatedCtx(ctx, limit, offset)
}

func (r *SQLRepository) GetCachedGooglePlaceDataCtx(ctx context.Context, venueID int64) (*models.GooglePlaceData, error) {
	return r.db.GetCachedGooglePlaceDataCtx(ctx, venueID)
}

func (r *SQLRepository) HasAnyValidationHistory(venueID int64) (bool, error) {
	return r.db.HasAnyValidationHistory(venueID)
}

func (r *SQLRepository) ValidateApprovalEligibility(venueID int64, threshold int) error {
	return r.db.ValidateApprovalEligibility(venueID, threshold)
}

// AuditLogRepository methods
func (r *SQLRepository) CreateAuditLogCtx(ctx context.Context, log *domain.VenueValidationAuditLog) error {
	return r.db.CreateAuditLogCtx(ctx, log)
}

func (r *SQLRepository) GetAuditLogsByHistoryIDCtx(ctx context.Context, historyID int64) ([]domain.VenueValidationAuditLog, error) {
	return r.db.GetAuditLogsByHistoryIDCtx(ctx, historyID)
}

func (r *SQLRepository) GetAuditLogsByAdminIDCtx(ctx context.Context, adminID int, limit int, offset int) ([]domain.VenueValidationAuditLog, int, error) {
	return r.db.GetAuditLogsByAdminIDCtx(ctx, adminID, limit, offset)
}

func (r *SQLRepository) GetAuditLogsByVenueIDCtx(ctx context.Context, venueID int64) ([]domain.VenueValidationAuditLog, error) {
	return r.db.GetAuditLogsByVenueIDCtx(ctx, venueID)
}

// FilterPendingBySpecCtx fetches pending venues and filters them using a Specification.
// Note: This applies the spec in-memory. For large datasets, consider adding SQL translations.
func (r *SQLRepository) FilterPendingBySpecCtx(ctx context.Context, s specs.Specification[models.Venue]) ([]models.VenueWithUser, error) {
	items, err := r.GetPendingVenuesWithUserCtx(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]models.VenueWithUser, 0, len(items))
	for _, v := range items {
		if s.IsSatisfiedBy(ctx, v.Venue) {
			out = append(out, v)
		}
	}
	return out, nil
}
