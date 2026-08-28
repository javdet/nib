package repository

import (
	"context"

	"github.com/javdet/nib/internal/domain"
	"github.com/google/uuid"
)

// EnvironmentRepository defines the data-access contract for environments within a project.
type EnvironmentRepository interface {
	ListByProject(ctx context.Context, projectID uuid.UUID) ([]domain.Environment, error)
	GetByID(ctx context.Context, projectID, envID uuid.UUID) (domain.Environment, error)
	Create(ctx context.Context, projectID uuid.UUID, name, description string) (domain.Environment, error)
	Update(ctx context.Context, projectID, envID uuid.UUID, name, description string) (domain.Environment, error)
	Delete(ctx context.Context, projectID, envID uuid.UUID) error
}
