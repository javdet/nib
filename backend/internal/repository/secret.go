package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
)

// SecretRepository defines the data-access contract for encrypted prompt secrets.
type SecretRepository interface {
	List(ctx context.Context) ([]domain.PromptSecret, error)
	GetByID(ctx context.Context, id uuid.UUID) (domain.PromptSecret, error)
	// GetByName addresses a secret by its full identity. (scope, scope_name, name)
	// is the UNIQUE key, so dropping scope_name would match several rows and
	// return an arbitrary one.
	GetByName(ctx context.Context, scope, scopeName, name string) (domain.PromptSecret, error)
	GetEncrypted(ctx context.Context, id uuid.UUID) ([]byte, error)
	GetEncryptedByName(ctx context.Context, scope, scopeName, name string) ([]byte, error)
	Create(ctx context.Context, s domain.PromptSecret, encrypted []byte) (domain.PromptSecret, error)
	Update(ctx context.Context, id uuid.UUID, s domain.PromptSecret, encrypted []byte) (domain.PromptSecret, error)
	Delete(ctx context.Context, id uuid.UUID) error
}
