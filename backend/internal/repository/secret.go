package repository

import (
	"context"

	"github.com/javdet/nib/internal/domain"
	"github.com/google/uuid"
)

// SecretRepository defines the data-access contract for encrypted prompt secrets.
type SecretRepository interface {
	List(ctx context.Context) ([]domain.PromptSecret, error)
	GetByID(ctx context.Context, id uuid.UUID) (domain.PromptSecret, error)
	GetByName(ctx context.Context, scope, name string) (domain.PromptSecret, error)
	GetEncrypted(ctx context.Context, id uuid.UUID) ([]byte, error)
	GetEncryptedByName(ctx context.Context, scope, name string) ([]byte, error)
	Create(ctx context.Context, s domain.PromptSecret, encrypted []byte) (domain.PromptSecret, error)
	Update(ctx context.Context, id uuid.UUID, s domain.PromptSecret, encrypted []byte) (domain.PromptSecret, error)
	Delete(ctx context.Context, id uuid.UUID) error
}
