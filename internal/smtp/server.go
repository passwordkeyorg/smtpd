package smtp

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	gosmtp "github.com/emersion/go-smtp"
	"github.com/passwordkeyorg/mail-ingest-service/internal/ratelimit"
	"github.com/passwordkeyorg/mail-ingest-service/internal/resolver"
	"github.com/passwordkeyorg/mail-ingest-service/internal/spool"
	"golang.org/x/time/rate"
)

type Config struct {
	ListenAddr   string
	MaxMsgBytes  int64
	MaxRcptCount int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	MaxConns     int

	TLSCertFile string
	TLSKeyFile  string
}

type Deps struct {
	Logger   *slog.Logger
	Resolver resolver.Resolver
	Spool    *spool.FS
	IPLimit  *ratelimit.Cache
	Metrics  Metrics
	Kafka    KafkaProducer
}

// KafkaProducer is an optional best-effort event sink.
// SMTP hot path must not depend on it.
type KafkaProducer interface {
	PublishIngest(ctx context.Context, ev IngestEvent) error
}

type IngestEvent struct {
	ID         string
	TraceID    string
	ReceivedAt string
	RemoteIP   string
	Domain     string
	Mailbox    string
	MailFrom   string
	RcptTo     []string
	Bytes      int64
	SHA256     string
	MetaPath   string
	EMLPath    string
}

func Run(ctx context.Context, cfg Config, deps Deps) error {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	if deps.Resolver == nil {
		return fmt.Errorf("resolver is required")
	}
	if deps.Spool == nil {
		return fmt.Errorf("spool is required")
	}
	if deps.IPLimit == nil {
		deps.IPLimit = ratelimit.NewCache(100000, 10*time.Minute, func() *rate.Limiter {
			return rate.NewLimiter(rate.Every(200*time.Millisecond), 20) // ~5 req/s burst 20
		})
	}

	be := &backend{cfg: cfg, deps: deps}
	s := gosmtp.NewServer(be)
	s.Addr = cfg.ListenAddr
	s.Domain = "mail-ingest"
	s.ReadTimeout = cfg.ReadTimeout
	s.WriteTimeout = cfg.WriteTimeout
	s.MaxMessageBytes = cfg.MaxMsgBytes
	s.MaxRecipients = cfg.MaxRcptCount
	s.AllowInsecureAuth = false

	if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
		if err != nil {
			return fmt.Errorf("load tls cert: %w", err)
		}
		// go-smtp advertises STARTTLS when TLSConfig is set.
		s.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	}

	ln, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", cfg.ListenAddr, err)
	}
	defer func() { _ = ln.Close() }()

	if cfg.MaxConns > 0 {
		ln = newLimitListener(ln, cfg.MaxConns)
	}

	// Run in background because Serve blocks.
	errCh := make(chan error, 1)
	go func() {
		deps.Logger.Info("smtpd starting",
			"addr", cfg.ListenAddr,
			"max_msg_bytes", cfg.MaxMsgBytes,
			"max_conns", cfg.MaxConns,
		)
		errCh <- s.Serve(ln)
	}()

	select {
	case <-ctx.Done():
		_ = s.Close()
		return ctx.Err()
	case err := <-errCh:
		if err == nil {
			return nil
		}
		if strings.Contains(err.Error(), "use of closed network connection") {
			return nil
		}
		return err
	}
}

func remoteIPFromConnState(state *gosmtp.Conn) string {
	if state == nil {
		return ""
	}
	c := state.Conn()
	if c == nil {
		return ""
	}
	addr := c.RemoteAddr()
	if addr == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	return host
}
