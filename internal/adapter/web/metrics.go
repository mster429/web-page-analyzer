package web

import "github.com/prometheus/client_golang/prometheus"

type metrics struct {
	requests   *prometheus.CounterVec
	duration   *prometheus.HistogramVec
	analyses   *prometheus.CounterVec
	linkChecks *prometheus.CounterVec
}

func newMetrics(registry prometheus.Registerer) *metrics {
	m := &metrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "HTTP requests handled, by route, method and status code.",
		}, []string{"route", "method", "status"}),

		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "http_request_duration_seconds",
			Help: "Time taken to handle a request, by route.",
			// a link check can take as long as its whole time budget
			Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 20, 30},
		}, []string{"route"}),

		analyses: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "analyses_total",
			Help: "Page analyses, by outcome.",
		}, []string{"outcome"}),

		linkChecks: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "link_checks_total",
			Help: "Link checks, by outcome.",
		}, []string{"outcome"}),
	}
	registry.MustRegister(m.requests, m.duration, m.analyses, m.linkChecks)
	return m
}
