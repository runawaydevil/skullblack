package content

import (
	"context"
	"errors"
	"html/template"
	"log"
	"sync"
	"time"

	"murad.world/murad-world/internal/baserow"
)

var ErrUnavailable = errors.New("content temporarily unavailable")

const refreshInterval = 5 * time.Minute
const refreshTimeout = 10 * time.Second

// Store keeps an in-memory snapshot of entries from Baserow.
type Store struct {
	mu sync.RWMutex

	client *baserow.Client

	ready   bool
	entries []PreparedEntry
}

type PreparedEntry struct {
	Entry baserow.Entry

	RenderedHTML template.HTML
	PlainText    string
	PreviewText  string

	HasMarkdownFile  bool
	MarkdownFileName string
}

// NewStore creates a store backed by the given Baserow client.
func NewStore(client *baserow.Client) *Store {
	return &Store{client: client}
}

// Refresh loads entries from Baserow and updates the cache on success.
func (s *Store) Refresh(ctx context.Context) error {
	fetchCtx, cancel := context.WithTimeout(ctx, refreshTimeout)
	defer cancel()

	raw, err := s.client.FetchPublicPublished(fetchCtx)
	if err != nil {
		return err
	}

	prepared := make([]PreparedEntry, 0, len(raw))
	for _, e := range raw {
		p, err := prepareEntry(ctx, e)
		if err != nil {
			log.Printf("content: prepare entry failed slug=%q: %v", e.Slug, err)
			// Still include a safe fallback entry.
			prepared = append(prepared, fallbackPreparedEntry(e))
			continue
		}
		prepared = append(prepared, p)
	}

	s.mu.Lock()
	s.entries = prepared
	s.ready = true
	s.mu.Unlock()
	return nil
}

// Start loads once, then refreshes on an interval. The initial error is returned
// if the first refresh fails (cache remains empty).
func (s *Store) Start(ctx context.Context) error {
	if err := s.Refresh(ctx); err != nil {
		log.Printf("content: initial baserow refresh failed: %v", err)
		return err
	}
	s.mu.RLock()
	n := len(s.entries)
	s.mu.RUnlock()
	log.Printf("content: initial baserow refresh ok (%d entries)", n)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("content: refresh loop panic recovered: %v", r)
			}
		}()

		t := time.NewTicker(refreshInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("content: scheduled refresh panic recovered: %v", r)
						}
					}()

					err := s.Refresh(ctx)

					if err != nil {
						log.Printf("content: scheduled refresh failed (serving last good cache): %v", err)
						return
					}
					s.mu.RLock()
					n := len(s.entries)
					s.mu.RUnlock()
					log.Printf("content: scheduled refresh ok (%d entries)", n)
				}()
			}
		}
	}()
	return nil
}

// All returns a copy of prepared entries if the store has ever loaded successfully.
func (s *Store) All() ([]PreparedEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.ready {
		return nil, ErrUnavailable
	}
	out := make([]PreparedEntry, len(s.entries))
	copy(out, s.entries)
	return out, nil
}

// BySlug returns a prepared entry by slug or nil if not found / not ready.
func (s *Store) BySlug(slug string) (*PreparedEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.ready {
		return nil, ErrUnavailable
	}
	for i := range s.entries {
		if s.entries[i].Entry.Slug == slug {
			e := s.entries[i]
			return &e, nil
		}
	}
	return nil, nil
}
