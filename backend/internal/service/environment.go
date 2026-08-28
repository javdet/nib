package service

import (
	"context"
	"fmt"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/repository"
	"github.com/google/uuid"
)

// EnvironmentService implements business logic for environments
// within a project scope.
type EnvironmentService struct {
	envRepo     repository.EnvironmentRepository
	projectRepo repository.ProjectRepository
}

func NewEnvironmentService(
	envRepo repository.EnvironmentRepository,
	projectRepo repository.ProjectRepository,
) *EnvironmentService {
	return &EnvironmentService{
		envRepo:     envRepo,
		projectRepo: projectRepo,
	}
}

func (s *EnvironmentService) ListByProject(ctx context.Context, projectID uuid.UUID) ([]domain.Environment, error) {
	if _, err := s.projectRepo.GetByID(ctx, projectID); err != nil {
		return nil, fmt.Errorf("verify project: %w", err)
	}

	envs, err := s.envRepo.ListByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list environments: %w", err)
	}
	return envs, nil
}

func (s *EnvironmentService) GetByID(ctx context.Context, projectID, envID uuid.UUID) (domain.Environment, error) {
	env, err := s.envRepo.GetByID(ctx, projectID, envID)
	if err != nil {
		return domain.Environment{}, fmt.Errorf("get environment: %w", err)
	}
	return env, nil
}

func (s *EnvironmentService) Create(ctx context.Context, projectID uuid.UUID, name, description string) (domain.Environment, error) {
	if _, err := s.projectRepo.GetByID(ctx, projectID); err != nil {
		return domain.Environment{}, fmt.Errorf("verify project: %w", err)
	}
	env, err := s.envRepo.Create(ctx, projectID, name, description)
	if err != nil {
		return domain.Environment{}, fmt.Errorf("create environment: %w", err)
	}
	return env, nil
}

func (s *EnvironmentService) Update(ctx context.Context, projectID, envID uuid.UUID, name, description string) (domain.Environment, error) {
	env, err := s.envRepo.Update(ctx, projectID, envID, name, description)
	if err != nil {
		return domain.Environment{}, fmt.Errorf("update environment: %w", err)
	}
	return env, nil
}

func (s *EnvironmentService) Delete(ctx context.Context, projectID, envID uuid.UUID) error {
	if err := s.envRepo.Delete(ctx, projectID, envID); err != nil {
		return fmt.Errorf("delete environment: %w", err)
	}
	return nil
}
