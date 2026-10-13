package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func get(t *testing.T, a *App, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	a.server.Handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil))
	return rec
}

func TestNew_Routes(t *testing.T) {
	t.Setenv("ENABLE_PPROF", "true")
	t.Setenv("LOG_LEVEL", "disabled")

	application, err := New()
	if err != nil {
		t.Fatalf("New returned an error: %v", err)
	}

	// none of these makes the application fetch anything from the network
	tests := []struct {
		target     string
		wantStatus int
		wantBody   string
	}{
		{"/", http.StatusOK, "<form"},
		{"/static/style.css", http.StatusOK, ".card"},
		{"/health", http.StatusOK, ""},
		{"/metrics", http.StatusOK, "go_goroutines"},
		{"/debug/pprof/", http.StatusOK, "goroutine"},
		{"/analyze?url=not-a-url", http.StatusBadRequest, "400 Bad Request"},
		{"/analyze/links?id=unknown", http.StatusNotFound, "expired"},
		{"/nope", http.StatusNotFound, ""},
	}

	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			rec := get(t, application, tt.target)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("the response does not contain %q", tt.wantBody)
			}
		})
	}
}

func TestNew_RequestsShowUpInMetrics(t *testing.T) {
	t.Setenv("LOG_LEVEL", "disabled")

	application, err := New()
	if err != nil {
		t.Fatal(err)
	}

	get(t, application, "/analyze?url=not-a-url")

	// the pages and /metrics have to share one registry
	metrics := get(t, application, "/metrics").Body.String()
	for _, want := range []string{
		`analyses_total{outcome="invalid_url"} 1`,
		`http_requests_total{method="GET",route="GET /analyze",status="400"} 1`,
	} {
		if !strings.Contains(metrics, want) {
			t.Errorf("/metrics does not contain %s", want)
		}
	}
}

func TestNew_ProfilingOff(t *testing.T) {
	t.Setenv("ENABLE_PPROF", "false")
	t.Setenv("LOG_LEVEL", "disabled")

	application, err := New()
	if err != nil {
		t.Fatal(err)
	}

	if rec := get(t, application, "/debug/pprof/"); rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 when profiling is off", rec.Code)
	}
}

func TestNew_InvalidConfig(t *testing.T) {
	t.Setenv("LINK_CHECK_WORKERS", "many")

	_, err := New()
	if err == nil {
		t.Fatal("New accepted an invalid setting")
	}
	if !strings.Contains(err.Error(), "LINK_CHECK_WORKERS") {
		t.Errorf("error = %q, want it to name the setting", err)
	}
}

func TestRun_StopsWhenContextIsCancelled(t *testing.T) {
	// port 0 lets the system pick a free one, so the test cannot clash with anything
	t.Setenv("HOST_PORT", "127.0.0.1:0")
	t.Setenv("LOG_LEVEL", "disabled")

	application, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- application.Run(ctx) }()

	// give it a moment to start listening, then ask it to stop
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned %v, want nil after a clean shutdown", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after the context was cancelled")
	}
}

func TestRun_CannotListen(t *testing.T) {
	t.Setenv("HOST_PORT", "not-an-address")
	t.Setenv("LOG_LEVEL", "disabled")

	application, err := New()
	if err != nil {
		t.Fatal(err)
	}

	err = application.Run(context.Background())
	if err == nil {
		t.Fatal("Run started on an address that cannot be listened on")
	}
	if !strings.Contains(err.Error(), "listen on not-an-address") {
		t.Errorf("error = %q", err)
	}
}
