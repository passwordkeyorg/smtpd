package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Registry struct {
	Reg *prometheus.Registry

	SMTP     SMTPMetrics
	API      APIMetrics
	Indexer  IndexerMetrics
	Uploader UploaderMetrics
}

func New() *Registry {
	reg := prometheus.NewRegistry()

	r := &Registry{Reg: reg}

	r.SMTP = SMTPMetrics{
		AcceptedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "mail_ingest_accepted_total",
			Help: "Total accepted SMTP messages",
		}),
		RejectedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "mail_ingest_rejected_total",
			Help: "Total rejected SMTP RCPT commands",
		}, []string{"reason"}),
		ReceivedBytes: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "mail_ingest_received_bytes_total",
			Help: "Total bytes accepted",
		}),
		SpoolErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "mail_ingest_spool_errors_total",
			Help: "Total spool store errors",
		}),
		ActiveConns: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "mail_ingest_active_connections",
			Help: "Current open SMTP connections",
		}),
		RatelimitIPSize: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "mail_ingest_ratelimit_ip_cache_size",
			Help: "Current number of IP limiter entries",
		}),
		ResolverDomains: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "mail_ingest_resolver_domains",
			Help: "Number of active domains in resolver snapshot",
		}),
		ResolverBoxes: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "mail_ingest_resolver_mailboxes",
			Help: "Number of active mailboxes in resolver snapshot",
		}),
		KafkaPublishedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "mail_ingest_kafka_published_total",
			Help: "Total Kafka messages successfully handed off for publish completion",
		}),
		KafkaPublishErrorTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "mail_ingest_kafka_publish_errors_total",
			Help: "Total Kafka message publish completions that returned error",
		}),
	}

	r.API = APIMetrics{
		RequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "mail_ingest_api_requests_total",
			Help: "Total API requests",
		}, []string{"method", "path", "status"}),
		RequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "mail_ingest_api_request_duration_seconds",
			Help:    "API request latency in seconds",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "path"}),
	}

	r.Indexer = IndexerMetrics{
		RunsTotal: prometheus.NewCounter(prometheus.CounterOpts{Name: "mail_ingest_indexer_runs_total", Help: "Total indexer runs"}),
		ErrorsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "mail_ingest_indexer_errors_total", Help: "Total indexer run errors",
		}),
		ScannedTotal: prometheus.NewCounter(prometheus.CounterOpts{Name: "mail_ingest_indexer_scanned_total", Help: "Total meta files scanned"}),
		IndexedTotal: prometheus.NewCounter(prometheus.CounterOpts{Name: "mail_ingest_indexer_indexed_total", Help: "Total meta files indexed"}),
		LastRunUnix:  prometheus.NewGauge(prometheus.GaugeOpts{Name: "mail_ingest_indexer_last_run_unix", Help: "Last indexer run time (unix seconds)"}),
	}

	r.Uploader = UploaderMetrics{
		RunsTotal: prometheus.NewCounter(prometheus.CounterOpts{Name: "mail_ingest_uploader_runs_total", Help: "Total uploader runs"}),
		ErrorsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "mail_ingest_uploader_errors_total", Help: "Total uploader run errors",
		}),
		ScannedTotal:  prometheus.NewCounter(prometheus.CounterOpts{Name: "mail_ingest_uploader_scanned_total", Help: "Total meta files scanned"}),
		UploadedTotal: prometheus.NewCounter(prometheus.CounterOpts{Name: "mail_ingest_uploader_uploaded_total", Help: "Total messages uploaded"}),
		LastRunUnix:   prometheus.NewGauge(prometheus.GaugeOpts{Name: "mail_ingest_uploader_last_run_unix", Help: "Last uploader run time (unix seconds)"}),
	}

	reg.MustRegister(
		r.SMTP.AcceptedTotal,
		r.SMTP.RejectedTotal,
		r.SMTP.ReceivedBytes,
		r.SMTP.SpoolErrors,
		r.SMTP.ActiveConns,
		r.SMTP.RatelimitIPSize,
		r.SMTP.ResolverDomains,
		r.SMTP.ResolverBoxes,
		r.SMTP.KafkaPublishedTotal,
		r.SMTP.KafkaPublishErrorTotal,
		r.API.RequestsTotal,
		r.API.RequestDuration,
		r.Indexer.RunsTotal,
		r.Indexer.ErrorsTotal,
		r.Indexer.ScannedTotal,
		r.Indexer.IndexedTotal,
		r.Indexer.LastRunUnix,
		r.Uploader.RunsTotal,
		r.Uploader.ErrorsTotal,
		r.Uploader.ScannedTotal,
		r.Uploader.UploadedTotal,
		r.Uploader.LastRunUnix,
	)
	return r
}

func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.Reg, promhttp.HandlerOpts{})
}
