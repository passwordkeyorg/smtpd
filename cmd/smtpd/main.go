package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/passwordkeyorg/mail-ingest-service/internal/kafka"
	"github.com/passwordkeyorg/mail-ingest-service/internal/metrics"
	"github.com/passwordkeyorg/mail-ingest-service/internal/resolver"
	"github.com/passwordkeyorg/mail-ingest-service/internal/smtp"
	"github.com/passwordkeyorg/mail-ingest-service/internal/spool"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	listen := getenv("SMTP_LISTEN", ":2525")
	metricsListen := getenv("METRICS_LISTEN", "127.0.0.1:9090")
	if err := metrics.RequireLocalhost(metricsListen); err != nil {
		logger.Error("invalid METRICS_LISTEN", "addr", metricsListen, "err", err)
		os.Exit(1)
	}
	snapshotPath := getenv("SNAPSHOT_PATH", "./data/snapshot.json")
	spoolDir := getenv("SPOOL_DIR", "./data/spool")
	maxBytes := getenvInt64("MAX_MSG_BYTES", 20*1024*1024)
	kafkaBrokers := getenv("KAFKA_BROKERS", "")
	kafkaTopic := getenv("KAFKA_TOPIC", "mail.ingest.v1")

	loader := &resolver.Loader{Path: snapshotPath}
	_, err := loader.Load()
	if err != nil {
		logger.Error("failed to load snapshot", "path", snapshotPath, "err", err)
		os.Exit(1)
	}

	fs := &spool.FS{BaseDir: spoolDir}
	mreg := metrics.New()

	var kp smtp.KafkaProducer
	if kafkaBrokers != "" {
		brokers := strings.FieldsFunc(kafkaBrokers, func(r rune) bool { return r == ',' || r == ' ' })
		p := kafka.NewSMTPProducer(brokers, kafkaTopic)
		p.SetCompletionHook(func(n int, err error) {
			if n <= 0 {
				return
			}
			if err != nil {
				mreg.SMTP.IncKafkaPublishError(n)
				return
			}
			mreg.SMTP.IncKafkaPublished(n)
		})
		defer func() { _ = p.Close() }()
		kp = kafka.SMTPAdapter{P: p}
		logger.Info("kafka enabled", "brokers", brokers, "topic", kafkaTopic)
	}

	cfg := smtp.Config{
		ListenAddr:   listen,
		MaxMsgBytes:  maxBytes,
		MaxRcptCount: 50,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		MaxConns:     2000,
	}

	// Metrics endpoint (localhost by default).
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

	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_, err := loader.Load()
				if err != nil {
					logger.Warn("snapshot reload failed", "err", err)
					continue
				}
				st := loader.Stats()
				mreg.SMTP.ResolverDomains.Set(float64(st.Domains))
				mreg.SMTP.ResolverBoxes.Set(float64(st.Mailboxes))
				mreg.SMTP.RatelimitIPSize.Set(float64(0))
			}
		}
	}()

	if err := smtp.Run(ctx, cfg, smtp.Deps{Logger: logger, Resolver: loader, Spool: fs, Metrics: &mreg.SMTP, Kafka: kp}); err != nil {
		logger.Error("smtpd stopped", "err", err)
		os.Exit(1)
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func getenvInt64(k string, def int64) int64 {
	if v := os.Getenv(k); v != "" {
		// minimal parse
		var n int64
		for _, ch := range v {
			if ch < '0' || ch > '9' {
				return def
			}
			n = n*10 + int64(ch-'0')
		}
		if n > 0 {
			return n
		}
	}
	return def
}
