#!/bin/bash

# Venue Validation System Deployment Script
# Phase 5.3 - Production Deployment

set -e

# Configuration
APP_NAME="venue-validation"
APP_USER="${APP_USER:-venue-app}"
APP_DIR="${APP_DIR:-/opt/venue-validation}"
LOG_DIR="/var/log/venue-validation"
CONFIG_DIR="/etc/venue-validation"
SERVICE_NAME="venue-validation.service"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if script is run as root
check_root() {
    if [ "$EUID" -ne 0 ]; then
        log_error "This script must be run as root"
        exit 1
    fi
}

# Create application user
create_app_user() {
    if ! id "$APP_USER" &>/dev/null; then
        log_info "Creating application user: $APP_USER"
        useradd --system --shell /bin/false --home-dir "$APP_DIR" --create-home "$APP_USER"
    else
        log_info "Application user $APP_USER already exists"
    fi
}

# Create required directories
create_directories() {
    log_info "Creating application directories"
    
    mkdir -p "$APP_DIR"
    mkdir -p "$LOG_DIR"
    mkdir -p "$CONFIG_DIR"
    mkdir -p "/var/run/venue-validation"
    
    chown -R "$APP_USER:$APP_USER" "$APP_DIR"
    chown -R "$APP_USER:$APP_USER" "$LOG_DIR"
    chown -R "$APP_USER:$APP_USER" "/var/run/venue-validation"
    
    chmod 755 "$APP_DIR"
    chmod 755 "$LOG_DIR"
    chmod 755 "$CONFIG_DIR"
}

# Build application
build_application() {
    log_info "Building application binary"
    
    if [ ! -f "main.go" ]; then
        log_error "main.go not found. Run this script from the project root directory."
        exit 1
    fi
    
    # Build with optimizations
    CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"' -o "$APP_NAME" .
    
    if [ $? -ne 0 ]; then
        log_error "Application build failed"
        exit 1
    fi
    
    # Move binary to application directory
    mv "$APP_NAME" "$APP_DIR/"
    chown "$APP_USER:$APP_USER" "$APP_DIR/$APP_NAME"
    chmod 755 "$APP_DIR/$APP_NAME"
    
    log_info "Application binary deployed to $APP_DIR/$APP_NAME"
}

# Copy static files
copy_static_files() {
    log_info "Copying static files"
    
    if [ -d "web/static" ]; then
        cp -r web/static "$APP_DIR/"
        chown -R "$APP_USER:$APP_USER" "$APP_DIR/static"
    else
        log_warn "Static files directory not found, skipping"
    fi
}

# Create environment file template
create_env_template() {
    log_info "Creating environment configuration template"
    
    cat > "$CONFIG_DIR/venue-validation.env" << 'EOF'
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
# WEBHOOK_URL=https://your-webhook-endpoint.com/alerts
# SLACK_WEBHOOK_URL=https://hooks.slack.com/services/your/slack/webhook
# EMAIL_SMTP_HOST=smtp.example.com
# EMAIL_SMTP_PORT=587
# EMAIL_USER=alerts@example.com
# EMAIL_PASSWORD=your_email_password
# EMAIL_FROM=alerts@example.com
# EMAIL_TO=admin@example.com,ops@example.com
EOF

    chown root:root "$CONFIG_DIR/venue-validation.env"
    chmod 600 "$CONFIG_DIR/venue-validation.env"
    
    log_info "Environment template created at $CONFIG_DIR/venue-validation.env"
    log_warn "Please update the environment file with your actual configuration values"
}

# Create systemd service
create_systemd_service() {
    log_info "Creating systemd service"
    
    cat > "/etc/systemd/system/$SERVICE_NAME" << EOF
[Unit]
Description=Venue Validation System
Documentation=https://github.com/your-org/venue-validation
After=network.target
Wants=network.target

[Service]
Type=simple
User=$APP_USER
Group=$APP_USER
ExecStart=$APP_DIR/$APP_NAME
WorkingDirectory=$APP_DIR
EnvironmentFile=$CONFIG_DIR/venue-validation.env

# Restart policy
Restart=always
RestartSec=5
StartLimitInterval=60s
StartLimitBurst=3

# Security settings
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=$LOG_DIR /var/run/venue-validation
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

    systemctl daemon-reload
    
    log_info "Systemd service created: $SERVICE_NAME"
}

# Setup log rotation
setup_log_rotation() {
    log_info "Setting up log rotation"
    
    cat > "/etc/logrotate.d/venue-validation" << EOF
$LOG_DIR/*.log {
    daily
    missingok
    rotate 30
    compress
    delaycompress
    notifempty
    create 0644 $APP_USER $APP_USER
    postrotate
        systemctl reload-or-restart $SERVICE_NAME > /dev/null 2>&1 || true
    endscript
}
EOF

    log_info "Log rotation configured"
}

# Setup firewall rules (if ufw is available)
setup_firewall() {
    if command -v ufw &> /dev/null; then
        log_info "Configuring firewall rules"
        
        # Allow application port
        ufw allow 8080/tcp comment "Venue Validation App"
        
        # Allow health check port (restrict to localhost)
        ufw allow from 127.0.0.1 to any port 8081 comment "Health Check"
        
        # Allow metrics port (restrict to monitoring systems)
        # ufw allow from YOUR_MONITORING_IP to any port 8082 comment "Metrics"
        
        log_info "Firewall rules configured"
    else
        log_warn "UFW not found, skipping firewall configuration"
    fi
}

# Validate configuration
validate_config() {
    log_info "Validating configuration"
    
    # Check if binary exists and is executable
    if [ ! -x "$APP_DIR/$APP_NAME" ]; then
        log_error "Application binary not found or not executable"
        return 1
    fi
    
    # Test configuration loading (dry run)
    sudo -u "$APP_USER" "$APP_DIR/$APP_NAME" --validate-config 2>/dev/null || {
        log_warn "Configuration validation failed - please check your environment file"
    }
    
    log_info "Configuration validation completed"
}

# Enable and start service
start_service() {
    log_info "Enabling and starting service"
    
    systemctl enable "$SERVICE_NAME"
    systemctl start "$SERVICE_NAME"
    
    # Wait a moment for service to start
    sleep 3
    
    if systemctl is-active --quiet "$SERVICE_NAME"; then
        log_info "Service started successfully"
        systemctl status "$SERVICE_NAME" --no-pager -l
    else
        log_error "Service failed to start"
        log_error "Check logs with: journalctl -u $SERVICE_NAME -f"
        exit 1
    fi
}

# Show post-deployment information
show_post_deploy_info() {
    echo
    log_info "Deployment completed successfully!"
    echo
    echo "Service Management:"
    echo "  Start:   systemctl start $SERVICE_NAME"
    echo "  Stop:    systemctl stop $SERVICE_NAME"
    echo "  Restart: systemctl restart $SERVICE_NAME"
    echo "  Status:  systemctl status $SERVICE_NAME"
    echo "  Logs:    journalctl -u $SERVICE_NAME -f"
    echo
    echo "Configuration:"
    echo "  Config file: $CONFIG_DIR/venue-validation.env"
    echo "  Log files:   $LOG_DIR/"
    echo "  Binary:      $APP_DIR/$APP_NAME"
    echo
    echo "Health Checks:"
    echo "  Application: http://localhost:8080/"
    echo "  Health:      http://localhost:8081/health"
    echo "  Metrics:     http://localhost:8082/metrics"
    echo
    log_warn "Remember to:"
    log_warn "1. Update $CONFIG_DIR/venue-validation.env with your actual configuration"
    log_warn "2. Ensure your database is accessible and properly configured"
    log_warn "3. Configure monitoring and alerting endpoints"
    log_warn "4. Set up SSL/TLS termination (nginx/Apache) for production"
}

# Main deployment function
main() {
    log_info "Starting venue validation system deployment"
    
    check_root
    create_app_user
    create_directories
    build_application
    copy_static_files
    create_env_template
    create_systemd_service
    setup_log_rotation
    setup_firewall
    validate_config
    start_service
    show_post_deploy_info
}

# Run main function
main "$@"