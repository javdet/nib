package domain

import (
	"time"

	"github.com/google/uuid"
)

// Project represents an infrastructure project that groups environments, clouds, and locations.
type Project struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}
