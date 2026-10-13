package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/mster429/web-page-analyzer/internal/analysis"
)

// Analyzer is the part of the core the pages use.
type Analyzer interface {
	Analyze(ctx context.Context, rawURL string) (analysis.Result, error)
}

type Handler struct {
	analyzer Analyzer
}

func New(analyzer Analyzer) *Handler {
	return &Handler{analyzer: analyzer}
}

// Index shows the empty form.
func (h *Handler) Index(w http.ResponseWriter, _ *http.Request) {
	render(w, http.StatusOK, pageData{})
}

// Analyze handles GET /analyze?url=...
func (h *Handler) Analyze(w http.ResponseWriter, r *http.Request) {
	rawURL := r.URL.Query().Get("url")

	result, err := h.analyzer.Analyze(r.Context(), rawURL)
	if errors.Is(err, analysis.ErrInvalidURL) {
		h.renderError(w, rawURL, http.StatusBadRequest,
			"Enter a full address that starts with http:// or https://.")
		return
	}
	if err != nil {
		h.renderError(w, rawURL, http.StatusBadGateway, "The page could not be reached.")
		return
	}

	render(w, http.StatusOK, pageData{URL: rawURL, Result: &result})
}

func (h *Handler) renderError(w http.ResponseWriter, rawURL string, status int, description string) {
	render(w, status, pageData{
		URL: rawURL,
		Error: &pageError{
			Status:      fmt.Sprintf("%d %s", status, http.StatusText(status)),
			Description: description,
		},
	})
}

// Static serves the stylesheet.
func (h *Handler) Static() http.Handler {
	return http.FileServerFS(staticFS)
}
