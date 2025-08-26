# Monitoring and Observability Guide

## Overview

The venue validation system includes comprehensive monitoring, logging, health checks, and alerting capabilities to ensure reliable operation and quick issue detection.

## Components

### 1. Structured Logging
- **Package**: `pkg/logging`
- **Features**: 
  - JSON and text output formats
  - Multiple log levels (trace, debug, info, warn, error, fatal)
  - Context-aware logging with request IDs, venue IDs, user IDs
  - Async logging for performance
  - File rotation and retention
  - Component-specific loggers

#### Configuration
```env
LOG_LEVEL=info              # trace, debug, info, warn, error, fatal
LOG_FORMAT=json             # json or text
LOG_FILE=/var/log/venue-validation/app.log
ENABLE_FILE_LOGGING=true
```

#### Usage Example
```go
logger := logging.NewLogger(logConfig)
componentLogger := logger.WithComponent("processor")

componentLogger.Info("Processing venue", 
    logging.Int64("venue_id", venueID),
    logging.String("status", "started"))
```

### 2. Metrics Collection
- **Package**: `pkg/monitoring`
- **Features**:
  - Prometheus-compatible metrics
  - Counter, Gauge, and Histogram metrics
  - Application-specific metrics for all components
  - HTTP endpoints for metrics exposure
  - JSON and Prometheus text formats

#### Available Metrics
- **Processing**: venues processed, approved, rejected, processing time
- **API**: request counts, errors, response times, rate limits
- **Database**: connections, query duration, errors
- **Queue**: size, depth, worker utilization
- **System**: memory usage, CPU, goroutines, error rates

#### Endpoints
- `GET /metrics` - Prometheus format metrics
- `GET /metrics/json` - JSON format metrics

### 3. Health Checks
- **Package**: `pkg/health`
- **Features**:
  - Component-level health monitoring
  - Database connectivity checks
  - External API health verification
  - System resource monitoring
  - Kubernetes-style liveness/readiness probes

#### Endpoints
- `GET /health` - Overall system health
- `GET /health/live` - Liveness probe (Kubernetes)
- `GET /health/ready` - Readiness probe (Kubernetes)
- `GET /health/components` - Detailed component health

#### Health Status Levels
- **Healthy**: All systems operating normally
- **Degraded**: Some issues but still functional
- **Unhealthy**: Critical issues requiring attention
- **Unknown**: Unable to determine status

### 4. Alerting System
- **Package**: `pkg/alerting`
- **Features**:
  - Multiple notification channels (log, webhook, email, Slack)
  - Alert levels (info, warning, critical, fatal)
  - Rate limiting and muting
  - Pre-configured alert rules
  - Context-rich alert metadata

#### Alert Rules
- High error rate (>10%)
- Database connection failures
- API rate limit exceeded
- Queue size too high (>1000 items)
- High memory usage (>1GB)
- Processing timeouts (>5 minutes)

#### Notification Channels
```env
# Webhook
WEBHOOK_URL=https://your-webhook-endpoint.com/alerts

# Slack
SLACK_WEBHOOK_URL=https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK

# Email
EMAIL_SMTP_HOST=smtp.gmail.com
EMAIL_SMTP_PORT=587
EMAIL_USER=your-email@domain.com
EMAIL_PASSWORD=your-app-password
EMAIL_FROM=venue-validation@your-domain.com
EMAIL_TO=admin@your-domain.com,ops@your-domain.com
```

### 5. Performance Profiling
- **Package**: `pkg/profiling`
- **Features**:
  - CPU, memory, goroutine profiling
  - Automatic profile generation
  - Performance snapshots
  - HTTP endpoints for live profiling
  - Profile file management

#### Endpoints
- `GET /debug/pprof/` - Profile index
- `GET /debug/pprof/profile?seconds=30` - CPU profile
- `GET /debug/pprof/heap` - Memory profile
- `GET /debug/pprof/goroutine` - Goroutine profile
- `GET /debug/performance` - Current performance snapshot
- `GET /debug/snapshots` - Historical snapshots

## Configuration

### Environment Variables

```env
# Monitoring and Logging
LOG_LEVEL=info
LOG_FORMAT=json
LOG_FILE=/var/log/venue-validation/app.log
ENABLE_FILE_LOGGING=true

# Health Check Configuration
HEALTH_CHECK_PORT=8081
HEALTH_CHECK_PATH=/health

# Metrics Configuration
METRICS_ENABLED=true
METRICS_PORT=8082
METRICS_PATH=/metrics

# Alerting Configuration
ALERTING_ENABLED=true
WEBHOOK_URL=
SLACK_WEBHOOK_URL=
EMAIL_SMTP_HOST=
EMAIL_SMTP_PORT=587
EMAIL_USER=
EMAIL_PASSWORD=
EMAIL_FROM=
EMAIL_TO=

# Profiling Configuration
PROFILING_ENABLED=false
PROFILING_PORT=6060
PROFILE_DIR=/var/log/venue-validation/profiles
AUTO_PROFILING=false
```

### Log Directory Structure
```
/var/log/venue-validation/
├── app.log                 # Main application log
├── app.log.1              # Rotated log file
├── app.log.2              # Older rotated log file
└── profiles/              # Performance profiles
    ├── cpu_profile_*.pprof
    ├── memory_profile_*.pprof
    └── goroutine_profile_*.pprof
```

## Integration with External Systems

### Prometheus
Configure Prometheus to scrape metrics:
```yaml
scrape_configs:
  - job_name: 'venue-validation'
    static_configs:
      - targets: ['venue-validation-app:8082']
    scrape_interval: 30s
    metrics_path: /metrics
```

### Grafana Dashboard
Key metrics to monitor:
- Processing throughput and success rate
- API response times and error rates
- Database connection pool utilization
- Queue depth and worker utilization
- Memory usage and garbage collection frequency

### Kubernetes Integration
Deploy with appropriate probes:
```yaml
spec:
  containers:
  - name: venue-validation
    livenessProbe:
      httpGet:
        path: /health/live
        port: 8081
      initialDelaySeconds: 30
      periodSeconds: 10
    readinessProbe:
      httpGet:
        path: /health/ready
        port: 8081
      initialDelaySeconds: 5
      periodSeconds: 5
```

### ELK Stack Integration
Configure Filebeat to collect logs:
```yaml
filebeat.inputs:
- type: log
  paths:
    - /var/log/venue-validation/*.log
  json.keys_under_root: true
  json.add_error_key: true
```

## Troubleshooting

### Common Issues

1. **High Memory Usage**
   - Check `/debug/pprof/heap` for memory hotspots
   - Monitor cache sizes and cleanup routines
   - Review goroutine count for leaks

2. **High Error Rate**
   - Check application logs for error patterns
   - Verify external API health and rate limits
   - Review database connection status

3. **Slow Processing**
   - Monitor queue depth and worker utilization
   - Check API response times
   - Review database query performance

### Debug Commands

```bash
# Get current health status
curl http://localhost:8081/health | jq '.'

# Get metrics
curl http://localhost:8082/metrics

# Get performance snapshot
curl http://localhost:6060/debug/performance | jq '.'

# Generate CPU profile
curl http://localhost:6060/debug/pprof/profile?seconds=30 > cpu.pprof
go tool pprof cpu.pprof

# Get memory profile
curl http://localhost:6060/debug/pprof/heap > mem.pprof
go tool pprof mem.pprof

# View logs
tail -f /var/log/venue-validation/app.log | jq '.'
```

## Best Practices

1. **Log Levels**
   - Use INFO for normal operations
   - Use WARN for recoverable issues
   - Use ERROR for failures requiring attention
   - Use DEBUG sparingly in production

2. **Metrics**
   - Monitor both business and technical metrics
   - Set up alerts on key performance indicators
   - Use histograms for latency measurements

3. **Health Checks**
   - Implement meaningful health checks
   - Include dependency health in overall status
   - Design for fast execution (<1 second)

4. **Alerting**
   - Avoid alert fatigue with proper thresholds
   - Use progressive severity levels
   - Include actionable information in alerts

5. **Performance Monitoring**
   - Enable profiling only when needed
   - Regular performance baseline reviews
   - Monitor resource trends over time

## Security Considerations

- Store sensitive configuration (passwords, API keys) in secure vaults
- Limit access to profiling endpoints in production
- Use HTTPS for webhook and external service communications
- Sanitize log output to avoid logging sensitive data
- Implement proper authentication for monitoring endpoints