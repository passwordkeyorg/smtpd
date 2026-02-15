package store

import (
	"context"
	"io"
)

type Composite struct {
	Local FS
	Obj   *MinIO
}

func (c Composite) OpenByPath(path string) (io.ReadCloser, error) {
	return c.Local.OpenByPath(path)
}

func (c Composite) OpenByObjectKey(ctx context.Context, key string) (io.ReadCloser, error) {
	if c.Obj == nil {
		return nil, io.EOF
	}
	return c.Obj.OpenByObjectKey(ctx, key)
}
