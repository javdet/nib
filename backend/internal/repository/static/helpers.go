package static

import (
	"fmt"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/repository"
	"github.com/google/uuid"
)

func lookupProjectName(store *Store, id uuid.UUID) (string, error) {
	for _, p := range store.Projects {
		if p.ID == id {
			if p.Name == anyName {
				return "", fmt.Errorf("cannot modify reserved project %q", anyName)
			}
			return p.Name, nil
		}
	}
	return "", repository.ErrNotFound
}

func lookupEnvironment(store *Store, projectID, envID uuid.UUID) (domain.Environment, error) {
	for _, e := range store.Environments {
		if e.ProjectID == projectID && e.ID == envID {
			return e, nil
		}
	}
	return domain.Environment{}, repository.ErrNotFound
}

func lookupEnvironmentByName(store *Store, projectID uuid.UUID, name string) (domain.Environment, error) {
	for _, e := range store.Environments {
		if e.ProjectID == projectID && e.Name == name {
			return e, nil
		}
	}
	return domain.Environment{}, repository.ErrNotFound
}

func lookupCloud(store *Store, projectID, cloudID uuid.UUID) (domain.Cloud, error) {
	for _, c := range store.Clouds {
		if c.ProjectID == projectID && c.ID == cloudID {
			return c, nil
		}
	}
	return domain.Cloud{}, repository.ErrNotFound
}

func lookupCloudByName(store *Store, projectID uuid.UUID, name string) (domain.Cloud, error) {
	for _, c := range store.Clouds {
		if c.ProjectID == projectID && c.Name == name {
			return c, nil
		}
	}
	return domain.Cloud{}, repository.ErrNotFound
}

func lookupLocation(store *Store, projectID, cloudID, locationID uuid.UUID) (domain.Location, error) {
	for _, l := range store.Locations {
		if l.ProjectID == projectID && l.CloudID == cloudID && l.ID == locationID {
			return l, nil
		}
	}
	return domain.Location{}, repository.ErrNotFound
}

func lookupLocationByName(store *Store, projectID, cloudID uuid.UUID, name string) (domain.Location, error) {
	for _, l := range store.Locations {
		if l.ProjectID == projectID && l.CloudID == cloudID && l.Name == name {
			return l, nil
		}
	}
	return domain.Location{}, repository.ErrNotFound
}

func reservedEntityName(name, entity string) error {
	if name == anyName {
		return fmt.Errorf("cannot modify reserved %s %q", entity, anyName)
	}
	return nil
}
