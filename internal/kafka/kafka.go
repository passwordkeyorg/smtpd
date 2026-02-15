package kafka

import (
	"context"
	"encoding/json"
	"time"

	"github.com/segmentio/kafka-go"
)

type Event struct {
	ID         string   `json:"id"`
	ReceivedAt string   `json:"received_at"`
	RemoteIP   string   `json:"remote_ip"`
	Domain     string   `json:"domain"`
	Mailbox    string   `json:"mailbox"`
	MailFrom   string   `json:"mail_from"`
	RcptTo     []string `json:"rcpt_to"`
	Bytes      int64    `json:"bytes"`
	SHA256     string   `json:"sha256"`
	ObjectKey  string   `json:"object_key"`
	MetaPath   string   `json:"meta_path,omitempty"`
	EMLPath    string   `json:"eml_path,omitempty"`
}

type Producer struct {
	W *kafka.Writer
}

func NewProducer(brokers []string, topic string) *Producer {
	w := &kafka.Writer{
		Addr:                   kafka.TCP(brokers...),
		Topic:                  topic,
		AllowAutoTopicCreation: true,
		BatchTimeout:           50 * time.Millisecond,
		RequiredAcks:           kafka.RequireOne,
	}
	return &Producer{W: w}
}

func (p *Producer) Close() error { return p.W.Close() }

func (p *Producer) Publish(ctx context.Context, key string, ev Event) error {
	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	return p.W.WriteMessages(ctx, kafka.Message{Key: []byte(key), Value: b})
}

type Consumer struct {
	R *kafka.Reader
}

func NewConsumer(brokers []string, topic, groupID string) *Consumer {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  brokers,
		Topic:    topic,
		GroupID:  groupID,
		MinBytes: 1e3,
		MaxBytes: 10e6,
	})
	return &Consumer{R: r}
}

func (c *Consumer) Close() error { return c.R.Close() }

func (c *Consumer) Fetch(ctx context.Context) (Event, error) {
	m, err := c.R.ReadMessage(ctx)
	if err != nil {
		return Event{}, err
	}
	var ev Event
	if err := json.Unmarshal(m.Value, &ev); err != nil {
		return Event{}, err
	}
	return ev, nil
}
