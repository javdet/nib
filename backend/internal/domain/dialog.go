package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Dialog is a persisted chat conversation scoped to a single mode.
type Dialog struct {
	ID        uuid.UUID  `json:"id"`
	Title     string     `json:"title"`
	Mode      string     `json:"mode"`
	ParentID  *uuid.UUID `json:"parentId,omitempty"`
	TaskID    *string    `json:"taskId,omitempty"`
	Categories []string `json:"categories,omitempty"`
	Pinned     bool     `json:"pinned"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// DialogMessage is one row in a dialog transcript (system, user, assistant, or tool).
type DialogMessage struct {
	ID         int64           `json:"id"`
	DialogID   uuid.UUID       `json:"dialogId"`
	Seq        int             `json:"seq"`
	Role       string          `json:"role"`
	Content    string          `json:"content"`
	ToolCalls  json.RawMessage `json:"toolCalls,omitempty"`
	ToolCallID string          `json:"toolCallId,omitempty"`
	Name        string          `json:"name,omitempty"`
	Attachments []Attachment    `json:"attachments,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
}
