package includedtools

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/javdet/nib/internal/atomicfile"
	"github.com/javdet/nib/internal/mode"
)

const knownFileName = "mcp-known.json"

// SyncResult reports what SyncCatalog changed across every mode.
type SyncResult struct {
	Added   []string
	Removed []string
}

// Changed reports whether the sync touched any mode list.
func (r SyncResult) Changed() bool {
	return len(r.Added) > 0 || len(r.Removed) > 0
}

// SyncCatalog reconciles the per-mode included lists with the tool catalog: a
// tool seen for the first time is switched on for every mode, and one that has
// left the catalog is dropped from every mode. So a newly added MCP server is
// usable straight away, and deleting one leaves nothing behind in the lists.
//
// mcp-known.json is the ledger of names already reconciled. Without it every
// reindex would look like a first run and undo an operator's exclusion, since
// there is no other way to tell a tool that was never offered from one that was
// offered and taken out in the UI.
func (s *Service) SyncCatalog(catalogTools []string) (SyncResult, error) {
	tools := normalizeTools(catalogTools)

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureInitialized(); err != nil {
		return SyncResult{}, err
	}

	known, knownExists, err := s.readKnownLocked()
	if err != nil {
		return SyncResult{}, err
	}

	current := make(map[string]struct{}, len(tools))
	for _, name := range tools {
		current[name] = struct{}{}
	}

	var res SyncResult
	for _, name := range tools {
		if _, ok := known[name]; !ok {
			res.Added = append(res.Added, name)
		}
	}
	for name := range known {
		if _, ok := current[name]; !ok {
			res.Removed = append(res.Removed, name)
		}
	}
	sort.Strings(res.Removed)

	// The ledger is still written on a no-op first run, so an install that
	// starts with an empty catalog does not treat every later tool as new twice.
	if !res.Changed() && knownExists {
		return res, nil
	}

	doc, err := s.readDocumentLocked()
	if err != nil {
		return SyncResult{}, err
	}
	if doc == nil {
		doc = document{}
	}
	for _, modeName := range mode.Modes {
		doc[modeName] = applyDelta(doc[modeName], res)
	}
	if err := s.writeDocumentLocked(doc); err != nil {
		return SyncResult{}, err
	}
	if err := s.writeKnownLocked(tools); err != nil {
		return SyncResult{}, err
	}
	return res, nil
}

// KnownPath returns the resolved mcp-known.json file path.
func (s *Service) KnownPath() string {
	return filepath.Join(filepath.Dir(s.filePath), knownFileName)
}

func applyDelta(tools []string, res SyncResult) []string {
	set := make(map[string]struct{}, len(tools)+len(res.Added))
	for _, name := range tools {
		set[name] = struct{}{}
	}
	for _, name := range res.Added {
		set[name] = struct{}{}
	}
	for _, name := range res.Removed {
		delete(set, name)
	}

	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

type knownDocument struct {
	Tools []string `json:"tools"`
}

// readKnownLocked loads the ledger. A missing file is the first-run signal; a
// corrupt one is an error, because reading it as empty would silently re-enable
// every tool an operator has excluded.
func (s *Service) readKnownLocked() (map[string]struct{}, bool, error) {
	data, err := os.ReadFile(s.KnownPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]struct{}{}, false, nil
		}
		return nil, false, fmt.Errorf("read mcp known file: %w", err)
	}

	var doc knownDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, false, fmt.Errorf("parse mcp known file: %w", err)
	}

	out := make(map[string]struct{}, len(doc.Tools))
	for _, name := range normalizeTools(doc.Tools) {
		out[name] = struct{}{}
	}
	return out, true, nil
}

func (s *Service) writeKnownLocked(tools []string) error {
	if tools == nil {
		tools = []string{}
	}
	data, err := json.MarshalIndent(knownDocument{Tools: tools}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal mcp known file: %w", err)
	}
	if err := atomicfile.WriteString(s.KnownPath(), string(data)+"\n"); err != nil {
		return fmt.Errorf("write mcp known file: %w", err)
	}
	return nil
}
