package scorer

import (
	"testing"

	"assisted-venue-approval/internal/models"
)

func BenchmarkBuildUnifiedPrompt(b *testing.B) {
	s := NewAIScorer("test")
	v := models.Venue{ID: 1, Name: "Test", Location: "Somewhere"}
	trust := 0.7
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = s.buildUnifiedPrompt(v, trust)
	}
}

func BenchmarkParseStructuredResponse(b *testing.B) {
	s := NewAIScorer("test")
	resp := `{"score": 90, "notes": "ok", "breakdown": {"legitimacy": 30, "completeness": 30, "relevance": 30}}`
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = s.parseStructuredResponse(resp, 1)
	}
}
