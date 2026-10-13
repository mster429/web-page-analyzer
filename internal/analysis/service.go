package analysis

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
)

// ErrInvalidURL is returned when the submitted address is not an http or https URL.
var ErrInvalidURL = errors.New("invalid URL")

// ErrLinksExpired is returned when the links of a page are no longer in the store.
var ErrLinksExpired = errors.New("links expired")

type Service struct {
	fetcher          PageFetcher
	checker          LinkChecker
	store            LinkStore
	linkCheckTimeout time.Duration
}

func NewService(fetcher PageFetcher, checker LinkChecker, store LinkStore, linkCheckTimeout time.Duration) *Service {
	return &Service{fetcher: fetcher, checker: checker, store: store, linkCheckTimeout: linkCheckTimeout}
}

// Analyze fetches the page at rawURL and reports what its HTML says.
func (s *Service) Analyze(ctx context.Context, rawURL string) (Result, error) {
	pageURL, err := parsePageURL(rawURL)
	if err != nil {
		return Result{}, err
	}

	page, err := s.fetcher.Fetch(ctx, pageURL)
	if err != nil {
		return Result{}, err
	}

	doc, err := ParseHTML(page.Body, page.URL)
	if err != nil {
		return Result{}, fmt.Errorf("parse page: %w", err)
	}

	result := Result{
		URL:          page.URL.String(),
		HTMLVersion:  doc.HTMLVersion,
		Title:        doc.Title,
		Headings:     doc.Headings,
		Links:        countLinks(doc.Links),
		HasLoginForm: doc.HasLoginForm,
	}
	// the links are checked in a second request, keep them until then
	if len(doc.Links) > 0 {
		result.LinkCheckID = s.store.Save(doc.Links)
	}

	return result, nil
}

func (s *Service) CheckLinks(ctx context.Context, id string) (LinkCheck, error) {
	links, ok := s.store.FetchAndDelete(id)
	if !ok {
		return LinkCheck{}, ErrLinksExpired
	}

	// count every link, but check the same address only once
	seen := make(map[string]bool)
	var unique []string
	for _, link := range links {
		if !seen[link.URL] {
			seen[link.URL] = true
			unique = append(unique, link.URL)
		}
	}

	ctx, cancel := context.WithTimeout(ctx, s.linkCheckTimeout)
	defer cancel()
	report := s.checker.Check(ctx, unique)

	// the workers finish in any order, sort so the page shows the same order every time
	slices.Sort(report.Inaccessible)
	slices.Sort(report.TimedOut)

	counts := countLinks(links)
	return LinkCheck{
		Internal:     counts.Internal,
		External:     counts.External,
		Inaccessible: report.Inaccessible,
		TimedOut:     report.TimedOut,
	}, nil
}

func parsePageURL(rawURL string) (*url.URL, error) {
	pageURL, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, ErrInvalidURL
	}
	if pageURL.Scheme != "http" && pageURL.Scheme != "https" {
		return nil, ErrInvalidURL
	}
	if pageURL.Hostname() == "" {
		return nil, ErrInvalidURL
	}
	return pageURL, nil
}

func countLinks(links []Link) LinkCounts {
	var counts LinkCounts
	for _, link := range links {
		if link.IsInternal {
			counts.Internal++
		} else {
			counts.External++
		}
	}
	return counts
}
