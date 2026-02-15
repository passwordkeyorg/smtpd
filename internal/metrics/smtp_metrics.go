package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// SMTPMetrics implements internal/smtp.Metrics.
type SMTPMetrics struct {
	AcceptedTotal   prometheus.Counter
	RejectedTotal   *prometheus.CounterVec
	ReceivedBytes   prometheus.Counter
	SpoolErrors     prometheus.Counter
	ActiveConns     prometheus.Gauge
	RatelimitIPSize prometheus.Gauge
	ResolverDomains prometheus.Gauge
	ResolverBoxes   prometheus.Gauge

	KafkaPublishedTotal    prometheus.Counter
	KafkaPublishErrorTotal prometheus.Counter
}

func (m *SMTPMetrics) IncAccepted(bytes int64) {
	m.AcceptedTotal.Inc()
	m.ReceivedBytes.Add(float64(bytes))
}

func (m *SMTPMetrics) IncRejected(reason string) {
	m.RejectedTotal.WithLabelValues(reason).Inc()
}

func (m *SMTPMetrics) IncSpoolError() {
	m.SpoolErrors.Inc()
}

func (m *SMTPMetrics) ConnOpen()  { m.ActiveConns.Inc() }
func (m *SMTPMetrics) ConnClose() { m.ActiveConns.Dec() }

func (m *SMTPMetrics) IncKafkaPublished(n int) {
	if n <= 0 {
		return
	}
	m.KafkaPublishedTotal.Add(float64(n))
}

func (m *SMTPMetrics) IncKafkaPublishError(n int) {
	if n <= 0 {
		return
	}
	m.KafkaPublishErrorTotal.Add(float64(n))
}
