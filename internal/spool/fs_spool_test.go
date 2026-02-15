package spool

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFSStore_WritesEMLAndMeta(t *testing.T) {
	dir := t.TempDir()
	fs := &FS{BaseDir: dir, Now: func() time.Time { return time.Date(2026, 2, 14, 0, 0, 0, 0, time.UTC) }}

	res, err := fs.Store(context.Background(), StoreRequest{
		MaxBytes:   1024,
		RemoteIP:   "1.2.3.4",
		MailFrom:   "from@example.com",
		RcptTo:     []string{"to@example.com"},
		Domain:     "example.com",
		Mailbox:    "inbound",
		ReceivedAt: time.Date(2026, 2, 14, 0, 0, 0, 0, time.UTC),
	}, bytes.NewBufferString("Subject: hi\r\n\r\nbody"))
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if res.ID == "" {
		t.Fatalf("expected id")
	}
	if _, err := os.Stat(res.EMLPath); err != nil {
		t.Fatalf("eml stat: %v", err)
	}
	if _, err := os.Stat(res.MetaPath); err != nil {
		t.Fatalf("meta stat: %v", err)
	}
	if filepath.Ext(res.EMLPath) != ".eml" {
		t.Fatalf("unexpected eml ext: %s", res.EMLPath)
	}
}

func TestFSStore_TooLarge(t *testing.T) {
	fs := &FS{BaseDir: t.TempDir()}
	_, err := fs.Store(context.Background(), StoreRequest{MaxBytes: 5}, bytes.NewBufferString("123456"))
	if err != ErrTooLarge {
		t.Fatalf("expected ErrTooLarge, got %v", err)
	}
}
