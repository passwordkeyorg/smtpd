package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/passwordkeyorg/mail-ingest-service/internal/metrics"
)

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func instrument(m metrics.APIMetrics, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w, status: 200}
		start := time.Now()
		next.ServeHTTP(sw, r)
		dur := time.Since(start).Seconds()

		path := r.URL.Path
		// coarse path labels to avoid cardinality explosions
		switch {
		case path == "/healthz":
			path = "/healthz"
		case path == "/v1/messages":
			path = "/v1/messages"
		case len(path) > 12 && path[:12] == "/v1/messages":
			path = "/v1/messages/{id}"
		}

		m.RequestsTotal.WithLabelValues(r.Method, path, strconv.Itoa(sw.status)).Inc()
		m.RequestDuration.WithLabelValues(r.Method, path).Observe(dur)
	})
}
