package domain

import (
	"time"

	"github.com/google/uuid"
)

// PromptSecret is metadata for an encrypted prompt secret.
// Values are never exposed on this type; they are stored encrypted in the database.
type PromptSecret struct {
	ID          uuid.UUID `json:"id"`
	Scope       string    `json:"scope"`
	ScopeName   string    `json:"scopeName"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}
