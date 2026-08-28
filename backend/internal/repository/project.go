package repository

import (
	"context"

	"github.com/javdet/nib/internal/domain"
	"github.com/google/uuid"
)

// ProjectRepository defines the data-access contract for projects.
type ProjectRepository interface {
	List(ctx context.Context) ([]domain.Project, error)
	GetByID(ctx context.Context, id uuid.UUID) (domain.Project, error)
	Create(ctx context.Context, name, description string) (domain.Project, error)
	Update(ctx context.Context, id uuid.UUID, name, description string) (domain.Project, error)
	Delete(ctx context.Context, id uuid.UUID) error
}
