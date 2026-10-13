package analysis

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"testing"
	"time"
)

type fakeFetcher struct {
	page    Page
	err     error
	fetched *url.URL
}

func (f *fakeFetcher) Fetch(_ context.Context, pageURL *url.URL) (Page, error) {
	f.fetched = pageURL
	return f.page, f.err
}

type fakeChecker struct {
	report  LinkReport
	checked []string
	// set by Check, to see that the service puts a deadline on the check
	hadDeadline bool
}

func (c *fakeChecker) Check(ctx context.Context, links []string) LinkReport {
	c.checked = links
	_, c.hadDeadline = ctx.Deadline()
	return c.report
}

// fakeStore keeps one set of links under the id "id-1".
type fakeStore struct {
	links []Link
	saved bool
}

func (s *fakeStore) Save(links []Link) string {
	s.links = links
	s.saved = true
	return "id-1"
}

func (s *fakeStore) FetchAndDelete(id string) ([]Link, bool) {
	if id != "id-1" || !s.saved {
		return nil, false
	}
	s.saved = false
	return s.links, true
}

const samplePage = `<!DOCTYPE html>
<html><head><title>Sample</title></head><body>
<h1>One</h1><h2>Two</h2>
<form><input type="password"></form>
<a href="/a">a</a>
<a href="/a">a again</a>
<a href="/b">b</a>
<a href="https://other.org/x">x</a>
</body></html>`

func TestAnalyze(t *testing.T) {
	fetcher := &fakeFetcher{page: Page{
		URL:        mustURL(t, "https://example.com/final"),
		StatusCode: 200,
		Body:       []byte(samplePage),
	}}
	store := &fakeStore{}
	service := NewService(fetcher, &fakeChecker{}, store, time.Second)

	result, err := service.Analyze(context.Background(), "  https://example.com/start  ")
	if err != nil {
		t.Fatalf("Analyze returned an error: %v", err)
	}

	if got := fetcher.fetched.String(); got != "https://example.com/start" {
		t.Errorf("fetched %q, want the trimmed address", got)
	}
	// the result carries the address after redirects, not the one typed in
	if result.URL != "https://example.com/final" {
		t.Errorf("URL = %q, want https://example.com/final", result.URL)
	}
	if result.HTMLVersion != "HTML5" || result.Title != "Sample" || !result.HasLoginForm {
		t.Errorf("unexpected result: %+v", result)
	}
	if result.Headings[0].Count != 1 || result.Headings[1].Count != 1 {
		t.Errorf("Headings = %+v, want one h1 and one h2", result.Headings)
	}
	if result.Links != (LinkCounts{Internal: 3, External: 1}) {
		t.Errorf("Links = %+v, want 3 internal and 1 external", result.Links)
	}
	if result.LinkCheckID != "id-1" {
		t.Errorf("LinkCheckID = %q, want id-1", result.LinkCheckID)
	}
	if len(store.links) != 4 {
		t.Errorf("%d links were saved, want all 4", len(store.links))
	}
}

func TestAnalyze_PageWithoutLinks(t *testing.T) {
	fetcher := &fakeFetcher{page: Page{
		URL:  mustURL(t, "https://example.com"),
		Body: []byte(`<!DOCTYPE html><title>Empty</title>`),
	}}
	store := &fakeStore{}
	service := NewService(fetcher, &fakeChecker{}, store, time.Second)

	result, err := service.Analyze(context.Background(), "https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	if result.LinkCheckID != "" {
		t.Errorf("LinkCheckID = %q, want it empty when there is nothing to check", result.LinkCheckID)
	}
	if store.saved {
		t.Error("nothing should be saved for a page without links")
	}
}

func TestAnalyze_InvalidURL(t *testing.T) {
	inputs := []string{
		"",
		"   ",
		"example.com",
		"ftp://example.com",
		"mailto:me@example.com",
		"javascript:alert(1)",
		"http://",
		"https:///path",
		"http://exa mple.com",
	}

	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			fetcher := &fakeFetcher{}
			service := NewService(fetcher, &fakeChecker{}, &fakeStore{}, time.Second)

			_, err := service.Analyze(context.Background(), input)
			if !errors.Is(err, ErrInvalidURL) {
				t.Errorf("error = %v, want ErrInvalidURL", err)
			}
			if fetcher.fetched != nil {
				t.Error("an invalid address must not be fetched")
			}
		})
	}
}

func TestAnalyze_FetchFails(t *testing.T) {
	fetchErr := &FetchError{StatusCode: 404, Reason: "not found"}
	service := NewService(&fakeFetcher{err: fetchErr}, &fakeChecker{}, &fakeStore{}, time.Second)

	_, err := service.Analyze(context.Background(), "https://example.com")

	var got *FetchError
	if !errors.As(err, &got) || got.StatusCode != 404 {
		t.Errorf("error = %v, want the fetcher's error passed on", err)
	}
}

func TestCheckLinks(t *testing.T) {
	store := &fakeStore{}
	store.Save([]Link{
		{URL: "https://example.com/a", IsInternal: true},
		{URL: "https://example.com/a", IsInternal: true},
		{URL: "https://example.com/b", IsInternal: true},
		{URL: "https://other.org/z"},
		{URL: "https://other.org/y"},
		{URL: "https://slow.org/"},
	})
	checker := &fakeChecker{report: LinkReport{
		Checked:      4,
		Inaccessible: []string{"https://other.org/z", "https://example.com/b"},
		TimedOut:     []string{"https://slow.org/"},
	}}
	service := NewService(&fakeFetcher{}, checker, store, time.Second)

	check, err := service.CheckLinks(context.Background(), "id-1")
	if err != nil {
		t.Fatalf("CheckLinks returned an error: %v", err)
	}

	// each address is checked once, however often it is on the page
	wantChecked := []string{"https://example.com/a", "https://example.com/b", "https://other.org/z", "https://other.org/y", "https://slow.org/"}
	if !slices.Equal(checker.checked, wantChecked) {
		t.Errorf("checked %v, want %v", checker.checked, wantChecked)
	}
	if !checker.hadDeadline {
		t.Error("the link check was started without a deadline")
	}

	if check.Internal != 3 || check.External != 3 {
		t.Errorf("Internal = %d, External = %d, want 3 and 3", check.Internal, check.External)
	}
	// sorted, so the page shows the same order every time
	wantInaccessible := []string{"https://example.com/b", "https://other.org/z"}
	if !slices.Equal(check.Inaccessible, wantInaccessible) {
		t.Errorf("Inaccessible = %v, want %v", check.Inaccessible, wantInaccessible)
	}
	if !slices.Equal(check.TimedOut, []string{"https://slow.org/"}) {
		t.Errorf("TimedOut = %v", check.TimedOut)
	}
	// 5 unique links were sent, 4 came back
	if check.NotChecked != 1 {
		t.Errorf("NotChecked = %d, want 1", check.NotChecked)
	}
}

func TestCheckLinks_UnknownID(t *testing.T) {
	checker := &fakeChecker{}
	service := NewService(&fakeFetcher{}, checker, &fakeStore{}, time.Second)

	_, err := service.CheckLinks(context.Background(), "nope")
	if !errors.Is(err, ErrLinksExpired) {
		t.Errorf("error = %v, want ErrLinksExpired", err)
	}
	if checker.checked != nil {
		t.Error("nothing should be checked for an unknown id")
	}
}

func TestCheckLinks_IDWorksOnce(t *testing.T) {
	store := &fakeStore{}
	store.Save([]Link{{URL: "https://example.com/a", IsInternal: true}})
	service := NewService(&fakeFetcher{}, &fakeChecker{report: LinkReport{Checked: 1}}, store, time.Second)

	if _, err := service.CheckLinks(context.Background(), "id-1"); err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if _, err := service.CheckLinks(context.Background(), "id-1"); !errors.Is(err, ErrLinksExpired) {
		t.Errorf("second call: error = %v, want ErrLinksExpired", err)
	}
}

func TestFetchError(t *testing.T) {
	cause := errors.New("connection refused")

	// without a cause, the error is the reason the user sees
	withStatus := &FetchError{StatusCode: 503, Reason: "the page answered with 503"}
	if got := withStatus.Error(); got != "fetch page: the page answered with 503" {
		t.Errorf("Error() = %q", got)
	}
	if withStatus.Unwrap() != nil {
		t.Error("Unwrap() should be nil when there is no cause")
	}

	// with a cause, both are there: what the user was told and what really happened
	withCause := &FetchError{Reason: "could not be reached", Err: cause}
	if got := withCause.Error(); got != "fetch page: could not be reached (connection refused)" {
		t.Errorf("Error() = %q", got)
	}
	// errors.Is has to see through the FetchError to the cause
	if !errors.Is(withCause, cause) {
		t.Error("errors.Is did not find the cause")
	}
}
