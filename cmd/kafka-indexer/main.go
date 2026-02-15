package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/passwordkeyorg/mail-ingest-service/internal/index"
	"github.com/passwordkeyorg/mail-ingest-service/internal/kafka"
	"github.com/passwordkeyorg/mail-ingest-service/internal/metrics"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	indexPath := getenv("INDEX_DB", "/var/lib/mail-ingest/index.db")
	metricsListen := getenv("METRICS_LISTEN", "127.0.0.1:9094")
	if err := metrics.RequireLocalhost(metricsListen); err != nil {
		logger.Error("invalid METRICS_LISTEN", "addr", metricsListen, "err", err)
		os.Exit(1)
	}

	brokers := strings.FieldsFunc(getenv("KAFKA_BROKERS", "127.0.0.1:9092"), func(r rune) bool { return r == ',' || r == ' ' })
	topic := getenv("KAFKA_TOPIC", "mail.ingest.v1")
	groupID := getenv("KAFKA_GROUP", "mail-ingest-indexer")

	db, err := index.Open(indexPath)
	if err != nil {
		logger.Error("index open failed", "err", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	mreg := metrics.New()

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

	c := kafka.NewConsumer(brokers, topic, groupID)
	defer func() { _ = c.Close() }()

	logger.Info("kafka-indexer started", "brokers", brokers, "topic", topic, "group", groupID, "index_db", indexPath)

	for {
		select {
		case <-ctx.Done():
			logger.Info("shutdown")
			return
		default:
		}

		ev, err := c.Fetch(ctx)
		if err != nil {
			logger.Error("kafka fetch failed", "err", err)
			mreg.Indexer.ErrorsTotal.Inc()
			time.Sleep(500 * time.Millisecond)
			continue
		}

		rcptJSON := "[]"
		// Store expects JSON string; simplest: reuse encoding/json.
		// If rcpt_to missing, keep []
		if len(ev.RcptTo) > 0 {
			b, _ := json.Marshal(ev.RcptTo)
			rcptJSON = string(b)
		}

		if err := db.UpsertMessage(ctx, index.MessageRow{
			ID:         ev.ID,
			ReceivedAt: ev.ReceivedAt,
			RemoteIP:   ev.RemoteIP,
			Domain:     ev.Domain,
			Mailbox:    ev.Mailbox,
			MailFrom:   ev.MailFrom,
			RcptToJSON: rcptJSON,
			Bytes:      ev.Bytes,
			SHA256:     ev.SHA256,
			EMLPath:    ev.EMLPath,
			MetaPath:   ev.MetaPath,
			ObjectKey:  ev.ObjectKey,
		}); err != nil {
			logger.Error("db upsert failed", "err", err, "id", ev.ID)
			mreg.Indexer.ErrorsTotal.Inc()
			continue
		}

		mreg.Indexer.RunsTotal.Inc()
		mreg.Indexer.IndexedTotal.Inc()
		mreg.Indexer.LastRunUnix.Set(float64(time.Now().Unix()))
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
