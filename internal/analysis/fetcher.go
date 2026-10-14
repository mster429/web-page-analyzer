package analysis

import (
	"context"
	"errors"
	"fmt"
	"net/url"
)

// ErrNotHTML throws when the address returned something other than a web page.
var ErrNotHTML = errors.New("not an HTML page")

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

// FetchError says why a page could not be downloaded.
type FetchError struct {
	StatusCode int    // what the page answered with, 0 when it did not answer at all
	Hint       string // a few words naming the problem, e.g. "No such host"
	Reason     string // a reason for the user
	Err        error
}

// Error gives the reason shown to the user, followed by the underlying cause when there is one.
func (e *FetchError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("fetch page: %s (%v)", e.Reason, e.Err)
	}
	return "fetch page: " + e.Reason
}

func (e *FetchError) Unwrap() error {
	return e.Err
}
