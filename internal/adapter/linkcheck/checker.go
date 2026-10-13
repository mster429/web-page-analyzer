// Package linkcheck finds out which links can/cannot be reached.
package linkcheck

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/mster429/web-page-analyzer/internal/analysis"
)

type outcome int

const (
	accessible outcome = iota
	inaccessible
	timedOut
)

type result struct {
	link    string
	outcome outcome
}

// Checker checks links concurrently with a pool of workers.
type Checker struct {
	client  *http.Client
	workers int
	timeout time.Duration
}

// New returns a Checker with the given number of workers and a timeout per link.
func New(client *http.Client, workers int, timeout time.Duration) *Checker {
	return &Checker{client: client, workers: workers, timeout: timeout}
}

// Check requests every link and reports the ones that failed.
// When ctx ends, links not yet checked are left out of the report.
func (c *Checker) Check(ctx context.Context, links []string) analysis.LinkReport {
	jobs := make(chan string)
	results := make(chan result)

	// workers read from jobs until it is closed
	var wg sync.WaitGroup
	for range c.workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for link := range jobs {
				outcome := c.check(ctx, link)
				// cut short by ctx, tells us nothing about the link
				if ctx.Err() != nil {
					continue
				}
				results <- result{link: link, outcome: outcome}
			}
		}()
	}

	// feed the links; closing jobs stops the workers
	go func() {
		defer close(jobs)
		for _, link := range links {
			select {
			case jobs <- link:
			case <-ctx.Done():
				return
			}
		}
	}()

	// close results when all workers are done
	go func() {
		wg.Wait()
		close(results)
	}()

	var report analysis.LinkReport
	for r := range results {
		report.Checked++
		switch r.outcome {
		case inaccessible:
			report.Inaccessible = append(report.Inaccessible, r.link)
		case timedOut:
			report.TimedOut = append(report.TimedOut, r.link)
		}
	}

	return report
}

func (c *Checker) check(ctx context.Context, link string) outcome {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// HEAD first, so no body is downloaded
	status, err := c.status(ctx, http.MethodHead, link)
	if err == nil && status >= http.StatusBadRequest {
		// some servers reject HEAD but answer GET
		status, err = c.status(ctx, http.MethodGet, link)
	}
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return timedOut
		}
		return inaccessible
	}

	if status >= http.StatusBadRequest {
		return inaccessible
	}
	return accessible
}

func (c *Checker) status(ctx context.Context, method, link string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, method, link, nil)
	if err != nil {
		return 0, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return 0, err
	}
	_ = resp.Body.Close()

	return resp.StatusCode, nil
}
