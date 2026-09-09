package domain

import (
	"time"

	"github.com/google/uuid"
)

// AttachmentKind classifies an uploaded file for LLM delivery.
type AttachmentKind string

const (
	AttachmentKindImage AttachmentKind = "image"
	AttachmentKindText  AttachmentKind = "text"
)

// Attachment is metadata for a file uploaded to a chat dialog.
type Attachment struct {
	ID          uuid.UUID      `json:"id"`
	DialogID    uuid.UUID      `json:"dialogId"`
	MessageID   *int64         `json:"messageId,omitempty"`
	Filename    string         `json:"filename"`
	ContentType string         `json:"contentType"`
	Kind        AttachmentKind `json:"kind"`
	SizeBytes   int64          `json:"sizeBytes"`
	Path        string         `json:"-"`
	URL         string         `json:"url,omitempty"`
	CreatedAt   time.Time      `json:"createdAt"`
}

// OrphanedAttachmentFile is one queued file deletion: a chat_attachments row is
// gone (deleted outright, or cascaded away with its dialog or message) and the
// bytes it pointed at are still on disk.
type OrphanedAttachmentFile struct {
	ID       int64
	Path     string
	DialogID uuid.UUID
}
