package linkstore

import (
	"slices"
	"testing"
	"time"

	"github.com/mster429/web-page-analyzer/internal/analysis"
)

var sampleLinks = []analysis.Link{
	{URL: "https://example.com/a", IsInternal: true},
	{URL: "https://other.org/b"},
}

func TestStore_SaveAndFetch(t *testing.T) {
	store := New(10, time.Minute)

	id := store.Save(sampleLinks)
	if id == "" {
		t.Fatal("Save returned an empty id")
	}

	links, ok := store.FetchAndDelete(id)
	if !ok {
		t.Fatal("the links that were just saved were not found")
	}
	if !slices.Equal(links, sampleLinks) {
		t.Errorf("links = %+v, want %+v", links, sampleLinks)
	}
}

func TestStore_IDWorksOnce(t *testing.T) {
	store := New(10, time.Minute)
	id := store.Save(sampleLinks)

	if _, ok := store.FetchAndDelete(id); !ok {
		t.Fatal("first fetch failed")
	}
	if _, ok := store.FetchAndDelete(id); ok {
		t.Error("the same id worked a second time")
	}
}

func TestStore_UnknownID(t *testing.T) {
	store := New(10, time.Minute)
	store.Save(sampleLinks)

	for _, id := range []string{"", "not-an-id"} {
		if _, ok := store.FetchAndDelete(id); ok {
			t.Errorf("FetchAndDelete(%q) found something", id)
		}
	}
}

func TestStore_IDsAreDifferent(t *testing.T) {
	store := New(100, time.Minute)

	seen := make(map[string]bool)
	for range 50 {
		id := store.Save(sampleLinks)
		if seen[id] {
			t.Fatalf("id %q was handed out twice", id)
		}
		seen[id] = true
	}
}

func TestStore_KeepsPagesApart(t *testing.T) {
	store := New(10, time.Minute)
	first := store.Save([]analysis.Link{{URL: "https://example.com/first"}})
	second := store.Save([]analysis.Link{{URL: "https://example.com/second"}})

	links, _ := store.FetchAndDelete(second)
	if len(links) != 1 || links[0].URL != "https://example.com/second" {
		t.Errorf("links = %+v, want the second page's", links)
	}
	links, _ = store.FetchAndDelete(first)
	if len(links) != 1 || links[0].URL != "https://example.com/first" {
		t.Errorf("links = %+v, want the first page's", links)
	}
}

func TestStore_Expires(t *testing.T) {
	store := New(10, 20*time.Millisecond)
	id := store.Save(sampleLinks)

	time.Sleep(60 * time.Millisecond)

	if _, ok := store.FetchAndDelete(id); ok {
		t.Error("the links were still there after they should have expired")
	}
}

func TestStore_DropsOldestWhenFull(t *testing.T) {
	store := New(2, time.Minute)
	oldest := store.Save(sampleLinks)
	middle := store.Save(sampleLinks)
	newest := store.Save(sampleLinks)

	if _, ok := store.FetchAndDelete(oldest); ok {
		t.Error("the oldest entry should have been dropped to make room")
	}
	for _, id := range []string{middle, newest} {
		if _, ok := store.FetchAndDelete(id); !ok {
			t.Errorf("entry %q is missing", id)
		}
	}
}
