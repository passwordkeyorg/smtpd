package store

import (
	"context"
	"io"
	"os"

	"github.com/passwordkeyorg/mail-ingest-service/internal/objectstore"
)

type MinIO struct{ C *objectstore.MinIO }

func NewMinIO(c *objectstore.MinIO) MinIO { return MinIO{C: c} }

func (m MinIO) OpenByPath(_ string) (io.ReadCloser, error) {
	return nil, os.ErrNotExist
}

func (m MinIO) OpenByObjectKey(ctx context.Context, key string) (io.ReadCloser, error) {
	return m.C.Get(ctx, key)
}
