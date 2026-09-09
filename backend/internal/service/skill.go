package service

import (
	"context"

	"github.com/javdet/nib/internal/repository"
	"github.com/javdet/nib/internal/skills"
)

// SkillService is the application-layer view of the skill files in
// [skills.Service]. It exists so the HTTP layer depends on one collaborator
// instead of three: rendering a skill needs prompt variables and the current
// selection, and reaching for those from a handler put a repository behind the
// transport boundary.
type SkillService struct {
	store        *skills.Service
	variableRepo repository.VariableRepository
	selection    *SelectionStore
}

// NewSkillService wires the skill file store to the sources a template render needs.
func NewSkillService(store *skills.Service, variableRepo repository.VariableRepository, selection *SelectionStore) *SkillService {
	return &SkillService{store: store, variableRepo: variableRepo, selection: selection}
}

// List returns the operator-visible skills (system skills are deliberately hidden).
func (s *SkillService) List() (skills.ListResult, error) { return s.store.List() }

// Get loads one operator-visible skill verbatim.
func (s *SkillService) Get(name string) (skills.Skill, error) { return s.store.Get(name) }

// GetRendered loads a skill with its text/template actions resolved against the
// prompt variables and the current selection — the same substitution the agent
// sees, so the UI preview and the run agree.
func (s *SkillService) GetRendered(ctx context.Context, name string) (skills.Skill, error) {
	skill, err := s.store.Get(name)
	if err != nil {
		return skills.Skill{}, err
	}
	rendered, err := RenderTemplateVariables(ctx, skill.Content, s.variableRepo, s.selection)
	if err != nil {
		return skills.Skill{}, err
	}
	skill.Content = rendered
	return skill, nil
}

// Create writes a new skill file.
func (s *SkillService) Create(name, content string) error { return s.store.Create(name, content) }

// Set overwrites an existing skill's content.
func (s *SkillService) Set(name, content string) error { return s.store.Set(name, content) }

// Rename moves a skill to a new name, writing content under it.
func (s *SkillService) Rename(oldName, newName, content string) error {
	return s.store.Rename(oldName, newName, content)
}

// Delete removes a skill file.
func (s *SkillService) Delete(name string) error { return s.store.Delete(name) }
