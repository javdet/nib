package service

import (
	"context"
	"fmt"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/repository"
	"github.com/google/uuid"
)

// CloudService implements business logic for clouds within a project scope.
type CloudService struct {
	cloudRepo   repository.CloudRepository
	projectRepo repository.ProjectRepository
}

func NewCloudService(
	cloudRepo repository.CloudRepository,
	projectRepo repository.ProjectRepository,
) *CloudService {
	return &CloudService{
		cloudRepo:   cloudRepo,
		projectRepo: projectRepo,
	}
}

func (s *CloudService) ListByProject(ctx context.Context, projectID uuid.UUID) ([]domain.Cloud, error) {
	if _, err := s.projectRepo.GetByID(ctx, projectID); err != nil {
		return nil, fmt.Errorf("verify project: %w", err)
	}

	clouds, err := s.cloudRepo.ListByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list clouds: %w", err)
	}
	return clouds, nil
}

func (s *CloudService) GetByID(ctx context.Context, projectID, cloudID uuid.UUID) (domain.Cloud, error) {
	cloud, err := s.cloudRepo.GetByID(ctx, projectID, cloudID)
	if err != nil {
		return domain.Cloud{}, fmt.Errorf("get cloud: %w", err)
	}
	return cloud, nil
}

func (s *CloudService) Create(ctx context.Context, projectID uuid.UUID, name, description string) (domain.Cloud, error) {
	if _, err := s.projectRepo.GetByID(ctx, projectID); err != nil {
		return domain.Cloud{}, fmt.Errorf("verify project: %w", err)
	}
	cloud, err := s.cloudRepo.Create(ctx, projectID, name, description)
	if err != nil {
		return domain.Cloud{}, fmt.Errorf("create cloud: %w", err)
	}
	return cloud, nil
}

func (s *CloudService) Update(ctx context.Context, projectID, cloudID uuid.UUID, name, description string) (domain.Cloud, error) {
	cloud, err := s.cloudRepo.Update(ctx, projectID, cloudID, name, description)
	if err != nil {
		return domain.Cloud{}, fmt.Errorf("update cloud: %w", err)
	}
	return cloud, nil
}

func (s *CloudService) Delete(ctx context.Context, projectID, cloudID uuid.UUID) error {
	if err := s.cloudRepo.Delete(ctx, projectID, cloudID); err != nil {
		return fmt.Errorf("delete cloud: %w", err)
	}
	return nil
}
