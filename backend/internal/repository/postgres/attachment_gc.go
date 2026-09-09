package postgres

import (
	"context"
	"fmt"

	"github.com/javdet/nib/internal/domain"
)

// ClaimOrphanedAttachmentFiles returns up to limit queued file deletions, oldest
// first. The rows stay queued until DropOrphanedAttachmentFiles confirms the
// files are gone, so a crash mid-sweep costs a repeat, not a leak.
func (r *AttachmentRepo) ClaimOrphanedAttachmentFiles(ctx context.Context, limit int) ([]domain.OrphanedAttachmentFile, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, path, dialog_id
		 FROM attachment_files_to_delete
		 ORDER BY queued_at ASC, id ASC
		 LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("claim orphaned attachment files: %w", err)
	}
	defer rows.Close()

	var out []domain.OrphanedAttachmentFile
	for rows.Next() {
		var f domain.OrphanedAttachmentFile
		if err := rows.Scan(&f.ID, &f.Path, &f.DialogID); err != nil {
			return nil, fmt.Errorf("scan orphaned attachment file: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// DropOrphanedAttachmentFiles removes queue rows whose files are gone from disk.
func (r *AttachmentRepo) DropOrphanedAttachmentFiles(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	if _, err := r.pool.Exec(ctx,
		`DELETE FROM attachment_files_to_delete WHERE id = ANY($1::bigint[])`, ids); err != nil {
		return fmt.Errorf("drop orphaned attachment files: %w", err)
	}
	return nil
}

// DeleteUnsentAttachments removes uploads that were never attached to a message.
// message_id IS NULL rows have no other garbage collection: nothing reads them
// except the link guard, and the composer they belong to may be long gone. The
// AFTER DELETE trigger turns each one into a queued file deletion.
func (r *AttachmentRepo) DeleteUnsentAttachments(ctx context.Context, olderThan string) (int64, error) {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM chat_attachments
		 WHERE message_id IS NULL AND created_at < now() - $1::interval`, olderThan)
	if err != nil {
		return 0, fmt.Errorf("delete unsent attachments: %w", err)
	}
	return tag.RowsAffected(), nil
}
