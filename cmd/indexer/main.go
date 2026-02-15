package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/passwordkeyorg/mail-ingest-service/internal/index"
	"github.com/passwordkeyorg/mail-ingest-service/internal/indexer"
	"github.com/passwordkeyorg/mail-ingest-service/internal/metrics"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	indexPath := getenv("INDEX_DB", "./data/index.db")
	spoolDir := getenv("SPOOL_DIR", "./data/spool")
	interval := getenvDur("INDEX_INTERVAL", 2*time.Second)
	metricsListen := getenv("METRICS_LISTEN", "127.0.0.1:9092")
	if err := metrics.RequireLocalhost(metricsListen); err != nil {
		logger.Error("invalid METRICS_LISTEN", "addr", metricsListen, "err", err)
		os.Exit(1)
	}

	db, err := index.Open(indexPath)
	if err != nil {
		logger.Error("index open failed", "err", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	mreg := metrics.New()
	ix := &indexer.Indexer{DB: db, Conf: indexer.Config{SpoolDir: spoolDir}, Metrics: metrics.IndexerAdapter{M: mreg.Indexer}}

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

	t := time.NewTicker(interval)
	defer t.Stop()

	logger.Info("indexer started", "index_db", indexPath, "spool_dir", spoolDir, "interval", interval.String())

	for {
		select {
		case <-ctx.Done():
			logger.Info("indexer shutdown")
			return
		case <-t.C:
			scanned, indexed, err := ix.RunOnce(ctx)
			if err != nil {
				logger.Error("indexer run failed", "err", err)
				continue
			}
			logger.Info("indexer run", "scanned", scanned, "indexed", indexed)
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
