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

var _ repository.LocationRepository = (*LocationRepo)(nil)

// LocationRepo serves locations from the in-memory config store and
// delegates writes to the config.Manager.
type LocationRepo struct {
	holder  *StoreHolder
	manager *config.Manager
}

func NewLocationRepo(holder *StoreHolder, manager *config.Manager) *LocationRepo {
	return &LocationRepo{holder: holder, manager: manager}
}

func (r *LocationRepo) ListByCloud(_ context.Context, projectID, cloudID uuid.UUID) ([]domain.Location, error) {
	var result []domain.Location
	for _, l := range r.holder.Load().Locations {
		if l.ProjectID == projectID && l.CloudID == cloudID {
			result = append(result, l)
		}
	}
	return result, nil
}

func (r *LocationRepo) GetByID(_ context.Context, projectID, cloudID, locationID uuid.UUID) (domain.Location, error) {
	for _, l := range r.holder.Load().Locations {
		if l.ProjectID == projectID && l.CloudID == cloudID && l.ID == locationID {
			return l, nil
		}
	}
	return domain.Location{}, repository.ErrNotFound
}

func (r *LocationRepo) Create(_ context.Context, projectID, cloudID uuid.UUID, name, description string) (domain.Location, error) {
	projectName, err := r.projectName(projectID)
	if err != nil {
		return domain.Location{}, err
	}
	cloudName, err := r.cloudName(projectID, cloudID)
	if err != nil {
		return domain.Location{}, err
	}
	if err := r.manager.AddLocation(projectName, cloudName, config.LocationConfig{Name: name, Description: description}); err != nil {
		if errors.Is(err, config.ErrDuplicate) {
			return domain.Location{}, fmt.Errorf("%w: location %q", repository.ErrAlreadyExists, name)
		}
		return domain.Location{}, fmt.Errorf("add location: %w", err)
	}
	return r.findByName(projectID, cloudID, name)
}

func (r *LocationRepo) Update(_ context.Context, projectID, cloudID, locationID uuid.UUID, name, description string) (domain.Location, error) {
	projectName, err := r.projectName(projectID)
	if err != nil {
		return domain.Location{}, err
	}
	cName, err := r.cloudName(projectID, cloudID)
	if err != nil {
		return domain.Location{}, err
	}
	oldName, err := r.locName(projectID, cloudID, locationID)
	if err != nil {
		return domain.Location{}, err
	}
	if err := r.manager.UpdateLocation(projectName, cName, oldName, config.LocationConfig{Name: name, Description: description}); err != nil {
		if errors.Is(err, config.ErrDuplicate) {
			return domain.Location{}, fmt.Errorf("%w: location %q", repository.ErrAlreadyExists, name)
		}
		return domain.Location{}, fmt.Errorf("update location: %w", err)
	}
	return r.findByName(projectID, cloudID, name)
}

func (r *LocationRepo) Delete(_ context.Context, projectID, cloudID, locationID uuid.UUID) error {
	projectName, err := r.projectName(projectID)
	if err != nil {
		return err
	}
	cName, err := r.cloudName(projectID, cloudID)
	if err != nil {
		return err
	}
	oldName, err := r.locName(projectID, cloudID, locationID)
	if err != nil {
		return err
	}
	return r.manager.RemoveLocation(projectName, cName, oldName)
}

func (r *LocationRepo) projectName(id uuid.UUID) (string, error) {
	return lookupProjectName(r.holder.Load(), id)
}

func (r *LocationRepo) cloudName(projectID, cloudID uuid.UUID) (string, error) {
	c, err := lookupCloud(r.holder.Load(), projectID, cloudID)
	if err != nil {
		return "", err
	}
	if err := reservedEntityName(c.Name, "cloud"); err != nil {
		return "", err
	}
	return c.Name, nil
}

func (r *LocationRepo) locName(projectID, cloudID, locationID uuid.UUID) (string, error) {
	l, err := lookupLocation(r.holder.Load(), projectID, cloudID, locationID)
	if err != nil {
		return "", err
	}
	if err := reservedEntityName(l.Name, "location"); err != nil {
		return "", err
	}
	return l.Name, nil
}

func (r *LocationRepo) findByName(projectID, cloudID uuid.UUID, name string) (domain.Location, error) {
	return lookupLocationByName(r.holder.Load(), projectID, cloudID, name)
}
