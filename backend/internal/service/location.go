package service

import (
	"context"
	"fmt"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/repository"
	"github.com/google/uuid"
)

// LocationService implements business logic for locations
// scoped under a cloud within a project.
type LocationService struct {
	locationRepo repository.LocationRepository
	cloudRepo    repository.CloudRepository
}

func NewLocationService(
	locationRepo repository.LocationRepository,
	cloudRepo repository.CloudRepository,
) *LocationService {
	return &LocationService{
		locationRepo: locationRepo,
		cloudRepo:    cloudRepo,
	}
}

func (s *LocationService) ListByCloud(ctx context.Context, projectID, cloudID uuid.UUID) ([]domain.Location, error) {
	if _, err := s.cloudRepo.GetByID(ctx, projectID, cloudID); err != nil {
		return nil, fmt.Errorf("verify cloud: %w", err)
	}

	locations, err := s.locationRepo.ListByCloud(ctx, projectID, cloudID)
	if err != nil {
		return nil, fmt.Errorf("list locations: %w", err)
	}
	return locations, nil
}

func (s *LocationService) GetByID(ctx context.Context, projectID, cloudID, locationID uuid.UUID) (domain.Location, error) {
	location, err := s.locationRepo.GetByID(ctx, projectID, cloudID, locationID)
	if err != nil {
		return domain.Location{}, fmt.Errorf("get location: %w", err)
	}
	return location, nil
}

func (s *LocationService) Create(ctx context.Context, projectID, cloudID uuid.UUID, name, description string) (domain.Location, error) {
	if _, err := s.cloudRepo.GetByID(ctx, projectID, cloudID); err != nil {
		return domain.Location{}, fmt.Errorf("verify cloud: %w", err)
	}
	loc, err := s.locationRepo.Create(ctx, projectID, cloudID, name, description)
	if err != nil {
		return domain.Location{}, fmt.Errorf("create location: %w", err)
	}
	return loc, nil
}

func (s *LocationService) Update(ctx context.Context, projectID, cloudID, locationID uuid.UUID, name, description string) (domain.Location, error) {
	loc, err := s.locationRepo.Update(ctx, projectID, cloudID, locationID, name, description)
	if err != nil {
		return domain.Location{}, fmt.Errorf("update location: %w", err)
	}
	return loc, nil
}

func (s *LocationService) Delete(ctx context.Context, projectID, cloudID, locationID uuid.UUID) error {
	if err := s.locationRepo.Delete(ctx, projectID, cloudID, locationID); err != nil {
		return fmt.Errorf("delete location: %w", err)
	}
	return nil
}
