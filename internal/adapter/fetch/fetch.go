// Package fetch downloads the page to analyze.
package fetch

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"syscall"
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
		return analysis.Page{}, &analysis.FetchError{Hint: "Invalid address", Reason: "The address is not valid.", Err: err}
	}

	resp, err := f.client.Do(req)
	if err != nil {
		hint, reason := f.describe(err)
		return analysis.Page{}, &analysis.FetchError{Hint: hint, Reason: reason, Err: err}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= http.StatusBadRequest {
		reason := fmt.Sprintf("The page answered with an error (%s), so there is nothing to analyze.", resp.Status)
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests:
			reason += " The site may be blocking automated requests."
		}
		return analysis.Page{}, &analysis.FetchError{StatusCode: resp.StatusCode, Reason: reason}
	}

	// checking content type header
	if contentType := mediaType(resp); contentType != "" && contentType != "text/html" && contentType != "application/xhtml+xml" {
		return analysis.Page{}, &analysis.FetchError{
			Hint:   "Not a web page",
			Reason: fmt.Sprintf("The address returned %s, not a web page.", contentType),
			Err:    analysis.ErrNotHTML,
		}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, f.maxBodySize))
	if err != nil {
		hint, reason := "Download interrupted", "The page started to load but the download broke off."
		if isTimeout(err) {
			hint, reason = f.describe(err)
		}
		return analysis.Page{}, &analysis.FetchError{Hint: hint, Reason: reason, Err: err}
	}

	return analysis.Page{
		URL:        resp.Request.URL,
		StatusCode: resp.StatusCode,
		Body:       body,
	}, nil
}

// mediaType returns the Content-Type of resp without its parameters, e.g. "text/html".
func mediaType(resp *http.Response) string {
	contentType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		return ""
	}
	return contentType
}

// describe turns a failed request into a short hint and a reason for the user.
func (f *Fetcher) describe(err error) (hint, reason string) {
	var dnsErr *net.DNSError
	var certErr *tls.CertificateVerificationError

	switch {
	case isTimeout(err):
		return "Timed out", fmt.Sprintf("The page did not answer within %s.", f.timeout)
	case errors.As(err, &dnsErr):
		return "No such host", "The host name could not be found. Check the address for typos."
	case errors.Is(err, syscall.ECONNREFUSED):
		return "Connection refused", "The server refused the connection. Nothing seems to be running at that address."
	case errors.As(err, &certErr):
		return "Invalid certificate", "A secure connection could not be made because the site's certificate is not valid."
	}
	return "Unreachable", "The page could not be reached."
}

func isTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
