package domain

import (
	"time"

	"github.com/google/uuid"
)

// Environment represents a deployment target within a project (e.g. "production", "staging").
type Environment struct {
	ID          uuid.UUID `json:"id"`
	ProjectID   uuid.UUID `json:"projectId"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}
