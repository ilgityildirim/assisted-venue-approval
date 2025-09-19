package processor_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"assisted-venue-approval/internal/decision"
	"assisted-venue-approval/internal/infrastructure/repository"
	"assisted-venue-approval/internal/models"
	"assisted-venue-approval/internal/processor"
	testutil "assisted-venue-approval/internal/testing"
	pdb "assisted-venue-approval/pkg/database"
)

// buildVenue creates a minimal venue with a unique ID for isolation.
func buildVenue(id int64, name string) models.Venue {
	url := "https://example.com"
	phone := "+1 555-0100"
	info := "Vegan friendly"
	return models.Venue{
		ID:             id,
		Name:           name,
		Location:       "Test City",
		URL:            &url,
		Phone:          &phone,
		AdditionalInfo: &info,
	}
}

func TestProcessingEngine_EndToEnd(t *testing.T) {
	t.Parallel()
	dbtest := testutil.NewDBTest(t)
	defer dbtest.Close()

	// repo/uow against real DB connection
	repo := repository.NewSQLRepository(dbtest.DB)
	uowf := repository.NewSQLUnitOfWorkFactory(dbtest.DB)

	// mocks for external services
	ms := testutil.NewMockScraper()
	msc := testutil.NewMockScorer()

	cfg := processor.DefaultProcessingConfig()
	cfg.WorkerCount = 2
	cfg.QueueSize = 10
	cfg.GoogleRPS = 100
	cfg.OpenAIRPS = 100
	cfg.RetryDelay = 10 * time.Millisecond
	cfg.JobTimeout = 2 * time.Second

	decCfg := decision.DecisionConfig{ApprovalThreshold: 75}
	eng := processor.NewProcessingEngine(repo, uowf, ms, msc, cfg, decCfg)
	eng.SetScoreOnly(false)
	eng.Start()
	defer func() { _ = eng.Stop(2 * time.Second) }()

	tcs := []struct {
		name   string
		setup  func(v models.Venue)
		assert func(t *testing.T, db *pdb.DB, v models.Venue)
	}{
		{
			name: "auto-approve via high score",
			setup: func(v models.Venue) {
				msc.Resp[v.ID] = &models.ValidationResult{VenueID: v.ID, Score: 90, Status: "approved", Notes: "great"}
			},
			assert: func(t *testing.T, db *pdb.DB, v models.Venue) {
				t.Helper()
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				vw, err := db.GetVenueWithUserByIDCtx(ctx, v.ID)
				if err != nil {
					t.Fatalf("get venue: %v", err)
				}
				if vw.Venue.Active == nil || *vw.Venue.Active != 1 {
					t.Fatalf("expected active=1 got %v", vw.Venue.Active)
				}
			},
		},
		{
			name:  "manual review default",
			setup: func(v models.Venue) {},
			assert: func(t *testing.T, db *pdb.DB, v models.Venue) {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				vw, err := db.GetVenueWithUserByIDCtx(ctx, v.ID)
				if err != nil {
					t.Fatalf("get venue: %v", err)
				}
				if vw.Venue.Active == nil || *vw.Venue.Active != 0 {
					t.Fatalf("expected active=0 got %v", vw.Venue.Active)
				}
			},
		},
		{
			name: "ai failure saves google data and sets manual",
			setup: func(v models.Venue) {
				msc.Err[v.ID] = errors.New("timeout")
				gd := &models.GooglePlaceData{PlaceID: fmt.Sprintf("gp_%d", v.ID)}
				mv := buildVenue(v.ID, v.Name)
				mv.GoogleData = gd
				d := mv // capture
				ms.Resp[v.ID] = &d
			},
			assert: func(t *testing.T, db *pdb.DB, v models.Venue) {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				vw, err := db.GetVenueWithUserByIDCtx(ctx, v.ID)
				if err != nil {
					t.Fatalf("get venue: %v", err)
				}
				if vw.Venue.Active == nil || *vw.Venue.Active != 0 {
					t.Fatalf("expected active=0 got %v", vw.Venue.Active)
				}
				// ensure a validation history exists
				h, err := db.GetVenueValidationHistoryCtx(ctx, v.ID)
				if err != nil {
					t.Fatalf("history: %v", err)
				}
				if len(h) == 0 {
					t.Fatalf("expected history saved on failure")
				}
			},
		},
	}

	for _, tc := range tcs {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			id := rand.Int63()
			v := buildVenue(id, fmt.Sprintf("V%v", id))
			// seed DB with venue row; minimal fields used by queries
			seedVenue(t, dbtest.DB, v)
			tc.setup(v)

			if err := eng.ProcessVenuesWithUsers([]models.VenueWithUser{{Venue: v, User: models.User{}}}); err != nil {
				t.Fatalf("queue: %v", err)
			}
			// wait a bit for workers
			time.Sleep(200 * time.Millisecond)
			tc.assert(t, dbtest.DB, v)
		})
	}
}

func seedVenue(t *testing.T, db *pdb.DB, v models.Venue) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	// Use UpdateVenueActive to upsert is not available; insert raw
	_, err := db.Conn().ExecContext(ctx, `INSERT INTO venues (id, name, active, location, url, phone) VALUES (?, ?, 0, ?, ?, ?) ON DUPLICATE KEY UPDATE name=VALUES(name)`, v.ID, v.Name, v.Location, valueOrNull(v.URL), valueOrNull(v.Phone))
	if err != nil {
		t.Fatalf("seed venue: %v", err)
	}
}

func valueOrNull(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func BenchmarkProcessingEngine(b *testing.B) {
	dbtest := testutil.NewDBTest(&testing.T{})
	defer dbtest.Close()
	repo := repository.NewSQLRepository(dbtest.DB)
	uowf := repository.NewSQLUnitOfWorkFactory(dbtest.DB)
	ms := testutil.NewMockScraper()
	msc := testutil.NewMockScorer()
	cfg := processor.DefaultProcessingConfig()
	cfg.WorkerCount = 4
	cfg.QueueSize = 100
	cfg.GoogleRPS = 1000
	cfg.OpenAIRPS = 1000
	cfg.RetryDelay = 0
	cfg.JobTimeout = 2 * time.Second
	eng := processor.NewProcessingEngine(repo, uowf, ms, msc, cfg, decision.DecisionConfig{ApprovalThreshold: 75})
	eng.Start()
	defer func() { _ = eng.Stop(2 * time.Second) }()

	venues := make([]models.Venue, b.N)
	for i := 0; i < b.N; i++ {
		id := int64(100000 + i)
		v := buildVenue(id, fmt.Sprintf("B%v", id))
		seedVenue(&testing.T{}, dbtest.DB, v)
		venues[i] = v
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = eng.ProcessVenuesWithUsers([]models.VenueWithUser{{Venue: venues[i], User: models.User{}}})
	}
}
