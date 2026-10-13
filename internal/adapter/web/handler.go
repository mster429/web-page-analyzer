package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"

	"github.com/mster429/web-page-analyzer/internal/analysis"
)

// Analyzer is the part of the core the pages use.
type Analyzer interface {
	Analyze(ctx context.Context, rawURL string) (analysis.Result, error)
	CheckLinks(ctx context.Context, id string) (analysis.LinkCheck, error)
}

type Handler struct {
	analyzer Analyzer
	logger   zerolog.Logger
	metrics  *metrics
}

func New(analyzer Analyzer, logger zerolog.Logger, registry prometheus.Registerer) *Handler {
	return &Handler{analyzer: analyzer, logger: logger, metrics: newMetrics(registry)}
}

// Index shows the empty form.
func (h *Handler) Index(w http.ResponseWriter, _ *http.Request) {
	render(w, http.StatusOK, pageData{})
}

// Analyze handles GET /analyze?url=...
func (h *Handler) Analyze(w http.ResponseWriter, r *http.Request) {
	rawURL := r.URL.Query().Get("url")

	result, err := h.analyzer.Analyze(r.Context(), rawURL)
	if err != nil {
		h.analyzeFailed(w, rawURL, err)
		return
	}
	h.metrics.analyses.WithLabelValues("ok").Inc()

	render(w, http.StatusOK, pageData{URL: rawURL, Result: &result})
}

// Links handles GET /analyze/links?id=... It returns only the link section,
func (h *Handler) Links(w http.ResponseWriter, r *http.Request) {
	check, err := h.analyzer.CheckLinks(r.Context(), r.URL.Query().Get("id"))
	if errors.Is(err, analysis.ErrLinksExpired) {
		h.metrics.linkChecks.WithLabelValues("expired").Inc()
		renderLinks(w, http.StatusNotFound, linksData{Note: "This result has expired. Analyze the page again to check its links."})
		return
	}
	if err != nil {
		h.metrics.linkChecks.WithLabelValues("error").Inc()
		h.logger.Error().Err(err).Msg("link check failed")
		renderLinks(w, http.StatusInternalServerError, linksData{Note: "The links could not be checked."})
		return
	}

	h.metrics.linkChecks.WithLabelValues("ok").Inc()
	renderLinks(w, http.StatusOK, linksData{
		Internal:     check.Internal,
		External:     check.External,
		Inaccessible: check.Inaccessible,
		TimedOut:     check.TimedOut,
		NotChecked:   check.NotChecked,
		Checked:      true,
	})
}

// analyzeFailed shows the error page for err.
func (h *Handler) analyzeFailed(w http.ResponseWriter, rawURL string, err error) {
	var fetchErr *analysis.FetchError

	switch {
	case errors.Is(err, analysis.ErrInvalidURL):
		h.metrics.analyses.WithLabelValues("invalid_url").Inc()
		h.renderError(w, rawURL, http.StatusBadRequest,
			"Enter a full address that starts with http:// or https://.")

	case errors.As(err, &fetchErr):
		h.metrics.analyses.WithLabelValues("fetch_failed").Inc()
		h.logger.Warn().Err(err).Str("url", rawURL).Msg("page could not be fetched")

		h.renderError(w, rawURL, fetchStatus(fetchErr), fetchErr.Reason)

	default:
		h.metrics.analyses.WithLabelValues("error").Inc()
		h.logger.Error().Err(err).Str("url", rawURL).Msg("analysis failed")
		h.renderError(w, rawURL, http.StatusInternalServerError,
			"Something went wrong while analyzing the page.")
	}
}

func fetchStatus(err *analysis.FetchError) int {
	switch {
	case err.StatusCode != 0:
		// the site answered with an error, pass its status on
		return err.StatusCode
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout
	case errors.Is(err, analysis.ErrNotHTML):
		return http.StatusUnsupportedMediaType
	}
	return http.StatusBadGateway
}

func (h *Handler) renderError(w http.ResponseWriter, rawURL string, status int, description string) {
	render(w, status, pageData{
		URL: rawURL,
		Error: &pageError{
			Status:      strings.TrimSpace(fmt.Sprintf("%d %s", status, http.StatusText(status))),
			Description: description,
		},
	})
}

// Static serves the stylesheet.
func (h *Handler) Static() http.Handler {
	return http.FileServerFS(staticFS)
}
