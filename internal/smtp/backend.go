package smtp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	gosmtp "github.com/emersion/go-smtp"
	"github.com/passwordkeyorg/mail-ingest-service/internal/spool"
)

type backend struct {
	cfg  Config
	deps Deps
}

func (b *backend) NewSession(conn *gosmtp.Conn) (gosmtp.Session, error) {
	ip := remoteIPFromConnState(conn)
	lim := b.deps.IPLimit.Get(ip, time.Now())
	if !lim.Allow() {
		return nil, &gosmtp.SMTPError{Code: 421, Message: "rate limited"}
	}
	sess := &session{backend: b, remoteIP: ip, log: b.deps.Logger.With("remote_ip", ip)}
	if b.deps.Metrics != nil {
		b.deps.Metrics.ConnOpen()
		sess.onClose = b.deps.Metrics.ConnClose
	}
	return sess, nil
}

func (b *backend) AnonymousLogin(state *gosmtp.Conn) (gosmtp.Session, error) {
	return b.NewSession(state)
}

type session struct {
	backend  *backend
	remoteIP string
	log      *slog.Logger

	mailFrom string
	rcpt     []string
	domain   string
	mailbox  string

	onClose func()
}

func (s *session) Reset() {
	s.mailFrom = ""
	s.rcpt = nil
	s.domain = ""
	s.mailbox = ""
}

func (s *session) Logout() error {
	if s.onClose != nil {
		s.onClose()
	}
	return nil
}

func (s *session) Mail(from string, opts *gosmtp.MailOptions) error {
	// Accept empty reverse-path for bounces
	if from != "" {
		if _, err := mail.ParseAddress(from); err != nil {
			return &gosmtp.SMTPError{Code: 501, Message: "invalid MAIL FROM"}
		}
	}
	s.mailFrom = strings.ToLower(from)
	return nil
}

func (s *session) Rcpt(to string, opts *gosmtp.RcptOptions) error {
	if len(s.rcpt) >= s.backend.cfg.MaxRcptCount {
		return &gosmtp.SMTPError{Code: 452, Message: "too many recipients"}
	}
	addr, err := mail.ParseAddress(to)
	if err != nil {
		return &gosmtp.SMTPError{Code: 501, Message: "invalid RCPT TO"}
	}
	local, domain, ok := strings.Cut(addr.Address, "@")
	if !ok {
		return &gosmtp.SMTPError{Code: 501, Message: "invalid RCPT TO"}
	}
	local = strings.ToLower(local)
	domain = strings.ToLower(domain)
	if local == "" || domain == "" {
		return &gosmtp.SMTPError{Code: 501, Message: "invalid RCPT TO"}
	}
	if !s.backend.deps.Resolver.Allowed(domain, local) {
		if s.backend.deps.Metrics != nil {
			s.backend.deps.Metrics.IncRejected("mailbox_not_found")
		}
		return &gosmtp.SMTPError{Code: 550, Message: "mailbox not found"}
	}
	// For MVP: only support a single validated domain/mailbox per message.
	if s.domain == "" {
		s.domain = domain
		s.mailbox = local
	}
	s.rcpt = append(s.rcpt, strings.ToLower(addr.Address))
	return nil
}

func (s *session) Data(r io.Reader) error {
	if s.domain == "" || s.mailbox == "" {
		return &gosmtp.SMTPError{Code: 503, Message: "need RCPT TO first"}
	}
	start := time.Now()
	traceID := spool.NewULID(time.Now())
	res, err := s.backend.deps.Spool.Store(context.Background(), spool.StoreRequest{
		MaxBytes:   s.backend.cfg.MaxMsgBytes,
		RemoteIP:   s.remoteIP,
		MailFrom:   s.mailFrom,
		RcptTo:     s.rcpt,
		Domain:     s.domain,
		Mailbox:    s.mailbox,
		ReceivedAt: time.Now(),
		TraceID:    traceID,
	}, r)
	if errors.Is(err, spool.ErrTooLarge) {
		if s.backend.deps.Metrics != nil {
			s.backend.deps.Metrics.IncRejected("too_large")
		}
		return &gosmtp.SMTPError{Code: 552, Message: "message too large"}
	}
	if err != nil {
		if s.backend.deps.Metrics != nil {
			s.backend.deps.Metrics.IncSpoolError()
		}
		s.log.Error("spool store failed", "err", err)
		return &gosmtp.SMTPError{Code: 451, Message: "temporary failure"}
	}
	elapsed := time.Since(start)
	if s.backend.deps.Metrics != nil {
		s.backend.deps.Metrics.IncAccepted(res.Bytes)
	}
	s.log.Info("message accepted",
		"id", res.ID,
		"trace_id", res.TraceID,
		"domain", s.domain,
		"mailbox", s.mailbox,
		"bytes", res.Bytes,
		"sha256", res.SHA256,
		"duration_ms", elapsed.Milliseconds(),
	)

	// Best-effort: publish ingest event asynchronously.
	if s.backend.deps.Kafka != nil {
		ev := IngestEvent{
			ID:         res.ID,
			TraceID:    res.TraceID,
			ReceivedAt: time.Now().UTC().Format(time.RFC3339Nano),
			RemoteIP:   s.remoteIP,
			Domain:     s.domain,
			Mailbox:    s.mailbox,
			MailFrom:   s.mailFrom,
			RcptTo:     append([]string(nil), s.rcpt...),
			Bytes:      res.Bytes,
			SHA256:     res.SHA256,
			MetaPath:   res.MetaPath,
			EMLPath:    res.EMLPath,
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
			defer cancel()
			if err := s.backend.deps.Kafka.PublishIngest(ctx, ev); err != nil {
				s.log.Warn("kafka publish failed", "err", err, "id", res.ID)
			}
		}()
	}
	return nil
}

func (s *session) AuthPlain(username, password string) error {
	return fmt.Errorf("auth not supported")
}
