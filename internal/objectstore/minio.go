package objectstore

import (
	"context"
	"io"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinIO struct {
	Client *minio.Client
	Bucket string
}

type MinIOConfig struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	Secure    bool
}

func NewMinIO(cfg MinIOConfig) (*MinIO, error) {
	c, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.Secure,
	})
	if err != nil {
		return nil, err
	}
	return &MinIO{Client: c, Bucket: cfg.Bucket}, nil
}

func (m *MinIO) EnsureBucket(ctx context.Context) error {
	exists, err := m.Client.BucketExists(ctx, m.Bucket)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return m.Client.MakeBucket(ctx, m.Bucket, minio.MakeBucketOptions{})
}

func (m *MinIO) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	_, err := m.Client.PutObject(ctx, m.Bucket, key, r, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	return err
}

func (m *MinIO) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := m.Client.GetObject(ctx, m.Bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	// validate early
	_, err = obj.Stat()
	if err != nil {
		_ = obj.Close()
		return nil, err
	}
	return obj, nil
}

func (m *MinIO) PresignGet(ctx context.Context, key string, expiry time.Duration) (string, error) {
	u, err := m.Client.PresignedGetObject(ctx, m.Bucket, key, expiry, nil)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}
