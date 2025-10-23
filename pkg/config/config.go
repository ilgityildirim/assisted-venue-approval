package config

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL       string
	GoogleMapsAPIKey  string
	OpenAIAPIKey      string
	Port              string
	ApprovalThreshold int
	WorkerCount       int
	// Database performance settings
	DBMaxOpenConns    int
	DBMaxIdleConns    int
	DBConnMaxLifetime int // minutes
	DBConnMaxIdleTime int // minutes
	DBReadTimeout     time.Duration
	DBWriteTimeout    time.Duration

	// OpenAI client settings
	OpenAITimeout time.Duration

	// Monitoring and logging settings
	LogLevel          string
	LogFormat         string // "json" or "text"
	LogFile           string
	EnableFileLogging bool

	// Health check settings
	HealthCheckPort string
	HealthCheckPath string

	// Web interface settings
	BasePath string

	// Environment & profiling/metrics
	Env              string // development, staging, production
	ProfilingEnabled bool
	ProfilingPort    string // also used as admin port
	MetricsEnabled   bool
	MetricsPath      string

	// Performance alerts
	AlertsEnabled    bool
	AlertP95Ms       float64       // trigger when p95 request duration exceeds this (ms)
	AlertGoroutines  int           // trigger when goroutine count exceeds this
	AlertMemAllocMB  float64       // trigger when Alloc exceeds this (MB)
	AlertGCPauseMs   float64       // trigger when last GC pause exceeds this (ms)
	AlertSampleEvery time.Duration // sampling interval

	// Prompts templates overrides
	PromptDir string // path to external templates dir; empty = use embedded only

	// New AI and prompt management knobs
	OpenAIModel                 string
	OpenAITemperature           float64
	OpenAIMaxTokens             int
	OpenAIRequestTimeoutSeconds int
	OpenAIMaxBatchSize          int
	PromptStableOnly            bool
	PromptWeights               string
	ConfigReloadIntervalSeconds int

	// Automatic Venue Approval (AVA) qualification requirements
	// MinUserPointsForAVA: Minimum ambassador points required for automated reviews (0 = disabled)
	MinUserPointsForAVA int
	// OnlyAmbassadors: If true, only ambassador submissions are eligible for automated review
	OnlyAmbassadors bool
}

func Load() *Config {
	threshold, _ := strconv.Atoi(getEnv("APPROVAL_THRESHOLD", "75"))
	workerCount, _ := strconv.Atoi(getEnv("WORKER_COUNT", "0")) // 0 = use default

	// Database performance settings with defaults
	dbMaxOpenConns, _ := strconv.Atoi(getEnv("DB_MAX_OPEN_CONNS", "50"))
	dbMaxIdleConns, _ := strconv.Atoi(getEnv("DB_MAX_IDLE_CONNS", "15"))
	dbConnMaxLifetime, _ := strconv.Atoi(getEnv("DB_CONN_MAX_LIFETIME_MINUTES", "10"))
	dbConnMaxIdleTime, _ := strconv.Atoi(getEnv("DB_CONN_MAX_IDLE_TIME_MINUTES", "5"))

	// Parse boolean environment variables
	enableFileLogging, _ := strconv.ParseBool(getEnv("ENABLE_FILE_LOGGING", "true"))

	// Environment and profiling defaults
	env := strings.ToLower(getEnv("ENV", "development"))
	profPort := getEnv("PROFILING_PORT", "6060")
	metricsPath := getEnv("METRICS_PATH", "/metrics")

	// Default toggles based on env
	profilingDefault := env == "development" || env == "staging"
	profilingEnabled, _ := strconv.ParseBool(getEnv("PROFILING_ENABLED", strconv.FormatBool(profilingDefault)))
	metricsDefault := profilingDefault
	metricsEnabled, _ := strconv.ParseBool(getEnv("METRICS_ENABLED", strconv.FormatBool(metricsDefault)))

	// Alerts defaults
	alertsDefault := profilingDefault
	alertsEnabled, _ := strconv.ParseBool(getEnv("ALERTS_ENABLED", strconv.FormatBool(alertsDefault)))
	alertP95Ms, _ := strconv.ParseFloat(getEnv("ALERT_P95_MS", "500"), 64)
	alertGoroutines, _ := strconv.Atoi(getEnv("ALERT_GOROUTINES", "500"))
	alertMemAllocMB, _ := strconv.ParseFloat(getEnv("ALERT_MEM_ALLOC_MB", "512"), 64)
	alertGCPauseMs, _ := strconv.ParseFloat(getEnv("ALERT_GC_PAUSE_MS", "200"), 64)
	alertSampleEverySec, _ := strconv.Atoi(getEnv("ALERT_SAMPLE_EVERY_SEC", "5"))

	// Timeouts
	dbReadTO, _ := time.ParseDuration(getEnv("DB_READ_TIMEOUT", "8s"))
	dbWriteTO, _ := time.ParseDuration(getEnv("DB_WRITE_TIMEOUT", "6s"))

	// New OpenAI config
	openAIModel := getEnv("OPENAI_MODEL", "gpt-4o-mini")
	openAITemp, _ := strconv.ParseFloat(getEnv("OPENAI_TEMPERATURE", "0.1"), 64)
	openAIMaxTokens, _ := strconv.Atoi(getEnv("OPENAI_MAX_TOKENS", "250"))
	openAIReqTimeoutSec, _ := strconv.Atoi(getEnv("OPENAI_REQUEST_TIMEOUT_SECONDS", "60"))
	openAIMaxBatchSize, _ := strconv.Atoi(getEnv("OPENAI_MAX_BATCH_SIZE", "5"))

	// Prompts
	promptDir := getEnv("PROMPT_DIR", "./prompts")
	promptStableOnly, _ := strconv.ParseBool(getEnv("PROMPT_STABLE_ONLY", "false"))
	promptWeights := getEnv("PROMPT_WEIGHTS", "")

	// Config reload
	reloadIntSec, _ := strconv.Atoi(getEnv("CONFIG_RELOAD_INTERVAL_SECONDS", "2"))

	// AVA qualification requirements
	minUserPoints, _ := strconv.Atoi(getEnv("MIN_USER_POINTS_FOR_AVA", "150"))
	onlyAmbassadors, _ := strconv.ParseBool(getEnv("ONLY_AMBASSADORS", "false"))

	// Validate AVA configuration
	if minUserPoints < 0 {
		log.Printf("[Warning] MIN_USER_POINTS_FOR_AVA is negative (%d), using 0 to disable check", minUserPoints)
		minUserPoints = 0
	}

	cfg := &Config{
		DatabaseURL:       getEnv("DATABASE_URL", ""),
		GoogleMapsAPIKey:  getEnv("GOOGLE_MAPS_API_KEY", ""),
		OpenAIAPIKey:      getEnv("OPENAI_API_KEY", ""),
		Port:              getEnv("PORT", "8080"),
		ApprovalThreshold: threshold,
		WorkerCount:       workerCount,
		DBMaxOpenConns:    dbMaxOpenConns,
		DBMaxIdleConns:    dbMaxIdleConns,
		DBConnMaxLifetime: dbConnMaxLifetime,
		DBConnMaxIdleTime: dbConnMaxIdleTime,
		DBReadTimeout:     dbReadTO,
		DBWriteTimeout:    dbWriteTO,
		OpenAITimeout:     time.Duration(openAIReqTimeoutSec) * time.Second,

		// Monitoring and logging settings
		LogLevel:          getEnv("LOG_LEVEL", "info"),
		LogFormat:         getEnv("LOG_FORMAT", "json"),
		LogFile:           getEnv("LOG_FILE", "/var/log/venue-validation/app.log"),
		EnableFileLogging: enableFileLogging,

		// Health check settings
		HealthCheckPort: getEnv("HEALTH_CHECK_PORT", "8081"),
		HealthCheckPath: getEnv("HEALTH_CHECK_PATH", "/health"),

		// Web interface settings
		BasePath: getEnv("BASE_PATH", "/"),

		// Environment & profiling/metrics
		Env:              env,
		ProfilingEnabled: profilingEnabled,
		ProfilingPort:    profPort,
		MetricsEnabled:   metricsEnabled,
		MetricsPath:      metricsPath,

		// Alerts
		AlertsEnabled:    alertsEnabled,
		AlertP95Ms:       alertP95Ms,
		AlertGoroutines:  alertGoroutines,
		AlertMemAllocMB:  alertMemAllocMB,
		AlertGCPauseMs:   alertGCPauseMs,
		AlertSampleEvery: time.Duration(alertSampleEverySec) * time.Second,

		// Prompts templates overrides and new knobs
		PromptDir:                   promptDir,
		OpenAIModel:                 openAIModel,
		OpenAITemperature:           openAITemp,
		OpenAIMaxTokens:             openAIMaxTokens,
		OpenAIRequestTimeoutSeconds: openAIReqTimeoutSec,
		OpenAIMaxBatchSize:          openAIMaxBatchSize,
		PromptStableOnly:            promptStableOnly,
		PromptWeights:               promptWeights,
		ConfigReloadIntervalSeconds: reloadIntSec,

		// AVA qualification requirements
		MinUserPointsForAVA: minUserPoints,
		OnlyAmbassadors:     onlyAmbassadors,
	}

	log.Printf("Config: MinUserPointsForAVA=%d, OnlyAmbassadors=%v",
		cfg.MinUserPointsForAVA, cfg.OnlyAmbassadors)

	return cfg
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
