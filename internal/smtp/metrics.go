package smtp

// Metrics is an optional interface for recording key counters/gauges.
// It is intentionally small and easy to mock.
type Metrics interface {
	IncAccepted(bytes int64)
	IncRejected(reason string)
	IncSpoolError()
	ConnOpen()
	ConnClose()

	// Kafka publish telemetry (best-effort). Implementations should be cheap and non-blocking.
	IncKafkaPublished(n int)
	IncKafkaPublishError(n int)
}
