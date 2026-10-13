package analysis

import (
	"context"
	"net/url"
)

// PageFetcher downloads the page to analyze.
type PageFetcher interface {
	Fetch(ctx context.Context, pageURL *url.URL) (Page, error)
}

// Page is a downloaded page.
type Page struct {
	URL        *url.URL // after redirects
	StatusCode int
	Body       []byte
}
