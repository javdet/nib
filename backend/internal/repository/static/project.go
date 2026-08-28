package static

import (
	"context"
	"errors"
	"fmt"

	"github.com/javdet/nib/internal/config"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/repository"
	"github.com/google/uuid"
)

var _ repository.ProjectRepository = (*ProjectRepo)(nil)

// ProjectRepo serves projects from the in-memory config store and
// delegates writes to the config.Manager.
type ProjectRepo struct {
	holder  *StoreHolder
	manager *config.Manager
}

func NewProjectRepo(holder *StoreHolder, manager *config.Manager) *ProjectRepo {
	return &ProjectRepo{holder: holder, manager: manager}
}

func (r *ProjectRepo) List(_ context.Context) ([]domain.Project, error) {
	return r.holder.Load().Projects, nil
}

func (r *ProjectRepo) GetByID(_ context.Context, id uuid.UUID) (domain.Project, error) {
	for _, p := range r.holder.Load().Projects {
		if p.ID == id {
			return p, nil
		}
	}
	return domain.Project{}, repository.ErrNotFound
}

func (r *ProjectRepo) Create(_ context.Context, name, description string) (domain.Project, error) {
	err := r.manager.AddProject(config.ProjectConfig{Name: name, Description: description})
	if err != nil {
		if errors.Is(err, config.ErrDuplicate) {
			return domain.Project{}, fmt.Errorf("%w: project %q", repository.ErrAlreadyExists, name)
		}
		return domain.Project{}, fmt.Errorf("add project: %w", err)
	}
	p, lookupErr := r.findByName(name)
	if lookupErr != nil {
		return domain.Project{}, lookupErr
	}
	return p, nil
}

func (r *ProjectRepo) Update(_ context.Context, id uuid.UUID, name, description string) (domain.Project, error) {
	oldName, err := r.nameByID(id)
	if err != nil {
		return domain.Project{}, err
	}
	if err := r.manager.UpdateProject(oldName, config.ProjectConfig{Name: name, Description: description}); err != nil {
		if errors.Is(err, config.ErrDuplicate) {
			return domain.Project{}, fmt.Errorf("%w: project %q", repository.ErrAlreadyExists, name)
		}
		return domain.Project{}, fmt.Errorf("update project: %w", err)
	}
	p, lookupErr := r.findByName(name)
	if lookupErr != nil {
		return domain.Project{}, lookupErr
	}
	return p, nil
}

func (r *ProjectRepo) Delete(_ context.Context, id uuid.UUID) error {
	oldName, err := r.nameByID(id)
	if err != nil {
		return err
	}
	if err := r.manager.RemoveProject(oldName); err != nil {
		return fmt.Errorf("remove project: %w", err)
	}
	return nil
}

func (r *ProjectRepo) nameByID(id uuid.UUID) (string, error) {
	for _, p := range r.holder.Load().Projects {
		if p.ID == id {
			if p.Name == anyName {
				return "", fmt.Errorf("cannot modify reserved project %q", anyName)
			}
			return p.Name, nil
		}
	}
	return "", repository.ErrNotFound
}

func (r *ProjectRepo) findByName(name string) (domain.Project, error) {
	for _, p := range r.holder.Load().Projects {
		if p.Name == name {
			return p, nil
		}
	}
	return domain.Project{}, repository.ErrNotFound
}
