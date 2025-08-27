package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"automatic-vendor-validation/internal/models"
	"automatic-vendor-validation/pkg/config"

	_ "github.com/go-sql-driver/mysql"
)

type DB struct {
	conn  *sql.DB
	stmts map[string]*sql.Stmt
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
		conn:  conn,
		stmts: make(map[string]*sql.Stmt),
	}

	if err := db.prepareStatements(); err != nil {
		return nil, fmt.Errorf("failed to prepare statements: %w", err)
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

	db := &DB{
		conn:  conn,
		stmts: make(map[string]*sql.Stmt),
	}

	if err := db.prepareStatements(); err != nil {
		return nil, fmt.Errorf("failed to prepare statements: %w", err)
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
                                    score_breakdown, google_place_id, google_place_found, google_place_data, processed_at) 
                                   VALUES (?, ?, ?, ?, ?, ?, ?, ?, NOW())`,
	}

	for name, query := range statements {
		stmt, err := db.conn.Prepare(query)
		if err != nil {
			return fmt.Errorf("failed to prepare statement %s: %w", name, err)
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
		return nil, fmt.Errorf("failed to query pending venues: %w", err)
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

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
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
        a.level as ambassador_level, a.points as ambassador_points, a.path as ambassador_region
        FROM venues v
        LEFT JOIN members m ON v.user_id = m.id
        LEFT JOIN venue_admin va ON v.id = va.venue_id AND v.user_id = va.user_id
        LEFT JOIN ambassadors a ON v.user_id = a.user_id
        WHERE v.active = 0
        ORDER BY v.created_at ASC`

	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending venues with user info: %w", err)
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
			&ambassadorRegion,
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

// BatchUpdateVenueStatus updates multiple venues in a single transaction
func (db *DB) BatchUpdateVenueStatus(venueIDs []int64, active int, notes string, updatedByID *int) error {
	if len(venueIDs) == 0 {
		return nil
	}

	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
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
		return fmt.Errorf("failed to batch update venues: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit batch update transaction: %w", err)
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

	// Save validation history
	historyQuery := `INSERT INTO venue_validation_histories 
	                     (venue_id, validation_score, validation_status, validation_notes, 
	                      score_breakdown, processed_at) 
	                     VALUES (?, ?, ?, ?, ?, NOW())`

	scoreBreakdownJSON, err := json.Marshal(result.ScoreBreakdown)
	if err != nil {
		return fmt.Errorf("failed to marshal score breakdown: %w", err)
	}

	_, err = tx.Exec(historyQuery, result.VenueID, result.Score, result.Status,
		result.Notes, string(scoreBreakdownJSON))
	if err != nil {
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
              score_breakdown, processed_at 
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

		err := rows.Scan(&h.ID, &h.VenueID, &h.ValidationScore, &h.ValidationStatus,
			&h.ValidationNotes, &scoreBreakdownJSON, &h.ProcessedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan validation history row: %w", err)
		}

		if err = json.Unmarshal([]byte(scoreBreakdownJSON), &h.ScoreBreakdown); err != nil {
			return nil, fmt.Errorf("failed to unmarshal score breakdown: %w", err)
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
        JOIN members m ON v.user_id = m.id 
        LEFT JOIN venue_admin va ON v.id = va.venue_id AND m.id = va.user_id
        LEFT JOIN ambassadors a ON m.id = a.user_id
        WHERE v.id = ?`

	var venueWithUser models.VenueWithUser
	var venue models.Venue
	var user models.User
	var isVenueAdmin bool
	var ambassadorLevel, ambassadorPoints sql.NullInt64
	var ambassadorPath sql.NullString
	var trustedInt int

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
		&user.ID, &user.Username, &trustedInt,
		// Authority fields
		&isVenueAdmin, &ambassadorLevel, &ambassadorPoints, &ambassadorPath,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get venue with user by ID: %w", err)
	}

	user.Trusted = trustedInt > 0 // Convert int to bool
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
        vvh.validation_notes, vvh.score_breakdown, vvh.processed_at,
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

		err := rows.Scan(&h.ID, &h.VenueID, &h.ValidationScore, &h.ValidationStatus,
			&h.ValidationNotes, &scoreBreakdownJSON, &h.ProcessedAt, &h.VenueName)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan validation history row: %w", err)
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
        score_breakdown, processed_at
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

		err := rows.Scan(&h.ID, &h.VenueID, &h.ValidationScore, &h.ValidationStatus,
			&h.ValidationNotes, &scoreBreakdownJSON, &h.ProcessedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan validation history row: %w", err)
		}

		if err = json.Unmarshal([]byte(scoreBreakdownJSON), &h.ScoreBreakdown); err != nil {
			return nil, fmt.Errorf("failed to unmarshal score breakdown: %w", err)
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
		result.Notes, string(scoreBreakdownJSON), googlePlaceID, googlePlaceFound, googlePlaceDataJSON)
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
