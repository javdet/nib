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

	// ClaimOrphanedAttachmentFiles reads queued file deletions, oldest first. The
	// queue is filled by an AFTER DELETE trigger, so it also catches the cascades
	// that delete a row without any Go code running.
	ClaimOrphanedAttachmentFiles(ctx context.Context, limit int) ([]domain.OrphanedAttachmentFile, error)
	// DropOrphanedAttachmentFiles clears queue entries whose files are gone.
	DropOrphanedAttachmentFiles(ctx context.Context, ids []int64) error
	// DeleteUnsentAttachments removes uploads never attached to a message, which
	// queues their files for deletion through the same trigger.
	DeleteUnsentAttachments(ctx context.Context, olderThan string) (int64, error)
}
