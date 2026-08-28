package repository

import (
	"context"

	"github.com/javdet/nib/internal/domain"
	"github.com/google/uuid"
)

// LocationRepository defines the data-access contract for locations within a cloud.
type LocationRepository interface {
	ListByCloud(ctx context.Context, projectID, cloudID uuid.UUID) ([]domain.Location, error)
	GetByID(ctx context.Context, projectID, cloudID, locationID uuid.UUID) (domain.Location, error)
	Create(ctx context.Context, projectID, cloudID uuid.UUID, name, description string) (domain.Location, error)
	Update(ctx context.Context, projectID, cloudID, locationID uuid.UUID, name, description string) (domain.Location, error)
	Delete(ctx context.Context, projectID, cloudID, locationID uuid.UUID) error
}
