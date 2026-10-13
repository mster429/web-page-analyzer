// Package linkstore keeps the links of a page in memory for a short time.
package linkstore

import (
	"crypto/rand"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"

	"github.com/mster429/web-page-analyzer/internal/analysis"
)

// Store is an in-memory store. Entries expire
type Store struct {
	cache *expirable.LRU[string, []analysis.Link]
}

// New returns a Store that holds page links.
func New(size int, ttl time.Duration) *Store {
	return &Store{cache: expirable.NewLRU[string, []analysis.Link](size, nil, ttl)}
}

// Save stores links and returns a random id for them.
func (s *Store) Save(links []analysis.Link) string {
	id := rand.Text()
	s.cache.Add(id, links)
	return id
}

// FetchAndDelete returns the links saved under id and delete them
func (s *Store) FetchAndDelete(id string) ([]analysis.Link, bool) {
	links, ok := s.cache.Get(id)
	if ok {
		s.cache.Remove(id)
	}
	return links, ok
}
