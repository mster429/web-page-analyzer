package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"web-analyzer/internal/analysis"
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

// Analyze handles GET /analyze?url=...
func (h *Handler) Analyze(w http.ResponseWriter, r *http.Request) {
	result, err := h.analyzer.Analyze(r.Context(), r.URL.Query().Get("url"))
	if errors.Is(err, analysis.ErrInvalidURL) {
		http.Error(w, "enter a full address starting with http:// or https://", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	// plain text until the templates are added
	_, _ = fmt.Fprintf(w, "URL: %s\nHTML version: %s\nTitle: %s\n", result.URL, result.HTMLVersion, result.Title)
	_, _ = fmt.Fprintf(w, "Login form: %t\n", result.HasLoginForm)
	_, _ = fmt.Fprintf(w, "URL: %s\nHTML version: %s\nTitle: %s\n", result.URL, result.HTMLVersion, result.Title)
	for _, h := range result.Headings {
		_, _ = fmt.Fprintf(w, "h%d: %d\n", h.Level, h.Count)
	}
}
