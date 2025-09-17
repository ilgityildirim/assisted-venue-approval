package events

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"assisted-venue-approval/pkg/database"
)

// SQLEventStore stores events in a SQL table with ordered IDs
// Table schema:
// CREATE TABLE IF NOT EXISTS venue_events (
//   id BIGINT AUTO_INCREMENT PRIMARY KEY,
//   venue_id BIGINT NOT NULL,
//   type VARCHAR(64) NOT NULL,
//   at DATETIME(6) NOT NULL,
//   admin VARCHAR(255) NULL,
//   admin_id INT NULL,
//   data JSON NOT NULL,
//   KEY idx_venue_id (venue_id),
//   KEY idx_venue_time (venue_id, id)
// );
// NOTE: JSON may be emulated as LONGTEXT on older MySQL variants.

type SQLEventStore struct {
	db *database.DB
}

func NewSQLEventStore(db *database.DB) *SQLEventStore {
	s := &SQLEventStore{db: db}
	if err := s.ensureTable(); err != nil {
		// Best effort; don't crash app start
		fmt.Printf("[events] ensure table error: %v\n", err)
	}
	return s
}

func (s *SQLEventStore) ensureTable() error {
	qry := `CREATE TABLE IF NOT EXISTS venue_events (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		venue_id BIGINT NOT NULL,
		type VARCHAR(64) NOT NULL,
		at DATETIME(6) NOT NULL,
		admin VARCHAR(255) NULL,
		admin_id INT NULL,
		data JSON NOT NULL,
		KEY idx_venue_id (venue_id),
		KEY idx_venue_time (venue_id, id)
	)`
	_, err := s.db.Conn().Exec(qry)
	return err
}

func (s *SQLEventStore) Append(ctx context.Context, ev ...Event) error {
	if len(ev) == 0 {
		return nil
	}
	tx, err := s.db.Conn().BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO venue_events (venue_id, type, at, admin, admin_id, data) VALUES (?,?,?,?,?,?)`)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	for _, e := range ev {
		payload := e.Payload()
		if payload == nil {
			payload = map[string]any{}
		}
		// Always include event type in payload for debugging
		payload["_type"] = e.Type()
		b, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal payload: %w", err)
		}

		at := e.GetTime()
		if at.IsZero() {
			at = time.Now()
		}

		if _, err := stmt.ExecContext(ctx, e.GetVenueID(), e.Type(), at, e.GetAdmin(), e.GetAdminID(), string(b)); err != nil {
			return fmt.Errorf("insert event: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func (s *SQLEventStore) ListByVenue(ctx context.Context, venueID int64) ([]StoredEvent, error) {
	rows, err := s.db.Conn().QueryContext(ctx, `SELECT id, venue_id, type, at, admin, admin_id, data FROM venue_events WHERE venue_id = ? ORDER BY id ASC`, venueID)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	var out []StoredEvent
	for rows.Next() {
		var se StoredEvent
		var admin sql.NullString
		var adminID sql.NullInt64
		var dataStr string
		if err := rows.Scan(&se.ID, &se.VenueID, &se.Type, &se.At, &admin, &adminID, &dataStr); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		if admin.Valid {
			v := admin.String
			se.Admin = &v
		}
		if adminID.Valid {
			v := int(adminID.Int64)
			se.AdminID = &v
		}
		se.Data = json.RawMessage(dataStr)
		out = append(out, se)
	}
	return out, nil
}

func (s *SQLEventStore) Replay(ctx context.Context, venueID int64) (*VenueState, error) {
	events, err := s.ListByVenue(ctx, venueID)
	if err != nil {
		return nil, err
	}
	st := RebuildState(events)
	return st, nil
}
