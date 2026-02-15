package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/passwordkeyorg/mail-ingest-service/internal/api"
	"github.com/passwordkeyorg/mail-ingest-service/internal/index"
	"github.com/passwordkeyorg/mail-ingest-service/internal/metrics"
	"github.com/passwordkeyorg/mail-ingest-service/internal/objectstore"
	"github.com/passwordkeyorg/mail-ingest-service/internal/store"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	listen := getenv("API_LISTEN", "127.0.0.1:8080")
	adminKey := getenv("ADMIN_API_KEY", "")
	indexPath := getenv("INDEX_DB", "./data/index.db")
	sPoolDir := getenv("SPOOL_DIR", "./data/spool")

	db, err := index.Open(indexPath)
	if err != nil {
		logger.Error("index open failed", "err", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	local := store.NewFS(sPoolDir)
	var st store.Store = local
	if ep := os.Getenv("MINIO_ENDPOINT"); ep != "" {
		c, err := objectstore.NewMinIO(objectstore.MinIOConfig{
			Endpoint:  ep,
			AccessKey: os.Getenv("MINIO_ACCESS_KEY"),
			SecretKey: os.Getenv("MINIO_SECRET_KEY"),
			Bucket:    getenv("MINIO_BUCKET", "mail-raw"),
			Secure:    os.Getenv("MINIO_SECURE") == "true",
		})
		if err != nil {
			logger.Error("minio init failed", "err", err)
			os.Exit(1)
		}
		if err := c.EnsureBucket(ctx); err != nil {
			logger.Error("minio ensure bucket failed", "err", err)
			os.Exit(1)
		}
		obj := store.NewMinIO(c)
		st = store.Composite{Local: local, Obj: &obj}
	}

	mreg := metrics.New()
	h := api.New(api.Deps{Logger: logger, DB: db, Store: st, AdminKey: adminKey, Metrics: &mreg.API})

	mux := http.NewServeMux()
	mux.Handle("/metrics", mreg.Handler())
	mux.Handle("/", h)

	srv := &http.Server{
		Addr:              listen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()

	logger.Info("api listening", "addr", listen)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("api server error", "err", err)
		os.Exit(1)
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
