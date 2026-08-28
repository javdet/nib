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

var _ repository.EnvironmentRepository = (*EnvironmentRepo)(nil)

// EnvironmentRepo serves environments from the in-memory config store and
// delegates writes to the config.Manager.
type EnvironmentRepo struct {
	holder  *StoreHolder
	manager *config.Manager
}

func NewEnvironmentRepo(holder *StoreHolder, manager *config.Manager) *EnvironmentRepo {
	return &EnvironmentRepo{holder: holder, manager: manager}
}

func (r *EnvironmentRepo) ListByProject(_ context.Context, projectID uuid.UUID) ([]domain.Environment, error) {
	var result []domain.Environment
	for _, e := range r.holder.Load().Environments {
		if e.ProjectID == projectID {
			result = append(result, e)
		}
	}
	return result, nil
}

func (r *EnvironmentRepo) GetByID(_ context.Context, projectID, envID uuid.UUID) (domain.Environment, error) {
	for _, e := range r.holder.Load().Environments {
		if e.ProjectID == projectID && e.ID == envID {
			return e, nil
		}
	}
	return domain.Environment{}, repository.ErrNotFound
}

func (r *EnvironmentRepo) Create(_ context.Context, projectID uuid.UUID, name, description string) (domain.Environment, error) {
	projectName, err := r.projectName(projectID)
	if err != nil {
		return domain.Environment{}, err
	}
	if err := r.manager.AddEnvironment(projectName, config.EnvironmentConfig{Name: name, Description: description}); err != nil {
		if errors.Is(err, config.ErrDuplicate) {
			return domain.Environment{}, fmt.Errorf("%w: environment %q", repository.ErrAlreadyExists, name)
		}
		return domain.Environment{}, fmt.Errorf("add environment: %w", err)
	}
	return r.findByName(projectID, name)
}

func (r *EnvironmentRepo) Update(_ context.Context, projectID, envID uuid.UUID, name, description string) (domain.Environment, error) {
	projectName, err := r.projectName(projectID)
	if err != nil {
		return domain.Environment{}, err
	}
	oldName, err := r.envName(projectID, envID)
	if err != nil {
		return domain.Environment{}, err
	}
	if err := r.manager.UpdateEnvironment(projectName, oldName, config.EnvironmentConfig{Name: name, Description: description}); err != nil {
		if errors.Is(err, config.ErrDuplicate) {
			return domain.Environment{}, fmt.Errorf("%w: environment %q", repository.ErrAlreadyExists, name)
		}
		return domain.Environment{}, fmt.Errorf("update environment: %w", err)
	}
	return r.findByName(projectID, name)
}

func (r *EnvironmentRepo) Delete(_ context.Context, projectID, envID uuid.UUID) error {
	projectName, err := r.projectName(projectID)
	if err != nil {
		return err
	}
	oldName, err := r.envName(projectID, envID)
	if err != nil {
		return err
	}
	return r.manager.RemoveEnvironment(projectName, oldName)
}

func (r *EnvironmentRepo) projectName(id uuid.UUID) (string, error) {
	return lookupProjectName(r.holder.Load(), id)
}

func (r *EnvironmentRepo) envName(projectID, envID uuid.UUID) (string, error) {
	e, err := lookupEnvironment(r.holder.Load(), projectID, envID)
	if err != nil {
		return "", err
	}
	if err := reservedEntityName(e.Name, "environment"); err != nil {
		return "", err
	}
	return e.Name, nil
}

func (r *EnvironmentRepo) findByName(projectID uuid.UUID, name string) (domain.Environment, error) {
	return lookupEnvironmentByName(r.holder.Load(), projectID, name)
}
