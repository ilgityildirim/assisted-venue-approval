package models

import (
	"time"
)

type Venue struct {
	ID                      int64      `json:"id" db:"id"`
	Path                    *string    `json:"path" db:"path"`
	EntryType               int        `json:"entrytype" db:"entrytype"`
	Name                    string     `json:"name" db:"name"`
	URL                     *string    `json:"url" db:"url"`
	FBUrl                   *string    `json:"fburl" db:"fburl"`
	InstagramUrl            *string    `json:"instagram_url" db:"instagram_url"`
	Location                string     `json:"location" db:"location"`
	Zipcode                 *string    `json:"zipcode" db:"zipcode"`
	Phone                   *string    `json:"phone" db:"phone"`
	OtherFoodType           *string    `json:"other_food_type" db:"other_food_type"`
	Price                   *int8      `json:"price" db:"price"`
	AdditionalInfo          *string    `json:"additionalinfo" db:"additionalinfo"`
	VDetails                string     `json:"vdetails" db:"vdetails"`
	OpenHours               *string    `json:"openhours" db:"openhours"`
	OpenHoursNote           *string    `json:"openhours_note" db:"openhours_note"`
	Timezone                *string    `json:"timezone" db:"timezone"`
	Hash                    *string    `json:"hash" db:"hash"`
	Email                   *string    `json:"email" db:"email"`
	OwnerName               *string    `json:"ownername" db:"ownername"`
	SentBy                  *string    `json:"sentby" db:"sentby"`
	UserID                  uint       `json:"user_id" db:"user_id"`
	Active                  *int       `json:"active" db:"active"`
	VegOnly                 int        `json:"vegonly" db:"vegonly"`
	Vegan                   int        `json:"vegan" db:"vegan"`
	SponsorLevel            int        `json:"sponsor_level" db:"sponsor_level"`
	CrossStreet             *string    `json:"crossstreet" db:"crossstreet"`
	Lat                     *float64   `json:"lat" db:"lat"`
	Lng                     *float64   `json:"lng" db:"lng"`
	CreatedAt               *time.Time `json:"created_at" db:"created_at"`
	DateAdded               *time.Time `json:"date_added" db:"date_added"`
	DateUpdated             *time.Time `json:"date_updated" db:"date_updated"`
	AdminLastUpdate         *time.Time `json:"admin_last_update" db:"admin_last_update"`
	AdminNote               *string    `json:"admin_note" db:"admin_note"`
	AdminHold               *time.Time `json:"admin_hold" db:"admin_hold"`
	AdminHoldEmailNote      *string    `json:"admin_hold_email_note" db:"admin_hold_email_note"`
	UpdatedByID             *int       `json:"updated_by_id" db:"updated_by_id"`
	MadeActiveByID          *int       `json:"made_active_by_id" db:"made_active_by_id"`
	MadeActiveAt            *time.Time `json:"made_active_at" db:"made_active_at"`
	ShowPremium             int        `json:"show_premium" db:"show_premium"`
	Category                int        `json:"category" db:"category"`
	PrettyUrl               *string    `json:"pretty_url" db:"pretty_url"`
	EditLock                *string    `json:"edit_lock" db:"edit_lock"`
	RequestVeganDecalAt     *time.Time `json:"request_vegan_decal_at" db:"request_vegan_decal_at"`
	RequestExcellentDecalAt *time.Time `json:"request_excellent_decal_at" db:"request_excellent_decal_at"`
	Source                  int        `json:"source" db:"source"`

	// Validation fields
	ValidationScore   int                `json:"validation_score,omitempty"`
	ValidationStatus  string             `json:"validation_status,omitempty"`
	ValidationNotes   string             `json:"validation_notes,omitempty"`
	ValidationDetails *ValidationDetails `json:"validation_details,omitempty"`
	ProcessedAt       *time.Time         `json:"processed_at,omitempty"`
	GooglePlaceID     string             `json:"google_place_id,omitempty"`
	GoogleData        *GooglePlaceData   `json:"google_data,omitempty"`
}

type ValidationResult struct {
	VenueID        int64          `json:"venue_id"`
	Score          int            `json:"score"`
	Status         string         `json:"status"` // "approved", "rejected", "manual_review"
	Notes          string         `json:"notes"`
	ScoreBreakdown map[string]int `json:"score_breakdown"`
	AIOutputData   *string        `json:"ai_output_data,omitempty"`
	PromptVersion  *string        `json:"prompt_version,omitempty"`

	// Extended validation fields (parsed from ai_output_data JSON)
	DescriptionReview *DescriptionReview `json:"description_review,omitempty"`
	NameReview        *NameReview        `json:"name_review,omitempty"`
}

// DescriptionReview contains AI assessment of venue description quality and language
type DescriptionReview struct {
	Language             string   `json:"language"`               // "en", "zh", "ko", "ja", "th", "mixed", "other"
	QualityScore         int      `json:"quality_score"`          // 0-10 scale
	ConformsToGuidelines bool     `json:"conforms_to_guidelines"` // matches category guidelines
	SuggestedDescription string   `json:"suggested_description,omitempty"`
	Issues               []string `json:"issues,omitempty"` // ["first-person", "too-short", "promotional", etc]
	CategoryMatch        bool     `json:"category_match"`   // description fits venue category
}

// NameReview contains AI assessment of venue name translation format
type NameReview struct {
	Format           string `json:"format"`           // "correct", "needs_translation", "missing_native", "incorrect"
	TranslationType  string `json:"translation_type"` // "phonetic", "official", "literal", "none"
	SuggestedName    string `json:"suggested_name,omitempty"`
	OriginalDetected string `json:"original_detected,omitempty"` // "zh", "ko", "ja", "th", "none"
	HasNativeScript  bool   `json:"has_native_script"`           // true if native characters present
}

// QualitySuggestions contains AI-suggested improvements for venue content
// This is stored in ai_output_data JSON alongside scoring data
type QualitySuggestions struct {
	Description    string          `json:"description"`              // Always provided: rewritten to follow ALL guidelines
	Name           string          `json:"name,omitempty"`           // Only if correction needed, omitted if already correct
	ClosedDays     *string         `json:"closed_days,omitempty"`    // Generated from hours: "Closed Mon", "Closed Sun & Tue", "Closed Mon-Wed"
	PathValidation *PathValidation `json:"pathValidation,omitempty"` // Path format and geographic validation
}

// PathValidation contains AI assessment of venue path format and geographic accuracy
type PathValidation struct {
	IsValid    bool   `json:"isValid"`              // Whether path format is correct and location matches
	Issue      string `json:"issue,omitempty"`      // Description of problem if invalid
	Confidence string `json:"confidence,omitempty"` // high/medium/low confidence level
}

type ValidationDetails struct {
	ScoreBreakdown     ScoreBreakdown `json:"score_breakdown"`
	GooglePlaceFound   bool           `json:"google_place_found"`
	DistanceMeters     float64        `json:"distance_meters"`
	Conflicts          []DataConflict `json:"conflicts,omitempty"`
	AutoDecisionReason string         `json:"auto_decision_reason"`
	ProcessingTimeMs   int64          `json:"processing_time_ms"`
	SuggestedPath      *string        `json:"suggested_path,omitempty"` // Generated path from Google Places address
}

type ScoreBreakdown struct {
	VenueNameMatch      int `json:"venue_name_match"`     // 0-25 points
	AddressAccuracy     int `json:"address_accuracy"`     // 0-20 points
	GeolocationAccuracy int `json:"geolocation_accuracy"` // 0-15 points
	PhoneVerification   int `json:"phone_verification"`   // 0-10 points
	BusinessHours       int `json:"business_hours"`       // 0-10 points
	WebsiteVerification int `json:"website_verification"` // 0-5 points
	BusinessStatus      int `json:"business_status"`      // 0-5 points
	PostalCode          int `json:"postal_code"`          // 0-5 points
	VeganRelevance      int `json:"vegan_relevance"`      // 0-5 points
	Total               int `json:"total"`                // Sum of all scores
}

type DataConflict struct {
	Field         string `json:"field"`
	HappyCowValue string `json:"happycow_value"`
	GoogleValue   string `json:"google_value"`
	Resolution    string `json:"resolution"` // "user_data", "google_data", "manual_review"
}

type GooglePlaceData struct {
	PlaceID           string              `json:"place_id"`
	Name              string              `json:"name"`
	FormattedAddress  string              `json:"formatted_address"`
	FormattedPhone    string              `json:"formatted_phone_number"`
	Website           string              `json:"website"`
	BusinessStatus    string              `json:"business_status"`
	Geometry          GoogleGeometry      `json:"geometry"`
	OpeningHours      *GoogleOpeningHours `json:"opening_hours,omitempty"`
	AddressComponents []AddressComponent  `json:"address_components"`
	Types             []string            `json:"types"`
	Rating            float64             `json:"rating"`
	UserRatingsTotal  int                 `json:"user_ratings_total"`
	FetchedAt         time.Time           `json:"fetched_at"`
}

type GoogleGeometry struct {
	Location GoogleLatLng `json:"location"`
	Viewport GoogleBounds `json:"viewport"`
}

type GoogleLatLng struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type GoogleBounds struct {
	Northeast GoogleLatLng `json:"northeast"`
	Southwest GoogleLatLng `json:"southwest"`
}

type GoogleOpeningHours struct {
	OpenNow     bool           `json:"open_now"`
	Periods     []GooglePeriod `json:"periods"`
	WeekdayText []string       `json:"weekday_text"`
}

type GooglePeriod struct {
	Close GoogleTime `json:"close"`
	Open  GoogleTime `json:"open"`
}

type GoogleTime struct {
	Day  int    `json:"day"`  // 0=Sunday, 1=Monday, etc.
	Time string `json:"time"` // HHMM format
}

type AddressComponent struct {
	LongName  string   `json:"long_name"`
	ShortName string   `json:"short_name"`
	Types     []string `json:"types"`
}

// User represents user information for authority checking
type User struct {
	ID                 uint    `json:"id"`
	Username           string  `json:"username"`
	Email              string  `json:"email"`
	Trusted            bool    `json:"trusted"`
	Contributions      int     `json:"contributions"`
	ApprovedVenueCount *int    `json:"approved_venue_count,omitempty" db:"approved_venue_count"`
	IsVenueAdmin       bool    `json:"is_venue_admin"`
	IsVenueOwner       bool    `json:"is_venue_owner"`
	AmbassadorLevel    *int    `json:"ambassador_level,omitempty"`
	AmbassadorPoints   *int    `json:"ambassador_points,omitempty"`
	AmbassadorRegion   *string `json:"ambassador_region,omitempty"`
}

// VenueWithUser combines venue and user information
type VenueWithUser struct {
	Venue            Venue   `json:"venue"`
	User             User    `json:"user"`
	IsVenueAdmin     bool    `json:"is_venue_admin"`
	AmbassadorLevel  *int64  `json:"ambassador_level,omitempty"`
	AmbassadorPoints *int64  `json:"ambassador_points,omitempty"`
	AmbassadorPath   *string `json:"ambassador_path,omitempty"`
}

// ValidationHistory tracks validation attempts with Google Places data
type ValidationHistory struct {
	ID               int64          `json:"id"`
	VenueID          int64          `json:"venue_id"`
	ValidationScore  int            `json:"validation_score"`
	ValidationStatus string         `json:"validation_status"`
	ValidationNotes  string         `json:"validation_notes"`
	ScoreBreakdown   map[string]int `json:"score_breakdown"`
	AIOutputData     *string        `json:"ai_output_data,omitempty"`
	PromptVersion    *string        `json:"prompt_version,omitempty"`

	// Google Places API data
	GooglePlaceID    *string          `json:"google_place_id,omitempty"`
	GooglePlaceFound bool             `json:"google_place_found"`
	GooglePlaceData  *GooglePlaceData `json:"google_place_data,omitempty"`

	ProcessedAt time.Time `json:"processed_at"`
	VenueName   string    `json:"venue_name,omitempty"`
}

// VenueStats contains processing statistics
type VenueStats struct {
	Pending  int `json:"pending"`
	Approved int `json:"approved"`
	Rejected int `json:"rejected"`
	Total    int `json:"total"`
}
