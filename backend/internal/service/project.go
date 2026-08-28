package service

import (
	"context"
	"fmt"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/repository"
	"github.com/google/uuid"
)

// ProjectService implements business logic for projects.
type ProjectService struct {
	repo repository.ProjectRepository
}

func NewProjectService(repo repository.ProjectRepository) *ProjectService {
	return &ProjectService{repo: repo}
}

func (s *ProjectService) List(ctx context.Context) ([]domain.Project, error) {
	projects, err := s.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	return projects, nil
}

func (s *ProjectService) GetByID(ctx context.Context, id uuid.UUID) (domain.Project, error) {
	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return domain.Project{}, fmt.Errorf("get project: %w", err)
	}
	return p, nil
}

func (s *ProjectService) Create(ctx context.Context, name, description string) (domain.Project, error) {
	p, err := s.repo.Create(ctx, name, description)
	if err != nil {
		return domain.Project{}, fmt.Errorf("create project: %w", err)
	}
	return p, nil
}

func (s *ProjectService) Update(ctx context.Context, id uuid.UUID, name, description string) (domain.Project, error) {
	p, err := s.repo.Update(ctx, id, name, description)
	if err != nil {
		return domain.Project{}, fmt.Errorf("update project: %w", err)
	}
	return p, nil
}

func (s *ProjectService) Delete(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	return nil
}
