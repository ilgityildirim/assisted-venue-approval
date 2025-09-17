package config

import (
	"os"
	"strconv"
	"strings"
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

	// Monitoring and logging settings
	LogLevel          string
	LogFormat         string // "json" or "text"
	LogFile           string
	EnableFileLogging bool

	// Health check settings
	HealthCheckPort string
	HealthCheckPath string

	// Environment & profiling/metrics
	Env              string // development, staging, production
	ProfilingEnabled bool
	ProfilingPort    string
	MetricsEnabled   bool
	MetricsPath      string
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

	return &Config{
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

		// Monitoring and logging settings
		LogLevel:          getEnv("LOG_LEVEL", "info"),
		LogFormat:         getEnv("LOG_FORMAT", "json"),
		LogFile:           getEnv("LOG_FILE", "/var/log/venue-validation/app.log"),
		EnableFileLogging: enableFileLogging,

		// Health check settings
		HealthCheckPort: getEnv("HEALTH_CHECK_PORT", "8081"),
		HealthCheckPath: getEnv("HEALTH_CHECK_PATH", "/health"),

		// Environment & profiling/metrics
		Env:              env,
		ProfilingEnabled: profilingEnabled,
		ProfilingPort:    profPort,
		MetricsEnabled:   metricsEnabled,
		MetricsPath:      metricsPath,
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
