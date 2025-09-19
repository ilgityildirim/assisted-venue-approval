package events

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"assisted-venue-approval/internal/constants"
	"assisted-venue-approval/pkg/database"
)

// SQLEventStore persists events in a single table with an auto-incrementing primary key
// to preserve total ordering and per-venue ordering by (venue_id, seq).
type SQLEventStore struct {
	db *database.DB
}

func NewSQLEventStore(db *database.DB) (*SQLEventStore, error) {
	es := &SQLEventStore{db: db}
	if err := es.ensureSchema(); err != nil {
		return nil, err
	}
	return es, nil
}

func (s *SQLEventStore) ensureSchema() error {
	conn := s.db.Conn()
	// Compatible with MySQL; uses JSON column type.
	q := `CREATE TABLE IF NOT EXISTS venue_events (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		venue_id BIGINT NOT NULL,
		type VARCHAR(64) NOT NULL,
		ts TIMESTAMP NOT NULL,
		admin VARCHAR(255) NULL,
		payload JSON NOT NULL,
		INDEX idx_venue_ts (venue_id, ts),
		INDEX idx_venue_id (venue_id)
	)`
	if _, err := conn.Exec(q); err != nil {
		return fmt.Errorf("create venue_events table: %w", err)
	}
	return nil
}

func (s *SQLEventStore) Append(ctx context.Context, e Event) error {
	data, err := e.MarshalData()
	if err != nil {
		return fmt.Errorf("marshal event %s: %w", e.Type(), err)
	}
	conn := s.db.Conn()
	ctx, cancel := context.WithTimeout(ctx, constants.EventsSQLTimeoutDefault)
	defer cancel()
	_, err = conn.ExecContext(ctx, `INSERT INTO venue_events (venue_id, type, ts, admin, payload) VALUES (?, ?, ?, ?, ?)`,
		e.VenueID(), e.Type(), e.Timestamp(), e.Admin(), json.RawMessage(data))
	if err != nil {
		return fmt.Errorf("append event: %w", err)
	}
	return nil
}

func (s *SQLEventStore) ListByVenue(ctx context.Context, venueID int64) ([]StoredEvent, error) {
	conn := s.db.Conn()
	ctx, cancel := context.WithTimeout(ctx, constants.EventsSQLTimeoutDefault)
	defer cancel()
	rows, err := conn.QueryContext(ctx, `SELECT id, venue_id, type, ts, admin, payload FROM venue_events WHERE venue_id = ? ORDER BY id ASC`, venueID)
	if err != nil {
		return nil, fmt.Errorf("select events: %w", err)
	}
	defer rows.Close()
	var out []StoredEvent
	for rows.Next() {
		var se StoredEvent
		var admin sql.NullString
		var payload []byte
		if err := rows.Scan(&se.Seq, &se.VenueID, &se.Type, &se.Ts, &admin, &payload); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		if admin.Valid {
			s := admin.String
			se.Admin = &s
		}
		se.Payload = payload
		out = append(out, se)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *SQLEventStore) ReplayVenue(ctx context.Context, venueID int64) (*RebuiltState, error) {
	evts, err := s.ListByVenue(ctx, venueID)
	if err != nil {
		return nil, err
	}
	st := Replay(evts)
	return st, nil
}
