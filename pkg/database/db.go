package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"assisted-venue-approval/internal/constants"
	"assisted-venue-approval/internal/models"
	"assisted-venue-approval/pkg/config"
	errs "assisted-venue-approval/pkg/errors"

	_ "github.com/go-sql-driver/mysql"
)

type DB struct {
	conn         *sql.DB
	stmts        map[string]*sql.Stmt
	readTimeout  time.Duration
	writeTimeout time.Duration
}

func New(databaseURL string) (*DB, error) {
	conn, err := sql.Open("mysql", databaseURL)
	if err != nil {
		return nil, err
	}

	// Use configurable or default settings
	conn.SetMaxOpenConns(50) // Default optimized settings
	conn.SetMaxIdleConns(15)
	conn.SetConnMaxLifetime(10 * time.Minute)
	conn.SetConnMaxIdleTime(5 * time.Minute)

	if err := conn.Ping(); err != nil {
		return nil, err
	}

	db := &DB{
		conn:         conn,
		stmts:        make(map[string]*sql.Stmt),
		readTimeout:  constants.DBReadTimeoutDefault,
		writeTimeout: constants.DBWriteTimeoutDefault,
	}

	if err := db.prepareStatements(); err != nil {
		return nil, errs.NewDB("database.New", "failed to prepare statements", err)
	}

	return db, nil
}

// NewWithConfig creates a database connection with custom configuration settings
func NewWithConfig(databaseURL string, cfg *config.Config) (*DB, error) {
	conn, err := sql.Open("mysql", databaseURL)
	if err != nil {
		return nil, err
	}

	// Use configuration values for connection pool settings
	conn.SetMaxOpenConns(cfg.DBMaxOpenConns)
	conn.SetMaxIdleConns(cfg.DBMaxIdleConns)
	conn.SetConnMaxLifetime(time.Duration(cfg.DBConnMaxLifetime) * time.Minute)
	conn.SetConnMaxIdleTime(time.Duration(cfg.DBConnMaxIdleTime) * time.Minute)

	if err := conn.Ping(); err != nil {
		return nil, err
	}

	rt := cfg.DBReadTimeout
	if rt == 0 {
		rt = constants.DBReadTimeoutDefault
	}
	wt := cfg.DBWriteTimeout
	if wt == 0 {
		wt = constants.DBWriteTimeoutDefault
	}

	db := &DB{
		conn:         conn,
		stmts:        make(map[string]*sql.Stmt),
		readTimeout:  rt,
		writeTimeout: wt,
	}

	if err := db.prepareStatements(); err != nil {
		return nil, errs.NewDB("database.NewWithConfig", "failed to prepare statements", err)
	}

	return db, nil
}

// prepareStatements prepares frequently used SQL statements
func (db *DB) prepareStatements() error {
	statements := map[string]string{
		"updateVenueStatus": `UPDATE venues SET active = ?, admin_note = ?, admin_last_update = NOW(), 
                             made_active_by_id = ?, made_active_at = CASE WHEN ? = 1 THEN NOW() ELSE made_active_at END 
                             WHERE id = ?`,
		"insertValidationHistory": `INSERT INTO venue_validation_histories 
                                   (venue_id, validation_score, validation_status, validation_notes, 
                                    score_breakdown, google_place_id, google_place_found, google_place_data, ai_output_data, processed_at) 
                                   VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NOW())`,
	}

	for name, query := range statements {
		stmt, err := db.conn.Prepare(query)
		if err != nil {
			return errs.NewDB("database.prepareStatements", fmt.Sprintf("failed to prepare statement %s", name), err)
		}
		db.stmts[name] = stmt
	}

	return nil
}

// Close closes database connection and prepared statements
func (db *DB) Close() error {
	for _, stmt := range db.stmts {
		stmt.Close()
	}
	return db.conn.Close()
}

// withReadTimeout creates a context with standard read timeout.
func (db *DB) withReadTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, db.readTimeout)
}

// withWriteTimeout creates a context with standard write timeout.
func (db *DB) withWriteTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, db.writeTimeout)
}

// GetPendingVenues retrieves all pending venues with complete field data
func (db *DB) GetPendingVenues() ([]models.Venue, error) {
	query := `SELECT 
        id, path, entrytype, name, url, fburl, instagram_url, location, zipcode, phone,
        other_food_type, price, additionalinfo, vdetails, openhours, openhours_note,
        timezone, hash, email, ownername, sentby, user_id, active, vegonly, vegan,
        sponsor_level, crossstreet, lat, lng, created_at, date_added, date_updated,
        admin_last_update, admin_note, admin_hold, admin_hold_email_note,
        updated_by_id, made_active_by_id, made_active_at, show_premium, category,
        pretty_url, edit_lock, request_vegan_decal_at, request_excellent_decal_at, source
        FROM venues 
        WHERE active = 0 
        ORDER BY created_at ASC`

	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, errs.NewDB("database.GetPendingVenues", "failed to query pending venues", err)
	}
	defer rows.Close()

	var venues []models.Venue
	for rows.Next() {
		v, err := db.scanVenueRow(rows)
		if err != nil {
			return nil, errs.NewDB("database.GetPendingVenues", "failed to scan venue row", err)
		}
		venues = append(venues, *v)
	}

	if err = rows.Err(); err != nil {
		return nil, errs.NewDB("database.GetPendingVenues", "row iteration error", err)
	}

	return venues, nil
}

// scanVenueRow scans a complete venue row into a Venue struct
func (db *DB) scanVenueRow(rows *sql.Rows) (*models.Venue, error) {
	var v models.Venue
	err := rows.Scan(
		&v.ID, &v.Path, &v.EntryType, &v.Name, &v.URL, &v.FBUrl, &v.InstagramUrl,
		&v.Location, &v.Zipcode, &v.Phone, &v.OtherFoodType, &v.Price,
		&v.AdditionalInfo, &v.VDetails, &v.OpenHours, &v.OpenHoursNote,
		&v.Timezone, &v.Hash, &v.Email, &v.OwnerName, &v.SentBy, &v.UserID,
		&v.Active, &v.VegOnly, &v.Vegan, &v.SponsorLevel, &v.CrossStreet,
		&v.Lat, &v.Lng, &v.CreatedAt, &v.DateAdded, &v.DateUpdated,
		&v.AdminLastUpdate, &v.AdminNote, &v.AdminHold, &v.AdminHoldEmailNote,
		&v.UpdatedByID, &v.MadeActiveByID, &v.MadeActiveAt, &v.ShowPremium,
		&v.Category, &v.PrettyUrl, &v.EditLock, &v.RequestVeganDecalAt,
		&v.RequestExcellentDecalAt, &v.Source,
	)
	return &v, err
}

// GetPendingVenuesWithUser retrieves pending venues with user information for authority checking
func (db *DB) GetPendingVenuesWithUser() ([]models.VenueWithUser, error) {
	query := `SELECT 
        v.id, v.path, v.entrytype, v.name, v.url, v.fburl, v.instagram_url, 
        v.location, v.zipcode, v.phone, v.other_food_type, v.price, v.additionalinfo,
        v.vdetails, v.openhours, v.openhours_note, v.timezone, v.hash, v.email,
        v.ownername, v.sentby, v.user_id, v.active, v.vegonly, v.vegan,
        v.sponsor_level, v.crossstreet, v.lat, v.lng, v.created_at, v.date_added,
        v.date_updated, v.admin_last_update, v.admin_note, v.admin_hold,
        v.admin_hold_email_note, v.updated_by_id, v.made_active_by_id,
        v.made_active_at, v.show_premium, v.category, v.pretty_url, v.edit_lock,
        v.request_vegan_decal_at, v.request_excellent_decal_at, v.source,
        m.username, m.email as user_email, m.trusted, m.contributions,
        CASE WHEN va.venue_id IS NOT NULL THEN 1 ELSE 0 END as is_venue_admin,
        a.level as ambassador_level, a.points as ambassador_points, a.path as ambassador_region,
        (SELECT COUNT(*) FROM venues v2 WHERE v2.user_id = m.id AND v2.active = 1) as approved_venue_count
        FROM venues v
        LEFT JOIN members m ON v.user_id = m.id
        LEFT JOIN venue_admin va ON v.id = va.venue_id AND v.user_id = va.user_id
        LEFT JOIN ambassadors a ON v.user_id = a.user_id
        WHERE v.active = 0
        ORDER BY v.created_at ASC`

	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, errs.NewDB("database.GetPendingVenuesWithUser", "failed to query pending venues with user info", err)
	}
	defer rows.Close()

	var venues []models.VenueWithUser
	for rows.Next() {
		var vu models.VenueWithUser
		var venue models.Venue
		var user models.User

		// Use nullable types to handle LEFT JOIN
		var username, email sql.NullString
		var trusted, contributions sql.NullInt64
		var isVenueAdmin sql.NullInt64
		var ambassadorLevel, ambassadorPoints sql.NullInt64
		var ambassadorRegion sql.NullString
		var approvedVenueCount sql.NullInt64

		err := rows.Scan(
			// Venue fields
			&venue.ID, &venue.Path, &venue.EntryType, &venue.Name, &venue.URL,
			&venue.FBUrl, &venue.InstagramUrl, &venue.Location, &venue.Zipcode,
			&venue.Phone, &venue.OtherFoodType, &venue.Price, &venue.AdditionalInfo,
			&venue.VDetails, &venue.OpenHours, &venue.OpenHoursNote, &venue.Timezone,
			&venue.Hash, &venue.Email, &venue.OwnerName, &venue.SentBy, &venue.UserID,
			&venue.Active, &venue.VegOnly, &venue.Vegan, &venue.SponsorLevel,
			&venue.CrossStreet, &venue.Lat, &venue.Lng, &venue.CreatedAt,
			&venue.DateAdded, &venue.DateUpdated, &venue.AdminLastUpdate,
			&venue.AdminNote, &venue.AdminHold, &venue.AdminHoldEmailNote,
			&venue.UpdatedByID, &venue.MadeActiveByID, &venue.MadeActiveAt,
			&venue.ShowPremium, &venue.Category, &venue.PrettyUrl, &venue.EditLock,
			&venue.RequestVeganDecalAt, &venue.RequestExcellentDecalAt, &venue.Source,
			// User fields (nullable)
			&username, &email, &trusted, &contributions,
			&isVenueAdmin, &ambassadorLevel, &ambassadorPoints,
			&ambassadorRegion, &approvedVenueCount,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan venue with user row: %w", err)
		}

		// Handle nullable user fields
		if username.Valid {
			user.Username = username.String
		}
		if email.Valid {
			user.Email = email.String
		}
		if trusted.Valid {
			user.Trusted = trusted.Int64 > 0
		}
		if contributions.Valid {
			user.Contributions = int(contributions.Int64)
		}

		user.ID = venue.UserID
		vu.Venue = venue
		vu.User = user

		// Handle venue admin and ambassador data
		if isVenueAdmin.Valid {
			vu.IsVenueAdmin = isVenueAdmin.Int64 > 0
			vu.User.IsVenueAdmin = isVenueAdmin.Int64 > 0
		}
		if ambassadorLevel.Valid {
			level := int(ambassadorLevel.Int64)
			vu.User.AmbassadorLevel = &level
		}
		if ambassadorPoints.Valid {
			points := int(ambassadorPoints.Int64)
			vu.User.AmbassadorPoints = &points
		}
		if ambassadorRegion.Valid {
			vu.User.AmbassadorRegion = &ambassadorRegion.String
		}
		if approvedVenueCount.Valid {
			count := int(approvedVenueCount.Int64)
			vu.User.ApprovedVenueCount = &count
		}

		venues = append(venues, vu)
	}

	return venues, nil
}

// UpdateVenueStatus updates venue status using prepared statement
func (db *DB) UpdateVenueStatus(venueID int64, active int, notes string, reviewer *string) error {
	query := `UPDATE venues SET 
        active = ?, 
        admin_note = ?, 
        admin_last_update = NOW()
        WHERE id = ?`

	_, err := db.conn.Exec(query, active, notes, venueID)
	if err != nil {
		return fmt.Errorf("failed to update venue status: %w", err)
	}
	return nil
}

// UpdateVenueStatusCtx updates venue status with context.
func (db *DB) UpdateVenueStatusCtx(ctx context.Context, venueID int64, active int, notes string, reviewer *string) error {
	ctx, cancel := db.withWriteTimeout(ctx)
	defer cancel()
	query := `UPDATE venues SET 
        active = ?, 
        admin_note = ?, 
        admin_last_update = NOW()
        WHERE id = ?`
	if _, err := db.conn.ExecContext(ctx, query, active, notes, venueID); err != nil {
		return fmt.Errorf("failed to update venue status: %w", err)
	}
	return nil
}

// BatchUpdateVenueStatus updates multiple venues in a single transaction
func (db *DB) BatchUpdateVenueStatus(venueIDs []int64, active int, notes string, updatedByID *int) error {
	if len(venueIDs) == 0 {
		return nil
	}

	tx, err := db.conn.Begin()
	if err != nil {
		return errs.NewDB("database.BatchUpdateVenueStatus", "failed to begin transaction", err)
	}
	defer tx.Rollback()

	// Build placeholders for IN clause
	placeholders := make([]string, len(venueIDs))
	args := make([]interface{}, 0, len(venueIDs)+4)
	args = append(args, active, notes, updatedByID, active)

	for i, id := range venueIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}

	query := fmt.Sprintf(`UPDATE venues SET active = ?, admin_note = ?, admin_last_update = NOW(), 
	                         made_active_by_id = ?, made_active_at = CASE WHEN ? = 1 THEN NOW() ELSE made_active_at END 
	                         WHERE id IN (%s)`, strings.Join(placeholders, ","))

	_, err = tx.Exec(query, args...)
	if err != nil {
		return errs.NewDB("database.BatchUpdateVenueStatus", "failed to batch update venues", err)
	}

	if err = tx.Commit(); err != nil {
		return errs.NewDB("database.BatchUpdateVenueStatus", "failed to commit batch update transaction", err)
	}

	return nil
}

// UpdateVenueActive updates only the active status and admin_last_update, keeping admin_note unchanged.
func (db *DB) UpdateVenueActive(venueID int64, active int) error {
	query := `UPDATE venues SET active = ?, admin_last_update = NOW() WHERE id = ?`
	_, err := db.conn.Exec(query, active, venueID)
	if err != nil {
		return fmt.Errorf("failed to update venue active status: %w", err)
	}
	return nil
}

// SaveValidationResult saves validation results ONLY into venue_validation_histories (no changes to venues table)
func (db *DB) SaveValidationResult(result *models.ValidationResult) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin validation result transaction: %w", err)
	}
	defer tx.Rollback()

	// Prepare payload
	scoreBreakdownJSON, err := json.Marshal(result.ScoreBreakdown)
	if err != nil {
		return fmt.Errorf("failed to marshal score breakdown: %w", err)
	}

	historyQuery := `INSERT INTO venue_validation_histories 
	    (venue_id, validation_score, validation_status, validation_notes, 
	     score_breakdown, ai_output_data, prompt_version, processed_at) 
	    VALUES (?, ?, ?, ?, ?, ?, ?, NOW())`
	args := []any{result.VenueID, result.Score, result.Status, result.Notes, string(scoreBreakdownJSON), result.AIOutputData, result.PromptVersion}

	if _, err = tx.Exec(historyQuery, args...); err != nil {
		return fmt.Errorf("failed to insert validation history: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit validation result transaction: %w", err)
	}

	return nil
}

// SaveValidationResultCtx is a context-aware variant with timeout for DB writes
func (db *DB) SaveValidationResultCtx(ctx context.Context, result *models.ValidationResult) error {
	ctx, cancel := db.withWriteTimeout(ctx)
	defer cancel()

	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin validation result transaction: %w", err)
	}
	defer tx.Rollback()

	// Prepare payload
	scoreBreakdownJSON, err := json.Marshal(result.ScoreBreakdown)
	if err != nil {
		return fmt.Errorf("failed to marshal score breakdown: %w", err)
	}

	historyQuery := `INSERT INTO venue_validation_histories 
	    (venue_id, validation_score, validation_status, validation_notes, 
	     score_breakdown, ai_output_data, prompt_version, processed_at) 
	    VALUES (?, ?, ?, ?, ?, ?, ?, NOW())`
	args := []any{result.VenueID, result.Score, result.Status, result.Notes, string(scoreBreakdownJSON), result.AIOutputData, result.PromptVersion}

	if _, err = tx.ExecContext(ctx, historyQuery, args...); err != nil {
		return fmt.Errorf("failed to insert validation history: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit validation result transaction: %w", err)
	}
	return nil
}

// GetValidationHistory retrieves validation history for a venue
func (db *DB) GetValidationHistory(venueID int64) ([]models.ValidationHistory, error) {
	query := `SELECT id, venue_id, validation_score, validation_status, validation_notes,
	            score_breakdown, ai_output_data, prompt_version, processed_at 
	            FROM venue_validation_histories 
	            WHERE venue_id = ? 
	            ORDER BY processed_at DESC`

	rows, err := db.conn.Query(query, venueID)
	if err != nil {
		return nil, fmt.Errorf("failed to query validation history: %w", err)
	}
	defer rows.Close()

	var history []models.ValidationHistory
	for rows.Next() {
		var h models.ValidationHistory
		var scoreBreakdownJSON string
		var aiOutput sql.NullString
		var pv sql.NullString

		if err := rows.Scan(&h.ID, &h.VenueID, &h.ValidationScore, &h.ValidationStatus,
			&h.ValidationNotes, &scoreBreakdownJSON, &aiOutput, &pv, &h.ProcessedAt); err != nil {
			return nil, fmt.Errorf("failed to scan validation history row: %w", err)
		}

		if err = json.Unmarshal([]byte(scoreBreakdownJSON), &h.ScoreBreakdown); err != nil {
			return nil, fmt.Errorf("failed to unmarshal score breakdown: %w", err)
		}
		if aiOutput.Valid {
			val := aiOutput.String
			h.AIOutputData = &val
		}
		if pv.Valid {
			val := pv.String
			h.PromptVersion = &val
		}

		history = append(history, h)
	}

	return history, nil
}

// GetVenuesByStatus retrieves venues by their status with pagination
func (db *DB) GetVenuesByStatus(status int, limit, offset int) ([]models.Venue, error) {
	query := `SELECT 
        id, path, entrytype, name, url, fburl, instagram_url, location, zipcode, phone,
        other_food_type, price, additionalinfo, vdetails, openhours, openhours_note,
        timezone, hash, email, ownername, sentby, user_id, active, vegonly, vegan,
        sponsor_level, crossstreet, lat, lng, created_at, date_added, date_updated,
        admin_last_update, admin_note, admin_hold, admin_hold_email_note,
        updated_by_id, made_active_by_id, made_active_at, show_premium, category,
        pretty_url, edit_lock, request_vegan_decal_at, request_excellent_decal_at, source
        FROM venues 
        WHERE active = ?
        ORDER BY admin_last_update DESC, created_at DESC
        LIMIT ? OFFSET ?`

	rows, err := db.conn.Query(query, status, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query venues by status: %w", err)
	}
	defer rows.Close()

	var venues []models.Venue
	for rows.Next() {
		v, err := db.scanVenueRow(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan venue row: %w", err)
		}
		venues = append(venues, *v)
	}

	return venues, nil
}

// GetVenueStats returns processing statistics
func (db *DB) GetVenueStats() (*models.VenueStats, error) {
	query := `SELECT 
        COUNT(CASE WHEN active = 0 THEN 1 END) as pending,
        COUNT(CASE WHEN active = 1 THEN 1 END) as approved,
        COUNT(CASE WHEN active = -1 THEN 1 END) as rejected,
        COUNT(*) as total
        FROM venues`

	var stats models.VenueStats
	err := db.conn.QueryRow(query).Scan(&stats.Pending, &stats.Approved,
		&stats.Rejected, &stats.Total)
	if err != nil {
		return nil, fmt.Errorf("failed to get venue stats: %w", err)
	}

	return &stats, nil
}

// Additional methods for admin interface

// GetVenueStatistics returns comprehensive venue statistics
func (db *DB) GetVenueStatistics() (*models.VenueStats, error) {
	return db.GetVenueStats()
}

// GetRecentValidationResults returns recent validation results
func (db *DB) GetRecentValidationResults(limit int) ([]models.ValidationResult, error) {
	query := `SELECT 
        venue_id, validation_score, validation_status, validation_notes,
        score_breakdown, processed_at
        FROM venue_validation_histories 
        ORDER BY processed_at DESC 
        LIMIT ?`

	rows, err := db.conn.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query recent validation results: %w", err)
	}
	defer rows.Close()

	var results []models.ValidationResult
	for rows.Next() {
		var result models.ValidationResult
		var scoreBreakdownJSON string
		var processedAt time.Time

		err := rows.Scan(&result.VenueID, &result.Score, &result.Status,
			&result.Notes, &scoreBreakdownJSON, &processedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan validation result row: %w", err)
		}

		if err = json.Unmarshal([]byte(scoreBreakdownJSON), &result.ScoreBreakdown); err != nil {
			return nil, fmt.Errorf("failed to unmarshal score breakdown: %w", err)
		}

		results = append(results, result)
	}

	return results, nil
}

// GetVenuesFiltered returns filtered venues with pagination
func (db *DB) GetVenuesFiltered(status, search string, limit, offset int) ([]models.VenueWithUser, int, error) {
	// Build WHERE clause based on filters
	whereClause := "WHERE 1=1"
	args := []interface{}{}

	if status != "" {
		switch status {
		case "pending":
			whereClause += " AND v.active = ?"
			args = append(args, 0)
		case "approved":
			whereClause += " AND v.active = ?"
			args = append(args, 1)
		case "rejected":
			whereClause += " AND v.active = ?"
			args = append(args, -1)
		}
	}

	if search != "" {
		whereClause += " AND (v.name LIKE ? OR v.location LIKE ? OR m.username LIKE ?)"
		searchPattern := "%" + search + "%"
		args = append(args, searchPattern, searchPattern, searchPattern)
	}

	// Get total count for pagination
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM venues v 
        LEFT JOIN members m ON v.user_id = m.id 
        LEFT JOIN venue_admin va ON v.id = va.venue_id AND m.id = va.user_id
        LEFT JOIN ambassadors a ON m.id = a.user_id %s`, whereClause)

	var total int
	err := db.conn.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get filtered venues count: %w", err)
	}

	// Get venues with pagination
	query := fmt.Sprintf(`SELECT v.id, v.path, v.entrytype, v.name, v.url, v.fburl, v.instagram_url, 
        v.location, v.zipcode, v.phone, v.other_food_type, v.price, v.additionalinfo, 
        v.vdetails, v.openhours, v.openhours_note, v.timezone, v.hash, v.email, 
        v.ownername, v.sentby, v.user_id, v.active, v.vegonly, v.vegan, v.sponsor_level, 
        v.crossstreet, v.lat, v.lng, v.created_at, v.date_added, v.date_updated, 
        v.admin_last_update, v.admin_note, v.admin_hold, v.admin_hold_email_note, 
        v.updated_by_id, v.made_active_by_id, v.made_active_at, v.show_premium, 
        v.category, v.pretty_url, v.edit_lock, v.request_vegan_decal_at, 
        v.request_excellent_decal_at, v.source,
        m.id as member_id, m.username, m.trusted,
        va.venue_id IS NOT NULL as is_venue_admin,
        a.level as ambassador_level, a.points as ambassador_points, a.path as ambassador_path
        FROM venues v 
        LEFT JOIN members m ON v.user_id = m.id 
        LEFT JOIN venue_admin va ON v.id = va.venue_id AND m.id = va.user_id
        LEFT JOIN ambassadors a ON m.id = a.user_id
        %s
        ORDER BY v.admin_last_update DESC, v.created_at DESC
        LIMIT ? OFFSET ?`, whereClause)

	args = append(args, limit, offset)

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query filtered venues: %w", err)
	}
	defer rows.Close()

	var venues []models.VenueWithUser
	for rows.Next() {
		var venueWithUser models.VenueWithUser
		var venue models.Venue
		var user models.User
		var isVenueAdmin bool
		var ambassadorLevel, ambassadorPoints sql.NullInt64
		var ambassadorPath sql.NullString

		// Use nullable types for user fields
		var memberID sql.NullInt64
		var username sql.NullString
		var trusted sql.NullInt64

		err := rows.Scan(
			&venue.ID, &venue.Path, &venue.EntryType, &venue.Name, &venue.URL,
			&venue.FBUrl, &venue.InstagramUrl, &venue.Location, &venue.Zipcode,
			&venue.Phone, &venue.OtherFoodType, &venue.Price, &venue.AdditionalInfo,
			&venue.VDetails, &venue.OpenHours, &venue.OpenHoursNote, &venue.Timezone,
			&venue.Hash, &venue.Email, &venue.OwnerName, &venue.SentBy, &venue.UserID,
			&venue.Active, &venue.VegOnly, &venue.Vegan, &venue.SponsorLevel,
			&venue.CrossStreet, &venue.Lat, &venue.Lng, &venue.CreatedAt,
			&venue.DateAdded, &venue.DateUpdated, &venue.AdminLastUpdate,
			&venue.AdminNote, &venue.AdminHold, &venue.AdminHoldEmailNote,
			&venue.UpdatedByID, &venue.MadeActiveByID, &venue.MadeActiveAt,
			&venue.ShowPremium, &venue.Category, &venue.PrettyUrl, &venue.EditLock,
			&venue.RequestVeganDecalAt, &venue.RequestExcellentDecalAt, &venue.Source,
			// User fields (nullable)
			&memberID, &username, &trusted,
			// Authority fields
			&isVenueAdmin, &ambassadorLevel, &ambassadorPoints, &ambassadorPath,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan venue with user row: %w", err)
		}

		// Handle nullable user fields
		if memberID.Valid {
			user.ID = uint(memberID.Int64)
		} else {
			user.ID = venue.UserID // Use venue's user_id as fallback
		}
		if username.Valid {
			user.Username = username.String
		}
		if trusted.Valid {
			user.Trusted = trusted.Int64 > 0
		}

		venueWithUser.Venue = venue
		venueWithUser.User = user
		venueWithUser.IsVenueAdmin = isVenueAdmin

		if ambassadorLevel.Valid {
			venueWithUser.AmbassadorLevel = &ambassadorLevel.Int64
		}
		if ambassadorPoints.Valid {
			venueWithUser.AmbassadorPoints = &ambassadorPoints.Int64
		}
		if ambassadorPath.Valid {
			venueWithUser.AmbassadorPath = &ambassadorPath.String
		}

		venues = append(venues, venueWithUser)
	}

	return venues, total, nil
}

// GetVenueWithUserByID returns a venue with user data by ID
func (db *DB) GetVenueWithUserByID(venueID int64) (*models.VenueWithUser, error) {
	query := `SELECT v.id, v.path, v.entrytype, v.name, v.url, v.fburl, v.instagram_url, 
        v.location, v.zipcode, v.phone, v.other_food_type, v.price, v.additionalinfo, 
        v.vdetails, v.openhours, v.openhours_note, v.timezone, v.hash, v.email, 
        v.ownername, v.sentby, v.user_id, v.active, v.vegonly, v.vegan, 
        v.sponsor_level, v.crossstreet, v.lat, v.lng, v.created_at, v.date_added, 
        v.date_updated, v.admin_last_update, v.admin_note, v.admin_hold, 
        v.admin_hold_email_note, v.updated_by_id, v.made_active_by_id, 
        v.made_active_at, v.show_premium, v.category, v.pretty_url, v.edit_lock, 
        v.request_vegan_decal_at, v.request_excellent_decal_at, v.source,
        m.id as member_id, m.username, m.email as user_email, m.trusted, m.contributions,
        CASE WHEN va.venue_id IS NOT NULL THEN 1 ELSE 0 END as is_venue_admin,
        a.level as ambassador_level, a.points as ambassador_points, a.path as ambassador_region,
        (SELECT COUNT(*) FROM venues v2 WHERE v2.user_id = m.id AND v2.active = 1) as approved_venue_count
        FROM venues v 
        JOIN members m ON v.user_id = m.id 
        LEFT JOIN venue_admin va ON v.id = va.venue_id AND m.id = va.user_id
        LEFT JOIN ambassadors a ON m.id = a.user_id
        WHERE v.id = ?`

	var venueWithUser models.VenueWithUser
	var venue models.Venue
	var user models.User
	var isVenueAdmin sql.NullInt64
	var ambassadorLevel, ambassadorPoints sql.NullInt64
	var trustedInt int
	var ambassadorRegion sql.NullString
	var approvedVenueCount sql.NullInt64

	err := db.conn.QueryRow(query, venueID).Scan(
		&venue.ID, &venue.Path, &venue.EntryType, &venue.Name, &venue.URL,
		&venue.FBUrl, &venue.InstagramUrl, &venue.Location, &venue.Zipcode,
		&venue.Phone, &venue.OtherFoodType, &venue.Price, &venue.AdditionalInfo,
		&venue.VDetails, &venue.OpenHours, &venue.OpenHoursNote, &venue.Timezone,
		&venue.Hash, &venue.Email, &venue.OwnerName, &venue.SentBy, &venue.UserID,
		&venue.Active, &venue.VegOnly, &venue.Vegan, &venue.SponsorLevel,
		&venue.CrossStreet, &venue.Lat, &venue.Lng, &venue.CreatedAt,
		&venue.DateAdded, &venue.DateUpdated, &venue.AdminLastUpdate,
		&venue.AdminNote, &venue.AdminHold, &venue.AdminHoldEmailNote,
		&venue.UpdatedByID, &venue.MadeActiveByID, &venue.MadeActiveAt,
		&venue.ShowPremium, &venue.Category, &venue.PrettyUrl, &venue.EditLock,
		&venue.RequestVeganDecalAt, &venue.RequestExcellentDecalAt, &venue.Source,
		// User fields
		&user.ID, &user.Username, &user.Email, &trustedInt, &user.Contributions,
		// Authority fields
		&isVenueAdmin, &ambassadorLevel, &ambassadorPoints, &ambassadorRegion, &approvedVenueCount,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get venue with user by ID: %w", err)
	}

	user.Trusted = trustedInt > 0 // Convert int to bool
	venueWithUser.Venue = venue

	if isVenueAdmin.Valid {
		venueWithUser.IsVenueAdmin = isVenueAdmin.Int64 > 0
		user.IsVenueAdmin = isVenueAdmin.Int64 > 0
	}

	if ambassadorLevel.Valid {
		level := int(ambassadorLevel.Int64)
		user.AmbassadorLevel = &level
	}
	if ambassadorPoints.Valid {
		points := int(ambassadorPoints.Int64)
		user.AmbassadorPoints = &points
	}
	if ambassadorRegion.Valid {
		user.AmbassadorRegion = &ambassadorRegion.String
	}
	if approvedVenueCount.Valid {
		count := int(approvedVenueCount.Int64)
		user.ApprovedVenueCount = &count
	}

	venueWithUser.User = user
	return &venueWithUser, nil
}

// GetSimilarVenues returns venues similar to the given venue
func (db *DB) GetSimilarVenues(venue models.Venue, limit int) ([]models.Venue, error) {
	// Simple similarity based on location keywords
	locationWords := strings.Fields(strings.ToLower(venue.Location))
	if len(locationWords) == 0 {
		return []models.Venue{}, nil
	}

	// Build LIKE clauses for location words
	var likeClauses []string
	var args []interface{}

	for _, word := range locationWords {
		if len(word) > 2 { // Only use words longer than 2 characters
			likeClauses = append(likeClauses, "LOWER(location) LIKE ?")
			args = append(args, "%"+word+"%")
		}
	}

	if len(likeClauses) == 0 {
		return []models.Venue{}, nil
	}

	query := fmt.Sprintf(`SELECT 
        id, path, entrytype, name, url, fburl, instagram_url, location, zipcode, phone,
        other_food_type, price, additionalinfo, vdetails, openhours, openhours_note,
        timezone, hash, email, ownername, sentby, user_id, active, vegonly, vegan,
        sponsor_level, crossstreet, lat, lng, created_at, date_added, date_updated,
        admin_last_update, admin_note, admin_hold, admin_hold_email_note,
        updated_by_id, made_active_by_id, made_active_at, show_premium, category,
        pretty_url, edit_lock, request_vegan_decal_at, request_excellent_decal_at, source
        FROM venues 
        WHERE id != ? AND (%s)
        ORDER BY active DESC
        LIMIT ?`, strings.Join(likeClauses, " OR "))

	// Add venue ID and limit to args
	allArgs := append([]interface{}{venue.ID}, args...)
	allArgs = append(allArgs, limit)

	rows, err := db.conn.Query(query, allArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to query similar venues: %w", err)
	}
	defer rows.Close()

	var venues []models.Venue
	for rows.Next() {
		v, err := db.scanVenueRow(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan similar venue row: %w", err)
		}
		venues = append(venues, *v)
	}

	return venues, nil
}

// GetSimilarVenuesCtx returns venues similar to the given venue with context
func (db *DB) GetSimilarVenuesCtx(ctx context.Context, venue models.Venue, limit int) ([]models.Venue, error) {
	ctx, cancel := db.withReadTimeout(ctx)
	defer cancel()
	locationWords := strings.Fields(strings.ToLower(venue.Location))
	if len(locationWords) == 0 {
		return []models.Venue{}, nil
	}
	var likeClauses []string
	var args []interface{}
	for _, word := range locationWords {
		if len(word) > 2 {
			likeClauses = append(likeClauses, "LOWER(location) LIKE ?")
			args = append(args, "%"+word+"%")
		}
	}
	if len(likeClauses) == 0 {
		return []models.Venue{}, nil
	}
	query := fmt.Sprintf(`SELECT 
        id, path, entrytype, name, url, fburl, instagram_url, location, zipcode, phone,
        other_food_type, price, additionalinfo, vdetails, openhours, openhours_note,
        timezone, hash, email, ownername, sentby, user_id, active, vegonly, vegan,
        sponsor_level, crossstreet, lat, lng, created_at, date_added, date_updated,
        admin_last_update, admin_note, admin_hold, admin_hold_email_note,
        updated_by_id, made_active_by_id, made_active_at, show_premium, category,
        pretty_url, edit_lock, request_vegan_decal_at, request_excellent_decal_at, source
        FROM venues 
        WHERE id != ? AND (%s)
        ORDER BY active DESC
        LIMIT ?`, strings.Join(likeClauses, " OR "))
	allArgs := append([]interface{}{venue.ID}, args...)
	allArgs = append(allArgs, limit)
	rows, err := db.conn.QueryContext(ctx, query, allArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to query similar venues: %w", err)
	}
	defer rows.Close()
	var venues []models.Venue
	for rows.Next() {
		v, err := db.scanVenueRow(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan similar venue row: %w", err)
		}
		venues = append(venues, *v)
	}
	return venues, nil
}

// CountVenuesByPathCtx counts venues with the same path, excluding a specific venue ID
func (db *DB) CountVenuesByPathCtx(ctx context.Context, path string, excludeVenueID int64) (int, error) {
	ctx, cancel := db.withReadTimeout(ctx)
	defer cancel()

	query := `SELECT COUNT(*) FROM venues WHERE path = ? AND id != ? AND active=1`
	var count int
	err := db.conn.QueryRowContext(ctx, query, path, excludeVenueID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count venues by path: %w", err)
	}
	return count, nil
}

// GetValidationHistoryPaginated returns validation history with pagination
func (db *DB) GetValidationHistoryPaginated(limit, offset int) ([]models.ValidationHistory, int, error) {
	// Get total count
	countQuery := `SELECT COUNT(*) FROM venue_validation_histories`
	var total int
	err := db.conn.QueryRow(countQuery).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get validation history count: %w", err)
	}

	// Get paginated results
	query := `SELECT 
        vvh.id, vvh.venue_id, vvh.validation_score, vvh.validation_status,
        vvh.validation_notes, vvh.score_breakdown, vvh.prompt_version, vvh.processed_at,
        v.name as venue_name
        FROM venue_validation_histories vvh
        JOIN venues v ON vvh.venue_id = v.id
        ORDER BY vvh.processed_at DESC
        LIMIT ? OFFSET ?`

	rows, err := db.conn.Query(query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query validation history: %w", err)
	}
	defer rows.Close()

	var history []models.ValidationHistory
	for rows.Next() {
		var h models.ValidationHistory
		var scoreBreakdownJSON string
		var pv sql.NullString

		if err := rows.Scan(&h.ID, &h.VenueID, &h.ValidationScore, &h.ValidationStatus,
			&h.ValidationNotes, &scoreBreakdownJSON, &pv, &h.ProcessedAt, &h.VenueName); err != nil {
			return nil, 0, fmt.Errorf("failed to scan validation history row: %w", err)
		}
		if pv.Valid {
			val := pv.String
			h.PromptVersion = &val
		}

		if err = json.Unmarshal([]byte(scoreBreakdownJSON), &h.ScoreBreakdown); err != nil {
			return nil, 0, fmt.Errorf("failed to unmarshal score breakdown: %w", err)
		}

		history = append(history, h)
	}

	return history, total, nil
}

// GetVenueValidationHistory returns validation history for a specific venue
func (db *DB) GetVenueValidationHistory(venueID int64) ([]models.ValidationHistory, error) {
	query := `SELECT 
        id, venue_id, validation_score, validation_status, validation_notes,
        score_breakdown, ai_output_data, prompt_version, processed_at
        FROM venue_validation_histories 
        WHERE venue_id = ? 
        ORDER BY processed_at DESC`

	rows, err := db.conn.Query(query, venueID)
	if err != nil {
		return nil, fmt.Errorf("failed to query venue validation history: %w", err)
	}
	defer rows.Close()

	var history []models.ValidationHistory
	for rows.Next() {
		var h models.ValidationHistory
		var scoreBreakdownJSON string
		var aiOutput sql.NullString
		var pv sql.NullString

		if err := rows.Scan(&h.ID, &h.VenueID, &h.ValidationScore, &h.ValidationStatus,
			&h.ValidationNotes, &scoreBreakdownJSON, &aiOutput, &pv, &h.ProcessedAt); err != nil {
			return nil, fmt.Errorf("failed to scan validation history row: %w", err)
		}
		if pv.Valid {
			val := pv.String
			h.PromptVersion = &val
		}
		if err = json.Unmarshal([]byte(scoreBreakdownJSON), &h.ScoreBreakdown); err != nil {
			return nil, fmt.Errorf("failed to unmarshal score breakdown: %w", err)
		}
		if aiOutput.Valid {
			val := aiOutput.String
			h.AIOutputData = &val
		}

		history = append(history, h)
	}

	return history, nil
}

// SaveValidationResultWithGoogleData saves validation history with Google Places data WITHOUT changing venue status
func (db *DB) SaveValidationResultWithGoogleData(result *models.ValidationResult, googleData *models.GooglePlaceData) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin validation result transaction: %w", err)
	}
	defer tx.Rollback()

	// Save validation history with Google Places data only
	var googlePlaceID *string
	var googlePlaceFound bool
	var googlePlaceDataJSON *string

	if googleData != nil {
		googlePlaceID = &googleData.PlaceID
		googlePlaceFound = true
		data, err := json.Marshal(googleData)
		if err != nil {
			return fmt.Errorf("failed to marshal Google Places data: %w", err)
		}
		jsonStr := string(data)
		googlePlaceDataJSON = &jsonStr
	}

	stmt := db.stmts["insertValidationHistory"]
	scoreBreakdownJSON, err := json.Marshal(result.ScoreBreakdown)
	if err != nil {
		return fmt.Errorf("failed to marshal score breakdown: %w", err)
	}

	_, err = tx.Stmt(stmt).Exec(result.VenueID, result.Score, result.Status,
		result.Notes, string(scoreBreakdownJSON), googlePlaceID, googlePlaceFound, googlePlaceDataJSON, result.AIOutputData)
	if err != nil {
		return fmt.Errorf("failed to insert validation history: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit validation result transaction: %w", err)
	}

	return nil
}

// GetCachedGooglePlaceData retrieves cached Google Places data for a venue
func (db *DB) GetCachedGooglePlaceData(venueID int64) (*models.GooglePlaceData, error) {
	query := `SELECT google_place_data FROM venue_validation_histories 
              WHERE venue_id = ? AND google_place_found = 1 AND google_place_data IS NOT NULL 
              ORDER BY processed_at DESC LIMIT 1`

	var googlePlaceDataJSON sql.NullString
	err := db.conn.QueryRow(query, venueID).Scan(&googlePlaceDataJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // No cached data found
		}
		return nil, fmt.Errorf("failed to query cached Google Places data: %w", err)
	}

	if !googlePlaceDataJSON.Valid {
		return nil, nil
	}

	var googleData models.GooglePlaceData
	if err = json.Unmarshal([]byte(googlePlaceDataJSON.String), &googleData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Google Places data: %w", err)
	}

	return &googleData, nil
}

// HasRecentValidationHistory checks if venue has been validated recently
func (db *DB) HasRecentValidationHistory(venueID int64, hours int) (bool, error) {
	query := `SELECT COUNT(*) FROM venue_validation_histories 
             WHERE venue_id = ? AND processed_at > DATE_SUB(NOW(), INTERVAL ? HOUR)`

	var count int
	err := db.conn.QueryRow(query, venueID, hours).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check recent validation history: %w", err)
	}

	return count > 0, nil
}

// HasAnyValidationHistory checks if venue has at least one validation history record
func (db *DB) HasAnyValidationHistory(venueID int64) (bool, error) {
	query := `SELECT COUNT(*) FROM venue_validation_histories WHERE venue_id = ?`
	var count int
	err := db.conn.QueryRow(query, venueID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check any validation history: %w", err)
	}
	return count > 0, nil
}

// GetManualReviewVenues returns pending venues (active=0) that have validation history
// along with their latest validation score. Supports optional search and pagination.
func (db *DB) GetManualReviewVenues(search string, limit, offset int) ([]models.VenueWithUser, []int, int, error) {
	where := "WHERE v.active = 0 AND EXISTS (SELECT 1 FROM venue_validation_histories h WHERE h.venue_id = v.id)"
	args := []interface{}{}
	if search != "" {
		where += " AND (v.name LIKE ? OR v.location LIKE ? OR m.username LIKE ?)"
		sp := "%" + search + "%"
		args = append(args, sp, sp, sp)
	}

	// total count
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM venues v 
        LEFT JOIN members m ON v.user_id = m.id %s`, where)
	var total int
	if err := db.conn.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, nil, 0, fmt.Errorf("failed to count manual review venues: %w", err)
	}

	// fetch rows with latest score
	query := fmt.Sprintf(`SELECT 
        v.id, v.path, v.entrytype, v.name, v.url, v.fburl, v.instagram_url,
        v.location, v.zipcode, v.phone, v.other_food_type, v.price, v.additionalinfo,
        v.vdetails, v.openhours, v.openhours_note, v.timezone, v.hash, v.email,
        v.ownername, v.sentby, v.user_id, v.active, v.vegonly, v.vegan, v.sponsor_level,
        v.crossstreet, v.lat, v.lng, v.created_at, v.date_added, v.date_updated,
        v.admin_last_update, v.admin_note, v.admin_hold, v.admin_hold_email_note,
        v.updated_by_id, v.made_active_by_id, v.made_active_at, v.show_premium,
        v.category, v.pretty_url, v.edit_lock, v.request_vegan_decal_at,
        v.request_excellent_decal_at, v.source,
        m.id as member_id, m.username, m.trusted,
        va.venue_id IS NOT NULL as is_venue_admin,
        a.level as ambassador_level, a.points as ambassador_points, a.path as ambassador_path,
        (
          SELECT vvh.validation_score 
          FROM venue_validation_histories vvh 
          WHERE vvh.venue_id = v.id 
          ORDER BY vvh.processed_at DESC 
          LIMIT 1
        ) as latest_score
        FROM venues v
        LEFT JOIN members m ON v.user_id = m.id
        LEFT JOIN venue_admin va ON v.id = va.venue_id AND m.id = va.user_id
        LEFT JOIN ambassadors a ON m.id = a.user_id
        %s
        ORDER BY v.admin_last_update DESC, v.created_at DESC
        LIMIT ? OFFSET ?`, where)

	argsRows := append([]interface{}{}, args...)
	argsRows = append(argsRows, limit, offset)
	rows, err := db.conn.Query(query, argsRows...)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("failed to query manual review venues: %w", err)
	}
	defer rows.Close()

	var venues []models.VenueWithUser
	var scores []int
	for rows.Next() {
		var venueWithUser models.VenueWithUser
		var venue models.Venue
		var user models.User
		var isVenueAdmin bool
		var ambassadorLevel, ambassadorPoints sql.NullInt64
		var ambassadorPath sql.NullString
		var memberID sql.NullInt64
		var username sql.NullString
		var trusted sql.NullInt64
		var latestScore sql.NullInt64

		if err := rows.Scan(
			&venue.ID, &venue.Path, &venue.EntryType, &venue.Name, &venue.URL,
			&venue.FBUrl, &venue.InstagramUrl, &venue.Location, &venue.Zipcode,
			&venue.Phone, &venue.OtherFoodType, &venue.Price, &venue.AdditionalInfo,
			&venue.VDetails, &venue.OpenHours, &venue.OpenHoursNote, &venue.Timezone,
			&venue.Hash, &venue.Email, &venue.OwnerName, &venue.SentBy, &venue.UserID,
			&venue.Active, &venue.VegOnly, &venue.Vegan, &venue.SponsorLevel,
			&venue.CrossStreet, &venue.Lat, &venue.Lng, &venue.CreatedAt,
			&venue.DateAdded, &venue.DateUpdated, &venue.AdminLastUpdate,
			&venue.AdminNote, &venue.AdminHold, &venue.AdminHoldEmailNote,
			&venue.UpdatedByID, &venue.MadeActiveByID, &venue.MadeActiveAt,
			&venue.ShowPremium, &venue.Category, &venue.PrettyUrl, &venue.EditLock,
			&venue.RequestVeganDecalAt, &venue.RequestExcellentDecalAt, &venue.Source,
			&memberID, &username, &trusted,
			&isVenueAdmin, &ambassadorLevel, &ambassadorPoints, &ambassadorPath,
			&latestScore,
		); err != nil {
			return nil, nil, 0, fmt.Errorf("failed to scan manual review venue row: %w", err)
		}

		if memberID.Valid {
			user.ID = uint(memberID.Int64)
		} else {
			user.ID = venue.UserID
		}
		if username.Valid {
			user.Username = username.String
		}
		if trusted.Valid {
			user.Trusted = trusted.Int64 > 0
		}

		venueWithUser.Venue = venue
		venueWithUser.User = user
		venueWithUser.IsVenueAdmin = isVenueAdmin
		if ambassadorLevel.Valid {
			venueWithUser.AmbassadorLevel = &ambassadorLevel.Int64
		}
		if ambassadorPoints.Valid {
			venueWithUser.AmbassadorPoints = &ambassadorPoints.Int64
		}
		if ambassadorPath.Valid {
			venueWithUser.AmbassadorPath = &ambassadorPath.String
		}

		venues = append(venues, venueWithUser)
		if latestScore.Valid {
			scores = append(scores, int(latestScore.Int64))
		} else {
			scores = append(scores, 0)
		}
	}

	return venues, scores, total, nil
}

// Context-aware DB methods appended for cancellation and timeouts
// NOTE: Prefer these in new code; legacy methods call non-ctx variants for compatibility.

// UpdateVenueActiveCtx updates only the active status with context.
func (db *DB) UpdateVenueActiveCtx(ctx context.Context, venueID int64, active int) error {
	ctx, cancel := db.withWriteTimeout(ctx)
	defer cancel()
	query := `UPDATE venues SET active = ?, admin_last_update = NOW() WHERE id = ?`
	if _, err := db.conn.ExecContext(ctx, query, active, venueID); err != nil {
		return fmt.Errorf("failed to update venue active status: %w", err)
	}
	return nil
}

// SaveValidationResultWithGoogleDataCtx saves validation history with Google data using context.
func (db *DB) SaveValidationResultWithGoogleDataCtx(ctx context.Context, result *models.ValidationResult, googleData *models.GooglePlaceData) error {
	ctx, cancel := db.withWriteTimeout(ctx)
	defer cancel()

	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin validation result transaction: %w", err)
	}
	defer tx.Rollback()

	var googlePlaceID *string
	var googlePlaceFound bool
	var googlePlaceDataJSON *string
	if googleData != nil {
		googlePlaceID = &googleData.PlaceID
		googlePlaceFound = true
		data, err := json.Marshal(googleData)
		if err != nil {
			return fmt.Errorf("failed to marshal Google Places data: %w", err)
		}
		jsonStr := string(data)
		googlePlaceDataJSON = &jsonStr
	}

	scoreBreakdownJSON, err := json.Marshal(result.ScoreBreakdown)
	if err != nil {
		return fmt.Errorf("failed to marshal score breakdown: %w", err)
	}

	stmt := db.stmts["insertValidationHistory"]
	if stmt == nil {
		return fmt.Errorf("prepared statement insertValidationHistory not initialized")
	}
	if _, err = tx.StmtContext(ctx, stmt).ExecContext(ctx, result.VenueID, result.Score, result.Status,
		result.Notes, string(scoreBreakdownJSON), googlePlaceID, googlePlaceFound, googlePlaceDataJSON, result.AIOutputData); err != nil {
		return fmt.Errorf("failed to insert validation history: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit validation result transaction: %w", err)
	}
	return nil
}

// GetRecentValidationResultsCtx returns recent validation results with context.
func (db *DB) GetRecentValidationResultsCtx(ctx context.Context, limit int) ([]models.ValidationResult, error) {
	ctx, cancel := db.withReadTimeout(ctx)
	defer cancel()
	query := `SELECT venue_id, validation_score, validation_status, validation_notes, score_breakdown, processed_at FROM venue_validation_histories ORDER BY processed_at DESC LIMIT ?`
	rows, err := db.conn.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query recent validation results: %w", err)
	}
	defer rows.Close()
	var results []models.ValidationResult
	for rows.Next() {
		var result models.ValidationResult
		var scoreBreakdownJSON string
		var processedAt time.Time
		if err := rows.Scan(&result.VenueID, &result.Score, &result.Status, &result.Notes, &scoreBreakdownJSON, &processedAt); err != nil {
			return nil, fmt.Errorf("failed to scan validation result row: %w", err)
		}
		if err := json.Unmarshal([]byte(scoreBreakdownJSON), &result.ScoreBreakdown); err != nil {
			return nil, fmt.Errorf("failed to unmarshal score breakdown: %w", err)
		}
		results = append(results, result)
	}
	return results, nil
}

// GetPendingVenuesWithUserCtx returns pending venues with user info using context.
func (db *DB) GetPendingVenuesWithUserCtx(ctx context.Context) ([]models.VenueWithUser, error) {
	ctx, cancel := db.withReadTimeout(ctx)
	defer cancel()
	query := `SELECT 
        v.id, v.path, v.entrytype, v.name, v.url, v.fburl, v.instagram_url, 
        v.location, v.zipcode, v.phone, v.other_food_type, v.price, v.additionalinfo,
        v.vdetails, v.openhours, v.openhours_note, v.timezone, v.hash, v.email,
        v.ownername, v.sentby, v.user_id, v.active, v.vegonly, v.vegan,
        v.sponsor_level, v.crossstreet, v.lat, v.lng, v.created_at, v.date_added,
        v.date_updated, v.admin_last_update, v.admin_note, v.admin_hold,
        v.admin_hold_email_note, v.updated_by_id, v.made_active_by_id,
        v.made_active_at, v.show_premium, v.category, v.pretty_url, v.edit_lock,
        v.request_vegan_decal_at, v.request_excellent_decal_at, v.source,
        m.username, m.email as user_email, m.trusted, m.contributions,
        CASE WHEN va.venue_id IS NOT NULL THEN 1 ELSE 0 END as is_venue_admin,
        a.level as ambassador_level, a.points as ambassador_points, a.path as ambassador_region
        FROM venues v
        LEFT JOIN members m ON v.user_id = m.id
        LEFT JOIN venue_admin va ON v.id = va.venue_id AND v.user_id = va.user_id
        LEFT JOIN ambassadors a ON v.user_id = a.user_id
        WHERE v.active = 0
        ORDER BY v.created_at ASC`
	rows, err := db.conn.QueryContext(ctx, query)
	if err != nil {
		return nil, errs.NewDB("database.GetPendingVenuesWithUser", "failed to query pending venues with user info", err)
	}
	defer rows.Close()
	var venues []models.VenueWithUser
	for rows.Next() {
		var vu models.VenueWithUser
		var venue models.Venue
		var user models.User
		var username, email sql.NullString
		var trusted, contributions sql.NullInt64
		var isVenueAdmin sql.NullInt64
		var ambassadorLevel, ambassadorPoints sql.NullInt64
		var ambassadorRegion sql.NullString
		if err := rows.Scan(
			&venue.ID, &venue.Path, &venue.EntryType, &venue.Name, &venue.URL,
			&venue.FBUrl, &venue.InstagramUrl, &venue.Location, &venue.Zipcode,
			&venue.Phone, &venue.OtherFoodType, &venue.Price, &venue.AdditionalInfo,
			&venue.VDetails, &venue.OpenHours, &venue.OpenHoursNote, &venue.Timezone,
			&venue.Hash, &venue.Email, &venue.OwnerName, &venue.SentBy, &venue.UserID,
			&venue.Active, &venue.VegOnly, &venue.Vegan, &venue.SponsorLevel,
			&venue.CrossStreet, &venue.Lat, &venue.Lng, &venue.CreatedAt,
			&venue.DateAdded, &venue.DateUpdated, &venue.AdminLastUpdate,
			&venue.AdminNote, &venue.AdminHold, &venue.AdminHoldEmailNote,
			&venue.UpdatedByID, &venue.MadeActiveByID, &venue.MadeActiveAt,
			&venue.ShowPremium, &venue.Category, &venue.PrettyUrl, &venue.EditLock,
			&venue.RequestVeganDecalAt, &venue.RequestExcellentDecalAt, &venue.Source,
			&username, &email, &trusted, &contributions,
			&isVenueAdmin, &ambassadorLevel, &ambassadorPoints,
			&ambassadorRegion,
		); err != nil {
			return nil, fmt.Errorf("failed to scan venue with user row: %w", err)
		}
		if username.Valid {
			user.Username = username.String
		}
		if email.Valid {
			user.Email = email.String
		}
		if trusted.Valid {
			user.Trusted = trusted.Int64 > 0
		}
		if contributions.Valid {
			user.Contributions = int(contributions.Int64)
		}
		user.ID = venue.UserID
		vu.Venue = venue
		vu.User = user
		if isVenueAdmin.Valid {
			vu.IsVenueAdmin = isVenueAdmin.Int64 > 0
		}
		if ambassadorLevel.Valid {
			level := int(ambassadorLevel.Int64)
			user.AmbassadorLevel = &level
		}
		if ambassadorPoints.Valid {
			points := int(ambassadorPoints.Int64)
			user.AmbassadorPoints = &points
		}
		if ambassadorRegion.Valid {
			user.AmbassadorRegion = &ambassadorRegion.String
		}
		venues = append(venues, vu)
	}
	return venues, nil
}

// GetVenuesFilteredCtx returns filtered venues with pagination and context.
func (db *DB) GetVenuesFilteredCtx(ctx context.Context, status, search string, limit, offset int) ([]models.VenueWithUser, int, error) {
	ctx, cancel := db.withReadTimeout(ctx)
	defer cancel()
	whereClause := "WHERE 1=1"
	args := []interface{}{}
	if status != "" {
		switch status {
		case "pending":
			whereClause += " AND v.active = ?"
			args = append(args, 0)
		case "approved":
			whereClause += " AND v.active = ?"
			args = append(args, 1)
		case "rejected":
			whereClause += " AND v.active = ?"
			args = append(args, -1)
		}
	}
	if search != "" {
		whereClause += " AND (v.name LIKE ? OR v.location LIKE ? OR m.username LIKE ?)"
		searchPattern := "%" + search + "%"
		args = append(args, searchPattern, searchPattern, searchPattern)
	}
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM venues v 
        LEFT JOIN members m ON v.user_id = m.id 
        LEFT JOIN venue_admin va ON v.id = va.venue_id AND m.id = va.user_id
        LEFT JOIN ambassadors a ON m.id = a.user_id %s`, whereClause)
	var total int
	if err := db.conn.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to get filtered venues count: %w", err)
	}
	query := fmt.Sprintf(`SELECT v.id, v.path, v.entrytype, v.name, v.url, v.fburl, v.instagram_url, 
        v.location, v.zipcode, v.phone, v.other_food_type, v.price, v.additionalinfo, 
        v.vdetails, v.openhours, v.openhours_note, v.timezone, v.hash, v.email, 
        v.ownername, v.sentby, v.user_id, v.active, v.vegonly, v.vegan, v.sponsor_level, 
        v.crossstreet, v.lat, v.lng, v.created_at, v.date_added, v.date_updated, 
        v.admin_last_update, v.admin_note, v.admin_hold, v.admin_hold_email_note, 
        v.updated_by_id, v.made_active_by_id, v.made_active_at, v.show_premium, 
        v.category, v.pretty_url, v.edit_lock, v.request_vegan_decal_at, 
        v.request_excellent_decal_at, v.source,
        m.id as member_id, m.username, m.trusted,
        va.venue_id IS NOT NULL as is_venue_admin,
        a.level as ambassador_level, a.points as ambassador_points, a.path as ambassador_path
        FROM venues v 
        LEFT JOIN members m ON v.user_id = m.id 
        LEFT JOIN venue_admin va ON v.id = va.venue_id AND m.id = va.user_id
        LEFT JOIN ambassadors a ON m.id = a.user_id
        %s
        ORDER BY v.admin_last_update DESC, v.created_at DESC
        LIMIT ? OFFSET ?`, whereClause)
	args = append(args, limit, offset)
	rows, err := db.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query filtered venues: %w", err)
	}
	defer rows.Close()
	var venues []models.VenueWithUser
	for rows.Next() {
		var venueWithUser models.VenueWithUser
		var venue models.Venue
		var user models.User
		var isVenueAdmin bool
		var ambassadorLevel, ambassadorPoints sql.NullInt64
		var ambassadorPath sql.NullString
		var memberID sql.NullInt64
		var username sql.NullString
		var trusted sql.NullInt64
		if err := rows.Scan(
			&venue.ID, &venue.Path, &venue.EntryType, &venue.Name, &venue.URL,
			&venue.FBUrl, &venue.InstagramUrl, &venue.Location, &venue.Zipcode,
			&venue.Phone, &venue.OtherFoodType, &venue.Price, &venue.AdditionalInfo,
			&venue.VDetails, &venue.OpenHours, &venue.OpenHoursNote, &venue.Timezone,
			&venue.Hash, &venue.Email, &venue.OwnerName, &venue.SentBy, &venue.UserID,
			&venue.Active, &venue.VegOnly, &venue.Vegan, &venue.SponsorLevel,
			&venue.CrossStreet, &venue.Lat, &venue.Lng, &venue.CreatedAt,
			&venue.DateAdded, &venue.DateUpdated, &venue.AdminLastUpdate,
			&venue.AdminNote, &venue.AdminHold, &venue.AdminHoldEmailNote,
			&venue.UpdatedByID, &venue.MadeActiveByID, &venue.MadeActiveAt,
			&venue.ShowPremium, &venue.Category, &venue.PrettyUrl, &venue.EditLock,
			&venue.RequestVeganDecalAt, &venue.RequestExcellentDecalAt, &venue.Source,
			&memberID, &username, &trusted,
			&isVenueAdmin, &ambassadorLevel, &ambassadorPoints, &ambassadorPath,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan venue with user row: %w", err)
		}
		if memberID.Valid {
			user.ID = uint(memberID.Int64)
		} else {
			user.ID = venue.UserID
		}
		if username.Valid {
			user.Username = username.String
		}
		if trusted.Valid {
			user.Trusted = trusted.Int64 > 0
		}
		venueWithUser.Venue = venue
		venueWithUser.User = user
		venueWithUser.IsVenueAdmin = isVenueAdmin
		if ambassadorLevel.Valid {
			venueWithUser.AmbassadorLevel = &ambassadorLevel.Int64
		}
		if ambassadorPoints.Valid {
			venueWithUser.AmbassadorPoints = &ambassadorPoints.Int64
		}
		if ambassadorPath.Valid {
			venueWithUser.AmbassadorPath = &ambassadorPath.String
		}
		venues = append(venues, venueWithUser)
	}
	return venues, total, nil
}

// GetVenueWithUserByIDCtx fetches a single venue with user info by ID.
func (db *DB) GetVenueWithUserByIDCtx(ctx context.Context, venueID int64) (*models.VenueWithUser, error) {
	ctx, cancel := db.withReadTimeout(ctx)
	defer cancel()
	query := `SELECT 
        v.id, v.path, v.entrytype, v.name, v.url, v.fburl, v.instagram_url, 
        v.location, v.zipcode, v.phone, v.other_food_type, v.price, v.additionalinfo,
        v.vdetails, v.openhours, v.openhours_note, v.timezone, v.hash, v.email,
        v.ownername, v.sentby, v.user_id, v.active, v.vegonly, v.vegan,
        v.sponsor_level, v.crossstreet, v.lat, v.lng, v.created_at, v.date_added,
        v.date_updated, v.admin_last_update, v.admin_note, v.admin_hold,
        v.admin_hold_email_note, v.updated_by_id, v.made_active_by_id,
        v.made_active_at, v.show_premium, v.category, v.pretty_url, v.edit_lock,
        v.request_vegan_decal_at, v.request_excellent_decal_at, v.source,
        m.username, m.email as user_email, m.trusted, m.contributions,
        CASE WHEN va.venue_id IS NOT NULL THEN 1 ELSE 0 END as is_venue_admin,
        a.level as ambassador_level, a.points as ambassador_points, a.path as ambassador_region,
        (SELECT COUNT(*) FROM venues v2 WHERE v2.user_id = m.id AND v2.active = 1) as approved_venue_count
        FROM venues v
        LEFT JOIN members m ON v.user_id = m.id
        LEFT JOIN venue_admin va ON v.id = va.venue_id AND v.user_id = va.user_id
        LEFT JOIN ambassadors a ON v.user_id = a.user_id
        WHERE v.id = ?`
	row := db.conn.QueryRowContext(ctx, query, venueID)
	var vu models.VenueWithUser
	var venue models.Venue
	var user models.User
	var username, email sql.NullString
	var trusted, contributions sql.NullInt64
	var isVenueAdmin sql.NullInt64
	var ambassadorLevel, ambassadorPoints sql.NullInt64
	var ambassadorRegion sql.NullString
	var approvedVenueCount sql.NullInt64
	if err := row.Scan(
		&venue.ID, &venue.Path, &venue.EntryType, &venue.Name, &venue.URL,
		&venue.FBUrl, &venue.InstagramUrl, &venue.Location, &venue.Zipcode,
		&venue.Phone, &venue.OtherFoodType, &venue.Price, &venue.AdditionalInfo,
		&venue.VDetails, &venue.OpenHours, &venue.OpenHoursNote, &venue.Timezone,
		&venue.Hash, &venue.Email, &venue.OwnerName, &venue.SentBy, &venue.UserID,
		&venue.Active, &venue.VegOnly, &venue.Vegan, &venue.SponsorLevel,
		&venue.CrossStreet, &venue.Lat, &venue.Lng, &venue.CreatedAt,
		&venue.DateAdded, &venue.DateUpdated, &venue.AdminLastUpdate,
		&venue.AdminNote, &venue.AdminHold, &venue.AdminHoldEmailNote,
		&venue.UpdatedByID, &venue.MadeActiveByID, &venue.MadeActiveAt,
		&venue.ShowPremium, &venue.Category, &venue.PrettyUrl, &venue.EditLock,
		&venue.RequestVeganDecalAt, &venue.RequestExcellentDecalAt, &venue.Source,
		&username, &email, &trusted, &contributions,
		&isVenueAdmin, &ambassadorLevel, &ambassadorPoints, &ambassadorRegion, &approvedVenueCount,
	); err != nil {
		return nil, fmt.Errorf("failed to scan venue with user row: %w", err)
	}
	if username.Valid {
		user.Username = username.String
	}
	if email.Valid {
		user.Email = email.String
	}
	if trusted.Valid {
		user.Trusted = trusted.Int64 > 0
	}
	if contributions.Valid {
		user.Contributions = int(contributions.Int64)
	}
	user.ID = venue.UserID
	vu.Venue = venue
	vu.User = user
	if isVenueAdmin.Valid {
		vu.IsVenueAdmin = isVenueAdmin.Int64 > 0
		vu.User.IsVenueAdmin = isVenueAdmin.Int64 > 0
	}
	if ambassadorLevel.Valid {
		level := int(ambassadorLevel.Int64)
		vu.User.AmbassadorLevel = &level
	}
	if ambassadorPoints.Valid {
		points := int(ambassadorPoints.Int64)
		vu.User.AmbassadorPoints = &points
	}
	if ambassadorRegion.Valid {
		vu.User.AmbassadorRegion = &ambassadorRegion.String
	}
	if approvedVenueCount.Valid {
		count := int(approvedVenueCount.Int64)
		vu.User.ApprovedVenueCount = &count
	}
	return &vu, nil
}

// GetVenueValidationHistoryCtx retrieves validation history for a venue with context.
func (db *DB) GetVenueValidationHistoryCtx(ctx context.Context, venueID int64) ([]models.ValidationHistory, error) {
	ctx, cancel := db.withReadTimeout(ctx)
	defer cancel()
	query := `SELECT 
        id, venue_id, validation_score, validation_status, validation_notes,
        score_breakdown, ai_output_data, prompt_version, processed_at
        FROM venue_validation_histories 
        WHERE venue_id = ? 
        ORDER BY processed_at DESC`
	rows, err := db.conn.QueryContext(ctx, query, venueID)
	if err != nil {
		return nil, fmt.Errorf("failed to query venue validation history: %w", err)
	}
	defer rows.Close()
	var history []models.ValidationHistory
	for rows.Next() {
		var h models.ValidationHistory
		var scoreBreakdownJSON string
		var aiOutput sql.NullString
		var pv sql.NullString
		if err := rows.Scan(&h.ID, &h.VenueID, &h.ValidationScore, &h.ValidationStatus,
			&h.ValidationNotes, &scoreBreakdownJSON, &aiOutput, &pv, &h.ProcessedAt); err != nil {
			return nil, fmt.Errorf("failed to scan validation history row: %w", err)
		}
		if pv.Valid {
			val := pv.String
			h.PromptVersion = &val
		}
		if err := json.Unmarshal([]byte(scoreBreakdownJSON), &h.ScoreBreakdown); err != nil {
			return nil, fmt.Errorf("failed to unmarshal score breakdown: %w", err)
		}
		if aiOutput.Valid {
			val := aiOutput.String
			h.AIOutputData = &val
		}
		history = append(history, h)
	}
	return history, nil
}

// GetCachedGooglePlaceDataCtx retrieves cached Google Places data for a venue with context.
func (db *DB) GetCachedGooglePlaceDataCtx(ctx context.Context, venueID int64) (*models.GooglePlaceData, error) {
	ctx, cancel := db.withReadTimeout(ctx)
	defer cancel()
	query := `SELECT google_place_data FROM venue_validation_histories 
              WHERE venue_id = ? AND google_place_found = 1 AND google_place_data IS NOT NULL 
              ORDER BY processed_at DESC LIMIT 1`
	var googlePlaceDataJSON sql.NullString
	if err := db.conn.QueryRowContext(ctx, query, venueID).Scan(&googlePlaceDataJSON); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query cached Google Places data: %w", err)
	}
	if !googlePlaceDataJSON.Valid {
		return nil, nil
	}
	var googleData models.GooglePlaceData
	if err := json.Unmarshal([]byte(googlePlaceDataJSON.String), &googleData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Google Places data: %w", err)
	}
	return &googleData, nil
}

// GetValidationHistoryPaginatedCtx returns paginated validation history with context.
func (db *DB) GetValidationHistoryPaginatedCtx(ctx context.Context, limit, offset int) ([]models.ValidationHistory, int, error) {
	ctx, cancel := db.withReadTimeout(ctx)
	defer cancel()
	countQuery := `SELECT COUNT(*) FROM venue_validation_histories`
	var total int
	if err := db.conn.QueryRowContext(ctx, countQuery).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count validation histories: %w", err)
	}
	query := `SELECT id, venue_id, validation_score, validation_status, validation_notes,
	             score_breakdown, ai_output_data, prompt_version, processed_at 
	             FROM venue_validation_histories 
	             ORDER BY processed_at DESC
	             LIMIT ? OFFSET ?`
	rows, err := db.conn.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query validation histories: %w", err)
	}
	defer rows.Close()
	var history []models.ValidationHistory
	for rows.Next() {
		var h models.ValidationHistory
		var scoreBreakdownJSON string
		var aiOutput sql.NullString
		var pv sql.NullString
		if err := rows.Scan(&h.ID, &h.VenueID, &h.ValidationScore, &h.ValidationStatus,
			&h.ValidationNotes, &scoreBreakdownJSON, &aiOutput, &pv, &h.ProcessedAt); err != nil {
			return nil, 0, fmt.Errorf("failed to scan validation history row: %w", err)
		}
		if pv.Valid {
			val := pv.String
			h.PromptVersion = &val
		}
		if err := json.Unmarshal([]byte(scoreBreakdownJSON), &h.ScoreBreakdown); err != nil {
			return nil, 0, fmt.Errorf("failed to unmarshal score breakdown: %w", err)
		}
		if aiOutput.Valid {
			val := aiOutput.String
			h.AIOutputData = &val
		}
		history = append(history, h)
	}
	return history, total, nil
}

// GetManualReviewVenuesCtx returns pending venues with validation history (search/pagination) with context.
func (db *DB) GetManualReviewVenuesCtx(ctx context.Context, search string, limit, offset int) ([]models.VenueWithUser, []int, int, error) {
	ctx, cancel := db.withReadTimeout(ctx)
	defer cancel()
	where := "WHERE v.active = 0 AND EXISTS (SELECT 1 FROM venue_validation_histories h WHERE h.venue_id = v.id)"
	args := []interface{}{}
	if search != "" {
		where += " AND (v.name LIKE ? OR v.location LIKE ? OR m.username LIKE ?)"
		pat := "%" + search + "%"
		args = append(args, pat, pat, pat)
	}
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM venues v
        LEFT JOIN members m ON v.user_id = m.id
        %s`, where)
	var total int
	if err := db.conn.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, nil, 0, fmt.Errorf("failed to count manual review venues: %w", err)
	}
	query := fmt.Sprintf(`SELECT 
        v.id, v.path, v.entrytype, v.name, v.url, v.fburl, v.instagram_url, v.location, v.zipcode, v.phone,
        v.other_food_type, v.price, v.additionalinfo, v.vdetails, v.openhours, v.openhours_note,
        v.timezone, v.hash, v.email, v.ownername, v.sentby, v.user_id, v.active, v.vegonly, v.vegan,
        v.sponsor_level, v.crossstreet, v.lat, v.lng, v.created_at, v.date_added, v.date_updated,
        v.admin_last_update, v.admin_note, v.admin_hold, v.admin_hold_email_note,
        v.updated_by_id, v.made_active_by_id, v.made_active_at, v.show_premium, v.category,
        v.pretty_url, v.edit_lock, v.request_vegan_decal_at, v.request_excellent_decal_at, v.source,
        m.id as member_id, m.username, m.trusted
        FROM venues v
        LEFT JOIN members m ON v.user_id = m.id
        %s
        ORDER BY v.created_at ASC
        LIMIT ? OFFSET ?`, where)
	args = append(args, limit, offset)
	rows, err := db.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("failed to query manual review venues: %w", err)
	}
	defer rows.Close()
	var venues []models.VenueWithUser
	var scores []int
	for rows.Next() {
		var venue models.Venue
		var user models.User
		var memberID sql.NullInt64
		var username sql.NullString
		var trusted sql.NullInt64
		if err := rows.Scan(
			&venue.ID, &venue.Path, &venue.EntryType, &venue.Name, &venue.URL, &venue.FBUrl, &venue.InstagramUrl,
			&venue.Location, &venue.Zipcode, &venue.Phone, &venue.OtherFoodType, &venue.Price, &venue.AdditionalInfo,
			&venue.VDetails, &venue.OpenHours, &venue.OpenHoursNote, &venue.Timezone, &venue.Hash, &venue.Email,
			&venue.OwnerName, &venue.SentBy, &venue.UserID, &venue.Active, &venue.VegOnly, &venue.Vegan,
			&venue.SponsorLevel, &venue.CrossStreet, &venue.Lat, &venue.Lng, &venue.CreatedAt, &venue.DateAdded,
			&venue.DateUpdated, &venue.AdminLastUpdate, &venue.AdminNote, &venue.AdminHold, &venue.AdminHoldEmailNote,
			&venue.UpdatedByID, &venue.MadeActiveByID, &venue.MadeActiveAt, &venue.ShowPremium, &venue.Category,
			&venue.PrettyUrl, &venue.EditLock, &venue.RequestVeganDecalAt, &venue.RequestExcellentDecalAt, &venue.Source,
			&memberID, &username, &trusted,
		); err != nil {
			return nil, nil, 0, fmt.Errorf("failed to scan manual review venue row: %w", err)
		}
		if memberID.Valid {
			user.ID = uint(memberID.Int64)
		}
		if username.Valid {
			user.Username = username.String
		}
		if trusted.Valid {
			user.Trusted = trusted.Int64 > 0
		}
		venues = append(venues, models.VenueWithUser{Venue: venue, User: user})

		// Fetch latest score for this venue
		scoreQuery := `SELECT validation_score FROM venue_validation_histories WHERE venue_id = ? ORDER BY processed_at DESC LIMIT 1`
		var score int
		if err := db.conn.QueryRowContext(ctx, scoreQuery, venue.ID).Scan(&score); err != nil {
			// If no score found, default to 0
			score = 0
		}
		scores = append(scores, score)
	}
	return venues, scores, total, nil
}

// GetVenueStatsCtx returns venue stats using context.
func (db *DB) GetVenueStatsCtx(ctx context.Context) (*models.VenueStats, error) {
	ctx, cancel := db.withReadTimeout(ctx)
	defer cancel()
	query := `SELECT 
        COUNT(CASE WHEN active = 0 THEN 1 END) as pending,
        COUNT(CASE WHEN active = 1 THEN 1 END) as approved,
        COUNT(CASE WHEN active = -1 THEN 1 END) as rejected,
        COUNT(*) as total
        FROM venues`
	var stats models.VenueStats
	if err := db.conn.QueryRowContext(ctx, query).Scan(&stats.Pending, &stats.Approved, &stats.Rejected, &stats.Total); err != nil {
		return nil, fmt.Errorf("failed to get venue stats: %w", err)
	}
	return &stats, nil
}

// GetVenueStatisticsCtx wraps GetVenueStatsCtx for symmetry.
func (db *DB) GetVenueStatisticsCtx(ctx context.Context) (*models.VenueStats, error) {
	return db.GetVenueStatsCtx(ctx)
}

// Conn exposes the underlying *sql.DB for starting transactions.
// Only infrastructure code should use this.
func (db *DB) Conn() *sql.DB { return db.conn }

// UpdateVenueActiveTx updates the active status within an existing transaction.
func (db *DB) UpdateVenueActiveTx(ctx context.Context, tx *sql.Tx, venueID int64, active int) error {
	ctx, cancel := db.withWriteTimeout(ctx)
	defer cancel()
	query := `UPDATE venues SET active = ?, admin_last_update = NOW() WHERE id = ?`
	if _, err := tx.ExecContext(ctx, query, active, venueID); err != nil {
		return fmt.Errorf("failed to update venue active status (tx): %w", err)
	}
	return nil
}

// SaveValidationResultTx saves a validation result within an existing transaction (no Google data fields).
func (db *DB) SaveValidationResultTx(ctx context.Context, tx *sql.Tx, result *models.ValidationResult) error {
	ctx, cancel := db.withWriteTimeout(ctx)
	defer cancel()

	insert := `INSERT INTO venue_validation_histories 
		(venue_id, validation_score, validation_status, validation_notes, score_breakdown, ai_output_data, processed_at)
		VALUES (?, ?, ?, ?, ?, ?, NOW())`

	scoreBreakdownJSON, err := json.Marshal(result.ScoreBreakdown)
	if err != nil {
		return fmt.Errorf("failed to marshal score breakdown: %w", err)
	}

	if _, err := tx.ExecContext(ctx, insert, result.VenueID, result.Score, result.Status, result.Notes, string(scoreBreakdownJSON), result.AIOutputData); err != nil {
		return fmt.Errorf("failed to insert validation history (tx): %w", err)
	}
	return nil
}

// SaveValidationResultWithGoogleDataTx saves validation result with Google data using an existing transaction.
func (db *DB) SaveValidationResultWithGoogleDataTx(ctx context.Context, tx *sql.Tx, result *models.ValidationResult, googleData *models.GooglePlaceData) error {
	ctx, cancel := db.withWriteTimeout(ctx)
	defer cancel()

	var googlePlaceID *string
	var googlePlaceFound bool
	var googlePlaceDataJSON *string
	if googleData != nil {
		googlePlaceID = &googleData.PlaceID
		googlePlaceFound = true
		data, err := json.Marshal(googleData)
		if err != nil {
			return fmt.Errorf("failed to marshal Google Places data: %w", err)
		}
		jsonStr := string(data)
		googlePlaceDataJSON = &jsonStr
	}

	scoreBreakdownJSON, err := json.Marshal(result.ScoreBreakdown)
	if err != nil {
		return fmt.Errorf("failed to marshal score breakdown: %w", err)
	}

	stmt := db.stmts["insertValidationHistory"]
	if stmt == nil {
		return fmt.Errorf("prepared statement insertValidationHistory not initialized")
	}
	if _, err = tx.StmtContext(ctx, stmt).ExecContext(ctx, result.VenueID, result.Score, result.Status,
		result.Notes, string(scoreBreakdownJSON), googlePlaceID, googlePlaceFound, googlePlaceDataJSON, result.AIOutputData); err != nil {
		return fmt.Errorf("failed to insert validation history (tx): %w", err)
	}
	return nil
}

// --- Editor Feedback operations ---

// CreateEditorFeedbackCtx inserts a new feedback row.
func (db *DB) CreateEditorFeedbackCtx(ctx context.Context, f *models.EditorFeedback) error {
	if f == nil {
		return errs.NewDB("database.CreateEditorFeedbackCtx", "nil feedback", nil)
	}
	ctx, cancel := db.withWriteTimeout(ctx)
	defer cancel()
	q := `INSERT INTO venue_validation_editor_feedback (venue_id, prompt_version, feedback_type, comment, ip, created_at)
	      VALUES (?, ?, ?, ?, ?, NOW())`
	res, err := db.conn.ExecContext(ctx, q, f.VenueID, f.PromptVersion, string(f.FeedbackType), f.Comment, f.IP)
	if err != nil {
		return errs.NewDB("database.CreateEditorFeedbackCtx", "insert failed", err)
	}
	id, err := res.LastInsertId()
	if err == nil {
		f.ID = id
	}
	// Best effort fetch created_at
	row := db.conn.QueryRowContext(ctx, "SELECT created_at FROM venue_validation_editor_feedback WHERE id = ?", f.ID)
	var ts time.Time
	if err := row.Scan(&ts); err == nil {
		f.CreatedAt = ts
	}
	return nil
}

func (db *DB) UpsertEditorFeedbackCtx(ctx context.Context, f *models.EditorFeedback) error {
	if f == nil {
		return errs.NewDB("database.UpsertEditorFeedbackCtx", "nil feedback", nil)
	}
	ctx, cancel := db.withWriteTimeout(ctx)
	defer cancel()

	// TODO: RPoC add UNIQUE(venue_id, ip, prompt_version) and switch to real UPSERT, too tired now to deal with migrations on prod again!
	var existingID int64
	var row *sql.Row
	if f.PromptVersion == nil {
		q := `SELECT id FROM venue_validation_editor_feedback WHERE venue_id = ? AND ip = ? AND prompt_version IS NULL LIMIT 1`
		row = db.conn.QueryRowContext(ctx, q, f.VenueID, f.IP)
	} else {
		q := `SELECT id FROM venue_validation_editor_feedback WHERE venue_id = ? AND ip = ? AND prompt_version = ? LIMIT 1`
		row = db.conn.QueryRowContext(ctx, q, f.VenueID, f.IP, *f.PromptVersion)
	}

	switch err := row.Scan(&existingID); err {
	case sql.ErrNoRows:
		// Insert new
		q := `INSERT INTO venue_validation_editor_feedback (venue_id, prompt_version, feedback_type, comment, ip, created_at)
              VALUES (?, ?, ?, ?, ?, NOW())`
		res, err := db.conn.ExecContext(ctx, q,
			f.VenueID, f.PromptVersion, string(f.FeedbackType), f.Comment, f.IP,
		)
		if err != nil {
			return errs.NewDB("database.UpsertEditorFeedbackCtx", "insert failed", err)
		}
		id, _ := res.LastInsertId()
		f.ID = id
	case nil:
		// Update existing
		f.ID = existingID
		q := `UPDATE venue_validation_editor_feedback
              SET feedback_type = ?, comment = ?, ip = ?, created_at = NOW()
              WHERE id = ?`
		if _, err := db.conn.ExecContext(ctx, q,
			string(f.FeedbackType), f.Comment, f.IP, f.ID,
		); err != nil {
			return errs.NewDB("database.UpsertEditorFeedbackCtx", "update failed", err)
		}
	default:
		return errs.NewDB("database.UpsertEditorFeedbackCtx", "lookup failed", err)
	}

	// Best effort fetch created_at
	row = db.conn.QueryRowContext(ctx, "SELECT created_at FROM venue_validation_editor_feedback WHERE id = ?", f.ID)
	var ts time.Time
	if err := row.Scan(&ts); err == nil {
		f.CreatedAt = ts
	}

	return nil
}

// HasVenueFeedbackFromIPCtx returns true if a feedback exists from the same IP for the venue and prompt_version (nullable).
func (db *DB) HasVenueFeedbackFromIPCtx(ctx context.Context, venueID int64, ip []byte, promptVersion *string) (bool, error) {
	ctx, cancel := db.withReadTimeout(ctx)
	defer cancel()
	var exists int
	if promptVersion == nil {
		q := `SELECT 1 FROM venue_validation_editor_feedback WHERE venue_id = ? AND ip = ? AND prompt_version IS NULL LIMIT 1`
		row := db.conn.QueryRowContext(ctx, q, venueID, ip)
		if err := row.Scan(&exists); err != nil {
			if err == sql.ErrNoRows {
				return false, nil
			}
			return false, errs.NewDB("database.HasVenueFeedbackFromIPCtx", "query failed", err)
		}
		return true, nil
	}
	q := `SELECT 1 FROM venue_validation_editor_feedback WHERE venue_id = ? AND ip = ? AND prompt_version = ? LIMIT 1`
	row := db.conn.QueryRowContext(ctx, q, venueID, ip, *promptVersion)
	if err := row.Scan(&exists); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, errs.NewDB("database.HasVenueFeedbackFromIPCtx", "query failed", err)
	}
	return true, nil
}

// GetVenueFeedbackCtx returns feedback list for a venue and simple counts.
func (db *DB) GetVenueFeedbackCtx(ctx context.Context, venueID int64, limit int) ([]models.EditorFeedback, int, int, error) {
	ctx, cancel := db.withReadTimeout(ctx)
	defer cancel()
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := `SELECT id, venue_id, prompt_version, feedback_type, comment, ip, created_at
	      FROM venue_validation_editor_feedback WHERE venue_id = ? ORDER BY created_at DESC LIMIT ?`
	rows, err := db.conn.QueryContext(ctx, q, venueID, limit)
	if err != nil {
		return nil, 0, 0, errs.NewDB("database.GetVenueFeedbackCtx", "query failed", err)
	}
	defer rows.Close()
	list := make([]models.EditorFeedback, 0, limit)
	for rows.Next() {
		var e models.EditorFeedback
		var ft string
		if err := rows.Scan(&e.ID, &e.VenueID, &e.PromptVersion, &ft, &e.Comment, &e.IP, &e.CreatedAt); err != nil {
			return nil, 0, 0, errs.NewDB("database.GetVenueFeedbackCtx", "scan failed", err)
		}
		e.FeedbackType = models.FeedbackType(ft)
		list = append(list, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, 0, errs.NewDB("database.GetVenueFeedbackCtx", "rows error", err)
	}
	// counts
	var up, down int
	q2 := `SELECT SUM(CASE WHEN feedback_type='thumbs_up' THEN 1 ELSE 0 END),
	              SUM(CASE WHEN feedback_type='thumbs_down' THEN 1 ELSE 0 END)
	       FROM venue_validation_editor_feedback WHERE venue_id = ?`
	row := db.conn.QueryRowContext(ctx, q2, venueID)
	if err := row.Scan(&up, &down); err != nil {
		return list, 0, 0, nil // non-fatal
	}
	return list, up, down, nil
}

// GetFeedbackStatsCtx returns aggregate stats, optionally filtered by promptVersion.
func (db *DB) GetFeedbackStatsCtx(ctx context.Context, promptVersion *string) (*models.FeedbackStats, error) {
	ctx, cancel := db.withReadTimeout(ctx)
	defer cancel()
	cond := ""
	var args []any
	if promptVersion != nil {
		cond = "WHERE prompt_version = ?"
		args = append(args, *promptVersion)
	}
	stats := &models.FeedbackStats{ByVersion: make(map[string]struct{ Up, Down int })}
	// totals
	q := fmt.Sprintf(`SELECT 
		SUM(CASE WHEN feedback_type='thumbs_up' THEN 1 ELSE 0 END) AS up,
		SUM(CASE WHEN feedback_type='thumbs_down' THEN 1 ELSE 0 END) AS down,
		COUNT(*) AS total
		FROM venue_validation_editor_feedback %s`, cond)
	row := db.conn.QueryRowContext(ctx, q, args...)
	if err := row.Scan(&stats.ThumbsUp, &stats.ThumbsDown, &stats.Total); err != nil {
		if err == sql.ErrNoRows {
			return stats, nil
		}
		return nil, errs.NewDB("database.GetFeedbackStatsCtx", "totals query failed", err)
	}
	// daily (last 30 days)
	qd := fmt.Sprintf(`SELECT DATE(created_at) as d,
		SUM(CASE WHEN feedback_type='thumbs_up' THEN 1 ELSE 0 END) AS up,
		SUM(CASE WHEN feedback_type='thumbs_down' THEN 1 ELSE 0 END) AS down
		FROM venue_validation_editor_feedback %s
		GROUP BY d ORDER BY d DESC LIMIT 30`, cond)
	rows, err := db.conn.QueryContext(ctx, qd, args...)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var d time.Time
			var up, down int
			if err := rows.Scan(&d, &up, &down); err == nil {
				stats.Daily = append(stats.Daily, models.DailyCount{Date: d.Format("2006-01-02"), ThumbsUp: up, ThumbsDown: down})
			}
		}
	}
	// by version
	qv := `SELECT COALESCE(prompt_version, '') as pv,
		SUM(CASE WHEN feedback_type='thumbs_up' THEN 1 ELSE 0 END) AS up,
		SUM(CASE WHEN feedback_type='thumbs_down' THEN 1 ELSE 0 END) AS down
		FROM venue_validation_editor_feedback GROUP BY pv ORDER BY pv`
	rows2, err := db.conn.QueryContext(ctx, qv)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var pv string
			var up, down int
			if err := rows2.Scan(&pv, &up, &down); err == nil {
				if pv == "" {
					pv = "(unknown)"
				}
				stats.ByVersion[pv] = struct{ Up, Down int }{Up: up, Down: down}
			}
		}
	}
	return stats, nil
}

// GetAllEditorFeedbackPaginatedCtx returns all editor feedback with venue information, paginated
func (db *DB) GetAllEditorFeedbackPaginatedCtx(ctx context.Context, limit, offset int) ([]models.EditorFeedbackWithVenue, int, error) {
	ctx, cancel := db.withReadTimeout(ctx)
	defer cancel()

	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	// Get total count
	var total int
	countQuery := `SELECT COUNT(*) FROM venue_validation_editor_feedback`
	if err := db.conn.QueryRowContext(ctx, countQuery).Scan(&total); err != nil {
		return nil, 0, errs.NewDB("database.GetAllEditorFeedbackPaginatedCtx", "count query failed", err)
	}

	// Get paginated feedback with venue information
	query := `SELECT
		ef.id, ef.venue_id, ef.prompt_version, ef.feedback_type, ef.comment, ef.ip, ef.created_at,
		COALESCE(v.name, '') AS venue_name
		FROM venue_validation_editor_feedback ef
		LEFT JOIN venues v ON ef.venue_id = v.id
		ORDER BY ef.created_at DESC
		LIMIT ? OFFSET ?`

	rows, err := db.conn.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, errs.NewDB("database.GetAllEditorFeedbackPaginatedCtx", "query failed", err)
	}
	defer rows.Close()

	list := make([]models.EditorFeedbackWithVenue, 0, limit)
	for rows.Next() {
		var efv models.EditorFeedbackWithVenue
		var ft string
		if err := rows.Scan(
			&efv.ID,
			&efv.VenueID,
			&efv.PromptVersion,
			&ft,
			&efv.Comment,
			&efv.IP,
			&efv.CreatedAt,
			&efv.VenueName,
		); err != nil {
			return nil, 0, errs.NewDB("database.GetAllEditorFeedbackPaginatedCtx", "scan failed", err)
		}
		efv.FeedbackType = models.FeedbackType(ft)
		list = append(list, efv)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, errs.NewDB("database.GetAllEditorFeedbackPaginatedCtx", "rows iteration failed", err)
	}

	return list, total, nil
}
