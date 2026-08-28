package oauth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"
)

// PKCEParams holds the PKCE verifier paired with the OAuth state parameter.
type PKCEParams struct {
	CodeVerifier string
	CreatedAt    time.Time
}

// StateStore manages short-lived PKCE state for in-flight OAuth flows.
// In production this could be backed by Redis; for now it uses an in-memory map
// with periodic eviction of entries older than the configured TTL.
type StateStore struct {
	mu      sync.Mutex
	entries map[string]PKCEParams
	ttl     time.Duration
}

func NewStateStore(ttl time.Duration) *StateStore {
	s := &StateStore{
		entries: make(map[string]PKCEParams),
		ttl:     ttl,
	}
	go s.evictLoop()
	return s
}

// GenerateState creates a cryptographically random state string.
func GenerateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate state: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// Put stores a PKCE params entry keyed by the OAuth state.
func (s *StateStore) Put(state string, params PKCEParams) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[state] = params
}

// Pop retrieves and removes the PKCE params for the given state.
// Returns false if the state is not found or expired.
func (s *StateStore) Pop(state string) (PKCEParams, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	params, ok := s.entries[state]
	if !ok {
		return PKCEParams{}, false
	}
	delete(s.entries, state)

	if time.Since(params.CreatedAt) > s.ttl {
		return PKCEParams{}, false
	}
	return params, true
}

func (s *StateStore) evictLoop() {
	ticker := time.NewTicker(s.ttl)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for k, v := range s.entries {
			if now.Sub(v.CreatedAt) > s.ttl {
				delete(s.entries, k)
			}
		}
		s.mu.Unlock()
	}
}
