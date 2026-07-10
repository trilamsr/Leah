package web

import (
	"net/http"
	"strconv"

	"github.com/trilam/leah/internal/platform/telemetry"
)

func MetricsMiddleware(registry *telemetry.Registry, h http.Handler) http.Handler {
	if registry == nil {
		return h
	}
	cnt := registry.Counter("leah_web_requests_total")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w, status: 200}
		h.ServeHTTP(sw, r)
		// Skip /metrics + /health to keep the probe surface from self-amplifying.
		if r.URL.Path == "/metrics" || r.URL.Path == "/health" {
			return
		}
		cnt.Inc(map[string]string{
			"path":   r.URL.Path,
			"status": strconv.Itoa(sw.status),
		})
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(c int) {
	s.status = c
	s.ResponseWriter.WriteHeader(c)
}
