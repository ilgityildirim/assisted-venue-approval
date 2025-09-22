package repository

import (
	"context"

	"assisted-venue-approval/internal/models"
)

// CreateFeedbackCtx stores editor feedback.
func (r *SQLRepository) CreateFeedbackCtx(ctx context.Context, f *models.EditorFeedback) error {
	return r.db.CreateEditorFeedbackCtx(ctx, f)
}

// GetFeedbackByVenueCtx returns latest feedback list and counts for a venue.
func (r *SQLRepository) GetFeedbackByVenueCtx(ctx context.Context, venueID int64, limit int) ([]models.EditorFeedback, int, int, error) {
	return r.db.GetVenueFeedbackCtx(ctx, venueID, limit)
}

// GetFeedbackStatsCtx returns aggregate stats.
func (r *SQLRepository) GetFeedbackStatsCtx(ctx context.Context, promptVersion *string) (*models.FeedbackStats, error) {
	return r.db.GetFeedbackStatsCtx(ctx, promptVersion)
}
