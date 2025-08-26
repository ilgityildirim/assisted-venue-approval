package health

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"automatic-vendor-validation/pkg/logging"
)

// HealthStatus represents the health status of a component
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
	HealthStatusUnknown   HealthStatus = "unknown"
)

// ComponentHealth represents the health of a single component
type ComponentHealth struct {
	Name        string                 `json:"name"`
	Status      HealthStatus           `json:"status"`
	Message     string                 `json:"message,omitempty"`
	LastChecked time.Time              `json:"last_checked"`
	Duration    time.Duration          `json:"duration"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Error       string                 `json:"error,omitempty"`
}

// SystemHealth represents the overall system health
type SystemHealth struct {
	Status     HealthStatus               `json:"status"`
	Timestamp  time.Time                  `json:"timestamp"`
	Version    string                     `json:"version,omitempty"`
	Uptime     time.Duration              `json:"uptime"`
	Components map[string]ComponentHealth `json:"components"`
	Summary    HealthSummary              `json:"summary"`
}

// HealthSummary provides aggregated health information
type HealthSummary struct {
	TotalComponents int `json:"total_components"`
	HealthyCount    int `json:"healthy_count"`
	DegradedCount   int `json:"degraded_count"`
	UnhealthyCount  int `json:"unhealthy_count"`
	UnknownCount    int `json:"unknown_count"`
}

// HealthChecker defines the interface for health check functions
type HealthChecker interface {
	Check(ctx context.Context) ComponentHealth
	Name() string
}

// HealthCheckFunc is a function that implements HealthChecker
type HealthCheckFunc struct {
	name string
	fn   func(ctx context.Context) ComponentHealth
}

func (hcf HealthCheckFunc) Check(ctx context.Context) ComponentHealth {
	return hcf.fn(ctx)
}

func (hcf HealthCheckFunc) Name() string {
	return hcf.name
}

// NewHealthCheckFunc creates a new HealthCheckFunc
func NewHealthCheckFunc(name string, fn func(ctx context.Context) ComponentHealth) HealthChecker {
	return HealthCheckFunc{name: name, fn: fn}
}

// HealthManager manages health checks for all system components
type HealthManager struct {
	checkers  map[string]HealthChecker
	results   map[string]ComponentHealth
	startTime time.Time
	version   string
	timeout   time.Duration
	logger    *logging.ComponentLogger
	mu        sync.RWMutex
}

// HealthConfig holds configuration for the health manager
type HealthConfig struct {
	Timeout time.Duration `json:"timeout"`
	Version string        `json:"version"`
}

// DefaultHealthConfig returns sensible defaults
func DefaultHealthConfig() HealthConfig {
	return HealthConfig{
		Timeout: 30 * time.Second,
		Version: "1.0.0",
	}
}

// NewHealthManager creates a new health manager
func NewHealthManager(config HealthConfig, logger *logging.Logger) *HealthManager {
	return &HealthManager{
		checkers:  make(map[string]HealthChecker),
		results:   make(map[string]ComponentHealth),
		startTime: time.Now(),
		version:   config.Version,
		timeout:   config.Timeout,
		logger:    logger.WithComponent("health"),
	}
}

// RegisterChecker registers a health checker
func (hm *HealthManager) RegisterChecker(checker HealthChecker) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	name := checker.Name()
	hm.checkers[name] = checker
	hm.results[name] = ComponentHealth{
		Name:        name,
		Status:      HealthStatusUnknown,
		LastChecked: time.Time{},
	}

	hm.logger.Info("Registered health checker",
		logging.String("checker", name))
}

// CheckAll runs all health checks
func (hm *HealthManager) CheckAll(ctx context.Context) SystemHealth {
	start := time.Now()

	hm.mu.RLock()
	checkers := make(map[string]HealthChecker)
	for name, checker := range hm.checkers {
		checkers[name] = checker
	}
	hm.mu.RUnlock()

	// Run all health checks concurrently
	results := make(chan ComponentHealth, len(checkers))
	var wg sync.WaitGroup

	for _, checker := range checkers {
		wg.Add(1)
		go func(c HealthChecker) {
			defer wg.Done()

			checkCtx, cancel := context.WithTimeout(ctx, hm.timeout)
			defer cancel()

			result := c.Check(checkCtx)
			results <- result
		}(checker)
	}

	// Wait for all checks to complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	components := make(map[string]ComponentHealth)
	for result := range results {
		components[result.Name] = result

		// Update cached results
		hm.mu.Lock()
		hm.results[result.Name] = result
		hm.mu.Unlock()

	}

	// Determine overall system health
	systemStatus := hm.determineSystemHealth(components)

	// Calculate summary
	summary := hm.calculateSummary(components)

	duration := time.Since(start)

	hm.logger.Debug("Completed health check",
		logging.String("status", string(systemStatus)),
		logging.Duration("duration", duration),
		logging.Int("components", len(components)))

	return SystemHealth{
		Status:     systemStatus,
		Timestamp:  time.Now(),
		Version:    hm.version,
		Uptime:     time.Since(hm.startTime),
		Components: components,
		Summary:    summary,
	}
}

// GetCachedHealth returns the last known health status
func (hm *HealthManager) GetCachedHealth() SystemHealth {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	components := make(map[string]ComponentHealth)
	for name, result := range hm.results {
		components[name] = result
	}

	systemStatus := hm.determineSystemHealth(components)
	summary := hm.calculateSummary(components)

	return SystemHealth{
		Status:     systemStatus,
		Timestamp:  time.Now(),
		Version:    hm.version,
		Uptime:     time.Since(hm.startTime),
		Components: components,
		Summary:    summary,
	}
}

// determineSystemHealth calculates overall system health from components
func (hm *HealthManager) determineSystemHealth(components map[string]ComponentHealth) HealthStatus {
	if len(components) == 0 {
		return HealthStatusUnknown
	}

	healthyCount := 0
	degradedCount := 0
	unhealthyCount := 0

	for _, component := range components {
		switch component.Status {
		case HealthStatusHealthy:
			healthyCount++
		case HealthStatusDegraded:
			degradedCount++
		case HealthStatusUnhealthy:
			unhealthyCount++
		}
	}

	// System is unhealthy if any critical component is unhealthy
	if unhealthyCount > 0 {
		return HealthStatusUnhealthy
	}

	// System is degraded if any component is degraded
	if degradedCount > 0 {
		return HealthStatusDegraded
	}

	// System is healthy if all components are healthy
	if healthyCount == len(components) {
		return HealthStatusHealthy
	}

	return HealthStatusUnknown
}

// calculateSummary creates a health summary
func (hm *HealthManager) calculateSummary(components map[string]ComponentHealth) HealthSummary {
	summary := HealthSummary{
		TotalComponents: len(components),
	}

	for _, component := range components {
		switch component.Status {
		case HealthStatusHealthy:
			summary.HealthyCount++
		case HealthStatusDegraded:
			summary.DegradedCount++
		case HealthStatusUnhealthy:
			summary.UnhealthyCount++
		default:
			summary.UnknownCount++
		}
	}

	return summary
}

// Standard Health Checkers

// DatabaseHealthChecker checks database connectivity
type DatabaseHealthChecker struct {
	db   *sql.DB
	name string
}

// NewDatabaseHealthChecker creates a database health checker
func NewDatabaseHealthChecker(db *sql.DB, name string) *DatabaseHealthChecker {
	return &DatabaseHealthChecker{db: db, name: name}
}

func (dhc *DatabaseHealthChecker) Name() string {
	return dhc.name
}

func (dhc *DatabaseHealthChecker) Check(ctx context.Context) ComponentHealth {
	start := time.Now()

	result := ComponentHealth{
		Name:        dhc.name,
		LastChecked: time.Now(),
		Metadata:    make(map[string]interface{}),
	}

	// Test database connection
	if err := dhc.db.PingContext(ctx); err != nil {
		result.Status = HealthStatusUnhealthy
		result.Error = err.Error()
		result.Message = "Database connection failed"
		result.Duration = time.Since(start)
		return result
	}

	// Test with a simple query
	var count int
	err := dhc.db.QueryRowContext(ctx, "SELECT 1").Scan(&count)
	if err != nil {
		result.Status = HealthStatusDegraded
		result.Error = err.Error()
		result.Message = "Database query failed"
	} else {
		result.Status = HealthStatusHealthy
		result.Message = "Database connection successful"
	}

	// Add connection pool stats
	stats := dhc.db.Stats()
	result.Metadata["open_connections"] = stats.OpenConnections
	result.Metadata["in_use"] = stats.InUse
	result.Metadata["idle"] = stats.Idle
	result.Metadata["wait_count"] = stats.WaitCount
	result.Metadata["wait_duration"] = stats.WaitDuration.String()

	result.Duration = time.Since(start)
	return result
}

// HTTPHealthChecker checks external HTTP services
type HTTPHealthChecker struct {
	client *http.Client
	url    string
	name   string
}

// NewHTTPHealthChecker creates an HTTP health checker
func NewHTTPHealthChecker(url, name string, timeout time.Duration) *HTTPHealthChecker {
	return &HTTPHealthChecker{
		client: &http.Client{Timeout: timeout},
		url:    url,
		name:   name,
	}
}

func (hhc *HTTPHealthChecker) Name() string {
	return hhc.name
}

func (hhc *HTTPHealthChecker) Check(ctx context.Context) ComponentHealth {
	start := time.Now()

	result := ComponentHealth{
		Name:        hhc.name,
		LastChecked: time.Now(),
		Metadata:    make(map[string]interface{}),
	}

	req, err := http.NewRequestWithContext(ctx, "GET", hhc.url, nil)
	if err != nil {
		result.Status = HealthStatusUnhealthy
		result.Error = err.Error()
		result.Message = "Failed to create HTTP request"
		result.Duration = time.Since(start)
		return result
	}

	resp, err := hhc.client.Do(req)
	if err != nil {
		result.Status = HealthStatusUnhealthy
		result.Error = err.Error()
		result.Message = "HTTP request failed"
		result.Duration = time.Since(start)
		return result
	}
	defer resp.Body.Close()

	result.Metadata["status_code"] = resp.StatusCode
	result.Metadata["url"] = hhc.url

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		result.Status = HealthStatusHealthy
		result.Message = fmt.Sprintf("HTTP service responding (status: %d)", resp.StatusCode)
	} else if resp.StatusCode >= 500 {
		result.Status = HealthStatusUnhealthy
		result.Message = fmt.Sprintf("HTTP service error (status: %d)", resp.StatusCode)
	} else {
		result.Status = HealthStatusDegraded
		result.Message = fmt.Sprintf("HTTP service degraded (status: %d)", resp.StatusCode)
	}

	result.Duration = time.Since(start)
	return result
}

// ProcessorHealthChecker checks processing engine health
type ProcessorHealthChecker struct {
	getStats func() interface{}
	name     string
}

// NewProcessorHealthChecker creates a processor health checker
func NewProcessorHealthChecker(name string, getStats func() interface{}) *ProcessorHealthChecker {
	return &ProcessorHealthChecker{
		getStats: getStats,
		name:     name,
	}
}

func (phc *ProcessorHealthChecker) Name() string {
	return phc.name
}

func (phc *ProcessorHealthChecker) Check(ctx context.Context) ComponentHealth {
	start := time.Now()

	result := ComponentHealth{
		Name:        phc.name,
		LastChecked: time.Now(),
		Metadata:    make(map[string]interface{}),
	}

	// Get processing statistics
	if phc.getStats != nil {
		stats := phc.getStats()
		result.Metadata["stats"] = stats

		// Determine health based on stats
		result.Status = HealthStatusHealthy
		result.Message = "Processor is running normally"
	} else {
		result.Status = HealthStatusUnknown
		result.Message = "Unable to get processor statistics"
	}

	result.Duration = time.Since(start)
	return result
}

// HealthServer provides HTTP endpoints for health checks
type HealthServer struct {
	manager *HealthManager
	server  *http.Server
	logger  *logging.ComponentLogger
}

// NewHealthServer creates a new health check HTTP server
func NewHealthServer(manager *HealthManager, addr string, logger *logging.Logger) *HealthServer {
	mux := http.NewServeMux()
	hs := &HealthServer{
		manager: manager,
		logger:  logger.WithComponent("health_server"),
	}

	// Health check endpoints
	mux.HandleFunc("/health", hs.handleHealth)
	mux.HandleFunc("/health/live", hs.handleLiveness)
	mux.HandleFunc("/health/ready", hs.handleReadiness)
	mux.HandleFunc("/health/components", hs.handleComponents)

	hs.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	return hs
}

// Start starts the health server
func (hs *HealthServer) Start() error {
	hs.logger.Info("Starting health server",
		logging.String("addr", hs.server.Addr))

	go func() {
		if err := hs.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			hs.logger.Error("Health server error", err)
		}
	}()

	return nil
}

// Stop gracefully stops the health server
func (hs *HealthServer) Stop(ctx context.Context) error {
	hs.logger.Info("Stopping health server")
	return hs.server.Shutdown(ctx)
}

// handleHealth provides overall system health
func (hs *HealthServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	health := hs.manager.CheckAll(ctx)

	// Set HTTP status based on health
	switch health.Status {
	case HealthStatusHealthy:
		w.WriteHeader(http.StatusOK)
	case HealthStatusDegraded:
		w.WriteHeader(http.StatusOK) // Still OK, but degraded
	case HealthStatusUnhealthy:
		w.WriteHeader(http.StatusServiceUnavailable)
	default:
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)
}

// handleLiveness provides Kubernetes-style liveness probe
func (hs *HealthServer) handleLiveness(w http.ResponseWriter, r *http.Request) {
	// Simple liveness check - service is running
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "alive",
		"timestamp": time.Now(),
		"uptime":    time.Since(hs.manager.startTime).String(),
	})
}

// handleReadiness provides Kubernetes-style readiness probe
func (hs *HealthServer) handleReadiness(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	health := hs.manager.CheckAll(ctx)

	// Ready if healthy or degraded, but not if unhealthy
	if health.Status == HealthStatusUnhealthy {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     health.Status,
		"ready":      health.Status != HealthStatusUnhealthy,
		"timestamp":  health.Timestamp,
		"components": len(health.Components),
	})
}

// handleComponents provides detailed component health
func (hs *HealthServer) handleComponents(w http.ResponseWriter, r *http.Request) {
	cached := r.URL.Query().Get("cached") == "true"

	var health SystemHealth
	if cached {
		health = hs.manager.GetCachedHealth()
	} else {
		health = hs.manager.CheckAll(r.Context())
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"components": health.Components,
		"summary":    health.Summary,
		"timestamp":  health.Timestamp,
	})
}
