package uploader

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/passwordkeyorg/mail-ingest-service/internal/objectstore"
)

type Config struct {
	SpoolDir string
	Now      func() time.Time
}

type Uploader struct {
	Obj     *objectstore.MinIO
	Conf    Config
	Metrics interface {
		IncRun(scanned, uploaded int)
		IncError()
		SetLastRunUnix(t float64)
	}
}

type meta struct {
	ID         string `json:"id"`
	Domain     string `json:"domain"`
	Mailbox    string `json:"mailbox"`
	ReceivedAt string `json:"received_at"`
	ObjectKey  string `json:"object_key"`
	UploadedAt string `json:"uploaded_at"`
}

func (u *Uploader) RunOnce(ctx context.Context) (scanned int, uploaded int, err error) {
	incoming := filepath.Join(u.Conf.SpoolDir, "incoming")
	now := u.Conf.Now
	if now == nil {
		now = time.Now
	}
	defer func() {
		if u.Metrics != nil {
			u.Metrics.SetLastRunUnix(float64(time.Now().Unix()))
			if err != nil {
				u.Metrics.IncError()
			} else {
				u.Metrics.IncRun(scanned, uploaded)
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
		var raw map[string]any
		if err := json.Unmarshal(b, &raw); err != nil {
			return fmt.Errorf("parse meta %s: %w", path, err)
		}
		if v, ok := raw["object_key"]; ok {
			if s, _ := v.(string); s != "" {
				return nil
			}
		}
		m := meta{
			ID:         str(raw["id"]),
			Domain:     str(raw["domain"]),
			Mailbox:    str(raw["mailbox"]),
			ReceivedAt: str(raw["received_at"]),
		}

		emlPath := strings.TrimSuffix(path, ".json") + ".eml"
		fi, err := os.Stat(emlPath)
		if err != nil {
			return fmt.Errorf("stat eml %s: %w", emlPath, err)
		}
		f, err := os.Open(emlPath)
		if err != nil {
			return fmt.Errorf("open eml %s: %w", emlPath, err)
		}
		defer func() { _ = f.Close() }()

		key := objectKey(m.Domain, m.Mailbox, m.ReceivedAt, m.ID)
		if err := u.Obj.Put(ctx, key, f, fi.Size(), "message/rfc822"); err != nil {
			return fmt.Errorf("put object %s: %w", key, err)
		}
		uploaded++

		raw["object_key"] = key
		raw["uploaded_at"] = now().UTC().Format(time.RFC3339Nano)
		out, _ := json.MarshalIndent(raw, "", "  ")
		out = append(out, '\n')
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, out, 0o640); err != nil {
			return fmt.Errorf("write meta tmp %s: %w", tmp, err)
		}
		if err := os.Rename(tmp, path); err != nil {
			return fmt.Errorf("rename meta %s: %w", path, err)
		}
		return nil
	})
	if os.IsNotExist(walkErr) {
		return 0, 0, nil
	}
	if walkErr != nil {
		return scanned, uploaded, walkErr
	}
	return scanned, uploaded, nil
}

func objectKey(domain, mailbox, receivedAt, id string) string {
	date := "unknown"
	if len(receivedAt) >= 10 {
		date = receivedAt[:10] // YYYY-MM-DD
	}
	parts := strings.Split(date, "-")
	if len(parts) == 3 {
		return fmt.Sprintf("%s/%s/%s/%s/%s/%s.eml", domain, mailbox, parts[0], parts[1], parts[2], id)
	}
	return fmt.Sprintf("%s/%s/%s.eml", domain, mailbox, id)
}
