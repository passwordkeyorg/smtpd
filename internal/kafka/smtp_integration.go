package kafka

import (
	"context"
	"encoding/json"
	"time"

	"github.com/segmentio/kafka-go"
)

type IngestEvent struct {
	ID         string   `json:"id"`
	TraceID    string   `json:"trace_id"`
	ReceivedAt string   `json:"received_at"`
	RemoteIP   string   `json:"remote_ip"`
	Domain     string   `json:"domain"`
	Mailbox    string   `json:"mailbox"`
	MailFrom   string   `json:"mail_from"`
	RcptTo     []string `json:"rcpt_to"`
	Bytes      int64    `json:"bytes"`
	SHA256     string   `json:"sha256"`
	MetaPath   string   `json:"meta_path"`
	EMLPath    string   `json:"eml_path"`
}

type SMTPProducer struct {
	W     *kafka.Writer
	topic string

	onComplete func(n int, err error)
}

func NewSMTPProducer(brokers []string, topic string) *SMTPProducer {
	p := &SMTPProducer{topic: topic}
	w := &kafka.Writer{
		Addr:                   kafka.TCP(brokers...),
		Topic:                  topic,
		AllowAutoTopicCreation: false,
		BatchTimeout:           10 * time.Millisecond,
		RequiredAcks:           kafka.RequireOne,
		Async:                  true, // async to not block SMTP
	}
	w.Completion = func(messages []kafka.Message, err error) {
		if p.onComplete != nil {
			p.onComplete(len(messages), err)
		}
	}
	p.W = w
	return p
}

func (p *SMTPProducer) SetCompletionHook(fn func(n int, err error)) {
	p.onComplete = fn
}

func (p *SMTPProducer) Close() error { return p.W.Close() }

func (p *SMTPProducer) Publish(ctx context.Context, ev IngestEvent) error {
	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	key := ev.Domain + "/" + ev.Mailbox
	return p.W.WriteMessages(ctx, kafka.Message{
		Key:   []byte(key),
		Value: b,
	})
}
