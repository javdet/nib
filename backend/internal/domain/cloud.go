package domain

import (
	"time"

	"github.com/google/uuid"
)

// Cloud represents a cloud provider configuration within a project (e.g. "digitalocean", "aws").
type Cloud struct {
	ID          uuid.UUID `json:"id"`
	ProjectID   uuid.UUID `json:"projectId"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}
