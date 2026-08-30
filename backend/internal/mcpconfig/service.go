package mcpconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/javdet/nib/internal/atomicfile"
	"github.com/javdet/nib/internal/mcpclient"
	"github.com/javdet/nib/internal/repository"
)

const jsonIndent = "  "

// Service manages a Cursor-style mcp.json file containing an mcpServers map.
type Service struct {
	mu         sync.RWMutex
	filePath   string
	changeHook func()
	secrets    SecretLookup

	initOnce sync.Once
	initErr  error
}

// NewService creates a Service. mcpFile is the raw value from config mcp.file;
// it is resolved with ResolveFile against dataDir.
func NewService(dataDir, mcpFile string) *Service {
	return &Service{
		filePath: ResolveFile(dataDir, mcpFile),
	}
}

// NewServiceAtPath creates a Service that reads and writes the given mcp.json path.
func NewServiceAtPath(filePath string) *Service {
	return &Service{filePath: filePath}
}

// SetChangeHook registers a callback invoked after successful mutations (add/update/delete/set raw).
func (s *Service) SetChangeHook(hook func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.changeHook = hook
}

func (s *Service) notifyChange() {
	if s.changeHook != nil {
		s.changeHook()
	}
}

// Path returns the resolved mcp.json file path.
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
		return fmt.Errorf("create mcp config directory: %w", err)
	}

	if _, err := os.Stat(s.filePath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat mcp config file: %w", err)
		}
		return s.writeDocumentLocked(Document{MCPServers: map[string]ServerEntry{}})
	}
	return nil
}

// ListServers returns all MCP servers sorted by name.
func (s *Service) ListServers() ([]Server, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureInitialized(); err != nil {
		return nil, err
	}

	doc, err := s.readDocumentLocked()
	if err != nil {
		return nil, err
	}
	return serversFromDocument(doc), nil
}

// ListServerTools discovers tools from a configured HTTP MCP server, expanding
// any ${NAME} references in its URL and headers first.
func (s *Service) ListServerTools(ctx context.Context, name string) ([]mcpclient.ToolInfo, error) {
	raw, err := s.GetServer(name)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw.URL) == "" {
		return nil, ErrStdioNotSupported
	}

	resolved, err := s.ResolveServer(ctx, raw)
	if err != nil {
		return nil, err
	}

	tools, err := mcpclient.ListToolsFromURLWithHTTPClient(
		ctx,
		resolved.URL,
		resolved.Headers,
		&http.Client{Timeout: mcpclient.DefaultUIDiscoveryTimeout},
	)
	if err != nil {
		// Wrapped so the handler answers with the cause — a rejected token, an
		// unreachable host — instead of a bare 500. RedactError has already
		// stripped every substituted secret from the message.
		return nil, fmt.Errorf("%w: %w", ErrDiscoveryFailed, resolved.RedactError(err))
	}
	return tools, nil
}

// GetServer returns a single MCP server by name.
func (s *Service) GetServer(name string) (Server, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureInitialized(); err != nil {
		return Server{}, err
	}
	if err := ValidateName(name); err != nil {
		return Server{}, err
	}

	doc, err := s.readDocumentLocked()
	if err != nil {
		return Server{}, err
	}

	entry, ok := doc.MCPServers[name]
	if !ok {
		return Server{}, repository.ErrNotFound
	}
	return Server{Name: name, ServerEntry: entry}, nil
}

// AddServer adds a new MCP server. Fails if the name already exists.
func (s *Service) AddServer(name string, entry ServerEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureInitialized(); err != nil {
		return err
	}
	if err := ValidateName(name); err != nil {
		return err
	}

	doc, err := s.readRawDocLocked()
	if err != nil {
		return err
	}
	if doc.has(name) {
		return repository.ErrAlreadyExists
	}

	if err := doc.set(name, entry); err != nil {
		return err
	}
	if err := s.writeRawDocLocked(doc); err != nil {
		return err
	}
	s.notifyChange()
	return nil
}

// UpdateServer updates an MCP server and optionally renames it.
func (s *Service) UpdateServer(oldName, newName string, entry ServerEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureInitialized(); err != nil {
		return err
	}
	if err := ValidateName(oldName); err != nil {
		return err
	}
	if err := ValidateName(newName); err != nil {
		return err
	}

	doc, err := s.readRawDocLocked()
	if err != nil {
		return err
	}
	if !doc.has(oldName) {
		return repository.ErrNotFound
	}

	if oldName != newName {
		if doc.has(newName) {
			return repository.ErrAlreadyExists
		}
		// A rename keeps the old entry's unmodelled fields: move it under the
		// new key before the edit is merged in.
		doc.servers[newName] = doc.servers[oldName]
		doc.delete(oldName)
	}

	if err := doc.set(newName, entry); err != nil {
		return err
	}
	if err := s.writeRawDocLocked(doc); err != nil {
		return err
	}
	s.notifyChange()
	return nil
}

// DeleteServer removes an MCP server by name.
func (s *Service) DeleteServer(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureInitialized(); err != nil {
		return err
	}
	if err := ValidateName(name); err != nil {
		return err
	}

	doc, err := s.readRawDocLocked()
	if err != nil {
		return err
	}
	if !doc.has(name) {
		return repository.ErrNotFound
	}

	doc.delete(name)
	if err := s.writeRawDocLocked(doc); err != nil {
		return err
	}
	s.notifyChange()
	return nil
}

// GetRaw returns the mcp.json file contents verbatim, so the editor shows the
// bytes on disk — comments, key order and all — rather than a re-rendering of
// the fields this package models.
func (s *Service) GetRaw() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureInitialized(); err != nil {
		return "", err
	}

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return "", fmt.Errorf("read mcp config: %w", err)
	}
	return string(data), nil
}

// SetRaw replaces the entire mcp.json file. Valid JSON is stored byte for byte:
// comments, formatting, other top-level keys and fields belonging to other MCP
// clients all survive, and a server that cannot be reached is still saved —
// connectivity shows up when its tools are listed, not on save.
func (s *Service) SetRaw(content string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureInitialized(); err != nil {
		return err
	}

	if err := validateRawContent(content); err != nil {
		return err
	}
	if err := s.writeContentLocked(content); err != nil {
		return err
	}
	s.notifyChange()
	return nil
}

// validateRawContent rejects content the reader could not load back: the file
// has to parse as JSON (comments allowed) into the document shape, and every
// server name has to be usable as a map key.
func validateRawContent(content string) error {
	doc, err := parseDocument(content)
	if err != nil {
		return err
	}
	for name := range doc.MCPServers {
		if err := ValidateName(name); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) readDocumentLocked() (Document, error) {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return Document{}, fmt.Errorf("read mcp config: %w", err)
	}
	return parseDocument(string(data))
}

func parseDocument(content string) (Document, error) {
	var doc Document
	content = stripJSONComments(content)
	if err := json.Unmarshal([]byte(content), &doc); err != nil {
		return Document{}, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}
	if doc.MCPServers == nil {
		doc.MCPServers = map[string]ServerEntry{}
	}
	return doc, nil
}

func formatDocument(doc Document) (string, error) {
	if doc.MCPServers == nil {
		doc.MCPServers = map[string]ServerEntry{}
	}
	data, err := json.MarshalIndent(doc, "", jsonIndent)
	if err != nil {
		return "", fmt.Errorf("marshal mcp config: %w", err)
	}
	return string(data) + "\n", nil
}

func serversFromDocument(doc Document) []Server {
	names := make([]string, 0, len(doc.MCPServers))
	for name := range doc.MCPServers {
		names = append(names, name)
	}
	sort.Strings(names)

	servers := make([]Server, 0, len(names))
	for _, name := range names {
		servers = append(servers, Server{
			Name:        name,
			ServerEntry: doc.MCPServers[name],
		})
	}
	return servers
}

func (s *Service) readRawDocLocked() (rawDoc, error) {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return rawDoc{}, fmt.Errorf("read mcp config: %w", err)
	}
	return parseRawDoc(string(data))
}

func (s *Service) writeRawDocLocked(doc rawDoc) error {
	formatted, err := doc.format()
	if err != nil {
		return err
	}
	return s.writeContentLocked(formatted)
}

func (s *Service) writeDocumentLocked(doc Document) error {
	formatted, err := formatDocument(doc)
	if err != nil {
		return err
	}
	return s.writeContentLocked(formatted)
}

// writeContentLocked replaces mcp.json atomically. Content is stored as given,
// with a trailing newline so the file stays diffable.
func (s *Service) writeContentLocked(content string) error {
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create mcp config directory: %w", err)
	}
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	if err := atomicfile.WriteString(s.filePath, content); err != nil {
		return fmt.Errorf("write mcp config: %w", err)
	}
	return nil
}
