package metrics

import "github.com/prometheus/client_golang/prometheus"

type IndexerMetrics struct {
	RunsTotal    prometheus.Counter
	ErrorsTotal  prometheus.Counter
	ScannedTotal prometheus.Counter
	IndexedTotal prometheus.Counter
	LastRunUnix  prometheus.Gauge
}
