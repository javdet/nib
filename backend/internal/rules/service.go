package rules

import (
	"github.com/javdet/nib/internal/filestore"
)

const mdSuffix = ".md"

// Meta holds parsed frontmatter metadata for a rule.
type Meta = filestore.Meta

// Rule holds a single rule file.
type Rule struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// ListResult is the aggregate view of rule files.
type ListResult struct {
	Rules []Meta `json:"rules"`
}

// Service manages file-based rules as {name}.md under a rules directory.
type Service struct {
	store *filestore.MarkdownStore
}

// NewService creates a Service. rulesDir is the raw value from config rules.dir;
// it is resolved with ResolveDir against dataDir.
func NewService(dataDir, rulesDir string) *Service {
	dir := ResolveDir(dataDir, rulesDir)
	return &Service{
		store: filestore.NewMarkdownStore(dir, ".rule-*.md", "rule", ValidateName),
	}
}

// Dir returns the resolved rules directory path.
func (s *Service) Dir() string {
	return s.store.Dir()
}

// List returns rule metadata sorted alphabetically by name.
func (s *Service) List() (ListResult, error) {
	metas, err := s.store.ListMeta()
	if err != nil {
		return ListResult{}, err
	}
	return ListResult{Rules: metas}, nil
}

// Get reads a single rule by name.
func (s *Service) Get(name string) (Rule, error) {
	doc, err := s.store.Get(name)
	if err != nil {
		return Rule{}, err
	}
	return Rule{Name: doc.Name, Content: doc.Content}, nil
}

// Create writes a new rule file. Fails if the rule already exists.
func (s *Service) Create(name, content string) error {
	return s.store.Create(name, content)
}

// Set overwrites rule content for an existing name.
func (s *Service) Set(name, content string) error {
	return s.store.Set(name, content)
}

// Rename changes the rule name and optionally updates content.
func (s *Service) Rename(oldName, newName, content string) error {
	return s.store.Rename(oldName, newName, content)
}

// Delete removes a rule file.
func (s *Service) Delete(name string) error {
	return s.store.Delete(name)
}
