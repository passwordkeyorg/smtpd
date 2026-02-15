package index

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

type DB struct{ *sql.DB }

func Open(path string) (*DB, error) {
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", path))
	if err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &DB{DB: db}, nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS messages (
  id TEXT PRIMARY KEY,
  trace_id TEXT NOT NULL DEFAULT '',
  received_at TEXT NOT NULL,
  remote_ip TEXT NOT NULL,
  domain TEXT NOT NULL,
  mailbox TEXT NOT NULL,
  mail_from TEXT NOT NULL,
  rcpt_to_json TEXT NOT NULL,
  bytes INTEGER NOT NULL,
  sha256 TEXT NOT NULL,
  eml_path TEXT NOT NULL,
  meta_path TEXT NOT NULL,
  object_key TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_messages_domain_mailbox_time ON messages(domain, mailbox, received_at, id);
CREATE INDEX IF NOT EXISTS idx_messages_trace_id ON messages(trace_id);

CREATE TABLE IF NOT EXISTS events (
  key TEXT PRIMARY KEY,
  trace_id TEXT NOT NULL,
  message_id TEXT NOT NULL,
  type TEXT NOT NULL,
  occurred_at TEXT NOT NULL,
  payload_json TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_events_trace_id_time ON events(trace_id, occurred_at, key);
`)
	if err != nil {
		return err
	}
	// For existing DBs created before columns existed.
	if err := ensureColumn(db, "messages", "object_key", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumn(db, "messages", "trace_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	return nil
}

type MessageRow struct {
	ID         string
	TraceID    string
	ReceivedAt string
	RemoteIP   string
	Domain     string
	Mailbox    string
	MailFrom   string
	RcptToJSON string
	Bytes      int64
	SHA256     string
	EMLPath    string
	MetaPath   string
	ObjectKey  string
}

type EventRow struct {
	Key         string
	TraceID     string
	MessageID   string
	Type        string
	OccurredAt  string
	PayloadJSON string
}

func (db *DB) UpsertMessage(ctx context.Context, r MessageRow) error {
	_, err := db.ExecContext(ctx, `
INSERT INTO messages (id, trace_id, received_at, remote_ip, domain, mailbox, mail_from, rcpt_to_json, bytes, sha256, eml_path, meta_path, object_key)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  trace_id=excluded.trace_id,
  received_at=excluded.received_at,
  remote_ip=excluded.remote_ip,
  domain=excluded.domain,
  mailbox=excluded.mailbox,
  mail_from=excluded.mail_from,
  rcpt_to_json=excluded.rcpt_to_json,
  bytes=excluded.bytes,
  sha256=excluded.sha256,
  eml_path=excluded.eml_path,
  meta_path=excluded.meta_path,
  object_key=excluded.object_key
`, r.ID, r.TraceID, r.ReceivedAt, r.RemoteIP, r.Domain, r.Mailbox, r.MailFrom, r.RcptToJSON, r.Bytes, r.SHA256, r.EMLPath, r.MetaPath, r.ObjectKey)
	return err
}

func (db *DB) ListMessages(ctx context.Context, domain, mailbox, after string, limit int) ([]MessageRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := `SELECT id, trace_id, received_at, remote_ip, domain, mailbox, mail_from, rcpt_to_json, bytes, sha256, eml_path, meta_path, object_key
FROM messages
WHERE domain = ? AND mailbox = ?`
	args := []any{domain, mailbox}
	if after != "" {
		q += ` AND (received_at || ':' || id) > ?`
		args = append(args, after)
	}
	q += ` ORDER BY received_at, id LIMIT ?`
	args = append(args, limit)

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []MessageRow
	for rows.Next() {
		var r MessageRow
		if err := rows.Scan(&r.ID, &r.TraceID, &r.ReceivedAt, &r.RemoteIP, &r.Domain, &r.Mailbox, &r.MailFrom, &r.RcptToJSON, &r.Bytes, &r.SHA256, &r.EMLPath, &r.MetaPath, &r.ObjectKey); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (db *DB) GetMessage(ctx context.Context, id string) (MessageRow, bool, error) {
	var r MessageRow
	err := db.QueryRowContext(ctx, `SELECT id, trace_id, received_at, remote_ip, domain, mailbox, mail_from, rcpt_to_json, bytes, sha256, eml_path, meta_path, object_key FROM messages WHERE id = ?`, id).
		Scan(&r.ID, &r.TraceID, &r.ReceivedAt, &r.RemoteIP, &r.Domain, &r.Mailbox, &r.MailFrom, &r.RcptToJSON, &r.Bytes, &r.SHA256, &r.EMLPath, &r.MetaPath, &r.ObjectKey)
	if err == sql.ErrNoRows {
		return MessageRow{}, false, nil
	}
	if err != nil {
		return MessageRow{}, false, err
	}
	return r, true, nil
}

func ensureColumn(db *sql.DB, table, col, ddl string) error {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if name == col {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + col + ` ` + ddl)
	return err
}
