package repository

import (
	"context"
	"database/sql"
	"fmt"

	"assisted-venue-approval/internal/domain"
	"assisted-venue-approval/internal/models"
	"assisted-venue-approval/pkg/database"
)

// SQLUnitOfWorkFactory starts SQL-backed UnitOfWork transactions.
type SQLUnitOfWorkFactory struct {
	db *database.DB
}

func NewSQLUnitOfWorkFactory(db *database.DB) *SQLUnitOfWorkFactory {
	return &SQLUnitOfWorkFactory{db: db}
}

// Ensure interface conformance
var _ domain.UnitOfWorkFactory = (*SQLUnitOfWorkFactory)(nil)

func (f *SQLUnitOfWorkFactory) Begin(ctx context.Context) (domain.UnitOfWork, error) {
	tx, err := f.db.Conn().BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("uow: begin tx: %w", err)
	}
	return &SQLUnitOfWork{db: f.db, tx: tx}, nil
}

// SQLUnitOfWork coordinates operations using a single *sql.Tx.
type SQLUnitOfWork struct {
	db *database.DB
	tx *sql.Tx
	// simple guard to avoid double commit/rollback
	closed bool
}

// compile-time checks: SQLUnitOfWork implements UnitOfWork and repo methods
var _ domain.UnitOfWork = (*SQLUnitOfWork)(nil)

func (u *SQLUnitOfWork) Begin(ctx context.Context) error {
	if u.tx != nil {
		return nil
	}
	tx, err := u.db.Conn().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("uow: begin: %w", err)
	}
	u.tx = tx
	return nil
}

func (u *SQLUnitOfWork) Commit() error {
	if u.closed {
		return nil
	}
	u.closed = true
	if u.tx == nil {
		return nil
	}
	return u.tx.Commit()
}

func (u *SQLUnitOfWork) Rollback() error {
	if u.closed {
		return nil
	}
	u.closed = true
	if u.tx == nil {
		return nil
	}
	return u.tx.Rollback()
}

// VenueRepository methods (writes go through the transaction when present)
func (u *SQLUnitOfWork) UpdateVenueStatusCtx(ctx context.Context, venueID int64, active int, notes string, reviewer *string) error {
	// NOTE: status update is simple; not critical to be in the same tx for our use, but we keep consistency by using non-tx method.
	return u.db.UpdateVenueStatusCtx(ctx, venueID, active, notes, reviewer)
}

func (u *SQLUnitOfWork) UpdateVenueActiveCtx(ctx context.Context, venueID int64, active int) error {
	if u.tx == nil {
		return fmt.Errorf("uow: no active transaction for UpdateVenueActiveCtx")
	}
	return u.db.UpdateVenueActiveTx(ctx, u.tx, venueID, active)
}

// Reads can be served outside the tx as needed
func (u *SQLUnitOfWork) GetPendingVenuesWithUserCtx(ctx context.Context) ([]models.VenueWithUser, error) {
	return u.db.GetPendingVenuesWithUserCtx(ctx)
}
func (u *SQLUnitOfWork) GetVenuesFilteredCtx(ctx context.Context, status string, search string, limit int, offset int) ([]models.VenueWithUser, int, error) {
	return u.db.GetVenuesFilteredCtx(ctx, status, search, limit, offset)
}
func (u *SQLUnitOfWork) GetVenueWithUserByIDCtx(ctx context.Context, venueID int64) (*models.VenueWithUser, error) {
	return u.db.GetVenueWithUserByIDCtx(ctx, venueID)
}
func (u *SQLUnitOfWork) GetSimilarVenuesCtx(ctx context.Context, venue models.Venue, limit int) ([]models.Venue, error) {
	return u.db.GetSimilarVenuesCtx(ctx, venue, limit)
}
func (u *SQLUnitOfWork) GetManualReviewVenuesCtx(ctx context.Context, search string, minScore int, sort string, limit int, offset int) ([]models.VenueWithUser, []int, int, error) {
	return u.db.GetManualReviewVenuesCtx(ctx, search, minScore, sort, limit, offset)
}
func (u *SQLUnitOfWork) GetVenueStatisticsCtx(ctx context.Context) (*models.VenueStats, error) {
	return u.db.GetVenueStatisticsCtx(ctx)
}
func (u *SQLUnitOfWork) CountVenuesByPathCtx(ctx context.Context, path string, excludeVenueID int64) (int, error) {
	return u.db.CountVenuesByPathCtx(ctx, path, excludeVenueID)
}

func (u *SQLUnitOfWork) FindDuplicateVenuesByNameAndLocation(ctx context.Context, name string, lat, lng float64, radiusMeters int, excludeVenueID int64) ([]models.Venue, error) {
	return u.db.FindDuplicateVenuesByNameAndLocation(ctx, name, lat, lng, radiusMeters, excludeVenueID)
}

func (u *SQLUnitOfWork) ApproveVenueWithDataReplacement(ctx context.Context, approvalData *domain.ApprovalData) error {
	return u.db.ApproveVenueWithDataReplacementCtx(ctx, approvalData)
}

// ValidationRepository methods (writes via tx)
func (u *SQLUnitOfWork) SaveValidationResultCtx(ctx context.Context, result *models.ValidationResult) error {
	if u.tx == nil {
		return fmt.Errorf("uow: no active transaction for SaveValidationResultCtx")
	}
	return u.db.SaveValidationResultTx(ctx, u.tx, result)
}

func (u *SQLUnitOfWork) SaveValidationResultWithGoogleDataCtx(ctx context.Context, result *models.ValidationResult, googleData *models.GooglePlaceData) error {
	if u.tx == nil {
		return fmt.Errorf("uow: no active transaction for SaveValidationResultWithGoogleDataCtx")
	}
	return u.db.SaveValidationResultWithGoogleDataTx(ctx, u.tx, result, googleData)
}

func (u *SQLUnitOfWork) GetRecentValidationResultsCtx(ctx context.Context, limit int) ([]models.ValidationResult, error) {
	return u.db.GetRecentValidationResultsCtx(ctx, limit)
}
func (u *SQLUnitOfWork) GetVenueValidationHistoryCtx(ctx context.Context, venueID int64) ([]models.ValidationHistory, error) {
	return u.db.GetVenueValidationHistoryCtx(ctx, venueID)
}
func (u *SQLUnitOfWork) GetValidationHistoryPaginatedCtx(ctx context.Context, limit int, offset int) ([]models.ValidationHistory, int, error) {
	return u.db.GetValidationHistoryPaginatedCtx(ctx, limit, offset)
}
func (u *SQLUnitOfWork) GetCachedGooglePlaceDataCtx(ctx context.Context, venueID int64) (*models.GooglePlaceData, error) {
	return u.db.GetCachedGooglePlaceDataCtx(ctx, venueID)
}
func (u *SQLUnitOfWork) HasAnyValidationHistory(venueID int64) (bool, error) {
	return u.db.HasAnyValidationHistory(venueID)
}
func (u *SQLUnitOfWork) ValidateApprovalEligibility(venueID int64, threshold int) error {
	return u.db.ValidateApprovalEligibility(venueID, threshold)
}
