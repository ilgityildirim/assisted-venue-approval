package trust

import (
	"fmt"
	"strings"

	"assisted-venue-approval/internal/models"
)

// Assessment is the unified result of user trust evaluation.
// Trust is 0.0-1.0. Authority is one of: "venue_admin", "high_ambassador", "ambassador", "trusted", "regular".
// Bonus holds decision bonus points aligned with decision engine expectations.
// Reason gives a concise human-friendly explanation for logs/debug.
// Note: Keep this small and stable; expand only with clear need.
// TODO: Consider exposing JSON tags if we decide to persist assessments.
type Assessment struct {
	Trust     float64
	Authority string
	Bonus     int
	Reason    string
}

// Config allows tuning the calculator without code changes.
// Defaults mirror existing rules used across the codebase.
type Config struct {
	BaseRegularTrust float64
	TrustedTrust     float64
	AmbassadorTrust  float64
	HighAmbTrust     float64

	ContributionBoost1Threshold int
	ContributionBoost2Threshold int
	ContributionBoostStep       float64

	ApprovedVenueBoost1Threshold int
	ApprovedVenueBoost2Threshold int
	ApprovedVenueBoost3Threshold int
	ApprovedVenueBoostStep       float64

	BonusVenueAdmin int
	BonusHighAmb    int
	BonusAmb        int
	BonusTrusted    int
	BonusRegular    int
}

// DefaultConfig returns thresholds that match existing logic.
func DefaultConfig() Config {
	return Config{
		BaseRegularTrust:             0.3,
		TrustedTrust:                 0.7,
		AmbassadorTrust:              0.6,
		HighAmbTrust:                 0.8,
		ContributionBoost1Threshold:  100,
		ContributionBoost2Threshold:  500,
		ContributionBoostStep:        0.1,
		ApprovedVenueBoost1Threshold: 2,
		ApprovedVenueBoost2Threshold: 5,
		ApprovedVenueBoost3Threshold: 10,
		ApprovedVenueBoostStep:       0.15,
		BonusVenueAdmin:              50,
		BonusHighAmb:                 30,
		BonusAmb:                     15,
		BonusTrusted:                 10,
		BonusRegular:                 0,
	}
}

// Calculator computes user trust/authority consistently.
type Calculator struct {
	cfg Config
}

func NewCalculator(cfg Config) *Calculator { return &Calculator{cfg: cfg} }
func NewDefault() *Calculator              { return NewCalculator(DefaultConfig()) }

// Assess computes the trust assessment for a user.
// venueLocation is optional but recommended for regional ambassador matching.
func (c *Calculator) Assess(user models.User, venueLocation string) Assessment {
	// Venue admin: highest trust and bonus
	if user.IsVenueAdmin {
		return Assessment{
			Trust:     1.0,
			Authority: "venue_admin",
			Bonus:     c.cfg.BonusVenueAdmin,
			Reason:    "venue admin submitted the venue",
		}
	}

	// Ambassador logic (level/points may be nil)
	if user.AmbassadorLevel != nil && user.AmbassadorPoints != nil {
		isHigh := (*user.AmbassadorLevel >= 3) || (*user.AmbassadorPoints >= 1000)
		regionMatch := c.matchesRegion(user.AmbassadorRegion, venueLocation)

		if isHigh && regionMatch {
			trust := c.cfg.HighAmbTrust
			trust = c.applyContributionBoosts(trust, user.Contributions)
			approvedCount := 0
			if user.ApprovedVenueCount != nil {
				approvedCount = *user.ApprovedVenueCount
			}
			trust = c.applyApprovedVenueBoosts(trust, approvedCount)
			return Assessment{
				Trust:     trust,
				Authority: "high_ambassador",
				Bonus:     c.cfg.BonusHighAmb,
				Reason:    c.buildAmbReason(isHigh, regionMatch, trust, approvedCount),
			}
		}

		trust := c.cfg.AmbassadorTrust
		trust = c.applyContributionBoosts(trust, user.Contributions)
		approvedCount := 0
		if user.ApprovedVenueCount != nil {
			approvedCount = *user.ApprovedVenueCount
		}
		trust = c.applyApprovedVenueBoosts(trust, approvedCount)
		return Assessment{
			Trust:     trust,
			Authority: "ambassador",
			Bonus:     c.cfg.BonusAmb,
			Reason:    c.buildAmbReason(isHigh, regionMatch, trust, approvedCount),
		}
	}

	// Trusted member baseline
	if user.Trusted {
		trust := c.cfg.TrustedTrust
		trust = c.applyContributionBoosts(trust, user.Contributions)
		approvedCount := 0
		if user.ApprovedVenueCount != nil {
			approvedCount = *user.ApprovedVenueCount
		}
		trust = c.applyApprovedVenueBoosts(trust, approvedCount)
		return Assessment{
			Trust:     trust,
			Authority: "trusted",
			Bonus:     c.cfg.BonusTrusted,
			Reason:    c.buildTrustedReason(trust, user.Contributions, approvedCount),
		}
	}

	// Regular user baseline
	trust := c.cfg.BaseRegularTrust
	trust = c.applyContributionBoosts(trust, user.Contributions)
	approvedCount := 0
	if user.ApprovedVenueCount != nil {
		approvedCount = *user.ApprovedVenueCount
	}
	trust = c.applyApprovedVenueBoosts(trust, approvedCount)
	return Assessment{
		Trust:     trust,
		Authority: "regular",
		Bonus:     c.cfg.BonusRegular,
		Reason:    c.buildRegularReason(trust, user.Contributions, approvedCount),
	}
}

func (c *Calculator) applyContributionBoosts(base float64, contrib int) float64 {
	trust := base
	if contrib > c.cfg.ContributionBoost1Threshold {
		trust += c.cfg.ContributionBoostStep
	}
	if contrib > c.cfg.ContributionBoost2Threshold {
		trust += c.cfg.ContributionBoostStep
	}
	if trust > 1.0 {
		trust = 1.0
	}
	if trust < 0.0 {
		trust = 0.0
	}
	return trust
}

func (c *Calculator) applyApprovedVenueBoosts(base float64, approvedCount int) float64 {
	trust := base
	if approvedCount >= c.cfg.ApprovedVenueBoost1Threshold {
		trust += c.cfg.ApprovedVenueBoostStep
	}
	if approvedCount >= c.cfg.ApprovedVenueBoost2Threshold {
		trust += c.cfg.ApprovedVenueBoostStep
	}
	if approvedCount >= c.cfg.ApprovedVenueBoost3Threshold {
		trust += c.cfg.ApprovedVenueBoostStep
	}
	if trust > 1.0 {
		trust = 1.0
	}
	return trust
}

func (c *Calculator) matchesRegion(userRegion *string, venueLocation string) bool {
	if userRegion == nil || *userRegion == "" || venueLocation == "" {
		return false
	}
	ur := strings.ToLower(strings.TrimSpace(*userRegion))
	vl := strings.ToLower(venueLocation)
	return strings.Contains(vl, ur)
}

// describeApproved returns a short descriptor for approved venue counts when
// they meet configured thresholds. Empty string otherwise.
func (c *Calculator) describeApproved(approvedCount int) string {
	if approvedCount >= c.cfg.ApprovedVenueBoost3Threshold {
		return ">=10 approved"
	}
	if approvedCount >= c.cfg.ApprovedVenueBoost2Threshold {
		return ">=5 approved"
	}
	if approvedCount >= c.cfg.ApprovedVenueBoost1Threshold {
		return ">=2 approved"
	}
	return ""
}

func (c *Calculator) buildAmbReason(isHigh, regionMatch bool, trust float64, approvedCount int) string {
	lvl := "ambassador"
	if isHigh && regionMatch {
		lvl = "high_ambassador"
	}
	why := []string{lvl}
	if isHigh {
		why = append(why, "high ranking")
	}
	if regionMatch {
		why = append(why, "region match")
	}
	// Mention approved venues if significant
	if ac := c.describeApproved(approvedCount); ac != "" {
		why = append(why, ac)
	}
	return fmt.Sprintf("%s (%s), trust=%.2f", lvl, strings.Join(why[1:], ", "), trust)
}

func (c *Calculator) buildTrustedReason(trust float64, contrib int, approvedCount int) string {
	parts := []string{"trusted member"}
	if contrib > c.cfg.ContributionBoost2Threshold {
		parts = append(parts, ">500 contrib")
	} else if contrib > c.cfg.ContributionBoost1Threshold {
		parts = append(parts, ">100 contrib")
	}
	if ac := c.describeApproved(approvedCount); ac != "" {
		parts = append(parts, ac)
	}
	return fmt.Sprintf("%s, trust=%.2f", strings.Join(parts, ", "), trust)
}

func (c *Calculator) buildRegularReason(trust float64, contrib int, approvedCount int) string {
	parts := []string{"regular"}
	if contrib > c.cfg.ContributionBoost2Threshold {
		parts = append(parts, ">500 contrib")
	} else if contrib > c.cfg.ContributionBoost1Threshold {
		parts = append(parts, ">100 contrib")
	}
	if ac := c.describeApproved(approvedCount); ac != "" {
		parts = append(parts, ac)
	}
	return fmt.Sprintf("%s, trust=%.2f", strings.Join(parts, ", "), trust)
}
