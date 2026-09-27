package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/repository"
	"github.com/javdet/nib/internal/textutil"
)

var _ repository.DialogImporter = (*DialogRepo)(nil)

// ImportDialogs inserts every dialog, message and attachment row in one
// transaction. Dialogs are inserted in slice order, so a parent has to come
// before its children.
//
// created_at is kept from the source rather than defaulted: every row of one
// transaction would otherwise share a single now(), and ListChildren orders by
// it. updated_at is now(), which is what puts an imported plan at the top of
// the list.
func (r *DialogRepo) ImportDialogs(ctx context.Context, dialogs []domain.DialogImport) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin import dialogs: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now := time.Now()
	for _, imp := range dialogs {
		d := imp.Dialog
		categories := make([]string, 0, len(d.Categories))
		for _, c := range d.Categories {
			categories = append(categories, textutil.Sanitize(c))
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO chat_dialogs (id, mode, title, parent_id, categories, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, now())`,
			d.ID, d.Mode, textutil.Sanitize(d.Title), d.ParentID, categories, timeOrNow(d.CreatedAt, now),
		); err != nil {
			return fmt.Errorf("insert imported dialog %s: %w", d.ID, err)
		}

		for seq, msg := range imp.Messages {
			var messageID int64
			if err := tx.QueryRow(ctx,
				`INSERT INTO chat_dialog_messages (dialog_id, seq, role, content, tool_calls, tool_call_id, name, created_at)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
				 RETURNING id`,
				d.ID, seq, msg.Role, textutil.Sanitize(msg.Content), nullableJSON(msg.ToolCalls),
				nullableString(textutil.Sanitize(msg.ToolCallID)), nullableString(textutil.Sanitize(msg.Name)),
				timeOrNow(msg.CreatedAt, now),
			).Scan(&messageID); err != nil {
				return fmt.Errorf("insert imported message %d of dialog %s: %w", seq, d.ID, err)
			}

			for _, a := range msg.Attachments {
				if _, err := tx.Exec(ctx,
					`INSERT INTO chat_attachments (id, dialog_id, message_id, filename, content_type, kind, size_bytes, path)
					 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
					a.ID, d.ID, messageID, a.Filename, a.ContentType, string(a.Kind), a.SizeBytes, a.Path,
				); err != nil {
					return fmt.Errorf("insert imported attachment %s: %w", a.ID, err)
				}
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit import dialogs: %w", err)
	}
	return nil
}

func timeOrNow(t, now time.Time) time.Time {
	if t.IsZero() {
		return now
	}
	return t
}
