package skills

import (
	"github.com/javdet/nib/internal/filestore"
)

const mdSuffix = ".md"

// Meta holds parsed frontmatter metadata for a skill.
type Meta = filestore.Meta

// Skill holds a single skill file.
type Skill struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// ListResult is the aggregate view of skill files.
type ListResult struct {
	Skills []Meta `json:"skills"`
}

// Service manages file-based skills as {name}.md under a skills directory.
type Service struct {
	store *filestore.MarkdownStore
}

// NewService creates a Service. skillsDir is the raw value from config skills.dir;
// it is resolved with ResolveDir against dataDir.
func NewService(dataDir, skillsDir string) *Service {
	dir := ResolveDir(dataDir, skillsDir)
	return &Service{
		store: filestore.NewMarkdownStore(dir, ".skill-*.md", "skill", ValidateName),
	}
}

// Dir returns the resolved skills directory path.
func (s *Service) Dir() string {
	return s.store.Dir()
}

// List returns skill metadata sorted alphabetically by name.
func (s *Service) List() (ListResult, error) {
	metas, err := s.store.ListMeta()
	if err != nil {
		return ListResult{}, err
	}
	return ListResult{Skills: metas}, nil
}

// Get reads a single skill by name.
func (s *Service) Get(name string) (Skill, error) {
	doc, err := s.store.Get(name)
	if err != nil {
		return Skill{}, err
	}
	return Skill{Name: doc.Name, Content: doc.Content}, nil
}

// Create writes a new skill file. Fails if the skill already exists.
func (s *Service) Create(name, content string) error {
	return s.store.Create(name, content)
}

// Set overwrites skill content for an existing name.
func (s *Service) Set(name, content string) error {
	return s.store.Set(name, content)
}

// Rename changes the skill name and optionally updates content.
func (s *Service) Rename(oldName, newName, content string) error {
	return s.store.Rename(oldName, newName, content)
}

// Delete removes a skill file.
func (s *Service) Delete(name string) error {
	return s.store.Delete(name)
}
