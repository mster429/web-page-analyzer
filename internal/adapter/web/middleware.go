package web

import (
	"net/http"
	"strconv"
	"time"
)

// statusRecorder remembers the status code a handler wrote.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// Middleware logs every request, counts it in the metrics and recovers from panics.
func (h *Handler) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		defer func() {
			if p := recover(); p != nil {
				h.logger.Error().Interface("panic", p).Str("path", r.URL.Path).Msg("handler panicked")
				http.Error(rec, "internal server error", http.StatusInternalServerError)
			}

			// label by route pattern, not by path, so the label set stays small
			route := r.Pattern
			if route == "" {
				route = "unmatched"
			}
			elapsed := time.Since(start)

			h.metrics.requests.WithLabelValues(route, r.Method, strconv.Itoa(rec.status)).Inc()
			h.metrics.duration.WithLabelValues(route).Observe(elapsed.Seconds())

			h.logger.Info().
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", rec.status).
				Dur("duration", elapsed).
				Msg("request")
		}()

		next.ServeHTTP(rec, r)
	})
}
