package skills

import (
	"fmt"
	"log/slog"
	"sort"

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

// List returns the operator's skill metadata, sorted alphabetically by name.
//
// It reads the data volume only. This is the listing the HTTP API serves, so a
// system skill never reaches the interface: the agent-facing listing is
// ListAll.
func (s *Service) List() (ListResult, error) {
	metas, err := s.store.ListMeta()
	if err != nil {
		return ListResult{}, err
	}
	return ListResult{Skills: metas}, nil
}

// ListAll returns the operator's skills plus the ones the image owns, sorted by
// name. This is what the system prompt's skill catalog is built from.
func (s *Service) ListAll() (ListResult, error) {
	onDisk, err := s.List()
	if err != nil {
		return ListResult{}, err
	}

	metas := make([]Meta, 0, len(onDisk.Skills)+len(systemSkills))
	metas = append(metas, SystemMeta()...)
	for _, meta := range onDisk.Skills {
		// Writes to a system name are refused, so a file can only shadow one if
		// it predates the system tier. The image's own answer wins, and the
		// shadow is worth a line in the log.
		if IsSystem(meta.Name) {
			slog.Warn("skill file shadows a system skill and is ignored",
				"skill", meta.Name, "dir", s.store.Dir())
			continue
		}
		metas = append(metas, meta)
	}
	sort.Slice(metas, func(i, j int) bool { return metas[i].Name < metas[j].Name })
	return ListResult{Skills: metas}, nil
}

// Get reads a single skill by name from the data volume.
//
// It cannot reach a system skill, which is what makes those a 404 over HTTP by
// construction rather than by a filter a later handler could forget. The
// agent-facing read is GetAny.
func (s *Service) Get(name string) (Skill, error) {
	doc, err := s.store.Get(name)
	if err != nil {
		return Skill{}, err
	}
	return Skill{Name: doc.Name, Content: doc.Content}, nil
}

// GetAny reads a skill, preferring the system tier so a stale file on the data
// volume cannot answer a question about nib in the image's place.
func (s *Service) GetAny(name string) (Skill, error) {
	if IsSystem(name) {
		content, err := systemContent(name)
		if err != nil {
			return Skill{}, err
		}
		return Skill{Name: name, Content: content}, nil
	}
	return s.Get(name)
}

// Create writes a new skill file. Fails if the skill already exists.
func (s *Service) Create(name, content string) error {
	if err := refuseSystem(name); err != nil {
		return err
	}
	return s.store.Create(name, content)
}

// Set overwrites skill content for an existing name.
func (s *Service) Set(name, content string) error {
	if err := refuseSystem(name); err != nil {
		return err
	}
	return s.store.Set(name, content)
}

// Rename changes the skill name and optionally updates content. Both names are
// checked: renaming a skill *onto* a system name is how a file would come to
// shadow the image's answer.
func (s *Service) Rename(oldName, newName, content string) error {
	if err := refuseSystem(oldName); err != nil {
		return err
	}
	if err := refuseSystem(newName); err != nil {
		return err
	}
	return s.store.Rename(oldName, newName, content)
}

// Delete removes a skill file.
func (s *Service) Delete(name string) error {
	if err := refuseSystem(name); err != nil {
		return err
	}
	return s.store.Delete(name)
}

func refuseSystem(name string) error {
	if IsSystem(name) {
		return fmt.Errorf("%w: %s", ErrSystemSkill, name)
	}
	return nil
}
