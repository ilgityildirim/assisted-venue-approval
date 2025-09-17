package repository

import (
	"context"
	"os"
	"testing"

	"assisted-venue-approval/pkg/database"
)

// Benchmark basic read path to catch regressions in DB access layer.
func BenchmarkGetRecentValidationResults(b *testing.B) {
	url := os.Getenv("DATABASE_URL_TEST")
	if url == "" {
		url = os.Getenv("DATABASE_URL")
	}
	if url == "" {
		b.Skip("DATABASE_URL_TEST or DATABASE_URL not set; skipping DB benchmark")
	}
	db, err := database.New(url)
	if err != nil {
		b.Fatalf("failed to create db: %v", err)
	}
	defer db.Close()

	repo := NewSQLRepository(db)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = repo.GetRecentValidationResultsCtx(ctx, 10)
	}
}
