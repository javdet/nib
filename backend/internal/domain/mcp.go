package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type MCPConnection struct {
	ID             uuid.UUID       `json:"id"`
	Type           string          `json:"type"`
	Name           string          `json:"name"`
	ServerURL      string          `json:"serverUrl"`
	AuthMethod     string          `json:"authMethod"`
	AccessToken    string          `json:"-"`
	RefreshToken   string          `json:"-"`
	TokenExpiresAt *time.Time      `json:"tokenExpiresAt,omitempty"`
	APIToken       string          `json:"-"`
	Status         string          `json:"status"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}
