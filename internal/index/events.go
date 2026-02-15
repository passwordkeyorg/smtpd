package index

import (
	"context"
)

func (db *DB) UpsertEvent(ctx context.Context, e EventRow) error {
	_, err := db.ExecContext(ctx, `
INSERT INTO events (key, trace_id, message_id, type, occurred_at, payload_json)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(key) DO UPDATE SET
  trace_id=excluded.trace_id,
  message_id=excluded.message_id,
  type=excluded.type,
  occurred_at=excluded.occurred_at,
  payload_json=excluded.payload_json
`, e.Key, e.TraceID, e.MessageID, e.Type, e.OccurredAt, e.PayloadJSON)
	return err
}
