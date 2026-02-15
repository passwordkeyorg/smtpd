package kafka

import (
	"context"

	"github.com/passwordkeyorg/mail-ingest-service/internal/smtp"
)

type SMTPAdapter struct {
	P *SMTPProducer
}

func (a SMTPAdapter) PublishIngest(ctx context.Context, ev smtp.IngestEvent) error {
	if a.P == nil {
		return nil
	}
	return a.P.Publish(ctx, IngestEvent{
		ID:         ev.ID,
		ReceivedAt: ev.ReceivedAt,
		RemoteIP:   ev.RemoteIP,
		Domain:     ev.Domain,
		Mailbox:    ev.Mailbox,
		MailFrom:   ev.MailFrom,
		RcptTo:     ev.RcptTo,
		Bytes:      ev.Bytes,
		SHA256:     ev.SHA256,
		MetaPath:   ev.MetaPath,
		EMLPath:    ev.EMLPath,
	})
}
