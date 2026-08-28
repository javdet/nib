package repository

import (
	"context"

	"github.com/javdet/nib/internal/domain"
	"github.com/google/uuid"
)

// AttachmentRepository defines data access for chat message attachments.
type AttachmentRepository interface {
	CreateAttachment(ctx context.Context, a domain.Attachment) (domain.Attachment, error)
	ListAttachmentsByDialog(ctx context.Context, dialogID uuid.UUID) ([]domain.Attachment, error)
	GetAttachment(ctx context.Context, dialogID, attachmentID uuid.UUID) (domain.Attachment, error)
	DeleteAttachment(ctx context.Context, dialogID, attachmentID uuid.UUID) error
	LinkAttachmentsToMessage(ctx context.Context, dialogID uuid.UUID, messageID int64, attachmentIDs []uuid.UUID) error
}
