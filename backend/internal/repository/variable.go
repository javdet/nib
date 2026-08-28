package repository

import (
	"context"

	"github.com/javdet/nib/internal/domain"
	"github.com/google/uuid"
)

// VariableRepository defines the data-access contract for prompt variable entities.
type VariableRepository interface {
	List(ctx context.Context) ([]domain.PromptVariable, error)
	GetByID(ctx context.Context, id uuid.UUID) (domain.PromptVariable, error)
	Create(ctx context.Context, v domain.PromptVariable) (domain.PromptVariable, error)
	Update(ctx context.Context, id uuid.UUID, v domain.PromptVariable) (domain.PromptVariable, error)
	Delete(ctx context.Context, id uuid.UUID) error
	// EnsureExists inserts a variable when (scope, name) is missing; existing rows are unchanged.
	EnsureExists(ctx context.Context, v domain.PromptVariable) error
	// EnsureBuiltin inserts or updates a built-in variable, marking it non-deletable.
	EnsureBuiltin(ctx context.Context, v domain.PromptVariable) error
	// Upsert sets value for (scope, name), inserting when missing.
	Upsert(ctx context.Context, v domain.PromptVariable) (domain.PromptVariable, error)
	// LoadAll returns every variable grouped as {scope: {name: value}} for text/template rendering.
	// List variables are parsed into []string; string variables remain strings.
	LoadAll(ctx context.Context) (map[string]map[string]any, error)
}
