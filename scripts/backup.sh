#!/bin/bash

# Database Backup Script for Venue Validation System
# Performs automated backups with rotation and compression

set -e

# Configuration
BACKUP_DIR="${BACKUP_DIR:-/var/backups/venue-validation}"
DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
DB_NAME="${DB_NAME:-venue_validation}"
DB_USER="${DB_USER:-postgres}"
BACKUP_RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-30}"
COMPRESS_BACKUPS="${COMPRESS_BACKUPS:-true}"
BACKUP_PREFIX="venue-validation"

# S3 Configuration (optional)
S3_BUCKET="${S3_BUCKET:-}"
S3_PREFIX="${S3_PREFIX:-backups/venue-validation}"
AWS_PROFILE="${AWS_PROFILE:-default}"

# Notification settings
WEBHOOK_URL="${WEBHOOK_URL:-}"
SLACK_WEBHOOK_URL="${SLACK_WEBHOOK_URL:-}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Logging functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $(date '+%Y-%m-%d %H:%M:%S') $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $(date '+%Y-%m-%d %H:%M:%S') $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $(date '+%Y-%m-%d %H:%M:%S') $1"
}

# Create backup directory
create_backup_dir() {
    if [ ! -d "$BACKUP_DIR" ]; then
        log_info "Creating backup directory: $BACKUP_DIR"
        mkdir -p "$BACKUP_DIR"
    fi
    
    # Set appropriate permissions
    chmod 750 "$BACKUP_DIR"
}

# Perform database backup
perform_backup() {
    local timestamp=$(date '+%Y%m%d_%H%M%S')
    local backup_file="${BACKUP_DIR}/${BACKUP_PREFIX}_${timestamp}.sql"
    local final_backup_file="$backup_file"
    
    log_info "Starting database backup to: $backup_file"
    
    # Check if pg_dump is available
    if ! command -v pg_dump &> /dev/null; then
        log_error "pg_dump not found. Please install PostgreSQL client tools."
        return 1
    fi
    
    # Create backup using pg_dump
    if PGPASSWORD="$PGPASSWORD" pg_dump \
        --host="$DB_HOST" \
        --port="$DB_PORT" \
        --username="$DB_USER" \
        --dbname="$DB_NAME" \
        --format=plain \
        --no-password \
        --verbose \
        --file="$backup_file" \
        --create \
        --clean; then
        
        log_info "Database backup completed successfully"
        
        # Get backup file size
        local backup_size=$(du -h "$backup_file" | cut -f1)
        log_info "Backup size: $backup_size"
        
        # Compress backup if requested
        if [ "$COMPRESS_BACKUPS" = "true" ]; then
            log_info "Compressing backup..."
            gzip "$backup_file"
            final_backup_file="${backup_file}.gz"
            local compressed_size=$(du -h "$final_backup_file" | cut -f1)
            log_info "Compressed backup size: $compressed_size"
        fi
        
        # Upload to S3 if configured
        if [ -n "$S3_BUCKET" ]; then
            upload_to_s3 "$final_backup_file" "$timestamp"
        fi
        
        # Send success notification
        send_notification "SUCCESS" "Database backup completed successfully" "Backup file: $(basename "$final_backup_file"), Size: $(du -h "$final_backup_file" | cut -f1)"
        
        echo "$final_backup_file"
        return 0
    else
        log_error "Database backup failed"
        send_notification "ERROR" "Database backup failed" "Failed to create backup for $DB_NAME"
        return 1
    fi
}

# Upload backup to S3
upload_to_s3() {
    local backup_file="$1"
    local timestamp="$2"
    
    if ! command -v aws &> /dev/null; then
        log_warn "AWS CLI not found, skipping S3 upload"
        return
    fi
    
    local s3_key="${S3_PREFIX}/$(basename "$backup_file")"
    
    log_info "Uploading backup to S3: s3://${S3_BUCKET}/${s3_key}"
    
    if aws s3 cp "$backup_file" "s3://${S3_BUCKET}/${s3_key}" --profile "$AWS_PROFILE"; then
        log_info "Backup uploaded to S3 successfully"
        
        # Add lifecycle tag for automated cleanup
        aws s3api put-object-tagging \
            --bucket "$S3_BUCKET" \
            --key "$s3_key" \
            --tagging "TagSet=[{Key=backup-type,Value=database},{Key=retention-days,Value=${BACKUP_RETENTION_DAYS}}]" \
            --profile "$AWS_PROFILE" 2>/dev/null || log_warn "Could not set S3 object tags"
    else
        log_error "Failed to upload backup to S3"
        send_notification "WARNING" "S3 upload failed" "Local backup completed but S3 upload failed"
    fi
}

# Clean up old backups
cleanup_old_backups() {
    log_info "Cleaning up backups older than $BACKUP_RETENTION_DAYS days"
    
    # Local cleanup
    local deleted_count=0
    while IFS= read -r -d '' backup_file; do
        log_info "Removing old backup: $(basename "$backup_file")"
        rm -f "$backup_file"
        deleted_count=$((deleted_count + 1))
    done < <(find "$BACKUP_DIR" -name "${BACKUP_PREFIX}_*.sql*" -type f -mtime +$BACKUP_RETENTION_DAYS -print0)
    
    if [ $deleted_count -gt 0 ]; then
        log_info "Removed $deleted_count old backup(s)"
    else
        log_info "No old backups to remove"
    fi
    
    # S3 cleanup (if configured and aws cli available)
    if [ -n "$S3_BUCKET" ] && command -v aws &> /dev/null; then
        log_info "Cleaning up old S3 backups"
        
        # List and delete old backups from S3
        local cutoff_date=$(date -d "$BACKUP_RETENTION_DAYS days ago" '+%Y-%m-%d')
        aws s3api list-objects-v2 \
            --bucket "$S3_BUCKET" \
            --prefix "$S3_PREFIX/" \
            --query "Contents[?LastModified<'${cutoff_date}'].Key" \
            --output text \
            --profile "$AWS_PROFILE" | while read -r key; do
            if [ -n "$key" ] && [ "$key" != "None" ]; then
                log_info "Removing old S3 backup: $key"
                aws s3 rm "s3://${S3_BUCKET}/${key}" --profile "$AWS_PROFILE"
            fi
        done
    fi
}

# Verify backup integrity
verify_backup() {
    local backup_file="$1"
    
    log_info "Verifying backup integrity: $(basename "$backup_file")"
    
    # Basic file checks
    if [ ! -f "$backup_file" ]; then
        log_error "Backup file not found: $backup_file"
        return 1
    fi
    
    if [ ! -s "$backup_file" ]; then
        log_error "Backup file is empty: $backup_file"
        return 1
    fi
    
    # Check if it's a gzipped file
    if [[ "$backup_file" == *.gz ]]; then
        if ! gzip -t "$backup_file"; then
            log_error "Backup file is corrupted (gzip test failed): $backup_file"
            return 1
        fi
        
        # Check SQL content in gzipped file
        if ! zcat "$backup_file" | head -n 10 | grep -q "PostgreSQL database dump"; then
            log_error "Backup file does not appear to be a valid PostgreSQL dump"
            return 1
        fi
    else
        # Check SQL content in uncompressed file
        if ! head -n 10 "$backup_file" | grep -q "PostgreSQL database dump"; then
            log_error "Backup file does not appear to be a valid PostgreSQL dump"
            return 1
        fi
    fi
    
    log_info "Backup integrity verification passed"
    return 0
}

# Send notification
send_notification() {
    local status="$1"
    local message="$2"
    local details="$3"
    local timestamp=$(date '+%Y-%m-%d %H:%M:%S')
    
    # Webhook notification
    if [ -n "$WEBHOOK_URL" ]; then
        local payload=$(cat <<EOF
{
    "timestamp": "$timestamp",
    "service": "venue-validation-backup",
    "status": "$status",
    "message": "$message",
    "details": "$details",
    "hostname": "$(hostname)"
}
EOF
)
        
        curl -s -X POST -H "Content-Type: application/json" -d "$payload" "$WEBHOOK_URL" &>/dev/null || {
            log_warn "Failed to send webhook notification"
        }
    fi
    
    # Slack notification
    if [ -n "$SLACK_WEBHOOK_URL" ]; then
        local color="good"
        [ "$status" = "ERROR" ] && color="danger"
        [ "$status" = "WARNING" ] && color="warning"
        
        local slack_payload=$(cat <<EOF
{
    "attachments": [
        {
            "color": "$color",
            "title": "Venue Validation Backup - $status",
            "text": "$message",
            "fields": [
                {
                    "title": "Details",
                    "value": "$details",
                    "short": false
                },
                {
                    "title": "Hostname",
                    "value": "$(hostname)",
                    "short": true
                },
                {
                    "title": "Timestamp",
                    "value": "$timestamp",
                    "short": true
                }
            ]
        }
    ]
}
EOF
)
        
        curl -s -X POST -H "Content-Type: application/json" -d "$slack_payload" "$SLACK_WEBHOOK_URL" &>/dev/null || {
            log_warn "Failed to send Slack notification"
        }
    fi
}

# Print usage information
usage() {
    echo "Usage: $0 [OPTIONS]"
    echo "Options:"
    echo "  -h, --help              Show this help message"
    echo "  -v, --verbose           Enable verbose output"
    echo "  --backup-dir DIR        Backup directory (default: $BACKUP_DIR)"
    echo "  --db-host HOST          Database host (default: $DB_HOST)"
    echo "  --db-port PORT          Database port (default: $DB_PORT)"
    echo "  --db-name NAME          Database name (default: $DB_NAME)"
    echo "  --db-user USER          Database user (default: $DB_USER)"
    echo "  --retention-days DAYS   Backup retention in days (default: $BACKUP_RETENTION_DAYS)"
    echo "  --no-compress           Disable backup compression"
    echo "  --s3-bucket BUCKET      S3 bucket for backup upload"
    echo "  --s3-prefix PREFIX      S3 key prefix (default: $S3_PREFIX)"
    echo "  --verify-only FILE      Only verify an existing backup file"
    echo "  --cleanup-only          Only perform cleanup of old backups"
    echo
    echo "Environment Variables:"
    echo "  PGPASSWORD              PostgreSQL password"
    echo "  WEBHOOK_URL             Webhook URL for notifications"
    echo "  SLACK_WEBHOOK_URL       Slack webhook URL for notifications"
}

# Parse command line arguments
VERIFY_ONLY=""
CLEANUP_ONLY=false

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
        --backup-dir)
            BACKUP_DIR="$2"
            shift 2
            ;;
        --db-host)
            DB_HOST="$2"
            shift 2
            ;;
        --db-port)
            DB_PORT="$2"
            shift 2
            ;;
        --db-name)
            DB_NAME="$2"
            shift 2
            ;;
        --db-user)
            DB_USER="$2"
            shift 2
            ;;
        --retention-days)
            BACKUP_RETENTION_DAYS="$2"
            shift 2
            ;;
        --no-compress)
            COMPRESS_BACKUPS="false"
            shift
            ;;
        --s3-bucket)
            S3_BUCKET="$2"
            shift 2
            ;;
        --s3-prefix)
            S3_PREFIX="$2"
            shift 2
            ;;
        --verify-only)
            VERIFY_ONLY="$2"
            shift 2
            ;;
        --cleanup-only)
            CLEANUP_ONLY=true
            shift
            ;;
        *)
            echo "Unknown option: $1"
            usage
            exit 1
            ;;
    esac
done

# Main execution
main() {
    log_info "Starting backup process for Venue Validation System"
    
    # Verify-only mode
    if [ -n "$VERIFY_ONLY" ]; then
        if verify_backup "$VERIFY_ONLY"; then
            log_info "Backup verification completed successfully"
            exit 0
        else
            log_error "Backup verification failed"
            exit 1
        fi
    fi
    
    # Cleanup-only mode
    if [ "$CLEANUP_ONLY" = true ]; then
        create_backup_dir
        cleanup_old_backups
        log_info "Backup cleanup completed"
        exit 0
    fi
    
    # Full backup process
    create_backup_dir
    
    if backup_file=$(perform_backup); then
        if verify_backup "$backup_file"; then
            log_info "Backup process completed successfully"
            cleanup_old_backups
            exit 0
        else
            log_error "Backup verification failed"
            exit 1
        fi
    else
        log_error "Backup process failed"
        exit 1
    fi
}

# Execute main function
main