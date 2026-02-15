package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/passwordkeyorg/mail-ingest-service/internal/metrics"
	"github.com/passwordkeyorg/mail-ingest-service/internal/objectstore"
	"github.com/passwordkeyorg/mail-ingest-service/internal/uploader"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	spoolDir := getenv("SPOOL_DIR", "./data/spool")
	interval := getenvDur("UPLOAD_INTERVAL", 2*time.Second)
	metricsListen := getenv("METRICS_LISTEN", "127.0.0.1:9093")
	if err := metrics.RequireLocalhost(metricsListen); err != nil {
		logger.Error("invalid METRICS_LISTEN", "addr", metricsListen, "err", err)
		os.Exit(1)
	}

	ep := getenv("MINIO_ENDPOINT", "")
	ak := getenv("MINIO_ACCESS_KEY", "")
	sk := getenv("MINIO_SECRET_KEY", "")
	bucket := getenv("MINIO_BUCKET", "mail-raw")
	secure := os.Getenv("MINIO_SECURE") == "true"
	if ep == "" || ak == "" || sk == "" {
		logger.Error("minio env missing", "MINIO_ENDPOINT", ep != "", "MINIO_ACCESS_KEY", ak != "", "MINIO_SECRET_KEY", sk != "")
		os.Exit(2)
	}

	obj, err := objectstore.NewMinIO(objectstore.MinIOConfig{Endpoint: ep, AccessKey: ak, SecretKey: sk, Bucket: bucket, Secure: secure})
	if err != nil {
		logger.Error("minio init failed", "err", err)
		os.Exit(1)
	}
	if err := obj.EnsureBucket(ctx); err != nil {
		logger.Error("minio ensure bucket failed", "err", err)
		os.Exit(1)
	}

	mreg := metrics.New()
	u := &uploader.Uploader{Obj: obj, Conf: uploader.Config{SpoolDir: spoolDir}, Metrics: metrics.UploaderAdapter{M: mreg.Uploader}}

	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", mreg.Handler())
		srv := &http.Server{Addr: metricsListen, Handler: mux, ReadHeaderTimeout: 3 * time.Second}
		logger.Info("metrics listening", "addr", metricsListen)
		go func() {
			<-ctx.Done()
			_ = srv.Close()
		}()
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("metrics server error", "err", err)
		}
	}()

	logger.Info("uploader started", "spool_dir", spoolDir, "bucket", bucket, "interval", interval.String())

	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			logger.Info("uploader shutdown")
			return
		case <-t.C:
			scanned, uploaded, err := u.RunOnce(ctx)
			if err != nil {
				logger.Error("uploader run failed", "err", err)
				continue
			}
			if uploaded > 0 {
				logger.Info("uploader run", "scanned", scanned, "uploaded", uploaded)
			}
		}
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func getenvDur(k string, def time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		d, err := time.ParseDuration(v)
		if err == nil {
			return d
		}
	}
	return def
}
