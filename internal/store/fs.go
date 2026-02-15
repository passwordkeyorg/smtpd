package store

import (
	"context"
	"io"
	"os"
	"path/filepath"
)

type FS struct{ Base string }

func NewFS(base string) FS { return FS{Base: base} }

func (fs FS) OpenByPath(relOrAbs string) (io.ReadCloser, error) {
	p := relOrAbs
	if !filepath.IsAbs(p) {
		p = filepath.Join(fs.Base, relOrAbs)
	}
	return os.Open(p)
}

func (fs FS) OpenByObjectKey(_ context.Context, _ string) (io.ReadCloser, error) {
	return nil, os.ErrNotExist
}
