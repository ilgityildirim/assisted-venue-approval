# Venue Validation System - Deployment Guide

## Overview

This guide covers the deployment of the Venue Validation System in production environments. The system is designed as a single binary with comprehensive monitoring, health checks, and operational capabilities.

## Prerequisites

### System Requirements

- **OS**: Linux (Ubuntu 20.04+ or CentOS 8+ recommended)
- **Memory**: Minimum 2GB RAM (4GB+ recommended)
- **CPU**: 2+ cores recommended
- **Storage**: 20GB+ available disk space
- **Database**: PostgreSQL 12+ (local or remote)

### Required Software

- **Go**: Version 1.25+ (for building from source)
- **PostgreSQL**: Client tools (`postgresql-client`)
- **systemd**: For service management
- **curl**: For health checks and API calls
- **jq**: For JSON parsing in scripts (optional but recommended)

### Network Requirements

- **Outbound Internet**: Required for Google Maps API and OpenAI API
- **Inbound Ports**:
  - `8080`: Main application (configurable)
  - `8081`: Health checks (configurable)
  - `8082`: Metrics endpoint (configurable)
  - `8083`: Profiling endpoint (optional, configurable)

## Quick Deployment

### Automated Deployment

The fastest way to deploy is using the provided deployment script:

```bash
# Clone the repository
git clone <repository-url>
cd automatic-vendor-validation

# Run automated deployment (requires root)
sudo ./scripts/deploy.sh
```

This script will:
- Create application user and directories
- Build the application binary
- Set up systemd service
- Configure log rotation
- Create environment template
- Start the service

### Manual Configuration

After running the deployment script, you **must** configure the environment variables:

```bash
# Edit the configuration file
sudo vim /etc/venue-validation/venue-validation.env
```

**Required Configuration**:
```bash
# Database Configuration
DATABASE_URL=postgres://username:password@localhost:5432/venue_validation?sslmode=require

# API Keys (REQUIRED)
GOOGLE_MAPS_API_KEY=your_google_maps_api_key_here
OPENAI_API_KEY=your_openai_api_key_here
```

**Restart the service** after configuration:
```bash
sudo systemctl restart venue-validation
```

## Manual Deployment

### 1. Prepare the Environment

```bash
# Create application user
sudo useradd --system --shell /bin/false --home-dir /opt/venue-validation --create-home venue-app

# Create directories
sudo mkdir -p /opt/venue-validation
sudo mkdir -p /var/log/venue-validation
sudo mkdir -p /etc/venue-validation
sudo mkdir -p /var/run/venue-validation

# Set permissions
sudo chown -R venue-app:venue-app /opt/venue-validation
sudo chown -R venue-app:venue-app /var/log/venue-validation
sudo chown -R venue-app:venue-app /var/run/venue-validation
```

### 2. Build and Deploy Binary

```bash
# Build the application
CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags \"-static\"' -o venue-validation .

# Deploy binary
sudo mv venue-validation /opt/venue-validation/
sudo chown venue-app:venue-app /opt/venue-validation/venue-validation
sudo chmod 755 /opt/venue-validation/venue-validation

# Copy static files (if any)
sudo cp -r web/static /opt/venue-validation/ 2>/dev/null || echo \"No static files found\"
sudo chown -R venue-app:venue-app /opt/venue-validation/static 2>/dev/null || true
```

### 3. Create Configuration

```bash
sudo tee /etc/venue-validation/venue-validation.env > /dev/null <<EOF
# Database Configuration
DATABASE_URL=postgres://user:password@localhost:5432/venue_validation?sslmode=require

# API Keys (REQUIRED)
GOOGLE_MAPS_API_KEY=your_google_maps_api_key_here
OPENAI_API_KEY=your_openai_api_key_here

# Server Configuration
PORT=8080
HEALTH_CHECK_PORT=8081
METRICS_PORT=8082
PROFILING_PORT=8083

# Processing Configuration
APPROVAL_THRESHOLD=75
WORKER_COUNT=5

# Database Connection Pool
DB_MAX_OPEN_CONNS=25
DB_MAX_IDLE_CONNS=10
DB_CONN_MAX_LIFETIME_MINUTES=15
DB_CONN_MAX_IDLE_TIME_MINUTES=5

# Logging Configuration
LOG_LEVEL=info
LOG_FORMAT=json
ENABLE_FILE_LOGGING=true
LOG_FILE=/var/log/venue-validation/app.log

# Monitoring and Metrics
METRICS_ENABLED=true
PROFILING_ENABLED=false
AUTO_PROFILING=false
PROFILE_DIR=/var/log/venue-validation/profiles

# Alerting Configuration
ALERTING_ENABLED=true
EOF

sudo chown root:root /etc/venue-validation/venue-validation.env
sudo chmod 600 /etc/venue-validation/venue-validation.env
```

### 4. Create systemd Service

```bash
sudo tee /etc/systemd/system/venue-validation.service > /dev/null <<EOF
[Unit]
Description=Venue Validation System
Documentation=https://github.com/your-org/venue-validation
After=network.target
Wants=network.target

[Service]
Type=simple
User=venue-app
Group=venue-app
ExecStart=/opt/venue-validation/venue-validation
WorkingDirectory=/opt/venue-validation
EnvironmentFile=/etc/venue-validation/venue-validation.env

# Restart policy
Restart=always
RestartSec=5
StartLimitInterval=60s
StartLimitBurst=3

# Security settings
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=/var/log/venue-validation /var/run/venue-validation
PrivateTmp=yes
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectControlGroups=yes

# Resource limits
LimitNOFILE=65536
LimitNPROC=4096

# Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=venue-validation

[Install]
WantedBy=multi-user.target
EOF

# Reload systemd and start service
sudo systemctl daemon-reload
sudo systemctl enable venue-validation
sudo systemctl start venue-validation
```

### 5. Verify Deployment

```bash
# Check service status
sudo systemctl status venue-validation

# Check application endpoints
curl http://localhost:8080/
curl http://localhost:8081/health
curl http://localhost:8082/metrics

# Check logs
journalctl -u venue-validation -f
```

## Configuration Reference

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | ✅ | - | PostgreSQL connection string |
| `GOOGLE_MAPS_API_KEY` | ✅ | - | Google Maps API key |
| `OPENAI_API_KEY` | ✅ | - | OpenAI API key |
| `PORT` | ✅ | `8080` | Main application port |
| `HEALTH_CHECK_PORT` | | `8081` | Health check endpoint port |
| `METRICS_PORT` | | `8082` | Metrics endpoint port |
| `PROFILING_PORT` | | `8083` | Profiling endpoint port |
| `APPROVAL_THRESHOLD` | | `75` | AI approval threshold (0-100) |
| `WORKER_COUNT` | | `5` | Number of processing workers |
| `LOG_LEVEL` | | `info` | Logging level (trace, debug, info, warn, error, fatal) |
| `LOG_FORMAT` | | `json` | Log format (json, text) |
| `ENABLE_FILE_LOGGING` | | `true` | Enable file logging |
| `LOG_FILE` | | `/var/log/venue-validation/app.log` | Log file path |
| `METRICS_ENABLED` | | `true` | Enable metrics collection |
| `ALERTING_ENABLED` | | `true` | Enable alerting system |

### Database Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `DB_MAX_OPEN_CONNS` | `25` | Maximum open database connections |
| `DB_MAX_IDLE_CONNS` | `10` | Maximum idle database connections |
| `DB_CONN_MAX_LIFETIME_MINUTES` | `15` | Connection maximum lifetime |
| `DB_CONN_MAX_IDLE_TIME_MINUTES` | `5` | Connection maximum idle time |

### Alerting Configuration

| Variable | Description |
|----------|-------------|
| `WEBHOOK_URL` | Generic webhook URL for alerts |
| `SLACK_WEBHOOK_URL` | Slack webhook URL |
| `EMAIL_SMTP_HOST` | SMTP server hostname |
| `EMAIL_SMTP_PORT` | SMTP server port |
| `EMAIL_USER` | SMTP username |
| `EMAIL_PASSWORD` | SMTP password |
| `EMAIL_FROM` | From email address |
| `EMAIL_TO` | Comma-separated recipient emails |

## Monitoring and Health Checks

### Health Check Endpoints

- **Health**: `http://localhost:8081/health` - Detailed system health
- **Metrics**: `http://localhost:8082/metrics` - Prometheus-compatible metrics
- **Application**: `http://localhost:8080/` - Main application

### Automated Health Monitoring

Use the provided health check script:

```bash
# Run health check
./scripts/health-check.sh

# Run with custom URLs
./scripts/health-check.sh --health-url http://localhost:8081/health --timeout 15

# Check specific service
./scripts/health-check.sh --service-name venue-validation.service
```

### Log Management

Logs are managed through systemd journal and file logging:

```bash
# View real-time logs
journalctl -u venue-validation -f

# View application logs
tail -f /var/log/venue-validation/app.log

# Search for errors
journalctl -u venue-validation --since \"1 hour ago\" | grep ERROR
```

Log rotation is automatically configured for `/var/log/venue-validation/*.log` files.

## Database Management

### Backup and Recovery

Use the provided backup script:

```bash
# Perform backup
./scripts/backup.sh

# Backup to S3
./scripts/backup.sh --s3-bucket my-backup-bucket

# Verify existing backup
./scripts/backup.sh --verify-only /var/backups/venue-validation/backup_20240101_120000.sql.gz

# Cleanup old backups
./scripts/backup.sh --cleanup-only
```

### Database Setup

```sql
-- Create database
CREATE DATABASE venue_validation;

-- Create user with appropriate permissions
CREATE USER venue_app WITH PASSWORD 'secure_password';
GRANT ALL PRIVILEGES ON DATABASE venue_validation TO venue_app;

-- Connect to the database and run migrations
\\c venue_validation;
-- Run your database migration scripts here
```

## Security Considerations

### Firewall Configuration

```bash
# Allow application port
sudo ufw allow 8080/tcp comment \"Venue Validation App\"

# Restrict health and metrics ports to localhost
sudo ufw allow from 127.0.0.1 to any port 8081 comment \"Health Check\"
sudo ufw allow from YOUR_MONITORING_IP to any port 8082 comment \"Metrics\"
```

### SSL/TLS Termination

For production deployments, use a reverse proxy like nginx:

```nginx
server {
    listen 443 ssl http2;
    server_name your-domain.com;

    ssl_certificate /path/to/certificate.pem;
    ssl_certificate_key /path/to/private-key.pem;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # Health checks (restrict access)
    location /health {
        proxy_pass http://127.0.0.1:8081/health;
        allow 127.0.0.1;
        allow YOUR_MONITORING_IP;
        deny all;
    }
}
```

### Environment Security

- Store sensitive configuration in `/etc/venue-validation/venue-validation.env` with `600` permissions
- Use strong database passwords
- Rotate API keys regularly
- Enable database SSL connections
- Run the application as a non-privileged user (`venue-app`)

## Troubleshooting

### Common Issues

**Service fails to start:**
```bash
# Check service status
sudo systemctl status venue-validation

# Check logs for errors
journalctl -u venue-validation --since \"10 minutes ago\"

# Validate configuration
sudo -u venue-app /opt/venue-validation/venue-validation --validate-config
```

**Database connection issues:**
```bash
# Test database connectivity
pg_isready -h localhost -p 5432 -U venue_app -d venue_validation

# Check database logs
sudo tail -f /var/log/postgresql/postgresql-*.log
```

**High resource usage:**
```bash
# Check system resources
top -p $(pgrep venue-validation)

# Enable profiling temporarily
curl http://localhost:8083/debug/pprof/heap > heap.prof
go tool pprof heap.prof
```

### Log Analysis

```bash
# Find application errors
journalctl -u venue-validation | grep ERROR

# Check processing statistics
curl http://localhost:8080/validate/stats

# Monitor API call rates
journalctl -u venue-validation | grep \"Google Maps API\" | tail -20
```

## Maintenance

### Updates and Upgrades

1. **Stop the service:**
   ```bash
   sudo systemctl stop venue-validation
   ```

2. **Backup current binary:**
   ```bash
   sudo cp /opt/venue-validation/venue-validation /opt/venue-validation/venue-validation.bak
   ```

3. **Deploy new binary:**
   ```bash
   # Build new version
   CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags \"-static\"' -o venue-validation .
   sudo mv venue-validation /opt/venue-validation/
   sudo chown venue-app:venue-app /opt/venue-validation/venue-validation
   sudo chmod 755 /opt/venue-validation/venue-validation
   ```

4. **Start the service:**
   ```bash
   sudo systemctl start venue-validation
   sudo systemctl status venue-validation
   ```

### Scheduled Maintenance

Set up cron jobs for routine maintenance:

```bash
# Add to root's crontab
sudo crontab -e

# Daily backup at 2 AM
0 2 * * * /opt/venue-validation/scripts/backup.sh

# Weekly health check report
0 6 * * 1 /opt/venue-validation/scripts/health-check.sh > /var/log/venue-validation/weekly-health-$(date +\%Y\%m\%d).log

# Monthly log cleanup
0 3 1 * * find /var/log/venue-validation -name \"*.log\" -mtime +30 -delete
```

## Support and Monitoring

### Performance Monitoring

- Monitor the `/metrics` endpoint with Prometheus
- Set up Grafana dashboards for visualization
- Configure alerts for high error rates, memory usage, and response times

### Alert Configuration

Configure webhook endpoints for critical alerts:

```bash
# Add to environment file
WEBHOOK_URL=https://your-monitoring-system.com/webhook
SLACK_WEBHOOK_URL=https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK
```

### Log Aggregation

For production environments, consider centralizing logs:

```bash
# Configure rsyslog to forward to central log server
echo \"*.* @log-server.example.com:514\" | sudo tee -a /etc/rsyslog.conf
sudo systemctl restart rsyslog
```

This completes the deployment guide for the Venue Validation System. For additional support, refer to the application logs and health check endpoints.