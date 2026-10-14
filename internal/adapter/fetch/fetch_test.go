package fetch

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/mster429/web-page-analyzer/internal/analysis"
)

// roundTripFunc stands in for the network, so no test makes a real request.
type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func response(req *http.Request, status int, contentType, body string) *http.Response {
	header := http.Header{}
	if contentType != "" {
		header.Set("Content-Type", contentType)
	}
	return &http.Response{
		Status:     http.StatusText(status),
		StatusCode: status,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

func newFetcher(transport roundTripFunc) *Fetcher {
	return New(&http.Client{Transport: transport}, time.Second, 1024)
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// fetchError fails the test unless err is a *analysis.FetchError.
func fetchError(t *testing.T, err error) *analysis.FetchError {
	t.Helper()
	var fetchErr *analysis.FetchError
	if !errors.As(err, &fetchErr) {
		t.Fatalf("error = %v, want a *analysis.FetchError", err)
	}
	return fetchErr
}

func TestFetch(t *testing.T) {
	var method, requested string
	fetcher := newFetcher(func(req *http.Request) (*http.Response, error) {
		method, requested = req.Method, req.URL.String()
		return response(req, http.StatusOK, "text/html; charset=utf-8", "<html>hello</html>"), nil
	})

	page, err := fetcher.Fetch(context.Background(), mustURL(t, "https://example.com/page"))
	if err != nil {
		t.Fatalf("Fetch returned an error: %v", err)
	}

	if method != http.MethodGet || requested != "https://example.com/page" {
		t.Errorf("sent %s %s, want GET https://example.com/page", method, requested)
	}
	if page.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", page.StatusCode)
	}
	if string(page.Body) != "<html>hello</html>" {
		t.Errorf("Body = %q", page.Body)
	}
	if page.URL.String() != "https://example.com/page" {
		t.Errorf("URL = %q", page.URL)
	}
}

func TestFetch_FollowsRedirects(t *testing.T) {
	fetcher := newFetcher(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/old" {
			resp := response(req, http.StatusMovedPermanently, "", "")
			resp.Header.Set("Location", "https://example.com/new")
			return resp, nil
		}
		return response(req, http.StatusOK, "text/html", "moved here"), nil
	})

	page, err := fetcher.Fetch(context.Background(), mustURL(t, "https://example.com/old"))
	if err != nil {
		t.Fatal(err)
	}
	// relative links have to be resolved against the address the page ended up at
	if page.URL.String() != "https://example.com/new" {
		t.Errorf("URL = %q, want the address after the redirect", page.URL)
	}
	if string(page.Body) != "moved here" {
		t.Errorf("Body = %q", page.Body)
	}
}

func TestFetch_BodyLimit(t *testing.T) {
	fetcher := New(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return response(req, http.StatusOK, "text/html", strings.Repeat("a", 500)), nil
	})}, time.Second, 100)

	page, err := fetcher.Fetch(context.Background(), mustURL(t, "https://example.com"))
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Body) != 100 {
		t.Errorf("read %d bytes, want it cut at 100", len(page.Body))
	}
}

func TestFetch_ErrorStatus(t *testing.T) {
	const hint = "blocking automated requests"

	tests := []struct {
		status   int
		wantHint bool
	}{
		{http.StatusNotFound, false},
		{http.StatusInternalServerError, false},
		{http.StatusBadRequest, false},
		{http.StatusUnauthorized, true},
		{http.StatusForbidden, true},
		{http.StatusTooManyRequests, true},
	}

	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			fetcher := newFetcher(func(req *http.Request) (*http.Response, error) {
				return response(req, tt.status, "text/html", "<html>error page</html>"), nil
			})

			_, err := fetcher.Fetch(context.Background(), mustURL(t, "https://example.com"))

			fetchErr := fetchError(t, err)
			if fetchErr.StatusCode != tt.status {
				t.Errorf("StatusCode = %d, want %d", fetchErr.StatusCode, tt.status)
			}
			if got := strings.Contains(fetchErr.Reason, hint); got != tt.wantHint {
				t.Errorf("Reason = %q, blocking hint present = %v, want %v", fetchErr.Reason, got, tt.wantHint)
			}
		})
	}
}

func TestFetch_ContentType(t *testing.T) {
	tests := []struct {
		contentType string
		wantOK      bool
	}{
		{"text/html", true},
		{"text/html; charset=utf-8", true},
		{"TEXT/HTML", true},
		{"application/xhtml+xml", true},
		{"", true},             // no header, assume it is a page
		{"text/html; =", true}, // a header that cannot be read is treated like a missing one
		{"application/pdf", false},
		{"image/png", false},
		{"application/json", false},
		{"text/plain", false},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			fetcher := newFetcher(func(req *http.Request) (*http.Response, error) {
				return response(req, http.StatusOK, tt.contentType, "content"), nil
			})

			_, err := fetcher.Fetch(context.Background(), mustURL(t, "https://example.com/file"))

			if tt.wantOK {
				if err != nil {
					t.Errorf("Fetch returned an error: %v", err)
				}
				return
			}
			if !errors.Is(err, analysis.ErrNotHTML) {
				t.Fatalf("error = %v, want ErrNotHTML", err)
			}
			fetchErr := fetchError(t, err)
			if fetchErr.Hint != "Not a web page" {
				t.Errorf("Hint = %q, want Not a web page", fetchErr.Hint)
			}
			if !strings.Contains(fetchErr.Reason, tt.contentType) {
				t.Errorf("Reason = %q, want it to name %q", fetchErr.Reason, tt.contentType)
			}
		})
	}
}

func TestFetch_Unreachable(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantHint   string
		wantReason string
	}{
		{"unknown host", &net.DNSError{Err: "no such host", Name: "nope.invalid", IsNotFound: true}, "No such host", "host name could not be found"},
		{"connection refused", &net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}, "Connection refused", "refused the connection"},
		{"bad certificate", &tls.CertificateVerificationError{Err: errors.New("certificate has expired")}, "Invalid certificate", "certificate is not valid"},
		{"anything else", errors.New("connection reset by peer"), "Unreachable", "could not be reached"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fetcher := newFetcher(func(_ *http.Request) (*http.Response, error) {
				return nil, tt.err
			})

			_, err := fetcher.Fetch(context.Background(), mustURL(t, "https://example.com"))

			fetchErr := fetchError(t, err)
			if fetchErr.StatusCode != 0 {
				t.Errorf("StatusCode = %d, want 0 because the page gave no answer", fetchErr.StatusCode)
			}
			if fetchErr.Hint != tt.wantHint {
				t.Errorf("Hint = %q, want %q", fetchErr.Hint, tt.wantHint)
			}
			if !strings.Contains(fetchErr.Reason, tt.wantReason) {
				t.Errorf("Reason = %q, want it to contain %q", fetchErr.Reason, tt.wantReason)
			}
			// the cause must stay reachable for logging and errors.Is
			if !errors.Is(err, tt.err) {
				t.Error("the original error is not wrapped")
			}
		})
	}
}

func TestFetch_Timeout(t *testing.T) {
	// the fake server never answers, it only gives up when the request is cancelled
	fetcher := New(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	})}, 20*time.Millisecond, 1024)

	_, err := fetcher.Fetch(context.Background(), mustURL(t, "https://example.com"))

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want a deadline error", err)
	}
	fetchErr := fetchError(t, err)
	if fetchErr.Hint != "Timed out" {
		t.Errorf("Hint = %q, want Timed out", fetchErr.Hint)
	}
	if !strings.Contains(fetchErr.Reason, "did not answer within 20ms") {
		t.Errorf("Reason = %q", fetchErr.Reason)
	}
}

// brokenBody fails on the first read, like a connection that drops mid-download.
type brokenBody struct{ err error }

func (b brokenBody) Read([]byte) (int, error) { return 0, b.err }
func (b brokenBody) Close() error             { return nil }

func TestFetch_BodyBreaksOff(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantHint string
	}{
		{"connection lost", errors.New("unexpected EOF"), "Download interrupted"},
		{"ran out of time", context.DeadlineExceeded, "Timed out"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fetcher := newFetcher(func(req *http.Request) (*http.Response, error) {
				resp := response(req, http.StatusOK, "text/html", "")
				resp.Body = brokenBody{err: tt.err}
				return resp, nil
			})

			_, err := fetcher.Fetch(context.Background(), mustURL(t, "https://example.com"))

			if hint := fetchError(t, err).Hint; hint != tt.wantHint {
				t.Errorf("Hint = %q, want %q", hint, tt.wantHint)
			}
		})
	}
}

func TestFetch_BadAddress(t *testing.T) {
	called := false
	fetcher := newFetcher(func(req *http.Request) (*http.Response, error) {
		called = true
		return response(req, http.StatusOK, "text/html", ""), nil
	})

	// a space in the host cannot be turned into a request
	_, err := fetcher.Fetch(context.Background(), &url.URL{Scheme: "http", Host: "exa mple.com"})

	if hint := fetchError(t, err).Hint; hint != "Invalid address" {
		t.Errorf("Hint = %q, want Invalid address", hint)
	}
	if called {
		t.Error("no request should have been sent")
	}
}
