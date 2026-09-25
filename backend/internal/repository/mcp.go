package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
)

// MCPRepository defines the data-access contract for MCP connection entities.
type MCPRepository interface {
	List(ctx context.Context) ([]domain.MCPConnection, error)
	GetByID(ctx context.Context, id uuid.UUID) (domain.MCPConnection, error)
	Create(ctx context.Context, conn domain.MCPConnection) (domain.MCPConnection, error)
	Update(ctx context.Context, id uuid.UUID, conn domain.MCPConnection) (domain.MCPConnection, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status string) error
	Delete(ctx context.Context, id uuid.UUID) error
}
