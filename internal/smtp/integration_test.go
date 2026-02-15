package smtp

import (
	"context"
	"log/slog"
	"net"
	"net/smtp"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/passwordkeyorg/mail-ingest-service/internal/resolver"
	"github.com/passwordkeyorg/mail-ingest-service/internal/spool"
)

func TestSMTPD_AcceptsAndSpools(t *testing.T) {
	// Pick a random free port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	spoolDir := t.TempDir()
	snapPath := filepath.Join(t.TempDir(), "snapshot.json")
	if err := os.WriteFile(snapPath, []byte(`{
  "generated_at": "2026-02-14T00:00:00Z",
  "domains": {
    "example.com": {
      "mode": "allowlist",
      "plus_tag": true,
      "mailboxes": ["inbound"],
      "disabled": false
    }
  }
}`), 0o600); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}

	loader := &resolver.Loader{Path: snapPath}
	if _, err := loader.Load(); err != nil {
		t.Fatalf("load snapshot: %v", err)
	}

	fs := &spool.FS{BaseDir: spoolDir}
	cfg := Config{
		ListenAddr:   addr,
		MaxMsgBytes:  20 * 1024 * 1024,
		MaxRcptCount: 10,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		MaxConns:     50,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = Run(ctx, cfg, Deps{Logger: slog.New(slog.NewTextHandler(os.Stdout, nil)), Resolver: loader, Spool: fs})
	}()

	// Wait for server to start.
	deadline := time.Now().Add(3 * time.Second)
	for {
		c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = c.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server did not start")
		}
		time.Sleep(50 * time.Millisecond)
	}

	client, err := smtp.Dial(addr)
	if err != nil {
		t.Fatalf("dial smtp: %v", err)
	}
	defer func() { _ = client.Quit() }()

	if err := client.Mail("from@example.net"); err != nil {
		t.Fatalf("MAIL: %v", err)
	}
	if err := client.Rcpt("inbound+123@example.com"); err != nil {
		t.Fatalf("RCPT: %v", err)
	}
	w, err := client.Data()
	if err != nil {
		t.Fatalf("DATA: %v", err)
	}
	_, _ = w.Write([]byte("Subject: hi\r\n\r\nbody\r\n"))
	if err := w.Close(); err != nil {
		t.Fatalf("DATA close: %v", err)
	}

	// Assert spool has at least one .eml and one .json.
	var eml, meta int
	_ = filepath.WalkDir(spoolDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		switch filepath.Ext(path) {
		case ".eml":
			eml++
		case ".json":
			meta++
		}
		return nil
	})
	if eml == 0 || meta == 0 {
		t.Fatalf("expected eml/meta files, got eml=%d meta=%d", eml, meta)
	}
}
