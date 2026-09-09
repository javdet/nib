package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
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
	//
	// The grouping has no room for scope_name, so several entities' copies of one
	// name compete for the same slot. scopeNames says which scope_name is active
	// per scope and wins that competition; without a match the lowest scope_name
	// wins, so the result is at least stable instead of heap-order dependent.
	LoadAll(ctx context.Context, scopeNames map[string]string) (map[string]map[string]any, error)
}
