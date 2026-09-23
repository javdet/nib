// Package kbsettings holds the operator-controlled knowledge-base switches that
// do not belong in config.yaml: config.yaml describes structure, this describes
// what nib is allowed to do on its own.
package kbsettings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	defaultFileName = "knowledge.json"
	jsonIndent      = "  "
)

// Settings holds the knowledge-base switches stored on the data volume.
type Settings struct {
	// AutoUpdate lets the sub-agent launched by Finish fold a plan's outcome
	// back into its project's collection.
	AutoUpdate bool `json:"autoUpdate"`
}

// ResolveFile returns the path used for the knowledge settings file. An empty
// settingsFile means {dataDir}/knowledge.json; absolute paths are used as-is.
func ResolveFile(dataDir, settingsFile string) string {
	settingsFile = strings.TrimSpace(settingsFile)
	if settingsFile == "" {
		return filepath.Join(dataDir, defaultFileName)
	}
	if filepath.IsAbs(settingsFile) {
		return settingsFile
	}
	return filepath.Join(dataDir, settingsFile)
}

// Store manages the knowledge settings file.
type Store struct {
	mu       sync.RWMutex
	filePath string

	initOnce sync.Once
	initErr  error
}

// NewStore creates a Store over the file resolved from dataDir and settingsFile.
func NewStore(dataDir, settingsFile string) *Store {
	return &Store{filePath: ResolveFile(dataDir, settingsFile)}
}

// Path returns the resolved settings file path.
func (s *Store) Path() string {
	return s.filePath
}

func (s *Store) ensureInitialized() error {
	s.initOnce.Do(func() {
		s.initErr = s.initialize()
	})
	return s.initErr
}

func (s *Store) initialize() error {
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create knowledge settings directory: %w", err)
	}

	if _, err := os.Stat(s.filePath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat knowledge settings file: %w", err)
		}
		return s.writeLocked(Defaults())
	}
	return nil
}

// Defaults returns the settings an install that has never been configured runs
// with. Auto-update is on so an upgrade delivers the feature without an operator
// having to find the switch first.
func Defaults() Settings {
	return Settings{AutoUpdate: true}
}

// Get returns the current settings.
func (s *Store) Get() (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureInitialized(); err != nil {
		return Settings{}, err
	}
	return s.readLocked()
}

// Set replaces the settings.
func (s *Store) Set(settings Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureInitialized(); err != nil {
		return err
	}
	return s.writeLocked(settings)
}

// rawSettings decodes with pointers so a key absent from a file written by an
// earlier version reads as the default rather than as Go's zero value -- which
// for a bool would silently turn the feature off on upgrade.
type rawSettings struct {
	AutoUpdate *bool `json:"autoUpdate"`
}

func parseSettings(content string) (Settings, error) {
	var raw rawSettings
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return Settings{}, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}
	settings := Defaults()
	if raw.AutoUpdate != nil {
		settings.AutoUpdate = *raw.AutoUpdate
	}
	return settings, nil
}

func (s *Store) readLocked() (Settings, error) {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return Settings{}, fmt.Errorf("read knowledge settings: %w", err)
	}
	return parseSettings(string(data))
}

func (s *Store) writeLocked(settings Settings) error {
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create knowledge settings directory: %w", err)
	}

	data, err := json.MarshalIndent(settings, "", jsonIndent)
	if err != nil {
		return fmt.Errorf("marshal knowledge settings: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".knowledge-*.json")
	if err != nil {
		return fmt.Errorf("create temp knowledge settings file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp knowledge settings file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp knowledge settings file: %w", err)
	}
	if err := os.Rename(tmpName, s.filePath); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename temp knowledge settings file: %w", err)
	}
	return nil
}
