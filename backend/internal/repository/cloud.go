package repository

import (
	"context"

	"github.com/javdet/nib/internal/domain"
	"github.com/google/uuid"
)

// CloudRepository defines the data-access contract for clouds within a project.
type CloudRepository interface {
	ListByProject(ctx context.Context, projectID uuid.UUID) ([]domain.Cloud, error)
	GetByID(ctx context.Context, projectID, cloudID uuid.UUID) (domain.Cloud, error)
	Create(ctx context.Context, projectID uuid.UUID, name, description string) (domain.Cloud, error)
	Update(ctx context.Context, projectID, cloudID uuid.UUID, name, description string) (domain.Cloud, error)
	Delete(ctx context.Context, projectID, cloudID uuid.UUID) error
}
