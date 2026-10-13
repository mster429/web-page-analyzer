package fetch

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/mster429/web-page-analyzer/internal/analysis"
)

// Fetcher downloads pages over HTTP.
type Fetcher struct {
	client      *http.Client
	timeout     time.Duration
	maxBodySize int64
}

// New returns a Fetcher. At most maxBodySize bytes of a page are read.
func New(client *http.Client, timeout time.Duration, maxBodySize int64) *Fetcher {
	return &Fetcher{client: client, timeout: timeout, maxBodySize: maxBodySize}
}

// Fetch downloads the page at pageURL.
func (f *Fetcher) Fetch(ctx context.Context, pageURL *url.URL) (analysis.Page, error) {
	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL.String(), nil)
	if err != nil {
		return analysis.Page{}, fmt.Errorf("build request failed: %w", err)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return analysis.Page{}, fmt.Errorf("fetch page failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= http.StatusBadRequest {
		return analysis.Page{}, fmt.Errorf("fetch page failed: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, f.maxBodySize))
	if err != nil {
		return analysis.Page{}, fmt.Errorf("read page failed: %w", err)
	}

	return analysis.Page{
		URL:        resp.Request.URL,
		StatusCode: resp.StatusCode,
		Body:       body,
	}, nil
}
