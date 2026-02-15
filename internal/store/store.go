package store

import (
	"context"
	"io"
)

type Store interface {
	OpenByPath(path string) (io.ReadCloser, error)
	OpenByObjectKey(ctx context.Context, key string) (io.ReadCloser, error)
}
