package decision

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"assisted-venue-approval/internal/domain/specs"
	"assisted-venue-approval/internal/models"
	"assisted-venue-approval/pkg/events"
)

// DecisionEngine handles venue approval/rejection logic with special case handling
type DecisionEngine struct {
	approvalThreshold   int
	rejectionThreshold  int
	enableSpecialCases  bool
	enableAuthorityMode bool
	eventStore          events.EventStore
	approvalSpec        specs.Specification[models.Venue]
}

// DecisionConfig configures the decision engine behavior
type DecisionConfig struct {
	ApprovalThreshold   int  // Score threshold for auto-approval (default: 85)
	RejectionThreshold  int  // Score threshold for auto-rejection (default: 50)
	EnableSpecialCases  bool // Enable Korean/Chinese venue special handling
	EnableAuthorityMode bool // Enable venue owner/ambassador authority rules
}

// DecisionResult contains the final decision with detailed reasoning
type DecisionResult struct {
	VenueID              int64                    `json:"venue_id"`
	FinalStatus          string                   `json:"final_status"` // "approved", "rejected", "manual_review"
	FinalScore           int                      `json:"final_score"`
	DecisionReason       string                   `json:"decision_reason"`
	Authority            *AuthorityInfo           `json:"authority,omitempty"`
	SpecialCaseFlags     []string                 `json:"special_case_flags,omitempty"`
	QualityFlags         []string                 `json:"quality_flags,omitempty"`
	ValidationResult     *models.ValidationResult `json:"validation_result"`
	ProcessedAt          time.Time                `json:"processed_at"`
	RequiresManualReview bool                     `json:"requires_manual_review"`
	ReviewReason         string                   `json:"review_reason,omitempty"`
}

// AuthorityInfo tracks user authority for decision making
type AuthorityInfo struct {
	UserID          uint                 `json:"user_id"`
	IsVenueAdmin    bool                 `json:"is_venue_admin"`
	IsTrustedMember bool                 `json:"is_trusted_member"`
	AmbassadorInfo  *AmbassadorAuthority `json:"ambassador_info,omitempty"`
	AuthorityLevel  string               `json:"authority_level"` // "venue_admin", "high_ambassador", "trusted", "regular"
	BonusPoints     int                  `json:"bonus_points"`
	TrustLevel      float64              `json:"trust_level"` // 0.0 to 1.0
}

// AmbassadorAuthority contains ambassador-specific authority information
type AmbassadorAuthority struct {
	Level         *int    `json:"level,omitempty"`
	Points        *int    `json:"points,omitempty"`
	Region        *string `json:"region,omitempty"`
	IsHighRanking bool    `json:"is_high_ranking"`
	RegionMatches bool    `json:"region_matches"`
}

// DefaultDecisionConfig returns sensible defaults for the decision engine
func DefaultDecisionConfig() DecisionConfig {
	return DecisionConfig{
		ApprovalThreshold:   85,
		RejectionThreshold:  50,
		EnableSpecialCases:  true,
		EnableAuthorityMode: true,
	}
}

// NewDecisionEngine creates a new decision engine with the given configuration
func NewDecisionEngine(config DecisionConfig) *DecisionEngine {
	return &DecisionEngine{
		approvalThreshold:   config.ApprovalThreshold,
		rejectionThreshold:  config.RejectionThreshold,
		enableSpecialCases:  config.EnableSpecialCases,
		enableAuthorityMode: config.EnableAuthorityMode,
		approvalSpec:        specs.BuildApprovalSpecFromEnv(),
	}
}

// ApplyConfig allows runtime updates of thresholds.
func (de *DecisionEngine) ApplyConfig(approvalThreshold int) {
	if approvalThreshold > 0 && approvalThreshold <= 100 {
		de.approvalThreshold = approvalThreshold
	}
}

// SetEventStore wires an EventStore for publishing decisions.
func (de *DecisionEngine) SetEventStore(es events.EventStore) { de.eventStore = es }

// MakeDecision processes a venue with user information and returns a final decision
func (de *DecisionEngine) MakeDecision(venue models.Venue, user models.User, validationResult *models.ValidationResult) *DecisionResult {
	startTime := time.Now()

	result := &DecisionResult{
		VenueID:          venue.ID,
		ValidationResult: validationResult,
		ProcessedAt:      startTime,
		FinalScore:       validationResult.Score,
	}

	authority := de.analyzeUserAuthority(venue, user)
	result.Authority = authority

	enhancedScore := de.applyAuthorityBonuses(validationResult.Score, authority)
	result.FinalScore = enhancedScore
	fmt.Printf("decision: id=%d score=%d\n", venue.ID, enhancedScore) // debug

	specialCases := de.detectSpecialCases(venue)
	qualityFlags := de.detectQualityFlags(venue, validationResult)

	result.SpecialCaseFlags = specialCases
	result.QualityFlags = qualityFlags

	decision := de.determineStatus(venue, user, enhancedScore, authority, specialCases, qualityFlags)
	result.FinalStatus = decision.Status
	result.DecisionReason = decision.Reason
	result.RequiresManualReview = decision.RequiresReview
	result.ReviewReason = decision.ReviewReason

	log.Printf("Decision for venue %d: %s (score: %d→%d) - %s",
		venue.ID, result.FinalStatus, validationResult.Score, enhancedScore, result.DecisionReason)

	// TODO: consider retries/backoff here if event store is flaky
	if de.eventStore != nil {
		flags := append([]string{}, result.SpecialCaseFlags...)
		flags = append(flags, result.QualityFlags...)
		switch result.FinalStatus {
		case "approved":
			_ = de.eventStore.Append(context.Background(), events.VenueApproved{
				Base:   events.Base{Ts: time.Now(), VID: venue.ID},
				Reason: result.DecisionReason,
				Score:  result.FinalScore,
				Flags:  flags,
			})
		case "rejected":
			_ = de.eventStore.Append(context.Background(), events.VenueRejected{
				Base:   events.Base{Ts: time.Now(), VID: venue.ID},
				Reason: result.DecisionReason,
				Score:  result.FinalScore,
				Flags:  flags,
			})
		case "manual_review":
			_ = de.eventStore.Append(context.Background(), events.VenueRequiresManualReview{
				Base:   events.Base{Ts: time.Now(), VID: venue.ID},
				Reason: result.ReviewReason,
				Score:  result.FinalScore,
				Flags:  flags,
			})
		}
	}

	return result
}

// analyzeUserAuthority determines the user's authority level for this venue
func (de *DecisionEngine) analyzeUserAuthority(venue models.Venue, user models.User) *AuthorityInfo {
	authority := &AuthorityInfo{
		UserID:          user.ID,
		IsVenueAdmin:    user.IsVenueAdmin,
		IsTrustedMember: user.Trusted,
		BonusPoints:     0,
		TrustLevel:      0.0,
	}

	// Determine authority level and trust
	if user.IsVenueAdmin {
		authority.AuthorityLevel = "venue_admin"
		authority.BonusPoints = 50 // Near-automatic approval
		authority.TrustLevel = 1.0

	} else if user.AmbassadorLevel != nil && user.AmbassadorPoints != nil {
		// Analyze ambassador authority
		ambassadorInfo := de.analyzeAmbassadorAuthority(venue, user)
		authority.AmbassadorInfo = ambassadorInfo

		if ambassadorInfo.IsHighRanking && ambassadorInfo.RegionMatches {
			authority.AuthorityLevel = "high_ambassador"
			authority.BonusPoints = 30
			authority.TrustLevel = 0.8
		} else {
			authority.AuthorityLevel = "ambassador"
			authority.BonusPoints = 15
			authority.TrustLevel = 0.6
		}

	} else if user.Trusted {
		authority.AuthorityLevel = "trusted"
		authority.BonusPoints = 10
		authority.TrustLevel = 0.7

	} else {
		authority.AuthorityLevel = "regular"
		authority.BonusPoints = 0
		authority.TrustLevel = 0.3
	}

	// Adjust trust based on contribution history
	if user.Contributions > 100 {
		authority.TrustLevel += 0.1
	}
	if user.Contributions > 500 {
		authority.TrustLevel += 0.1
	}

	// Cap trust level at 1.0
	if authority.TrustLevel > 1.0 {
		authority.TrustLevel = 1.0
	}

	return authority
}

// analyzeAmbassadorAuthority determines ambassador-specific authority
func (de *DecisionEngine) analyzeAmbassadorAuthority(venue models.Venue, user models.User) *AmbassadorAuthority {
	info := &AmbassadorAuthority{
		Level:         user.AmbassadorLevel,
		Points:        user.AmbassadorPoints,
		Region:        user.AmbassadorRegion,
		IsHighRanking: false,
		RegionMatches: false,
	}

	// Determine if ambassador is high-ranking
	if user.AmbassadorLevel != nil && user.AmbassadorPoints != nil {
		level := *user.AmbassadorLevel
		points := *user.AmbassadorPoints

		// High-ranking criteria (adjust thresholds as needed)
		info.IsHighRanking = level >= 3 || points >= 1000
	}

	// Check if ambassador region matches venue location
	if user.AmbassadorRegion != nil && venue.Location != "" {
		region := strings.ToLower(*user.AmbassadorRegion)
		location := strings.ToLower(venue.Location)

		// Simple region matching (can be enhanced with more sophisticated logic)
		if strings.Contains(location, region) || strings.Contains(region, location) {
			info.RegionMatches = true
		}

		// Check for country-level matches
		regionCountries := []string{"korea", "korean", "china", "chinese", "japan", "japanese"}
		for _, country := range regionCountries {
			if strings.Contains(region, country) && strings.Contains(location, country) {
				info.RegionMatches = true
				break
			}
		}
	}

	return info
}

// applyAuthorityBonuses adds authority-based score bonuses
func (de *DecisionEngine) applyAuthorityBonuses(baseScore int, authority *AuthorityInfo) int {
	if !de.enableAuthorityMode {
		return baseScore
	}

	enhancedScore := baseScore + authority.BonusPoints

	// Cap score at 100
	if enhancedScore > 100 {
		enhancedScore = 100
	}

	return enhancedScore
}

// detectSpecialCases identifies venues requiring special handling
func (de *DecisionEngine) detectSpecialCases(venue models.Venue) []string {
	var flags []string

	if !de.enableSpecialCases {
		return flags
	}

	location := strings.ToLower(venue.Location)
	name := strings.ToLower(venue.Name)

	// Korean/Chinese venue detection
	koreanIndicators := []string{"korea", "korean", "seoul", "busan", "대구", "부산", "서울"}
	chineseIndicators := []string{"china", "chinese", "beijing", "shanghai", "guangzhou", "中国", "北京", "上海"}

	for _, indicator := range koreanIndicators {
		if strings.Contains(location, indicator) || strings.Contains(name, indicator) {
			flags = append(flags, "korean_venue")
			break
		}
	}

	for _, indicator := range chineseIndicators {
		if strings.Contains(location, indicator) || strings.Contains(name, indicator) {
			flags = append(flags, "chinese_venue")
			break
		}
	}

	// New business detection (less than 6 months old)
	if venue.CreatedAt != nil {
		sixMonthsAgo := time.Now().AddDate(0, -6, 0)
		if venue.CreatedAt.After(sixMonthsAgo) {
			flags = append(flags, "new_business")
		}
	}

	// Minimal data detection
	if venue.Phone == nil && venue.URL == nil {
		flags = append(flags, "minimal_contact_info")
	}

	// Suspicious patterns
	if venue.AdditionalInfo != nil {
		info := strings.ToLower(*venue.AdditionalInfo)
		suspiciousPatterns := []string{"test", "fake", "spam", "promotional"}
		for _, pattern := range suspiciousPatterns {
			if strings.Contains(info, pattern) {
				flags = append(flags, "suspicious_content")
				break
			}
		}
	}

	return flags
}

// detectQualityFlags identifies data quality issues
func (de *DecisionEngine) detectQualityFlags(venue models.Venue, validation *models.ValidationResult) []string {
	var flags []string

	// Google data availability
	if venue.ValidationDetails != nil {
		if !venue.ValidationDetails.GooglePlaceFound {
			flags = append(flags, "no_google_data")
		}

		if len(venue.ValidationDetails.Conflicts) > 3 {
			flags = append(flags, "multiple_conflicts")
		}

		// Distance check
		if venue.ValidationDetails.DistanceMeters > 500 {
			flags = append(flags, "location_mismatch")
		}
	}

	// Score distribution analysis
	if validation != nil && validation.ScoreBreakdown != nil {
		breakdown := validation.ScoreBreakdown

		// Check for zero scores in critical areas
		criticalScores := map[string]int{
			"venue_name_match":     breakdown["venue_name_match"],
			"address_accuracy":     breakdown["address_accuracy"],
			"geolocation_accuracy": breakdown["geolocation_accuracy"],
			"vegan_relevance":      breakdown["vegan_relevance"],
		}

		for field, score := range criticalScores {
			if score == 0 {
				flags = append(flags, fmt.Sprintf("zero_%s", field))
			}
		}
	}

	// Missing critical data
	if venue.Name == "" {
		flags = append(flags, "missing_name")
	}
	if venue.Location == "" {
		flags = append(flags, "missing_location")
	}
	if venue.Lat == nil || venue.Lng == nil {
		flags = append(flags, "missing_coordinates")
	}

	return flags
}

// DecisionOutcome represents the final decision
type DecisionOutcome struct {
	Status         string
	Reason         string
	RequiresReview bool
	ReviewReason   string
}

// determineStatus makes the final approval/rejection decision
func (de *DecisionEngine) determineStatus(venue models.Venue, user models.User, score int, authority *AuthorityInfo, specialCases, qualityFlags []string) DecisionOutcome {

	// Authority-based auto-approval rules (highest priority)
	if de.enableAuthorityMode {
		if authority.AuthorityLevel == "venue_admin" && de.hasCompleteCriticalData(venue) {
			return DecisionOutcome{
				Status: "approved",
				Reason: fmt.Sprintf("Auto-approved: Venue admin with complete data (score: %d)", score),
			}
		}

		if authority.AuthorityLevel == "high_ambassador" && authority.AmbassadorInfo.RegionMatches && de.hasCompleteCriticalData(venue) {
			return DecisionOutcome{
				Status: "approved",
				Reason: fmt.Sprintf("Auto-approved: High-ranking regional ambassador with complete data (score: %d)", score),
			}
		}
	}

	// Special case handling (Korean/Chinese venues)
	if de.enableSpecialCases {
		for _, flag := range specialCases {
			if flag == "korean_venue" || flag == "chinese_venue" {
				// Only auto-approve Korean/Chinese venues if venue admin
				if authority.AuthorityLevel != "venue_admin" {
					return DecisionOutcome{
						Status:         "manual_review",
						Reason:         fmt.Sprintf("Manual review required: %s venue (language barriers)", strings.Title(strings.TrimSuffix(flag, "_venue"))),
						RequiresReview: true,
						ReviewReason:   "Korean/Chinese venue requires manual validation unless submitted by venue admin",
					}
				}
			}
		}
	}

	// Quality-based mandatory manual review
	for _, flag := range qualityFlags {
		switch flag {
		case "no_google_data":
			return DecisionOutcome{
				Status:         "manual_review",
				Reason:         fmt.Sprintf("Manual review required: No Google data found (score: %d)", score),
				RequiresReview: true,
				ReviewReason:   "Unable to verify venue information through Google Places",
			}
		case "multiple_conflicts":
			return DecisionOutcome{
				Status:         "manual_review",
				Reason:         fmt.Sprintf("Manual review required: Multiple data conflicts (score: %d)", score),
				RequiresReview: true,
				ReviewReason:   "Significant discrepancies between submitted and Google data",
			}
		case "location_mismatch":
			return DecisionOutcome{
				Status:         "manual_review",
				Reason:         fmt.Sprintf("Manual review required: Location mismatch >500m (score: %d)", score),
				RequiresReview: true,
				ReviewReason:   "Venue location significantly different from Google Places data",
			}
		case "suspicious_content":
			return DecisionOutcome{
				Status:         "manual_review",
				Reason:         fmt.Sprintf("Manual review required: Suspicious content detected (score: %d)", score),
				RequiresReview: true,
				ReviewReason:   "Venue submission contains potentially suspicious content",
			}
		}
	}

	// Special case flags that require review
	for _, flag := range specialCases {
		switch flag {
		case "new_business":
			if score < de.approvalThreshold {
				return DecisionOutcome{
					Status:         "manual_review",
					Reason:         fmt.Sprintf("Manual review required: New business with moderate score (score: %d)", score),
					RequiresReview: true,
					ReviewReason:   "New businesses require additional verification",
				}
			}
		}
	}

	// Score-based decision (final fallback)
	if score >= de.approvalThreshold {
		return DecisionOutcome{
			Status: "approved",
			Reason: fmt.Sprintf("Auto-approved: High confidence score (score: %d)", score),
		}
	} else if score < de.rejectionThreshold {
		// Only auto-reject if no special circumstances
		if len(specialCases) == 0 && authority.TrustLevel < 0.7 {
			return DecisionOutcome{
				Status: "rejected",
				Reason: fmt.Sprintf("Auto-rejected: Low confidence score (score: %d)", score),
			}
		} else {
			return DecisionOutcome{
				Status:         "manual_review",
				Reason:         fmt.Sprintf("Manual review required: Low score with special circumstances (score: %d)", score),
				RequiresReview: true,
				ReviewReason:   "Low score but special circumstances prevent auto-rejection",
			}
		}
	} else {
		// Medium score - manual review
		return DecisionOutcome{
			Status:         "manual_review",
			Reason:         fmt.Sprintf("Manual review required: Medium confidence score (score: %d)", score),
			RequiresReview: true,
			ReviewReason:   "Score in manual review range",
		}
	}
}

// hasCompleteCriticalData checks if venue has all critical data for authority-based approval
func (de *DecisionEngine) hasCompleteCriticalData(venue models.Venue) bool {
	if de.approvalSpec == nil {
		// Fallback to conservative check if spec not initialized
		return venue.Name != "" && venue.Location != "" && venue.Lat != nil && venue.Lng != nil
	}
	return de.approvalSpec.IsSatisfiedBy(context.TODO(), venue)
}

// GetDecisionSummary returns a human-readable summary of the decision logic
func (de *DecisionEngine) GetDecisionSummary() map[string]interface{} {
	return map[string]interface{}{
		"approval_threshold":     de.approvalThreshold,
		"rejection_threshold":    de.rejectionThreshold,
		"special_cases_enabled":  de.enableSpecialCases,
		"authority_mode_enabled": de.enableAuthorityMode,
		"decision_rules": map[string]string{
			"venue_admin_complete":     "Auto-approve venue admins with complete critical data",
			"high_ambassador_regional": "Auto-approve high-ranking regional ambassadors with complete data",
			"korean_chinese_special":   "Korean/Chinese venues require manual review unless venue admin",
			"no_google_data":           "Manual review if no Google Places data found",
			"multiple_conflicts":       "Manual review if >3 data conflicts with Google",
			"location_mismatch":        "Manual review if venue >500m from Google location",
			"score_based_approval":     fmt.Sprintf("Auto-approve if score >= %d", de.approvalThreshold),
			"score_based_rejection":    fmt.Sprintf("Auto-reject if score < %d (with conditions)", de.rejectionThreshold),
			"default":                  "Manual review for medium scores or special circumstances",
		},
	}
}
