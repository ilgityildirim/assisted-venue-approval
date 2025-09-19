package constants

import "time"

// Centralized default values for timeouts, intervals, and related settings.
// These provide sane defaults; environment/config may override where supported.

const (
	// Database
	DBReadTimeoutDefault  = 8 * time.Second
	DBWriteTimeoutDefault = 6 * time.Second

	// Google Maps
	GoogleMapsOperationTimeout  = 10 * time.Second
	GoogleMapsOpenFor           = 30 * time.Second
	GoogleMapsRequestTimeout    = 12 * time.Second
	GoogleMapsSlowCallThreshold = 1500 * time.Millisecond

	// AI Scorer / OpenAI
	AIScorerDefaultAPITimeout = 60 * time.Second
	AIScorerOperationTimeout  = 50 * time.Second
	AIScorerOpenFor           = 45 * time.Second
	AIScorerSlowCallThreshold = 20 * time.Second

	// Health
	HealthTimeoutDefault = 30 * time.Second

	// Processing engine
	ProcessorRetryDelayDefault = 5 * time.Second
	ProcessorJobTimeoutDefault = 90 * time.Second

	// Config watcher
	ConfigWatcherIntervalDefault = 2 * time.Second

	// App shutdown
	GracefulShutdownTimeoutDefault = 10 * time.Second

	// Events store SQL operations
	EventsSQLTimeoutDefault = 5 * time.Second

	// Monitoring
	MonitoringIntervalDefault = 5 * time.Second
)
