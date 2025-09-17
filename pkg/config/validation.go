package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	errs "assisted-venue-approval/pkg/errors"
)

// ValidationError represents a configuration validation error
type ValidationError struct {
	Field   string
	Value   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("config validation error for field '%s' with value '%s': %s", e.Field, e.Value, e.Message)
}

// ConfigValidator handles configuration validation
type ConfigValidator struct {
	errors []ValidationError
}

// NewConfigValidator creates a new configuration validator
func NewConfigValidator() *ConfigValidator {
	return &ConfigValidator{
		errors: make([]ValidationError, 0),
	}
}

// AddError adds a validation error
func (cv *ConfigValidator) AddError(field, value, message string) {
	cv.errors = append(cv.errors, ValidationError{
		Field:   field,
		Value:   value,
		Message: message,
	})
}

// HasErrors returns true if there are validation errors
func (cv *ConfigValidator) HasErrors() bool {
	return len(cv.errors) > 0
}

// GetErrors returns all validation errors
func (cv *ConfigValidator) GetErrors() []ValidationError {
	return cv.errors
}

// GetErrorsAsString returns all validation errors as a formatted string
func (cv *ConfigValidator) GetErrorsAsString() string {
	var errorStrings []string
	for _, err := range cv.errors {
		errorStrings = append(errorStrings, err.Error())
	}
	return strings.Join(errorStrings, "\n")
}

// Validate validates the entire configuration
func (c *Config) Validate() error {
	validator := NewConfigValidator()

	// Validate required fields
	c.validateRequired(validator)

	// Validate formats and values
	c.validateFormats(validator)

	// Validate ranges
	c.validateRanges(validator)

	// Check for environment-specific validation
	c.validateEnvironment(validator)

	if validator.HasErrors() {
		return errs.NewValidation("config.Validate", fmt.Sprintf("configuration validation failed:\n%s", validator.GetErrorsAsString()), nil)
	}

	return nil
}

// validateRequired checks required configuration fields
func (c *Config) validateRequired(validator *ConfigValidator) {
	// Database URL is required
	if c.DatabaseURL == "" {
		validator.AddError("DATABASE_URL", c.DatabaseURL, "database URL is required")
	}

	// API keys are required
	if c.GoogleMapsAPIKey == "" {
		validator.AddError("GOOGLE_MAPS_API_KEY", c.GoogleMapsAPIKey, "Google Maps API key is required")
	}

	if c.OpenAIAPIKey == "" {
		validator.AddError("OPENAI_API_KEY", c.OpenAIAPIKey, "OpenAI API key is required")
	}

	// Port is required
	if c.Port == "" {
		validator.AddError("PORT", c.Port, "port is required")
	}
}

// validateFormats checks format validity of configuration values
func (c *Config) validateFormats(validator *ConfigValidator) {
	// Validate database URL format
	if c.DatabaseURL != "" {
		if !strings.Contains(c.DatabaseURL, "@") || !strings.Contains(c.DatabaseURL, "/") {
			validator.AddError("DATABASE_URL", c.DatabaseURL, "invalid database URL format")
		}
	}

	// Validate port format
	if c.Port != "" {
		if port, err := strconv.Atoi(c.Port); err != nil || port < 1 || port > 65535 {
			validator.AddError("PORT", c.Port, "invalid port number (must be 1-65535)")
		}
	}

	// Validate health check port
	if c.HealthCheckPort != "" {
		if port, err := strconv.Atoi(c.HealthCheckPort); err != nil || port < 1 || port > 65535 {
			validator.AddError("HEALTH_CHECK_PORT", c.HealthCheckPort, "invalid health check port number")
		}
	}

	// Validate log level
	validLogLevels := []string{"trace", "debug", "info", "warn", "error", "fatal"}
	if c.LogLevel != "" && !contains(validLogLevels, strings.ToLower(c.LogLevel)) {
		validator.AddError("LOG_LEVEL", c.LogLevel, "invalid log level (must be one of: trace, debug, info, warn, error, fatal)")
	}

	// Validate log format
	if c.LogFormat != "" && c.LogFormat != "json" && c.LogFormat != "text" {
		validator.AddError("LOG_FORMAT", c.LogFormat, "invalid log format (must be 'json' or 'text')")
	}

}

// validateRanges checks value ranges
func (c *Config) validateRanges(validator *ConfigValidator) {
	// Validate approval threshold
	if c.ApprovalThreshold < 0 || c.ApprovalThreshold > 100 {
		validator.AddError("APPROVAL_THRESHOLD", strconv.Itoa(c.ApprovalThreshold), "approval threshold must be between 0 and 100")
	}

	// Validate worker count
	if c.WorkerCount < 0 || c.WorkerCount > 100 {
		validator.AddError("WORKER_COUNT", strconv.Itoa(c.WorkerCount), "worker count must be between 0 and 100")
	}

	// Validate database connection settings
	if c.DBMaxOpenConns < 1 || c.DBMaxOpenConns > 1000 {
		validator.AddError("DB_MAX_OPEN_CONNS", strconv.Itoa(c.DBMaxOpenConns), "max open connections must be between 1 and 1000")
	}

	if c.DBMaxIdleConns < 0 || c.DBMaxIdleConns > c.DBMaxOpenConns {
		validator.AddError("DB_MAX_IDLE_CONNS", strconv.Itoa(c.DBMaxIdleConns), "max idle connections must be between 0 and max open connections")
	}

	if c.DBConnMaxLifetime < 1 || c.DBConnMaxLifetime > 60 {
		validator.AddError("DB_CONN_MAX_LIFETIME_MINUTES", strconv.Itoa(c.DBConnMaxLifetime), "connection max lifetime must be between 1 and 60 minutes")
	}

	if c.DBConnMaxIdleTime < 1 || c.DBConnMaxIdleTime > 30 {
		validator.AddError("DB_CONN_MAX_IDLE_TIME_MINUTES", strconv.Itoa(c.DBConnMaxIdleTime), "connection max idle time must be between 1 and 30 minutes")
	}
}

// validateEnvironment performs environment-specific validation
func (c *Config) validateEnvironment(validator *ConfigValidator) {
	// Check if log directory is writable if file logging is enabled
	if c.EnableFileLogging && c.LogFile != "" {
		if err := checkDirectoryWritable(c.LogFile); err != nil {
			validator.AddError("LOG_FILE", c.LogFile, fmt.Sprintf("log directory is not writable: %v", err))
		}
	}

	// Validate port conflicts
	ports := map[string]string{
		"PORT":              c.Port,
		"HEALTH_CHECK_PORT": c.HealthCheckPort,
	}

	usedPorts := make(map[string]string)
	for name, port := range ports {
		if port != "" && port != "0" {
			if existing, exists := usedPorts[port]; exists {
				validator.AddError(name, port, fmt.Sprintf("port conflict with %s", existing))
			} else {
				usedPorts[port] = name
			}
		}
	}
}

// checkDirectoryWritable checks if a directory is writable
func checkDirectoryWritable(filePath string) error {
	// Extract directory from file path
	dir := filePath
	if !strings.HasSuffix(filePath, "/") {
		// It's a file path, get the directory
		lastSlash := strings.LastIndex(filePath, "/")
		if lastSlash > 0 {
			dir = filePath[:lastSlash]
		} else {
			dir = "."
		}
	}

	// Check if directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		// Try to create directory
		if err := os.MkdirAll(dir, 0755); err != nil {
			return errs.NewValidation("config.checkDirectoryWritable", "cannot create directory", err)
		}
	}

	// Test write permission by creating a temporary file
	tempFile := fmt.Sprintf("%s/.write_test_%d", dir, os.Getpid())
	file, err := os.Create(tempFile)
	if err != nil {
		return errs.NewValidation("config.checkDirectoryWritable", "directory is not writable", err)
	}
	file.Close()
	os.Remove(tempFile)

	return nil
}

// contains checks if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// GetConfigSummary returns a summary of the configuration (excluding sensitive data)
func (c *Config) GetConfigSummary() map[string]interface{} {
	return map[string]interface{}{
		"database_url":        maskString(c.DatabaseURL, 20),
		"google_maps_api_key": maskString(c.GoogleMapsAPIKey, 10),
		"openai_api_key":      maskString(c.OpenAIAPIKey, 10),
		"port":                c.Port,
		"approval_threshold":  c.ApprovalThreshold,
		"worker_count":        c.WorkerCount,
		"db_max_open_conns":   c.DBMaxOpenConns,
		"db_max_idle_conns":   c.DBMaxIdleConns,
		"log_level":           c.LogLevel,
		"log_format":          c.LogFormat,
		"log_file":            c.LogFile,
		"enable_file_logging": c.EnableFileLogging,
		"health_check_port":   c.HealthCheckPort,
	}
}

// maskString masks sensitive strings for logging/display
func maskString(s string, keepFirst int) string {
	if s == "" {
		return ""
	}
	if len(s) <= keepFirst {
		return strings.Repeat("*", len(s))
	}
	return s[:keepFirst] + strings.Repeat("*", len(s)-keepFirst)
}
