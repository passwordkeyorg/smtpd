package spool

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

var ErrTooLarge = errors.New("message too large")

type StoreRequest struct {
	MaxBytes   int64
	RemoteIP   string
	MailFrom   string
	RcptTo     []string
	Domain     string
	Mailbox    string
	ReceivedAt time.Time
	TraceID    string
}

type StoreResult struct {
	ID       string
	TraceID  string
	Bytes    int64
	SHA256   string
	EMLPath  string
	MetaPath string
}

type FS struct {
	BaseDir string
	Now     func() time.Time
}

func (s *FS) Store(ctx context.Context, req StoreRequest, r io.Reader) (StoreResult, error) {
	_ = ctx // reserved for future cancellation checks

	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	ts := req.ReceivedAt
	if ts.IsZero() {
		ts = now()
	}

	id := NewULID(ts)
	dateDir := filepath.Join(s.BaseDir, "incoming", ts.Format("2006/01/02"))
	if err := os.MkdirAll(dateDir, 0o750); err != nil {
		return StoreResult{}, fmt.Errorf("mkdir %s: %w", dateDir, err)
	}

	tmpEML := filepath.Join(dateDir, id+".eml.tmp")
	eml := filepath.Join(dateDir, id+".eml")
	metaTmp := filepath.Join(dateDir, id+".json.tmp")
	meta := filepath.Join(dateDir, id+".json")

	f, err := os.OpenFile(tmpEML, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return StoreResult{}, fmt.Errorf("create tmp eml: %w", err)
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	bw := bufio.NewWriterSize(f, 32*1024)
	lr := &limitedReader{R: r, N: req.MaxBytes}
	written, copyErr := io.Copy(io.MultiWriter(bw, h), lr)
	if err := bw.Flush(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpEML)
		return StoreResult{}, fmt.Errorf("flush: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpEML)
		return StoreResult{}, fmt.Errorf("fsync: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpEML)
		return StoreResult{}, fmt.Errorf("close: %w", err)
	}

	if errors.Is(copyErr, ErrTooLarge) {
		_ = os.Remove(tmpEML)
		return StoreResult{}, ErrTooLarge
	}
	if copyErr != nil {
		_ = os.Remove(tmpEML)
		return StoreResult{}, fmt.Errorf("copy: %w", copyErr)
	}

	traceID := req.TraceID
	if traceID == "" {
		traceID = id
	}
	metaObj := map[string]any{
		"id":          id,
		"trace_id":    traceID,
		"received_at": ts.UTC().Format(time.RFC3339Nano),
		"remote_ip":   req.RemoteIP,
		"mail_from":   req.MailFrom,
		"rcpt_to":     req.RcptTo,
		"domain":      req.Domain,
		"mailbox":     req.Mailbox,
		"bytes":       written,
		"sha256":      hex.EncodeToString(h.Sum(nil)),
	}
	metaBytes, err := json.MarshalIndent(metaObj, "", "  ")
	if err != nil {
		_ = os.Remove(tmpEML)
		return StoreResult{}, fmt.Errorf("marshal meta: %w", err)
	}
	if err := os.WriteFile(metaTmp, metaBytes, 0o640); err != nil {
		_ = os.Remove(tmpEML)
		return StoreResult{}, fmt.Errorf("write meta: %w", err)
	}

	// Atomic publish: rename tmp -> final
	if err := os.Rename(tmpEML, eml); err != nil {
		_ = os.Remove(tmpEML)
		_ = os.Remove(metaTmp)
		return StoreResult{}, fmt.Errorf("rename eml: %w", err)
	}
	if err := os.Rename(metaTmp, meta); err != nil {
		// meta failed: keep eml but surface error; operator can reconcile
		_ = os.Remove(metaTmp)
		return StoreResult{}, fmt.Errorf("rename meta: %w", err)
	}

	return StoreResult{
		ID:       id,
		TraceID:  traceID,
		Bytes:    written,
		SHA256:   hex.EncodeToString(h.Sum(nil)),
		EMLPath:  eml,
		MetaPath: meta,
	}, nil
}

type limitedReader struct {
	R io.Reader
	N int64
}

func (l *limitedReader) Read(p []byte) (int, error) {
	if l.N <= 0 {
		return 0, ErrTooLarge
	}
	if int64(len(p)) > l.N {
		p = p[:l.N]
	}
	n, err := l.R.Read(p)
	l.N -= int64(n)
	return n, err
}
