package database

import (
	"context"
	"database/sql"

	"assisted-venue-approval/internal/domain"
	errs "assisted-venue-approval/pkg/errors"
)

// CreateAuditLogCtx inserts a new audit log entry
func (db *DB) CreateAuditLogCtx(ctx context.Context, log *domain.VenueValidationAuditLog) error {
	ctx, cancel := db.withWriteTimeout(ctx)
	defer cancel()

	query := `INSERT INTO venue_validation_audit_logs
	          (venue_id, history_id, admin_id, status, reason, data_replacements, created_at)
	          VALUES (?, ?, ?, ?, ?, ?, ?)`

	result, err := db.conn.ExecContext(ctx, query,
		log.VenueID,
		log.HistoryID,
		log.AdminID,
		log.Status,
		log.Reason,
		log.DataReplacements,
		log.CreatedAt,
	)

	if err != nil {
		return errs.NewDB("CreateAuditLogCtx", "failed to insert audit log", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return errs.NewDB("CreateAuditLogCtx", "failed to get last insert ID", err)
	}

	log.ID = id
	return nil
}

// GetAuditLogsByHistoryIDCtx retrieves all audit logs for a specific validation history
func (db *DB) GetAuditLogsByHistoryIDCtx(ctx context.Context, historyID int64) ([]domain.VenueValidationAuditLog, error) {
	ctx, cancel := db.withReadTimeout(ctx)
	defer cancel()

	query := `SELECT id, venue_id, history_id, admin_id, status, reason, data_replacements, created_at
	          FROM venue_validation_audit_logs
	          WHERE history_id = ?
	          ORDER BY created_at DESC`

	rows, err := db.conn.QueryContext(ctx, query, historyID)
	if err != nil {
		return nil, errs.NewDB("GetAuditLogsByHistoryIDCtx", "failed to query audit logs", err)
	}
	defer rows.Close()

	var logs []domain.VenueValidationAuditLog
	for rows.Next() {
		var log domain.VenueValidationAuditLog
		var historyID sql.NullInt64
		var adminID sql.NullInt32
		var reason sql.NullString
		var dataReplacements sql.NullString

		if err := rows.Scan(
			&log.ID,
			&log.VenueID,
			&historyID,
			&adminID,
			&log.Status,
			&reason,
			&dataReplacements,
			&log.CreatedAt,
		); err != nil {
			return nil, errs.NewDB("GetAuditLogsByHistoryIDCtx", "failed to scan audit log", err)
		}

		if historyID.Valid {
			hid := historyID.Int64
			log.HistoryID = &hid
		}

		if adminID.Valid {
			id := int(adminID.Int32)
			log.AdminID = &id
		}

		if reason.Valid {
			log.Reason = &reason.String
		}

		if dataReplacements.Valid {
			log.DataReplacements = &dataReplacements.String
		}

		logs = append(logs, log)
	}

	if err = rows.Err(); err != nil {
		return nil, errs.NewDB("GetAuditLogsByHistoryIDCtx", "row iteration error", err)
	}

	return logs, nil
}

// GetAuditLogsByAdminIDCtx retrieves audit logs for a specific admin with pagination
func (db *DB) GetAuditLogsByAdminIDCtx(ctx context.Context, adminID int, limit int, offset int) ([]domain.VenueValidationAuditLog, int, error) {
	ctx, cancel := db.withReadTimeout(ctx)
	defer cancel()

	// Get total count
	var total int
	countQuery := `SELECT COUNT(*) FROM venue_validation_audit_logs WHERE admin_id = ?`
	if err := db.conn.QueryRowContext(ctx, countQuery, adminID).Scan(&total); err != nil {
		return nil, 0, errs.NewDB("GetAuditLogsByAdminIDCtx", "failed to count audit logs", err)
	}

	// Get logs
	query := `SELECT id, venue_id, history_id, admin_id, status, reason, data_replacements, created_at
	          FROM venue_validation_audit_logs
	          WHERE admin_id = ?
	          ORDER BY created_at DESC
	          LIMIT ? OFFSET ?`

	rows, err := db.conn.QueryContext(ctx, query, adminID, limit, offset)
	if err != nil {
		return nil, 0, errs.NewDB("GetAuditLogsByAdminIDCtx", "failed to query audit logs", err)
	}
	defer rows.Close()

	var logs []domain.VenueValidationAuditLog
	for rows.Next() {
		var log domain.VenueValidationAuditLog
		var historyID sql.NullInt64
		var adminIDVal sql.NullInt32
		var reason sql.NullString
		var dataReplacements sql.NullString

		if err := rows.Scan(
			&log.ID,
			&log.VenueID,
			&historyID,
			&adminIDVal,
			&log.Status,
			&reason,
			&dataReplacements,
			&log.CreatedAt,
		); err != nil {
			return nil, 0, errs.NewDB("GetAuditLogsByAdminIDCtx", "failed to scan audit log", err)
		}

		if historyID.Valid {
			hid := historyID.Int64
			log.HistoryID = &hid
		}

		if adminIDVal.Valid {
			id := int(adminIDVal.Int32)
			log.AdminID = &id
		}

		if reason.Valid {
			log.Reason = &reason.String
		}

		if dataReplacements.Valid {
			log.DataReplacements = &dataReplacements.String
		}

		logs = append(logs, log)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, errs.NewDB("GetAuditLogsByAdminIDCtx", "row iteration error", err)
	}

	return logs, total, nil
}
