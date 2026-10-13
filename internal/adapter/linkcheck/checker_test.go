package linkcheck

import (
	"context"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// roundTripFunc stands in for the network, so no test makes a real request.
type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func response(status int) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader("")),
	}
}

// fakeSite answers depending on the path of the link.
func fakeSite(req *http.Request) (*http.Response, error) {
	switch req.URL.Path {
	case "/404":
		return response(http.StatusNotFound), nil
	case "/500":
		return response(http.StatusInternalServerError), nil
	case "/head-rejected":
		if req.Method == http.MethodHead {
			return response(http.StatusMethodNotAllowed), nil
		}
	case "/refused":
		return nil, errors.New("connection refused")
	case "/slow":
		<-req.Context().Done()
		return nil, req.Context().Err()
	}
	return response(http.StatusOK), nil
}

func TestCheck(t *testing.T) {
	checker := New(&http.Client{Transport: roundTripFunc(fakeSite)}, 3, 30*time.Millisecond)

	links := []string{
		"https://example.com/ok",
		"https://example.com/head-rejected",
		"https://example.com/404",
		"https://example.com/500",
		"https://example.com/refused",
		"https://example.com/slow",
		"://not-a-url",
	}
	report := checker.Check(context.Background(), links)

	if report.Checked != len(links) {
		t.Errorf("Checked = %d, want %d", report.Checked, len(links))
	}

	// the order depends on which worker finishes first
	slices.Sort(report.Inaccessible)
	wantInaccessible := []string{
		"://not-a-url",
		"https://example.com/404",
		"https://example.com/500",
		"https://example.com/refused",
	}
	if !slices.Equal(report.Inaccessible, wantInaccessible) {
		t.Errorf("Inaccessible = %v, want %v", report.Inaccessible, wantInaccessible)
	}

	wantTimedOut := []string{"https://example.com/slow"}
	if !slices.Equal(report.TimedOut, wantTimedOut) {
		t.Errorf("TimedOut = %v, want %v", report.TimedOut, wantTimedOut)
	}
}

func TestCheck_NoLinks(t *testing.T) {
	checker := New(&http.Client{Transport: roundTripFunc(fakeSite)}, 3, time.Second)

	report := checker.Check(context.Background(), nil)

	if report.Checked != 0 || len(report.Inaccessible) != 0 || len(report.TimedOut) != 0 {
		t.Errorf("report = %+v, want it empty", report)
	}
}

func TestCheck_HeadThenGet(t *testing.T) {
	tests := []struct {
		name             string
		headStatus       int
		getStatus        int
		wantMethods      []string
		wantInaccessible bool
	}{
		{"head accepted", http.StatusOK, http.StatusOK, []string{"HEAD"}, false},
		{"head rejected, get works", http.StatusMethodNotAllowed, http.StatusOK, []string{"HEAD", "GET"}, false},
		{"head forbidden, get works", http.StatusForbidden, http.StatusOK, []string{"HEAD", "GET"}, false},
		{"both fail", http.StatusNotFound, http.StatusNotFound, []string{"HEAD", "GET"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var methods []string
			transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
				methods = append(methods, req.Method)
				if req.Method == http.MethodHead {
					return response(tt.headStatus), nil
				}
				return response(tt.getStatus), nil
			})
			// one worker, so the two requests are recorded in order
			checker := New(&http.Client{Transport: transport}, 1, time.Second)

			report := checker.Check(context.Background(), []string{"https://example.com/page"})

			if !slices.Equal(methods, tt.wantMethods) {
				t.Errorf("methods = %v, want %v", methods, tt.wantMethods)
			}
			if got := len(report.Inaccessible) == 1; got != tt.wantInaccessible {
				t.Errorf("Inaccessible = %v, want inaccessible = %v", report.Inaccessible, tt.wantInaccessible)
			}
		})
	}
}

func TestCheck_WorkerLimit(t *testing.T) {
	const workers = 4

	var mu sync.Mutex
	running, maxRunning := 0, 0
	transport := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		mu.Lock()
		running++
		maxRunning = max(maxRunning, running)
		mu.Unlock()

		time.Sleep(5 * time.Millisecond)

		mu.Lock()
		running--
		mu.Unlock()
		return response(http.StatusOK), nil
	})
	checker := New(&http.Client{Transport: transport}, workers, time.Second)

	links := make([]string, 40)
	for i := range links {
		links[i] = "https://example.com/page"
	}
	report := checker.Check(context.Background(), links)

	if report.Checked != len(links) {
		t.Errorf("Checked = %d, want %d", report.Checked, len(links))
	}
	if maxRunning > workers {
		t.Errorf("%d requests ran at the same time, want at most %d", maxRunning, workers)
	}
	if maxRunning < 2 {
		t.Errorf("only %d request ran at a time, the links are not checked concurrently", maxRunning)
	}
}

func TestCheck_ContextEnds(t *testing.T) {
	// the per link timeout is long, so it is the context that ends first
	checker := New(&http.Client{Transport: roundTripFunc(fakeSite)}, 2, 5*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	var checked, inaccessible, timedOut int
	go func() {
		defer close(done)
		report := checker.Check(ctx, []string{
			"https://example.com/slow",
			"https://example.com/slow",
			"https://example.com/slow",
			"https://example.com/slow",
		})
		checked, inaccessible, timedOut = report.Checked, len(report.Inaccessible), len(report.TimedOut)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Check did not return after the context ended")
	}

	// a link that was cut short was not really checked, so it must not be reported as failed
	if checked != 0 || inaccessible != 0 || timedOut != 0 {
		t.Errorf("checked = %d, inaccessible = %d, timed out = %d, want all zero", checked, inaccessible, timedOut)
	}
}
