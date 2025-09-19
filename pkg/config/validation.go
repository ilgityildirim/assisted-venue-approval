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
	return fmt.Sprintf("%s=%q: %s", e.Field, e.Value, e.Message)
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
	v := NewConfigValidator()
	// TODO: make this pluggable if we ever need fancy config rules

	c.validateRequired(v)
	c.validateFormats(v)
	c.validateRanges(v)
	c.validateEnvironment(v)

	if v.HasErrors() {
		fmt.Printf("config: %d errs\n", len(v.errors))
		return errs.NewValidation("cfg.Validate", v.GetErrorsAsString(), nil)
	}
	return nil
}

// validateRequired checks required configuration fields
func (c *Config) validateRequired(v *ConfigValidator) {
	if c.DatabaseURL == "" {
		v.AddError("DATABASE_URL", c.DatabaseURL, "required")
	}
	if c.GoogleMapsAPIKey == "" {
		v.AddError("GOOGLE_MAPS_API_KEY", c.GoogleMapsAPIKey, "required")
	}
	if c.OpenAIAPIKey == "" {
		v.AddError("OPENAI_API_KEY", c.OpenAIAPIKey, "required")
	}
	if c.Port == "" {
		v.AddError("PORT", c.Port, "required")
	}
}

// validateFormats checks format validity of configuration values
func (c *Config) validateFormats(v *ConfigValidator) {
	if c.DatabaseURL != "" {
		if !strings.Contains(c.DatabaseURL, "@") || !strings.Contains(c.DatabaseURL, "/") {
			v.AddError("DATABASE_URL", c.DatabaseURL, "bad db url")
		}
	}
	if c.Port != "" {
		if port, err := strconv.Atoi(c.Port); err != nil || port < 1 || port > 65535 {
			v.AddError("PORT", c.Port, "bad port (1-65535)")
		}
	}
	if c.HealthCheckPort != "" {
		if port, err := strconv.Atoi(c.HealthCheckPort); err != nil || port < 1 || port > 65535 {
			v.AddError("HEALTH_CHECK_PORT", c.HealthCheckPort, "bad health port")
		}
	}
	if (c.ProfilingEnabled || c.MetricsEnabled) && c.ProfilingPort != "" {
		if port, err := strconv.Atoi(c.ProfilingPort); err != nil || port < 1 || port > 65535 {
			v.AddError("PROFILING_PORT", c.ProfilingPort, "bad profiling port")
		}
	}
	validLogLevels := []string{"trace", "debug", "info", "warn", "error", "fatal"}
	if c.LogLevel != "" && !contains(validLogLevels, strings.ToLower(c.LogLevel)) {
		v.AddError("LOG_LEVEL", c.LogLevel, "bad log level")
	}
	if c.LogFormat != "" && c.LogFormat != "json" && c.LogFormat != "text" {
		v.AddError("LOG_FORMAT", c.LogFormat, "bad log format")
	}
}

// validateRanges checks value ranges
func (c *Config) validateRanges(v *ConfigValidator) {
	if c.ApprovalThreshold < 0 || c.ApprovalThreshold > 100 {
		v.AddError("APPROVAL_THRESHOLD", strconv.Itoa(c.ApprovalThreshold), "out of range (0-100)")
	}
	if c.WorkerCount < 0 || c.WorkerCount > 100 {
		v.AddError("WORKER_COUNT", strconv.Itoa(c.WorkerCount), "out of range (0-100)")
	}
	if c.DBMaxOpenConns < 1 || c.DBMaxOpenConns > 1000 {
		v.AddError("DB_MAX_OPEN_CONNS", strconv.Itoa(c.DBMaxOpenConns), "out of range (1-1000)")
	}
	if c.DBMaxIdleConns < 0 || c.DBMaxIdleConns > c.DBMaxOpenConns {
		v.AddError("DB_MAX_IDLE_CONNS", strconv.Itoa(c.DBMaxIdleConns), "must be 0..max_open")
	}
	if c.DBConnMaxLifetime < 1 || c.DBConnMaxLifetime > 60 {
		v.AddError("DB_CONN_MAX_LIFETIME_MINUTES", strconv.Itoa(c.DBConnMaxLifetime), "out of range (1-60m)")
	}
	if c.DBConnMaxIdleTime < 1 || c.DBConnMaxIdleTime > 30 {
		v.AddError("DB_CONN_MAX_IDLE_TIME_MINUTES", strconv.Itoa(c.DBConnMaxIdleTime), "out of range (1-30m)")
	}
}

// validateEnvironment performs environment-specific validation
func (c *Config) validateEnvironment(v *ConfigValidator) {
	if c.EnableFileLogging && c.LogFile != "" {
		if err := checkDirectoryWritable(c.LogFile); err != nil {
			v.AddError("LOG_FILE", c.LogFile, fmt.Sprintf("not writable: %v", err))
		}
	}

	ports := map[string]string{
		"PORT":              c.Port,
		"HEALTH_CHECK_PORT": c.HealthCheckPort,
	}
	if (c.ProfilingEnabled || c.MetricsEnabled) && c.ProfilingPort != "" && c.ProfilingPort != "0" {
		ports["PROFILING_PORT"] = c.ProfilingPort
	}

	used := make(map[string]string)
	for name, port := range ports {
		if port != "" && port != "0" {
			if exist, ok := used[port]; ok {
				v.AddError(name, port, fmt.Sprintf("conflicts with %s", exist))
			} else {
				used[port] = name
			}
		}
	}
}

// checkDirectoryWritable checks if a directory is writable
func checkDirectoryWritable(path string) error {
	dir := path
	if !strings.HasSuffix(path, "/") {
		if i := strings.LastIndex(path, "/"); i > 0 {
			dir = path[:i]
		} else {
			dir = "."
		}
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return errs.NewValidation("cfg.dirWritable", "mkdir", err)
		}
	}

	tmp := fmt.Sprintf("%s/.write_test_%d", dir, os.Getpid())
	f, err := os.Create(tmp)
	if err != nil {
		return errs.NewValidation("cfg.dirWritable", "not writable", err)
	}
	f.Close()
	os.Remove(tmp)
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
		// Profiling & metrics
		"env":               c.Env,
		"profiling_enabled": c.ProfilingEnabled,
		"profiling_port":    c.ProfilingPort,
		"metrics_enabled":   c.MetricsEnabled,
		"metrics_path":      c.MetricsPath,
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
