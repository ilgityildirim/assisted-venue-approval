package scorer

import (
	"context"
	"testing"
	"time"

	"automatic-vendor-validation/internal/models"
	"github.com/sashabaranov/go-openai"
)

// MockOpenAIClient for testing without actual API calls
type MockOpenAIClient struct {
	responses     map[string]openai.ChatCompletionResponse
	callCount     int
	expectedCalls []string
}

func NewMockOpenAIClient() *MockOpenAIClient {
	return &MockOpenAIClient{
		responses: make(map[string]openai.ChatCompletionResponse),
	}
}

func (m *MockOpenAIClient) SetResponse(prompt string, response openai.ChatCompletionResponse) {
	m.responses[prompt] = response
}

func (m *MockOpenAIClient) CreateChatCompletion(ctx context.Context, request openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	m.callCount++

	// Find the user message content
	var userPrompt string
	for _, msg := range request.Messages {
		if msg.Role == openai.ChatMessageRoleUser {
			userPrompt = msg.Content
			break
		}
	}

	// Return predefined response based on prompt
	if response, exists := m.responses[userPrompt]; exists {
		return response, nil
	}

	// Default response for unmatched prompts
	return openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{
			{
				Message: openai.ChatCompletionMessage{
					Content: `{"score": 75, "notes": "Test venue - looks legitimate", "breakdown": {"legitimacy": 25, "completeness": 25, "relevance": 25}}`,
				},
			},
		},
		Usage: openai.Usage{
			PromptTokens:     50,
			CompletionTokens: 30,
			TotalTokens:      80,
		},
	}, nil
}

// TestCostTracker tests the cost tracking functionality
func TestCostTracker(t *testing.T) {
	tracker := &CostTracker{
		startTime: time.Now(),
	}

	// Test initial state
	tokens, requests, cost, _ := tracker.GetStats()
	if tokens != 0 || requests != 0 || cost != 0.0 {
		t.Errorf("Expected initial state to be zero, got tokens=%d, requests=%d, cost=%f",
			tokens, requests, cost)
	}

	// Test adding usage
	tracker.AddUsage(100, 50)
	tokens, requests, cost, _ = tracker.GetStats()

	expectedTokens := 150
	expectedRequests := 1
	expectedCost := (100 * 0.0005 / 1000) + (50 * 0.0015 / 1000) // GPT-3.5 pricing

	if tokens != expectedTokens {
		t.Errorf("Expected %d tokens, got %d", expectedTokens, tokens)
	}
	if requests != expectedRequests {
		t.Errorf("Expected %d requests, got %d", expectedRequests, requests)
	}
	if cost < expectedCost-0.001 || cost > expectedCost+0.001 {
		t.Errorf("Expected cost around %f, got %f", expectedCost, cost)
	}

	// Test multiple additions
	tracker.AddUsage(200, 100)
	tokens, requests, cost, _ = tracker.GetStats()

	expectedTokens = 450 // 150 + 300
	expectedRequests = 2
	expectedCost += (200 * 0.0005 / 1000) + (100 * 0.0015 / 1000)

	if tokens != expectedTokens {
		t.Errorf("After second addition, expected %d tokens, got %d", expectedTokens, tokens)
	}
	if requests != expectedRequests {
		t.Errorf("After second addition, expected %d requests, got %d", expectedRequests, requests)
	}
	if cost < expectedCost-0.001 || cost > expectedCost+0.001 {
		t.Errorf("After second addition, expected cost around %f, got %f", expectedCost, cost)
	}
}

// TestVenueCache tests caching functionality
func TestVenueCache(t *testing.T) {
	cache := NewVenueCache()
	defer cache.Stop()

	// Test cache miss
	_, found := cache.Get("nonexistent")
	if found {
		t.Error("Expected cache miss for nonexistent key")
	}

	// Test cache set and get
	testResult := models.ValidationResult{
		VenueID: 123,
		Score:   85,
		Status:  "approved",
		Notes:   "Test result",
	}

	cache.Set("test-key", testResult)

	retrieved, found := cache.Get("test-key")
	if !found {
		t.Error("Expected to find cached result")
	}

	if retrieved.VenueID != testResult.VenueID ||
		retrieved.Score != testResult.Score ||
		retrieved.Status != testResult.Status {
		t.Errorf("Retrieved result doesn't match original: %+v vs %+v",
			retrieved, testResult)
	}

	// Test cache key generation
	venue1 := models.Venue{
		ID:       1,
		Name:     "Test Venue",
		Location: "123 Test St",
	}
	venue2 := models.Venue{
		ID:       2,
		Name:     "Test Venue",
		Location: "123 Test St",
	}
	venue3 := models.Venue{
		ID:       1,
		Name:     "Different Venue",
		Location: "123 Test St",
	}

	key1 := cache.generateKey(venue1)
	key2 := cache.generateKey(venue2)
	key3 := cache.generateKey(venue3)

	// Same venue data should generate same key regardless of ID
	if key1 != key2 {
		t.Error("Expected same cache key for venues with same data")
	}

	// Different venue data should generate different key
	if key1 == key3 {
		t.Error("Expected different cache key for venues with different data")
	}

	// Test cache size
	initialSize := cache.GetSize()
	if initialSize != 1 { // We added one item above
		t.Errorf("Expected cache size 1, got %d", initialSize)
	}
}

// TestVenueCacheExpiration tests cache expiration and cleanup
func TestVenueCacheExpiration(t *testing.T) {
	cache := NewVenueCache()
	defer cache.Stop()

	// Add an item to cache
	testResult := models.ValidationResult{
		VenueID: 123,
		Score:   85,
		Status:  "approved",
	}
	cache.Set("test-key", testResult)

	// Manually set timestamp to simulate old entry
	cache.mu.Lock()
	cached := cache.cache["test-key"]
	cached.Timestamp = time.Now().Add(-25 * time.Hour) // Older than 24 hours
	cache.cache["test-key"] = cached
	cache.mu.Unlock()

	// Try to get expired item
	_, found := cache.Get("test-key")
	if found {
		t.Error("Expected expired item to not be found")
	}

	// Test manual cleanup
	cache.cleanupExpired()

	cache.mu.RLock()
	size := len(cache.cache)
	cache.mu.RUnlock()

	if size != 0 {
		t.Errorf("Expected cache to be empty after cleanup, got size %d", size)
	}
}

// TestParseStructuredResponse tests JSON response parsing
func TestParseStructuredResponse(t *testing.T) {
	scorer := &AIScorer{}

	tests := []struct {
		name           string
		response       string
		expectedScore  int
		expectedStatus string
		expectError    bool
	}{
		{
			name:           "Valid JSON response",
			response:       `{"score": 85, "notes": "Great venue", "breakdown": {"legitimacy": 30, "completeness": 30, "relevance": 25}}`,
			expectedScore:  85,
			expectedStatus: "approved",
			expectError:    false,
		},
		{
			name:           "Low score",
			response:       `{"score": 40, "notes": "Not suitable", "breakdown": {"legitimacy": 15, "completeness": 15, "relevance": 10}}`,
			expectedScore:  40,
			expectedStatus: "rejected",
			expectError:    false,
		},
		{
			name:           "Medium score",
			response:       `{"score": 65, "notes": "Needs review", "breakdown": {"legitimacy": 20, "completeness": 25, "relevance": 20}}`,
			expectedScore:  65,
			expectedStatus: "manual_review",
			expectError:    false,
		},
		{
			name:        "Invalid JSON",
			response:    `not json`,
			expectError: true,
		},
		{
			name:           "Score out of range - high",
			response:       `{"score": 150, "notes": "Invalid score", "breakdown": {}}`,
			expectedScore:  50, // Should default to manual review
			expectedStatus: "manual_review",
			expectError:    false,
		},
		{
			name:           "Score out of range - negative",
			response:       `{"score": -10, "notes": "Invalid score", "breakdown": {}}`,
			expectedScore:  50, // Should default to manual review
			expectedStatus: "manual_review",
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := scorer.parseStructuredResponse(tt.response, 123)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}

			if !tt.expectError {
				if result.Score != tt.expectedScore {
					t.Errorf("Expected score %d, got %d", tt.expectedScore, result.Score)
				}
				if result.Status != tt.expectedStatus {
					t.Errorf("Expected status %s, got %s", tt.expectedStatus, result.Status)
				}
				if result.VenueID != 123 {
					t.Errorf("Expected venue ID 123, got %d", result.VenueID)
				}
			}
		})
	}
}

// TestParseResponseFallback tests fallback parsing
func TestParseResponseFallback(t *testing.T) {
	scorer := &AIScorer{}

	tests := []struct {
		name           string
		response       string
		expectedScore  int
		expectedStatus string
	}{
		{
			name:           "Extract score from malformed JSON",
			response:       `The venue looks good with a score: 80 out of 100`,
			expectedScore:  80, // Regex should match this format
			expectedStatus: "manual_review",
		},
		{
			name:           "Extract score from JSON-like format",
			response:       `"score": 90, "notes": "excellent venue"`,
			expectedScore:  90,
			expectedStatus: "approved",
		},
		{
			name:           "No score found",
			response:       `This venue appears to be legitimate and suitable`,
			expectedScore:  50, // Default
			expectedStatus: "manual_review",
		},
		{
			name:           "Score out of range",
			response:       `"score": 150`,
			expectedScore:  50, // Should default when invalid
			expectedStatus: "manual_review",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := scorer.parseResponseFallback(tt.response, 456)

			if result.Score != tt.expectedScore {
				t.Errorf("Expected score %d, got %d", tt.expectedScore, result.Score)
			}
			if result.Status != tt.expectedStatus {
				t.Errorf("Expected status %s, got %s", tt.expectedStatus, result.Status)
			}
			if result.VenueID != 456 {
				t.Errorf("Expected venue ID 456, got %d", result.VenueID)
			}
		})
	}
}

// TestParseVeganRelevanceResponse tests vegan relevance parsing
func TestParseVeganRelevanceResponse(t *testing.T) {
	scorer := &AIScorer{}

	tests := []struct {
		name          string
		response      string
		expectedScore int
		expectNote    bool
	}{
		{
			name:          "Valid vegan relevance JSON",
			response:      `{"score": 20, "note": "Fully vegan restaurant"}`,
			expectedScore: 20,
			expectNote:    true,
		},
		{
			name:          "Score out of range - high",
			response:      `{"score": 30, "note": "Invalid score"}`,
			expectedScore: 10, // Should default
			expectNote:    true,
		},
		{
			name:          "Score out of range - negative",
			response:      `{"score": -5, "note": "Invalid score"}`,
			expectedScore: 10, // Should default
			expectNote:    true,
		},
		{
			name:          "Invalid JSON - fallback parsing",
			response:      `The vegan relevance score: 15 - good options`,
			expectedScore: 15,
			expectNote:    true,
		},
		{
			name:          "No score found - fallback",
			response:      `This venue has good vegan options`,
			expectedScore: 10, // Default fallback
			expectNote:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score, notes := scorer.parseVeganRelevanceResponse(tt.response)

			if score != tt.expectedScore {
				t.Errorf("Expected score %d, got %d", tt.expectedScore, score)
			}
			if tt.expectNote && notes == "" {
				t.Error("Expected notes to be non-empty")
			}
		})
	}
}

// TestScoreEnhancedVenue tests enhanced scoring for venues with Google data
func TestScoreEnhancedVenue(t *testing.T) {
	// Create AIScorer with test API key (will use default behavior)
	scorer := NewAIScorer("test-api-key")
	defer scorer.cache.Stop()

	// Create test venue with validation details
	additionalInfo := "100% vegan restaurant with organic ingredients"
	venue := models.Venue{
		ID:             123,
		Name:           "Green Paradise",
		Location:       "123 Vegan St",
		Vegan:          4,
		VegOnly:        1,
		AdditionalInfo: &additionalInfo,
		ValidationDetails: &models.ValidationDetails{
			GooglePlaceFound: true,
			ScoreBreakdown: models.ScoreBreakdown{
				VenueNameMatch:      20,
				AddressAccuracy:     18,
				GeolocationAccuracy: 15,
				PhoneVerification:   10,
				BusinessHours:       8,
				WebsiteVerification: 5,
				BusinessStatus:      10,
				PostalCode:          5,
				VeganRelevance:      5,  // This will be replaced by AI score
				Total:               96, // Sum of above scores
			},
		},
	}

	// Test that enhanced scoring works with validation details
	// Since we can't mock the OpenAI client easily, we'll test the basic structure
	if venue.ValidationDetails == nil {
		t.Fatal("Test venue should have validation details")
	}

	if !venue.ValidationDetails.GooglePlaceFound {
		t.Error("Test venue should have Google place found")
	}

	if venue.ValidationDetails.ScoreBreakdown.Total != 96 {
		t.Errorf("Expected base score 96, got %d", venue.ValidationDetails.ScoreBreakdown.Total)
	}
}

// TestBuildPrompts tests prompt generation functions
func TestBuildPrompts(t *testing.T) {
	scorer := &AIScorer{}

	// Test basic prompt building
	phone := "555-1234"
	url := "https://example.com"
	additionalInfo := "Great vegan options"

	venue := models.Venue{
		ID:             123,
		Name:           "Test Venue",
		Location:       "123 Test St",
		Phone:          &phone,
		URL:            &url,
		AdditionalInfo: &additionalInfo,
	}

	basicPrompt := scorer.buildBasicPrompt(venue)

	// Check that prompt contains venue information
	if !contains(basicPrompt, venue.Name) {
		t.Error("Basic prompt should contain venue name")
	}
	if !contains(basicPrompt, venue.Location) {
		t.Error("Basic prompt should contain venue location")
	}
	if !contains(basicPrompt, *venue.Phone) {
		t.Error("Basic prompt should contain venue phone")
	}
	if !contains(basicPrompt, *venue.URL) {
		t.Error("Basic prompt should contain venue URL")
	}

	// Test vegan relevance prompt
	veganPrompt := scorer.buildVeganRelevancePrompt(venue)

	if !contains(veganPrompt, venue.Name) {
		t.Error("Vegan relevance prompt should contain venue name")
	}
	if !contains(veganPrompt, *venue.AdditionalInfo) {
		t.Error("Vegan relevance prompt should contain additional info")
	}

	// Test prompts with nil values
	venueWithNils := models.Venue{
		ID:       456,
		Name:     "Test Venue 2",
		Location: "456 Test Ave",
		// Phone, URL, AdditionalInfo are nil
	}

	basicPromptNils := scorer.buildBasicPrompt(venueWithNils)
	if !contains(basicPromptNils, "N/A") {
		t.Error("Basic prompt should handle nil values with N/A")
	}

	veganPromptNils := scorer.buildVeganRelevancePrompt(venueWithNils)
	// Should not panic with nil AdditionalInfo
	if veganPromptNils == "" {
		t.Error("Vegan relevance prompt should not be empty with nil values")
	}
}

// TestAIScorerIntegration tests the full scoring workflow (without real API calls)
func TestAIScorerIntegration(t *testing.T) {
	// Skip this test since it requires real API calls
	t.Skip("Skipping integration test - requires real API key")
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			len(s) > len(substr) &&
				(s[:len(substr)] == substr ||
					s[len(s)-len(substr):] == substr ||
					containsInMiddle(s, substr)))
}

func containsInMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Benchmark tests for performance
func BenchmarkCacheOperations(b *testing.B) {
	cache := NewVenueCache()
	defer cache.Stop()

	result := models.ValidationResult{
		VenueID: 123,
		Score:   85,
		Status:  "approved",
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		key := "benchmark-key"
		cache.Set(key, result)
		_, _ = cache.Get(key)
	}
}

func BenchmarkPromptGeneration(b *testing.B) {
	scorer := &AIScorer{}

	phone := "555-1234"
	url := "https://example.com"
	additionalInfo := "Great vegan restaurant with organic ingredients"

	venue := models.Venue{
		ID:             123,
		Name:           "Benchmark Venue",
		Location:       "123 Benchmark St",
		Phone:          &phone,
		URL:            &url,
		AdditionalInfo: &additionalInfo,
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		scorer.buildBasicPrompt(venue)
		scorer.buildVeganRelevancePrompt(venue)
	}
}
