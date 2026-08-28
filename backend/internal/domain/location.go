package domain

import (
	"time"

	"github.com/google/uuid"
)

// Location represents a geographic location within a cloud (e.g. "fra1", "nyc1").
// Replaces the former Region concept, scoped under a specific cloud.
type Location struct {
	ID          uuid.UUID `json:"id"`
	ProjectID   uuid.UUID `json:"projectId"`
	CloudID     uuid.UUID `json:"cloudId"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}
