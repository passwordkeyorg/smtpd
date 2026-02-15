package metrics

import "github.com/prometheus/client_golang/prometheus"

type APIMetrics struct {
	RequestsTotal   *prometheus.CounterVec
	RequestDuration *prometheus.HistogramVec
}
