package testutil

import (
	"context"
	"sync"
	"time"

	"assisted-venue-approval/internal/models"
)

// MockScraper implements processor.GoogleScraper for tests.
type MockScraper struct {
	Mu   sync.Mutex
	Resp map[int64]*models.Venue
	Err  map[int64]error
}

func NewMockScraper() *MockScraper {
	return &MockScraper{Resp: map[int64]*models.Venue{}, Err: map[int64]error{}}
}

func (m *MockScraper) EnhanceVenueWithValidation(ctx context.Context, v models.Venue) (*models.Venue, error) {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	if err, ok := m.Err[v.ID]; ok {
		return nil, err
	}
	if r, ok := m.Resp[v.ID]; ok {
		return r, nil
	}
	// default: pass-through venue
	vv := v // copy
	return &vv, nil
}

// MockScorer implements processor.VenueScorer for tests.
type MockScorer struct {
	Mu   sync.Mutex
	Resp map[int64]*models.ValidationResult
	Err  map[int64]error
}

func NewMockScorer() *MockScorer {
	return &MockScorer{Resp: map[int64]*models.ValidationResult{}, Err: map[int64]error{}}
}

func (m *MockScorer) ScoreVenue(ctx context.Context, v models.Venue, u models.User) (*models.ValidationResult, error) {
	m.Mu.Lock()
	defer m.Mu.Unlock()
	if err, ok := m.Err[v.ID]; ok {
		return nil, err
	}
	if r, ok := m.Resp[v.ID]; ok {
		return r, nil
	}
	// default: neutral manual review
	return &models.ValidationResult{VenueID: v.ID, Score: 50, Status: "manual_review", Notes: "mock default"}, nil
}

func (m *MockScorer) GetCostStats() (int, int, float64, time.Duration) { return 0, 0, 0, 0 }
func (m *MockScorer) GetBufferPoolStats() (int64, int64, int64)        { return -1, -1, -1 }
