package indexer

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/passwordkeyorg/mail-ingest-service/internal/index"
)

type Config struct {
	SpoolDir string
}

type Indexer struct {
	DB      *index.DB
	Conf    Config
	Metrics interface {
		IncRun(scanned, indexed int)
		IncError()
		SetLastRunUnix(t float64)
	}
}

type meta struct {
	ID         string   `json:"id"`
	TraceID    string   `json:"trace_id"`
	ReceivedAt string   `json:"received_at"`
	RemoteIP   string   `json:"remote_ip"`
	Domain     string   `json:"domain"`
	Mailbox    string   `json:"mailbox"`
	MailFrom   string   `json:"mail_from"`
	RcptTo     []string `json:"rcpt_to"`
	Bytes      int64    `json:"bytes"`
	SHA256     string   `json:"sha256"`
	ObjectKey  string   `json:"object_key"`
}

func (ix *Indexer) RunOnce(ctx context.Context) (scanned int, indexed int, err error) {
	incoming := filepath.Join(ix.Conf.SpoolDir, "incoming")
	defer func() {
		if ix.Metrics != nil {
			ix.Metrics.SetLastRunUnix(float64(time.Now().Unix()))
			if err != nil {
				ix.Metrics.IncError()
			} else {
				ix.Metrics.IncRun(scanned, indexed)
			}
		}
	}()
	walkErr := filepath.WalkDir(incoming, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".json") {
			return nil
		}
		scanned++

		b, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read meta %s: %w", path, err)
		}
		var m meta
		if err := json.Unmarshal(b, &m); err != nil {
			return fmt.Errorf("parse meta %s: %w", path, err)
		}
		emlPath := strings.TrimSuffix(path, ".json") + ".eml"
		rcptJSON, _ := json.Marshal(m.RcptTo)

		err = ix.DB.UpsertMessage(ctx, index.MessageRow{
			ID:         m.ID,
			TraceID:    m.TraceID,
			ReceivedAt: m.ReceivedAt,
			RemoteIP:   m.RemoteIP,
			Domain:     m.Domain,
			Mailbox:    m.Mailbox,
			MailFrom:   m.MailFrom,
			RcptToJSON: string(rcptJSON),
			Bytes:      m.Bytes,
			SHA256:     m.SHA256,
			EMLPath:    emlPath,
			MetaPath:   path,
			ObjectKey:  m.ObjectKey,
		})
		if err != nil {
			return fmt.Errorf("db upsert %s: %w", path, err)
		}
		// Event log: record ingest.received (idempotent key)
		payload, _ := json.Marshal(map[string]any{"meta_path": path, "eml_path": emlPath, "object_key": m.ObjectKey})
		_ = ix.DB.UpsertEvent(ctx, index.EventRow{
			Key:         "ingest.received:" + m.ID,
			TraceID:     m.TraceID,
			MessageID:   m.ID,
			Type:        "ingest.received",
			OccurredAt:  m.ReceivedAt,
			PayloadJSON: string(payload),
		})
		indexed++
		return nil
	})
	if os.IsNotExist(walkErr) {
		return 0, 0, nil
	}
	if walkErr != nil {
		return scanned, indexed, walkErr
	}
	return scanned, indexed, nil
}
