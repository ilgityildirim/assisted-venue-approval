package constants

// Centralized threshold values used across the application.
// Keep these stable; change deliberately and document why.
// These are not configuration knobs; use pkg/config for env-driven settings.

const (
	// Trust levels (0.0 - 1.0)
	BaseRegularTrust = 0.0
	TrustedTrust     = 0.7
	AmbassadorTrust  = 0.6
	HighAmbTrust     = 0.8

	// Ambassador high-rank thresholds
	AmbHighLevel  = 3
	AmbHighPoints = 1000

	// Contribution-based boost thresholds
	ContributionBoost1Threshold = 1000
	ContributionBoost2Threshold = 5000
	ContributionBoostStep       = 0.1

	// Approved venue boost thresholds
	ApprovedVenueBoost1Threshold = 2
	ApprovedVenueBoost2Threshold = 5
	ApprovedVenueBoost3Threshold = 10
	ApprovedVenueBoostStep       = 0.15

	// Decision bonuses (points) for authority
	BonusVenueAdmin = 50
	BonusHighAmb    = 30
	BonusAmb        = 15
	BonusTrusted    = 10
	BonusRegular    = 0

	// Decision trust gate: minimum trust to allow auto-reject at low score
	DecisionTrustGate = 0.7

	// Circuit breaker rate thresholds
	CircuitFailureRate        = 0.6 // default for external HTTP
	CircuitSlowCallRate       = 0.7
	OpenAICircuitFailureRate  = 0.5
	OpenAICircuitSlowCallRate = 0.5
)
