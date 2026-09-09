package repository

import (
	"context"
	"time"

	"github.com/javdet/nib/internal/domain"
	"github.com/google/uuid"
)

// MCPRepository defines the data-access contract for MCP connection entities.
type MCPRepository interface {
	List(ctx context.Context) ([]domain.MCPConnection, error)
	GetByID(ctx context.Context, id uuid.UUID) (domain.MCPConnection, error)
	Create(ctx context.Context, conn domain.MCPConnection) (domain.MCPConnection, error)
	// UpsertByTypeName creates the connection or refreshes the existing row with
	// the same (type, name). The OAuth callback needs it: it hard-codes the name
	// to the provider type, so re-running the flow is a re-authentication of one
	// upstream, not a second one.
	UpsertByTypeName(ctx context.Context, conn domain.MCPConnection) (domain.MCPConnection, error)
	Update(ctx context.Context, id uuid.UUID, conn domain.MCPConnection) (domain.MCPConnection, error)
	UpdateTokens(ctx context.Context, id uuid.UUID, accessToken, refreshToken string, expiresAt *time.Time) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status string) error
	Delete(ctx context.Context, id uuid.UUID) error
}
