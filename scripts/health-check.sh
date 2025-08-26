#!/bin/bash

# Health Check Script for Venue Validation System
# This script performs comprehensive health checks for monitoring and alerting

set -e

# Configuration
HEALTH_URL="${HEALTH_URL:-http://localhost:8081/health}"
METRICS_URL="${METRICS_URL:-http://localhost:8082/metrics}"
APP_URL="${APP_URL:-http://localhost:8080/}"
TIMEOUT="${TIMEOUT:-10}"
SERVICE_NAME="${SERVICE_NAME:-venue-validation.service}"

# Exit codes
EXIT_OK=0
EXIT_WARNING=1
EXIT_CRITICAL=2
EXIT_UNKNOWN=3

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Logging functions
log_ok() {
    echo -e "${GREEN}[OK]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_critical() {
    echo -e "${RED}[CRITICAL]${NC} $1"
}

log_info() {
    echo -e "[INFO] $1"
}

# Global status tracking
OVERALL_STATUS=$EXIT_OK
CHECKS_PERFORMED=0
CHECKS_PASSED=0

# Function to update overall status
update_status() {
    local new_status=$1
    CHECKS_PERFORMED=$((CHECKS_PERFORMED + 1))
    
    if [ $new_status -eq $EXIT_OK ]; then
        CHECKS_PASSED=$((CHECKS_PASSED + 1))
    fi
    
    # Update overall status (worst case wins)
    if [ $new_status -gt $OVERALL_STATUS ]; then
        OVERALL_STATUS=$new_status
    fi
}

# Check if service is running
check_service_status() {
    log_info "Checking service status..."
    
    if systemctl is-active --quiet "$SERVICE_NAME"; then
        log_ok "Service $SERVICE_NAME is running"
        update_status $EXIT_OK
    else
        log_critical "Service $SERVICE_NAME is not running"
        update_status $EXIT_CRITICAL
        return
    fi
    
    # Check if service is enabled
    if systemctl is-enabled --quiet "$SERVICE_NAME"; then
        log_ok "Service $SERVICE_NAME is enabled"
    else
        log_warning "Service $SERVICE_NAME is not enabled"
        update_status $EXIT_WARNING
    fi
}

# Check HTTP endpoint availability
check_http_endpoint() {
    local url=$1
    local name=$2
    local expected_status=${3:-200}
    
    log_info "Checking $name endpoint: $url"
    
    if command -v curl &> /dev/null; then
        local response=$(curl -s -w "%{http_code}" --connect-timeout $TIMEOUT "$url" -o /dev/null 2>/dev/null)
        
        if [ "$response" = "$expected_status" ]; then
            log_ok "$name endpoint is responding correctly (HTTP $response)"
            update_status $EXIT_OK
        else
            log_critical "$name endpoint returned HTTP $response (expected $expected_status)"
            update_status $EXIT_CRITICAL
        fi
    else
        log_warning "curl not available, skipping HTTP endpoint check for $name"
        update_status $EXIT_WARNING
    fi
}

# Check detailed health endpoint
check_health_details() {
    log_info "Checking detailed health status..."
    
    if command -v curl &> /dev/null && command -v jq &> /dev/null; then
        local health_json=$(curl -s --connect-timeout $TIMEOUT "$HEALTH_URL" 2>/dev/null)
        
        if [ $? -eq 0 ] && [ -n "$health_json" ]; then
            local overall_status=$(echo "$health_json" | jq -r '.status // "unknown"')
            local healthy_count=$(echo "$health_json" | jq -r '.summary.healthy_count // 0')
            local unhealthy_count=$(echo "$health_json" | jq -r '.summary.unhealthy_count // 0')
            local degraded_count=$(echo "$health_json" | jq -r '.summary.degraded_count // 0')
            
            log_info "Health Summary: $healthy_count healthy, $degraded_count degraded, $unhealthy_count unhealthy"
            
            case "$overall_status" in
                "healthy")
                    log_ok "Overall health status: $overall_status"
                    update_status $EXIT_OK
                    ;;
                "degraded")
                    log_warning "Overall health status: $overall_status"
                    update_status $EXIT_WARNING
                    ;;
                "unhealthy")
                    log_critical "Overall health status: $overall_status"
                    update_status $EXIT_CRITICAL
                    ;;
                *)
                    log_warning "Unknown health status: $overall_status"
                    update_status $EXIT_WARNING
                    ;;
            esac
        else
            log_warning "Could not parse health endpoint response"
            update_status $EXIT_WARNING
        fi
    else
        log_info "jq not available, skipping detailed health parsing"
    fi
}

# Check system resources
check_system_resources() {
    log_info "Checking system resources..."
    
    # Memory usage
    if command -v free &> /dev/null; then
        local memory_usage=$(free | grep Mem | awk '{printf("%.1f", ($3/$2) * 100.0)}')
        local memory_usage_int=$(echo "$memory_usage" | cut -d. -f1)
        
        if [ "$memory_usage_int" -lt 80 ]; then
            log_ok "Memory usage: ${memory_usage}%"
            update_status $EXIT_OK
        elif [ "$memory_usage_int" -lt 90 ]; then
            log_warning "Memory usage high: ${memory_usage}%"
            update_status $EXIT_WARNING
        else
            log_critical "Memory usage critical: ${memory_usage}%"
            update_status $EXIT_CRITICAL
        fi
    fi
    
    # Disk usage (check log directory)
    if [ -d "/var/log/venue-validation" ]; then
        local disk_usage=$(df /var/log/venue-validation | awk 'NR==2 {print $5}' | sed 's/%//')
        
        if [ "$disk_usage" -lt 80 ]; then
            log_ok "Log disk usage: ${disk_usage}%"
            update_status $EXIT_OK
        elif [ "$disk_usage" -lt 90 ]; then
            log_warning "Log disk usage high: ${disk_usage}%"
            update_status $EXIT_WARNING
        else
            log_critical "Log disk usage critical: ${disk_usage}%"
            update_status $EXIT_CRITICAL
        fi
    fi
    
    # Load average
    if command -v uptime &> /dev/null; then
        local load_avg=$(uptime | awk -F'load average:' '{print $2}' | awk '{print $1}' | sed 's/,//')
        local cpu_count=$(nproc)
        local load_ratio=$(echo "scale=2; $load_avg / $cpu_count" | bc -l 2>/dev/null || echo "0")
        local load_percent=$(echo "scale=0; $load_ratio * 100 / 1" | bc -l 2>/dev/null || echo "0")
        
        if [ "$load_percent" -lt 80 ]; then
            log_ok "Load average: $load_avg (${load_percent}% of CPU capacity)"
            update_status $EXIT_OK
        elif [ "$load_percent" -lt 100 ]; then
            log_warning "Load average high: $load_avg (${load_percent}% of CPU capacity)"
            update_status $EXIT_WARNING
        else
            log_critical "Load average critical: $load_avg (${load_percent}% of CPU capacity)"
            update_status $EXIT_CRITICAL
        fi
    fi
}

# Check log files for errors
check_log_errors() {
    log_info "Checking recent log entries for errors..."
    
    if command -v journalctl &> /dev/null; then
        # Check for recent errors in the service logs
        local error_count=$(journalctl -u "$SERVICE_NAME" --since "5 minutes ago" --no-pager -q | grep -i "error\|fatal\|panic" | wc -l)
        
        if [ "$error_count" -eq 0 ]; then
            log_ok "No recent errors in service logs"
            update_status $EXIT_OK
        elif [ "$error_count" -lt 5 ]; then
            log_warning "Found $error_count recent error(s) in service logs"
            update_status $EXIT_WARNING
        else
            log_critical "Found $error_count recent error(s) in service logs"
            update_status $EXIT_CRITICAL
        fi
    else
        log_warning "journalctl not available, skipping log error check"
        update_status $EXIT_WARNING
    fi
}

# Check database connectivity (if metrics endpoint provides DB stats)
check_database_connectivity() {
    log_info "Checking database connectivity..."
    
    if command -v curl &> /dev/null; then
        # Try to get metrics that would indicate DB health
        local metrics=$(curl -s --connect-timeout $TIMEOUT "$METRICS_URL" 2>/dev/null)
        
        if [ $? -eq 0 ] && [ -n "$metrics" ]; then
            # Look for database-related metrics
            if echo "$metrics" | grep -q "db_"; then
                log_ok "Database metrics available"
                update_status $EXIT_OK
            else
                log_warning "Database metrics not found in metrics endpoint"
                update_status $EXIT_WARNING
            fi
        else
            log_warning "Could not retrieve metrics for database check"
            update_status $EXIT_WARNING
        fi
    fi
}

# Generate summary report
generate_summary() {
    echo
    echo "========================="
    echo "Health Check Summary"
    echo "========================="
    echo "Checks performed: $CHECKS_PERFORMED"
    echo "Checks passed: $CHECKS_PASSED"
    echo "Success rate: $(echo "scale=1; $CHECKS_PASSED * 100 / $CHECKS_PERFORMED" | bc -l)%"
    echo
    
    case $OVERALL_STATUS in
        $EXIT_OK)
            log_ok "Overall status: HEALTHY"
            ;;
        $EXIT_WARNING)
            log_warning "Overall status: WARNING - Some issues detected"
            ;;
        $EXIT_CRITICAL)
            log_critical "Overall status: CRITICAL - Immediate attention required"
            ;;
        *)
            echo "Overall status: UNKNOWN"
            ;;
    esac
    echo
}

# Print usage information
usage() {
    echo "Usage: $0 [OPTIONS]"
    echo "Options:"
    echo "  -h, --help           Show this help message"
    echo "  -v, --verbose        Enable verbose output"
    echo "  --health-url URL     Health check endpoint URL (default: $HEALTH_URL)"
    echo "  --metrics-url URL    Metrics endpoint URL (default: $METRICS_URL)"
    echo "  --app-url URL        Application URL (default: $APP_URL)"
    echo "  --timeout SECONDS    HTTP timeout in seconds (default: $TIMEOUT)"
    echo "  --service-name NAME  Systemd service name (default: $SERVICE_NAME)"
    echo
    echo "Exit codes:"
    echo "  0 - OK: All checks passed"
    echo "  1 - WARNING: Some non-critical issues detected"
    echo "  2 - CRITICAL: Critical issues require immediate attention"
    echo "  3 - UNKNOWN: Unable to determine status"
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            usage
            exit 0
            ;;
        -v|--verbose)
            set -x
            shift
            ;;
        --health-url)
            HEALTH_URL="$2"
            shift 2
            ;;
        --metrics-url)
            METRICS_URL="$2"
            shift 2
            ;;
        --app-url)
            APP_URL="$2"
            shift 2
            ;;
        --timeout)
            TIMEOUT="$2"
            shift 2
            ;;
        --service-name)
            SERVICE_NAME="$2"
            shift 2
            ;;
        *)
            echo "Unknown option: $1"
            usage
            exit $EXIT_UNKNOWN
            ;;
    esac
done

# Main execution
main() {
    echo "Starting health check for Venue Validation System..."
    echo "Timestamp: $(date)"
    echo
    
    check_service_status
    check_http_endpoint "$HEALTH_URL" "Health Check"
    check_http_endpoint "$METRICS_URL" "Metrics"
    check_http_endpoint "$APP_URL" "Application"
    check_health_details
    check_system_resources
    check_log_errors
    check_database_connectivity
    
    generate_summary
    exit $OVERALL_STATUS
}

# Execute main function
main