package metrics

import "github.com/prometheus/client_golang/prometheus"

type UploaderMetrics struct {
	RunsTotal     prometheus.Counter
	ErrorsTotal   prometheus.Counter
	ScannedTotal  prometheus.Counter
	UploadedTotal prometheus.Counter
	LastRunUnix   prometheus.Gauge
}
