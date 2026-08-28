package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ repository.AttachmentRepository = (*AttachmentRepo)(nil)

const attachmentColumns = `id, dialog_id, message_id, filename, content_type, kind, size_bytes, path, created_at`

// AttachmentRepo implements AttachmentRepository using PostgreSQL.
type AttachmentRepo struct {
	pool *pgxpool.Pool
}

func NewAttachmentRepo(pool *pgxpool.Pool) *AttachmentRepo {
	return &AttachmentRepo{pool: pool}
}

func (r *AttachmentRepo) CreateAttachment(ctx context.Context, a domain.Attachment) (domain.Attachment, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO chat_attachments (id, dialog_id, message_id, filename, content_type, kind, size_bytes, path)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING `+attachmentColumns,
		a.ID, a.DialogID, nullableInt64Ptr(a.MessageID), a.Filename, a.ContentType, string(a.Kind), a.SizeBytes, a.Path)

	result, err := scanAttachmentRow(row)
	if err != nil {
		return domain.Attachment{}, fmt.Errorf("insert attachment: %w", err)
	}
	return result, nil
}

func (r *AttachmentRepo) ListAttachmentsByDialog(ctx context.Context, dialogID uuid.UUID) ([]domain.Attachment, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+attachmentColumns+`
		 FROM chat_attachments
		 WHERE dialog_id = $1
		 ORDER BY created_at ASC`, dialogID)
	if err != nil {
		return nil, fmt.Errorf("list attachments: %w", err)
	}
	defer rows.Close()

	var attachments []domain.Attachment
	for rows.Next() {
		a, err := scanAttachment(rows)
		if err != nil {
			return nil, fmt.Errorf("scan attachment: %w", err)
		}
		attachments = append(attachments, a)
	}
	return attachments, rows.Err()
}

func (r *AttachmentRepo) GetAttachment(ctx context.Context, dialogID, attachmentID uuid.UUID) (domain.Attachment, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+attachmentColumns+`
		 FROM chat_attachments
		 WHERE id = $1 AND dialog_id = $2`, attachmentID, dialogID)

	a, err := scanAttachmentRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Attachment{}, repository.ErrNotFound
		}
		return domain.Attachment{}, fmt.Errorf("get attachment: %w", err)
	}
	return a, nil
}

func (r *AttachmentRepo) DeleteAttachment(ctx context.Context, dialogID, attachmentID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM chat_attachments WHERE id = $1 AND dialog_id = $2`,
		attachmentID, dialogID)
	if err != nil {
		return fmt.Errorf("delete attachment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *AttachmentRepo) LinkAttachmentsToMessage(ctx context.Context, dialogID uuid.UUID, messageID int64, attachmentIDs []uuid.UUID) error {
	if len(attachmentIDs) == 0 {
		return nil
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin link attachments: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, id := range attachmentIDs {
		tag, err := tx.Exec(ctx,
			`UPDATE chat_attachments
			 SET message_id = $3
			 WHERE id = $1 AND dialog_id = $2 AND message_id IS NULL`,
			id, dialogID, messageID)
		if err != nil {
			return fmt.Errorf("link attachment %s: %w", id, err)
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("attachment %s not found or already linked", id)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit link attachments: %w", err)
	}
	return nil
}

func scanAttachment(rows pgx.Rows) (domain.Attachment, error) {
	var a domain.Attachment
	var messageID sql.NullInt64
	var kind string
	var path string
	err := rows.Scan(
		&a.ID, &a.DialogID, &messageID, &a.Filename, &a.ContentType,
		&kind, &a.SizeBytes, &path, &a.CreatedAt,
	)
	if err != nil {
		return domain.Attachment{}, err
	}
	a.Kind = domain.AttachmentKind(kind)
	a.Path = path
	if messageID.Valid {
		a.MessageID = &messageID.Int64
	}
	return a, nil
}

func scanAttachmentRow(row pgx.Row) (domain.Attachment, error) {
	var a domain.Attachment
	var messageID sql.NullInt64
	var kind string
	var path string
	err := row.Scan(
		&a.ID, &a.DialogID, &messageID, &a.Filename, &a.ContentType,
		&kind, &a.SizeBytes, &path, &a.CreatedAt,
	)
	if err != nil {
		return domain.Attachment{}, err
	}
	a.Kind = domain.AttachmentKind(kind)
	a.Path = path
	if messageID.Valid {
		a.MessageID = &messageID.Int64
	}
	return a, nil
}

func nullableInt64Ptr(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}
