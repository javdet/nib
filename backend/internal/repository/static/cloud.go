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

var _ repository.CloudRepository = (*CloudRepo)(nil)

// CloudRepo serves clouds from the in-memory config store and
// delegates writes to the config.Manager.
type CloudRepo struct {
	holder  *StoreHolder
	manager *config.Manager
}

func NewCloudRepo(holder *StoreHolder, manager *config.Manager) *CloudRepo {
	return &CloudRepo{holder: holder, manager: manager}
}

func (r *CloudRepo) ListByProject(_ context.Context, projectID uuid.UUID) ([]domain.Cloud, error) {
	var result []domain.Cloud
	for _, c := range r.holder.Load().Clouds {
		if c.ProjectID == projectID {
			result = append(result, c)
		}
	}
	return result, nil
}

func (r *CloudRepo) GetByID(_ context.Context, projectID, cloudID uuid.UUID) (domain.Cloud, error) {
	for _, c := range r.holder.Load().Clouds {
		if c.ProjectID == projectID && c.ID == cloudID {
			return c, nil
		}
	}
	return domain.Cloud{}, repository.ErrNotFound
}

func (r *CloudRepo) Create(_ context.Context, projectID uuid.UUID, name, description string) (domain.Cloud, error) {
	projectName, err := r.projectName(projectID)
	if err != nil {
		return domain.Cloud{}, err
	}
	if err := r.manager.AddCloud(projectName, config.CloudConfig{Name: name, Description: description}); err != nil {
		if errors.Is(err, config.ErrDuplicate) {
			return domain.Cloud{}, fmt.Errorf("%w: cloud %q", repository.ErrAlreadyExists, name)
		}
		return domain.Cloud{}, fmt.Errorf("add cloud: %w", err)
	}
	return r.findByName(projectID, name)
}

func (r *CloudRepo) Update(_ context.Context, projectID, cloudID uuid.UUID, name, description string) (domain.Cloud, error) {
	projectName, err := r.projectName(projectID)
	if err != nil {
		return domain.Cloud{}, err
	}
	oldName, err := r.cloudName(projectID, cloudID)
	if err != nil {
		return domain.Cloud{}, err
	}
	if err := r.manager.UpdateCloud(projectName, oldName, config.CloudConfig{Name: name, Description: description}); err != nil {
		if errors.Is(err, config.ErrDuplicate) {
			return domain.Cloud{}, fmt.Errorf("%w: cloud %q", repository.ErrAlreadyExists, name)
		}
		return domain.Cloud{}, fmt.Errorf("update cloud: %w", err)
	}
	return r.findByName(projectID, name)
}

func (r *CloudRepo) Delete(_ context.Context, projectID, cloudID uuid.UUID) error {
	projectName, err := r.projectName(projectID)
	if err != nil {
		return err
	}
	oldName, err := r.cloudName(projectID, cloudID)
	if err != nil {
		return err
	}
	return r.manager.RemoveCloud(projectName, oldName)
}

func (r *CloudRepo) projectName(id uuid.UUID) (string, error) {
	return lookupProjectName(r.holder.Load(), id)
}

func (r *CloudRepo) cloudName(projectID, cloudID uuid.UUID) (string, error) {
	c, err := lookupCloud(r.holder.Load(), projectID, cloudID)
	if err != nil {
		return "", err
	}
	if err := reservedEntityName(c.Name, "cloud"); err != nil {
		return "", err
	}
	return c.Name, nil
}

func (r *CloudRepo) findByName(projectID uuid.UUID, name string) (domain.Cloud, error) {
	return lookupCloudByName(r.holder.Load(), projectID, name)
}
