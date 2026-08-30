package domain

import (
	"time"

	"github.com/google/uuid"
)

// PromptVariable is a named template value scoped for system-prompt rendering.
type PromptVariable struct {
	ID          uuid.UUID `json:"id"`
	Scope       string    `json:"scope"`
	ScopeName   string    `json:"scopeName"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Value       string    `json:"value"`
	Kind        string    `json:"kind"`
	Deletable   bool      `json:"deletable"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}
