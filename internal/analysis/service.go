package analysis

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// ErrInvalidURL is returned when the submitted address is not an http or https URL.
var ErrInvalidURL = errors.New("invalid URL")

type Service struct {
	fetcher PageFetcher
}

func NewService(fetcher PageFetcher) *Service {
	return &Service{fetcher: fetcher}
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

	return Result{
		URL:         page.URL.String(),
		HTMLVersion: doc.HTMLVersion,
		Title:       doc.Title,
		Headings:    doc.Headings,
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
