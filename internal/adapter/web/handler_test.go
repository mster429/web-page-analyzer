package web

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/rs/zerolog"

	"github.com/mster429/web-page-analyzer/internal/analysis"
)

// fakeAnalyzer returns whatever the test puts in it and remembers what it was asked.
type fakeAnalyzer struct {
	result analysis.Result
	err    error
	gotURL string

	check    analysis.LinkCheck
	checkErr error
	gotID    string
}

func (f *fakeAnalyzer) Analyze(_ context.Context, rawURL string) (analysis.Result, error) {
	f.gotURL = rawURL
	return f.result, f.err
}

func (f *fakeAnalyzer) CheckLinks(_ context.Context, id string) (analysis.LinkCheck, error) {
	f.gotID = id
	return f.check, f.checkErr
}

type panicAnalyzer struct{}

func (panicAnalyzer) Analyze(context.Context, string) (analysis.Result, error) {
	panic("boom")
}

func (panicAnalyzer) CheckLinks(context.Context, string) (analysis.LinkCheck, error) {
	return analysis.LinkCheck{}, nil
}

// site is the handler wired up the way the application does it.
type site struct {
	handler http.Handler
	pages   *Handler
	logs    *bytes.Buffer
}

func newSite(analyzer Analyzer) site {
	logs := &bytes.Buffer{}
	pages := New(analyzer, zerolog.New(logs), prometheus.NewRegistry())

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", pages.Index)
	mux.HandleFunc("GET /analyze", pages.Analyze)
	mux.HandleFunc("GET /analyze/links", pages.Links)
	mux.Handle("GET /static/", pages.Static())

	return site{handler: pages.Middleware(mux), pages: pages, logs: logs}
}

func (s site) get(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil))
	return rec
}

func wantContains(t *testing.T, body string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not contain %q", want)
		}
	}
}

var sampleResult = analysis.Result{
	URL:         "https://example.com/final",
	HTMLVersion: "HTML5",
	Title:       "Sample page",
	Headings: []analysis.HeadingCount{
		{Level: 1, Count: 1}, {Level: 2, Count: 4}, {Level: 3, Count: 0},
		{Level: 4, Count: 0}, {Level: 5, Count: 0}, {Level: 6, Count: 0},
	},
	Links:        analysis.LinkCounts{Internal: 7, External: 3},
	HasLoginForm: true,
	LinkCheckID:  "ABC123",
}

func TestIndex(t *testing.T) {
	rec := newSite(&fakeAnalyzer{}).get(t, "/")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	body := rec.Body.String()
	wantContains(t, body, `<form action="/analyze" method="get">`, `name="url"`, "Analyze</button>")
	if strings.Contains(body, `id="links"`) || strings.Contains(body, "card error") {
		t.Error("the empty form shows a result or an error")
	}
}

func TestAnalyze_ShowsResult(t *testing.T) {
	analyzer := &fakeAnalyzer{result: sampleResult}
	rec := newSite(analyzer).get(t, "/analyze?url=https://example.com/start")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if analyzer.gotURL != "https://example.com/start" {
		t.Errorf("the analyzer was given %q", analyzer.gotURL)
	}

	body := rec.Body.String()
	wantContains(t, body,
		`value="https://example.com/start"`, // the form keeps what was typed
		"https://example.com/final",
		"HTML5",
		"Sample page",
		"<dd>Yes</dd>",
		`<span class="count">4</span>`, // the h2 count
		`<span class="count">7</span>`, // internal links
		`<span class="count">3</span>`, // external links
	)
	// the link check has not run yet: placeholders, and the script that asks for it
	wantContains(t, body, `class="pending"`, "Checking the links", `encodeURIComponent("ABC123")`)
}

func TestAnalyze_PageWithoutLinks(t *testing.T) {
	result := sampleResult
	result.Links = analysis.LinkCounts{}
	result.LinkCheckID = ""
	result.HasLoginForm = false
	result.Title = ""

	body := newSite(&fakeAnalyzer{result: result}).get(t, "/analyze?url=https://example.com").Body.String()

	// nothing to wait for, so no script and no placeholders
	if strings.Contains(body, "<script>") || strings.Contains(body, `class="pending"`) {
		t.Error("a page without links should not ask for a link check")
	}
	wantContains(t, body, "<dd>No</dd>", `<span class="muted">none</span>`)
}

func TestAnalyze_EscapesPageContent(t *testing.T) {
	result := sampleResult
	result.Title = `<script>alert("x")</script>`

	body := newSite(&fakeAnalyzer{result: result}).get(t, "/analyze?url=https://example.com").Body.String()

	if strings.Contains(body, `<script>alert("x")</script>`) {
		t.Error("the title was written to the page without escaping")
	}
	wantContains(t, body, "&lt;script&gt;")
}

func TestAnalyze_Errors(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantStatus  int
		wantHeading string
		wantText    string
		wantOutcome string
	}{
		{
			name:        "invalid address",
			err:         analysis.ErrInvalidURL,
			wantStatus:  http.StatusBadRequest,
			wantHeading: "400 Bad Request",
			wantText:    "starts with http:// or https://",
			wantOutcome: "invalid_url",
		},
		{
			name:        "page answers with an error",
			err:         &analysis.FetchError{StatusCode: 404, Reason: "The page answered with an error."},
			wantStatus:  http.StatusNotFound,
			wantHeading: "404 Not Found",
			wantText:    "The page answered with an error.",
			wantOutcome: "fetch_failed",
		},
		{
			name:        "page cannot be reached",
			err:         &analysis.FetchError{Reason: "The host name could not be found.", Err: errors.New("no such host")},
			wantStatus:  http.StatusBadGateway,
			wantHeading: "502 Bad Gateway",
			wantText:    "The host name could not be found.",
			wantOutcome: "fetch_failed",
		},
		{
			name:        "page does not answer in time",
			err:         &analysis.FetchError{Reason: "The page did not answer within 10s.", Err: context.DeadlineExceeded},
			wantStatus:  http.StatusGatewayTimeout,
			wantHeading: "504 Gateway Timeout",
			wantText:    "did not answer within 10s",
			wantOutcome: "fetch_failed",
		},
		{
			name:        "not a web page",
			err:         &analysis.FetchError{Reason: "The address returned application/pdf, not a web page.", Err: analysis.ErrNotHTML},
			wantStatus:  http.StatusUnsupportedMediaType,
			wantHeading: "415 Unsupported Media Type",
			wantText:    "application/pdf",
			wantOutcome: "fetch_failed",
		},
		{
			name:        "wrapped fetch error",
			err:         fmt.Errorf("analyze: %w", &analysis.FetchError{StatusCode: 503, Reason: "Unavailable."}),
			wantStatus:  http.StatusServiceUnavailable,
			wantHeading: "503 Service Unavailable",
			wantText:    "Unavailable.",
			wantOutcome: "fetch_failed",
		},
		{
			name:        "status code without a standard name",
			err:         &analysis.FetchError{StatusCode: 999, Reason: "Blocked."},
			wantStatus:  999,
			wantHeading: "<h2>999</h2>",
			wantText:    "Blocked.",
			wantOutcome: "fetch_failed",
		},
		{
			name:        "something unexpected",
			err:         errors.New("out of cheese"),
			wantStatus:  http.StatusInternalServerError,
			wantHeading: "500 Internal Server Error",
			wantText:    "Something went wrong",
			wantOutcome: "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSite(&fakeAnalyzer{err: tt.err})
			rec := s.get(t, "/analyze?url=https://example.com/x")

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			body := rec.Body.String()
			wantContains(t, body, "card error", tt.wantHeading, tt.wantText, `value="https://example.com/x"`)
			if strings.Contains(body, "out of cheese") {
				t.Error("an internal error message was shown to the user")
			}
			if got := testutil.ToFloat64(s.pages.metrics.analyses.WithLabelValues(tt.wantOutcome)); got != 1 {
				t.Errorf("analyses_total{outcome=%q} = %v, want 1", tt.wantOutcome, got)
			}
		})
	}
}

func TestAnalyze_LogsWhyItFailed(t *testing.T) {
	s := newSite(&fakeAnalyzer{err: &analysis.FetchError{Reason: "Not reachable.", Err: errors.New("connection reset by peer")}})
	s.get(t, "/analyze?url=https://example.com/x")

	logs := s.logs.String()
	// the user sees the short reason, the log keeps the real cause and the address
	for _, want := range []string{`"level":"warn"`, "connection reset by peer", "https://example.com/x"} {
		if !strings.Contains(logs, want) {
			t.Errorf("the log does not contain %q:\n%s", want, logs)
		}
	}
}

func TestLinks(t *testing.T) {
	analyzer := &fakeAnalyzer{check: analysis.LinkCheck{
		Internal:     7,
		External:     3,
		Inaccessible: []string{"https://example.com/gone", "https://other.org/broken"},
		TimedOut:     []string{"https://slow.org/"},
		NotChecked:   2,
	}}
	s := newSite(analyzer)
	rec := s.get(t, "/analyze/links?id=ABC123")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if analyzer.gotID != "ABC123" {
		t.Errorf("the analyzer was given id %q", analyzer.gotID)
	}

	body := rec.Body.String()
	// only the link section comes back, not a whole page
	if strings.Contains(body, "<html") || !strings.Contains(body, `<section class="card" id="links">`) {
		t.Errorf("expected just the link section, got:\n%s", body)
	}
	wantContains(t, body,
		`<span class="count">7</span>`,
		`<span class="count">3</span>`,
		`<span class="count">2</span>`, // two inaccessible
		`<span class="count">1</span>`, // one timed out
		"Inaccessible links (2)",
		"https://example.com/gone",
		"https://other.org/broken",
		"Timed out links (1)",
		"https://slow.org/",
		"2 links were not checked",
	)
	if strings.Contains(body, "<script>") || strings.Contains(body, `class="pending"`) {
		t.Error("the finished link section still looks pending")
	}
	if got := testutil.ToFloat64(s.pages.metrics.linkChecks.WithLabelValues("ok")); got != 1 {
		t.Errorf("link_checks_total{outcome=ok} = %v, want 1", got)
	}
}

func TestLinks_NothingFailed(t *testing.T) {
	body := newSite(&fakeAnalyzer{check: analysis.LinkCheck{Internal: 2, External: 1, NotChecked: 1}}).
		get(t, "/analyze/links?id=ABC123").Body.String()

	if strings.Contains(body, "<details>") {
		t.Error("there are no failed links, so no list should be shown")
	}
	wantContains(t, body, "1 link was not checked")
}

func TestLinks_Errors(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantStatus  int
		wantText    string
		wantOutcome string
	}{
		{"expired id", analysis.ErrLinksExpired, http.StatusNotFound, "This result has expired", "expired"},
		{"something unexpected", errors.New("out of cheese"), http.StatusInternalServerError, "could not be checked", "error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSite(&fakeAnalyzer{checkErr: tt.err})
			rec := s.get(t, "/analyze/links?id=old")

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			body := rec.Body.String()
			// still the link section, so the script can put it in place as usual
			wantContains(t, body, `id="links"`, tt.wantText)
			if strings.Contains(body, "out of cheese") {
				t.Error("an internal error message was shown to the user")
			}
			if got := testutil.ToFloat64(s.pages.metrics.linkChecks.WithLabelValues(tt.wantOutcome)); got != 1 {
				t.Errorf("link_checks_total{outcome=%q} = %v, want 1", tt.wantOutcome, got)
			}
		})
	}
}

func TestStatic(t *testing.T) {
	rec := newSite(&fakeAnalyzer{}).get(t, "/static/style.css")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/css") {
		t.Errorf("Content-Type = %q, want text/css", got)
	}
	wantContains(t, rec.Body.String(), ".card")
}

func TestUnknownPath(t *testing.T) {
	s := newSite(&fakeAnalyzer{})

	if rec := s.get(t, "/nope"); rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	// "GET /{$}" must only match the root, not every path
	if rec := s.get(t, "/analyze/extra"); rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestMiddleware_CountsAndLogsRequests(t *testing.T) {
	s := newSite(&fakeAnalyzer{err: analysis.ErrInvalidURL})

	s.get(t, "/")
	s.get(t, "/")
	s.get(t, "/analyze?url=abc")
	s.get(t, "/nope")

	requests := s.pages.metrics.requests
	counts := []struct {
		route, status string
		want          float64
	}{
		{"GET /{$}", "200", 2},
		{"GET /analyze", "400", 1},
		// unknown paths share one label, so they cannot create endless label values
		{"unmatched", "404", 1},
	}
	for _, c := range counts {
		if got := testutil.ToFloat64(requests.WithLabelValues(c.route, "GET", c.status)); got != c.want {
			t.Errorf("http_requests_total{route=%q,status=%q} = %v, want %v", c.route, c.status, got, c.want)
		}
	}
	if got := testutil.CollectAndCount(s.pages.metrics.duration); got != 3 {
		t.Errorf("the duration histogram has %d routes, want 3", got)
	}

	logs := s.logs.String()
	for _, want := range []string{`"message":"request"`, `"path":"/analyze"`, `"status":400`, `"method":"GET"`} {
		if !strings.Contains(logs, want) {
			t.Errorf("the request log does not contain %s", want)
		}
	}
}

func TestMiddleware_RecoversFromPanic(t *testing.T) {
	s := newSite(panicAnalyzer{})

	rec := s.get(t, "/analyze?url=https://example.com")

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if got := testutil.ToFloat64(s.pages.metrics.requests.WithLabelValues("GET /analyze", "GET", "500")); got != 1 {
		t.Errorf("the panicking request was counted %v times with status 500, want 1", got)
	}
	logs := s.logs.String()
	if !strings.Contains(logs, "handler panicked") || !strings.Contains(logs, "boom") {
		t.Errorf("the panic was not logged:\n%s", logs)
	}
}

func TestNew_SameRegistryTwice(t *testing.T) {
	registry := prometheus.NewRegistry()
	New(&fakeAnalyzer{}, zerolog.Nop(), registry)

	defer func() {
		if recover() == nil {
			t.Error("registering the same metrics twice should panic, like any setup mistake")
		}
	}()
	New(&fakeAnalyzer{}, zerolog.Nop(), registry)
}
