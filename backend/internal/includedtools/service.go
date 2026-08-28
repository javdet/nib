package includedtools

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/javdet/nib/internal/mode"
)

const defaultFileName = "mcp-included.json"

// Service manages per-mode MCP tool included lists in a single JSON file.
type Service struct {
	mu       sync.RWMutex
	filePath string

	initOnce sync.Once
	initErr  error
}

// NewService creates a Service. toolsDir is the resolved included-tools directory
// (typically data/tools); the file is stored as mcp-included.json inside it.
func NewService(toolsDir string) *Service {
	return &Service{
		filePath: filepath.Join(toolsDir, defaultFileName),
	}
}

// Path returns the resolved mcp-included.json file path.
func (s *Service) Path() string {
	return s.filePath
}

func (s *Service) ensureInitialized() error {
	s.initOnce.Do(func() {
		s.initErr = s.initialize()
	})
	return s.initErr
}

func (s *Service) initialize() error {
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create included-tools directory: %w", err)
	}

	if _, err := os.Stat(s.filePath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat mcp included file: %w", err)
		}
		return s.writeDocumentLocked(document{})
	}
	return nil
}

// Get returns the MCP tool included list for a mode (empty slice if unset).
func (s *Service) Get(modeName string) ([]string, error) {
	if !mode.IsValid(modeName) {
		return nil, fmt.Errorf("invalid mode %q", modeName)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if err := s.ensureInitialized(); err != nil {
		return nil, err
	}

	doc, err := s.readDocumentLocked()
	if err != nil {
		return nil, err
	}
	return sortedTools(doc[modeName]), nil
}

// GetAll returns MCP tool included lists for all modes that have entries.
func (s *Service) GetAll() (map[string][]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if err := s.ensureInitialized(); err != nil {
		return nil, err
	}

	doc, err := s.readDocumentLocked()
	if err != nil {
		return nil, err
	}

	out := make(map[string][]string, len(doc))
	for modeName, tools := range doc {
		out[modeName] = sortedTools(tools)
	}
	return out, nil
}

// Set replaces the MCP tool included list for a mode.
func (s *Service) Set(modeName string, tools []string) error {
	if !mode.IsValid(modeName) {
		return fmt.Errorf("invalid mode %q", modeName)
	}

	normalized := normalizeTools(tools)

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureInitialized(); err != nil {
		return err
	}

	doc, err := s.readDocumentLocked()
	if err != nil {
		return err
	}
	if doc == nil {
		doc = document{}
	}
	doc[modeName] = normalized
	return s.writeDocumentLocked(doc)
}

type document map[string][]string

func (s *Service) readDocumentLocked() (document, error) {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return nil, fmt.Errorf("read mcp included file: %w", err)
	}
	var doc document
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse mcp included file: %w", err)
	}
	if doc == nil {
		doc = document{}
	}
	return doc, nil
}

func (s *Service) writeDocumentLocked(doc document) error {
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create included-tools directory: %w", err)
	}

	formatted, err := formatDocument(doc)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".mcp-included-*.json")
	if err != nil {
		return fmt.Errorf("create temp mcp included file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.WriteString(formatted); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp mcp included file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp mcp included file: %w", err)
	}
	if err := os.Rename(tmpName, s.filePath); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename temp mcp included file: %w", err)
	}
	return nil
}

func formatDocument(doc document) (string, error) {
	if doc == nil {
		doc = document{}
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal mcp included file: %w", err)
	}
	return string(data) + "\n", nil
}

func normalizeTools(tools []string) []string {
	seen := make(map[string]struct{}, len(tools))
	out := make([]string, 0, len(tools))
	for _, name := range tools {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func sortedTools(tools []string) []string {
	if len(tools) == 0 {
		return []string{}
	}
	out := make([]string, len(tools))
	copy(out, tools)
	sort.Strings(out)
	return out
}
