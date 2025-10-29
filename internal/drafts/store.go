package drafts

import (
	"sync"
	"time"
)

// DraftStore provides thread-safe in-memory storage for venue editor drafts
type DraftStore struct {
	mu     sync.RWMutex
	drafts map[int64]*VenueDraft
}

// VenueDraft represents editor modifications to a venue
type VenueDraft struct {
	VenueID   int64                 `json:"venue_id"`
	EditorID  int                   `json:"editor_id"`
	Fields    map[string]DraftField `json:"fields"`
	UpdatedAt time.Time             `json:"updated_at"`
}

// DraftField represents a single field modification with source tracking
type DraftField struct {
	Value          interface{} `json:"value"`
	OriginalSource string      `json:"original_source"`
}

// NewDraftStore creates a new in-memory draft store
func NewDraftStore() *DraftStore {
	return &DraftStore{
		drafts: make(map[int64]*VenueDraft),
	}
}

// Save stores or updates a draft for a venue
func (s *DraftStore) Save(venueID int64, editorID int, fields map[string]DraftField) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.drafts[venueID] = &VenueDraft{
		VenueID:   venueID,
		EditorID:  editorID,
		Fields:    fields,
		UpdatedAt: time.Now(),
	}

	return nil
}

// Get retrieves a draft for a venue if it exists
func (s *DraftStore) Get(venueID int64) (*VenueDraft, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	draft, exists := s.drafts[venueID]
	return draft, exists
}

// Delete removes a draft from the store
func (s *DraftStore) Delete(venueID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.drafts, venueID)
}

// GetEditorInfo returns information about who last edited a draft
func (s *DraftStore) GetEditorInfo(venueID int64) (editorID int, updatedAt time.Time, exists bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	draft, exists := s.drafts[venueID]
	if !exists {
		return 0, time.Time{}, false
	}

	return draft.EditorID, draft.UpdatedAt, true
}

// Count returns the total number of drafts in the store
func (s *DraftStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.drafts)
}
