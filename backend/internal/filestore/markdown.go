package filestore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/javdet/nib/internal/atomicfile"
	"github.com/javdet/nib/internal/repository"
)

// Document holds a single markdown file.
type Document struct {
	Name    string
	Content string
}

// MarkdownStore manages file-based markdown documents as {name}.md under a directory.
type MarkdownStore struct {
	mu           sync.RWMutex
	dir          string
	initOnce     sync.Once
	initErr      error
	tempPrefix   string
	entityLabel  string
	validateName func(string) error
}

// NewMarkdownStore creates a store for markdown documents.
func NewMarkdownStore(dir, tempPrefix, entityLabel string, validateName func(string) error) *MarkdownStore {
	return &MarkdownStore{
		dir:          dir,
		tempPrefix:   tempPrefix,
		entityLabel:  entityLabel,
		validateName: validateName,
	}
}

// Dir returns the resolved directory path.
func (s *MarkdownStore) Dir() string {
	return s.dir
}

func (s *MarkdownStore) docPath(name string) (string, error) {
	if err := s.validateName(name); err != nil {
		return "", err
	}
	return filepath.Join(s.dir, name+mdSuffix), nil
}

func (s *MarkdownStore) ensureInitialized() error {
	s.initOnce.Do(func() {
		s.initErr = s.initialize()
	})
	return s.initErr
}

func (s *MarkdownStore) initialize() error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("create %s directory: %w", s.entityLabel, err)
	}
	return nil
}

// ListNames returns document basenames (without .md), sorted alphabetically.
func (s *MarkdownStore) ListNames() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureInitialized(); err != nil {
		return nil, err
	}
	return s.listNamesLocked()
}

// Get reads a single document by name.
func (s *MarkdownStore) Get(name string) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureInitialized(); err != nil {
		return Document{}, err
	}

	path, err := s.docPath(name)
	if err != nil {
		return Document{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Document{}, repository.ErrNotFound
		}
		return Document{}, fmt.Errorf("read %s %s: %w", s.entityLabel, name, err)
	}
	return Document{Name: name, Content: string(data)}, nil
}

// Create writes a new document. Fails if the document already exists.
func (s *MarkdownStore) Create(name, content string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureInitialized(); err != nil {
		return err
	}

	path, err := s.docPath(name)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return repository.ErrAlreadyExists
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat %s %s: %w", s.entityLabel, name, err)
	}
	return s.writeFileLocked(name, content)
}

// Set overwrites document content for an existing name.
func (s *MarkdownStore) Set(name, content string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureInitialized(); err != nil {
		return err
	}

	path, err := s.docPath(name)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return repository.ErrNotFound
		}
		return fmt.Errorf("stat %s %s: %w", s.entityLabel, name, err)
	}
	return s.writeFileLocked(name, content)
}

// Rename changes the document name and optionally updates content.
func (s *MarkdownStore) Rename(oldName, newName, content string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureInitialized(); err != nil {
		return err
	}

	oldPath, err := s.docPath(oldName)
	if err != nil {
		return err
	}
	if _, err := os.Stat(oldPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return repository.ErrNotFound
		}
		return fmt.Errorf("stat %s %s: %w", s.entityLabel, oldName, err)
	}

	if oldName == newName {
		return s.writeFileLocked(oldName, content)
	}

	newPath, err := s.docPath(newName)
	if err != nil {
		return err
	}
	if _, err := os.Stat(newPath); err == nil {
		return repository.ErrAlreadyExists
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat %s %s: %w", s.entityLabel, newName, err)
	}

	if err := s.writeFileLocked(newName, content); err != nil {
		return err
	}
	if err := os.Remove(oldPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove old %s %s: %w", s.entityLabel, oldName, err)
	}
	return nil
}

// Delete removes a document file.
func (s *MarkdownStore) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureInitialized(); err != nil {
		return err
	}

	path, err := s.docPath(name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return repository.ErrNotFound
		}
		return fmt.Errorf("delete %s %s: %w", s.entityLabel, name, err)
	}
	return nil
}

// WriteFileLocked writes content for name; caller must hold s.mu.
func (s *MarkdownStore) WriteFileLocked(name, content string) error {
	return s.writeFileLocked(name, content)
}

// EnsureInitializedLocked ensures the store directory exists; caller must hold s.mu.
func (s *MarkdownStore) EnsureInitializedLocked() error {
	return s.ensureInitialized()
}

func (s *MarkdownStore) listNamesLocked() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("read %s directory: %w", s.entityLabel, err)
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), mdSuffix) {
			continue
		}
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		base := strings.TrimSuffix(e.Name(), mdSuffix)
		if err := s.validateName(base); err != nil {
			continue
		}
		names = append(names, base)
	}
	sort.Strings(names)
	if names == nil {
		names = []string{}
	}
	return names, nil
}

func (s *MarkdownStore) writeFileLocked(name, content string) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("create %s directory: %w", s.entityLabel, err)
	}

	path := filepath.Join(s.dir, name+mdSuffix)
	if err := atomicfile.WriteString(path, content); err != nil {
		return fmt.Errorf("write %s %s: %w", s.entityLabel, name, err)
	}
	return nil
}
