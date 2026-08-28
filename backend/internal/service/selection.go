package service

import (
	"strings"
	"sync"

	"github.com/javdet/nib/internal/domain"
)

const selectionAny = "any"

// SelectionStore holds the global UI selection in memory.
type SelectionStore struct {
	mu sync.RWMutex
	s  domain.Selection
}

// NewSelectionStore returns a store initialized to all "any".
func NewSelectionStore() *SelectionStore {
	return &SelectionStore{
		s: domain.Selection{
			Project:     selectionAny,
			Environment: selectionAny,
			Cloud:       selectionAny,
			Location:    selectionAny,
		},
	}
}

// Get returns a copy of the current selection.
func (st *SelectionStore) Get() domain.Selection {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.s
}

// Set stores a new selection after normalizing empty fields and enforcing cascade rules.
func (st *SelectionStore) Set(sel domain.Selection) domain.Selection {
	normalized := normalizeSelection(sel)
	st.mu.Lock()
	st.s = normalized
	st.mu.Unlock()
	return normalized
}

func normalizeSelection(sel domain.Selection) domain.Selection {
	out := domain.Selection{
		Project:     normalizeSelectionField(sel.Project),
		Environment: normalizeSelectionField(sel.Environment),
		Cloud:       normalizeSelectionField(sel.Cloud),
		Location:    normalizeSelectionField(sel.Location),
	}

	if out.Project == selectionAny {
		out.Environment = selectionAny
		out.Cloud = selectionAny
		out.Location = selectionAny
		return out
	}

	if out.Cloud == selectionAny {
		out.Location = selectionAny
	}

	return out
}

func normalizeSelectionField(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return selectionAny
	}
	return v
}
