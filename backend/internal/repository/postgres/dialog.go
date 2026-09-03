package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/repository"
	"github.com/javdet/nib/internal/textutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ repository.DialogRepository = (*DialogRepo)(nil)

// DialogRepo implements DialogRepository using PostgreSQL.
type DialogRepo struct {
	pool *pgxpool.Pool
}

func NewDialogRepo(pool *pgxpool.Pool) *DialogRepo {
	return &DialogRepo{pool: pool}
}

const dialogColumns = `id, title, mode, parent_id, task_id, subjects, categories, pinned, created_at, updated_at`

const messageColumns = `id, dialog_id, seq, role, content, tool_calls, tool_call_id, name, created_at`

func (r *DialogRepo) CreateDialog(ctx context.Context, mode, title string, parentID *uuid.UUID) (domain.Dialog, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO chat_dialogs (mode, title, parent_id)
		 VALUES ($1, $2, $3)
		 RETURNING `+dialogColumns,
		mode, title, parentID)

	d, err := scanDialogRow(row)
	if err != nil {
		return domain.Dialog{}, fmt.Errorf("insert chat dialog: %w", err)
	}
	return d, nil
}

func (r *DialogRepo) ListRecentDialogs(ctx context.Context, limit, offset int) ([]domain.Dialog, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+dialogColumns+`
		 FROM chat_dialogs
		 WHERE parent_id IS NULL AND mode NOT IN ('discuss', 'incident') AND pinned = false
		 ORDER BY updated_at DESC
		 LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list chat dialogs: %w", err)
	}
	defer rows.Close()

	var dialogs []domain.Dialog
	for rows.Next() {
		d, err := scanDialog(rows)
		if err != nil {
			return nil, fmt.Errorf("scan chat dialog: %w", err)
		}
		dialogs = append(dialogs, d)
	}
	return dialogs, rows.Err()
}

func (r *DialogRepo) CountDialogs(ctx context.Context) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM chat_dialogs WHERE parent_id IS NULL AND mode NOT IN ('discuss', 'incident') AND pinned = false`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count chat dialogs: %w", err)
	}
	return count, nil
}

func (r *DialogRepo) ListDialogsByMode(ctx context.Context, mode string, limit, offset int) ([]domain.Dialog, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+dialogColumns+`
		 FROM chat_dialogs
		 WHERE parent_id IS NULL AND mode = $1
		 ORDER BY updated_at DESC
		 LIMIT $2 OFFSET $3`, mode, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list chat dialogs by mode: %w", err)
	}
	defer rows.Close()

	var dialogs []domain.Dialog
	for rows.Next() {
		d, err := scanDialog(rows)
		if err != nil {
			return nil, fmt.Errorf("scan chat dialog: %w", err)
		}
		dialogs = append(dialogs, d)
	}
	return dialogs, rows.Err()
}

func (r *DialogRepo) CountDialogsByMode(ctx context.Context, mode string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM chat_dialogs WHERE parent_id IS NULL AND mode = $1`, mode).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count chat dialogs by mode: %w", err)
	}
	return count, nil
}

func (r *DialogRepo) ListAllRecentDialogs(ctx context.Context, limit, offset int) ([]domain.Dialog, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+dialogColumns+`
		 FROM chat_dialogs
		 ORDER BY updated_at DESC
		 LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list all chat dialogs: %w", err)
	}
	defer rows.Close()

	var dialogs []domain.Dialog
	for rows.Next() {
		d, err := scanDialog(rows)
		if err != nil {
			return nil, fmt.Errorf("scan chat dialog: %w", err)
		}
		dialogs = append(dialogs, d)
	}
	return dialogs, rows.Err()
}

func (r *DialogRepo) CountAllDialogs(ctx context.Context) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM chat_dialogs`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count all chat dialogs: %w", err)
	}
	return count, nil
}

func escapeILIKEPattern(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

func (r *DialogRepo) SearchDialogs(ctx context.Context, mode, query string, limit, offset int) ([]domain.Dialog, error) {
	pattern := "%" + escapeILIKEPattern(query) + "%"
	var rows pgx.Rows
	var err error

	if mode == "" {
		rows, err = r.pool.Query(ctx,
			`SELECT `+dialogColumns+`
			 FROM chat_dialogs
			 WHERE parent_id IS NULL AND mode NOT IN ('discuss', 'incident')
			   AND title ILIKE $1 ESCAPE '\'
			 ORDER BY updated_at DESC
			 LIMIT $2 OFFSET $3`, pattern, limit, offset)
	} else {
		rows, err = r.pool.Query(ctx,
			`SELECT `+dialogColumns+`
			 FROM chat_dialogs
			 WHERE parent_id IS NULL AND mode = $1
			   AND title ILIKE $2 ESCAPE '\'
			 ORDER BY updated_at DESC
			 LIMIT $3 OFFSET $4`, mode, pattern, limit, offset)
	}
	if err != nil {
		return nil, fmt.Errorf("search chat dialogs: %w", err)
	}
	defer rows.Close()

	var dialogs []domain.Dialog
	for rows.Next() {
		d, err := scanDialog(rows)
		if err != nil {
			return nil, fmt.Errorf("scan chat dialog: %w", err)
		}
		dialogs = append(dialogs, d)
	}
	return dialogs, rows.Err()
}

func (r *DialogRepo) CountDialogsSearch(ctx context.Context, mode, query string) (int, error) {
	pattern := "%" + escapeILIKEPattern(query) + "%"
	var count int
	var err error

	if mode == "" {
		err = r.pool.QueryRow(ctx,
			`SELECT count(*) FROM chat_dialogs
			 WHERE parent_id IS NULL AND mode NOT IN ('discuss', 'incident')
			   AND title ILIKE $1 ESCAPE '\'`, pattern).Scan(&count)
	} else {
		err = r.pool.QueryRow(ctx,
			`SELECT count(*) FROM chat_dialogs
			 WHERE parent_id IS NULL AND mode = $1
			   AND title ILIKE $2 ESCAPE '\'`, mode, pattern).Scan(&count)
	}
	if err != nil {
		return 0, fmt.Errorf("count search chat dialogs: %w", err)
	}
	return count, nil
}

func (r *DialogRepo) ListChildren(ctx context.Context, parentID uuid.UUID) ([]domain.Dialog, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+dialogColumns+`
		 FROM chat_dialogs
		 WHERE parent_id = $1
		 ORDER BY created_at ASC`, parentID)
	if err != nil {
		return nil, fmt.Errorf("list child dialogs: %w", err)
	}
	defer rows.Close()

	var dialogs []domain.Dialog
	for rows.Next() {
		d, err := scanDialog(rows)
		if err != nil {
			return nil, fmt.Errorf("scan child dialog: %w", err)
		}
		dialogs = append(dialogs, d)
	}
	return dialogs, rows.Err()
}

func (r *DialogRepo) GetDialog(ctx context.Context, id uuid.UUID) (domain.Dialog, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+dialogColumns+`
		 FROM chat_dialogs
		 WHERE id = $1`, id)

	d, err := scanDialogRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Dialog{}, repository.ErrNotFound
		}
		return domain.Dialog{}, fmt.Errorf("get chat dialog: %w", err)
	}
	return d, nil
}

func (r *DialogRepo) UpdateTitle(ctx context.Context, id uuid.UUID, title string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE chat_dialogs SET title = $2, updated_at = now() WHERE id = $1`,
		id, textutil.Sanitize(title))
	if err != nil {
		return fmt.Errorf("update chat dialog title: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *DialogRepo) SetDialogTaskID(ctx context.Context, id uuid.UUID, taskID *string) error {
	if taskID != nil {
		sanitized := textutil.Sanitize(*taskID)
		taskID = &sanitized
	}
	tag, err := r.pool.Exec(ctx,
		`UPDATE chat_dialogs SET task_id = $2, updated_at = now() WHERE id = $1`,
		id, taskID)
	if err != nil {
		return fmt.Errorf("update chat dialog task_id: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *DialogRepo) SetDialogSubjects(ctx context.Context, id uuid.UUID, subjects []string) error {
	if subjects == nil {
		subjects = []string{}
	}
	for i := range subjects {
		subjects[i] = textutil.Sanitize(subjects[i])
	}
	tag, err := r.pool.Exec(ctx,
		`UPDATE chat_dialogs SET subjects = $2, updated_at = now() WHERE id = $1`,
		id, subjects)
	if err != nil {
		return fmt.Errorf("update chat dialog subjects: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *DialogRepo) SetDialogCategories(ctx context.Context, id uuid.UUID, categories []string) error {
	if categories == nil {
		categories = []string{}
	}
	for i := range categories {
		categories[i] = textutil.Sanitize(categories[i])
	}
	tag, err := r.pool.Exec(ctx,
		`UPDATE chat_dialogs SET categories = $2, updated_at = now() WHERE id = $1`,
		id, categories)
	if err != nil {
		return fmt.Errorf("update chat dialog categories: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *DialogRepo) SetDialogPinned(ctx context.Context, id uuid.UUID, pinned bool) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE chat_dialogs
		 SET pinned = $2,
		     pinned_at = CASE WHEN $2 THEN now() ELSE NULL END,
		     updated_at = now()
		 WHERE id = $1`,
		id, pinned)
	if err != nil {
		return fmt.Errorf("update chat dialog pinned: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *DialogRepo) ListPinnedDialogs(ctx context.Context) ([]domain.Dialog, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+dialogColumns+`
		 FROM chat_dialogs
		 WHERE parent_id IS NULL AND pinned = true AND mode NOT IN ('discuss', 'incident')
		 ORDER BY pinned_at DESC NULLS LAST, updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list pinned chat dialogs: %w", err)
	}
	defer rows.Close()

	var dialogs []domain.Dialog
	for rows.Next() {
		d, err := scanDialog(rows)
		if err != nil {
			return nil, fmt.Errorf("scan pinned chat dialog: %w", err)
		}
		dialogs = append(dialogs, d)
	}
	return dialogs, rows.Err()
}

func (r *DialogRepo) DeleteDialog(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM chat_dialogs WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete chat dialog: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *DialogRepo) ListMessages(ctx context.Context, dialogID uuid.UUID) ([]domain.DialogMessage, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+messageColumns+`
		 FROM chat_dialog_messages
		 WHERE dialog_id = $1
		 ORDER BY seq ASC`, dialogID)
	if err != nil {
		return nil, fmt.Errorf("list chat messages: %w", err)
	}
	defer rows.Close()

	var msgs []domain.DialogMessage
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("scan chat message: %w", err)
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

func (r *DialogRepo) AppendMessage(ctx context.Context, dialogID uuid.UUID, msg domain.DialogMessage) (domain.DialogMessage, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.DialogMessage{}, fmt.Errorf("begin append message: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	seq, err := nextDialogMessageSeq(ctx, tx, dialogID)
	if err != nil {
		return domain.DialogMessage{}, err
	}

	toolCalls := nullableJSON(msg.ToolCalls)
	toolCallID := nullableString(textutil.Sanitize(msg.ToolCallID))
	name := nullableString(textutil.Sanitize(msg.Name))
	content := textutil.Sanitize(msg.Content)

	row := tx.QueryRow(ctx,
		`INSERT INTO chat_dialog_messages (dialog_id, seq, role, content, tool_calls, tool_call_id, name)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING `+messageColumns,
		dialogID, seq, msg.Role, content, toolCalls, toolCallID, name)

	result, err := scanMessageRow(row)
	if err != nil {
		return domain.DialogMessage{}, fmt.Errorf("insert chat message: %w", err)
	}

	_, err = tx.Exec(ctx, `UPDATE chat_dialogs SET updated_at = now() WHERE id = $1`, dialogID)
	if err != nil {
		return domain.DialogMessage{}, fmt.Errorf("bump chat dialog updated_at: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.DialogMessage{}, fmt.Errorf("commit append message: %w", err)
	}
	return result, nil
}

func (r *DialogRepo) DeleteMessagesAfterSeq(ctx context.Context, dialogID uuid.UUID, afterSeq int) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete messages: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var lock int
	err = tx.QueryRow(ctx, `SELECT 1 FROM chat_dialogs WHERE id = $1 FOR UPDATE`, dialogID).Scan(&lock)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return repository.ErrNotFound
		}
		return fmt.Errorf("lock chat dialog: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`DELETE FROM chat_dialog_messages WHERE dialog_id = $1 AND seq > $2`,
		dialogID, afterSeq); err != nil {
		return fmt.Errorf("delete chat messages after seq %d: %w", afterSeq, err)
	}

	if _, err := tx.Exec(ctx, `UPDATE chat_dialogs SET updated_at = now() WHERE id = $1`, dialogID); err != nil {
		return fmt.Errorf("bump chat dialog updated_at: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit delete messages: %w", err)
	}
	return nil
}

func nextDialogMessageSeq(ctx context.Context, tx pgx.Tx, dialogID uuid.UUID) (int, error) {
	var lock int
	err := tx.QueryRow(ctx, `SELECT 1 FROM chat_dialogs WHERE id = $1 FOR UPDATE`, dialogID).Scan(&lock)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, repository.ErrNotFound
		}
		return 0, fmt.Errorf("lock chat dialog: %w", err)
	}

	var max sql.NullInt32
	if err = tx.QueryRow(ctx,
		`SELECT MAX(seq) FROM chat_dialog_messages WHERE dialog_id = $1`, dialogID).Scan(&max); err != nil {
		return 0, fmt.Errorf("get max message seq: %w", err)
	}
	if !max.Valid {
		return 0, nil
	}
	return int(max.Int32) + 1, nil
}

func scanDialog(rows pgx.Rows) (domain.Dialog, error) {
	var d domain.Dialog
	err := rows.Scan(&d.ID, &d.Title, &d.Mode, &d.ParentID, &d.TaskID, &d.Subjects, &d.Categories, &d.Pinned, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return domain.Dialog{}, err
	}
	return d, nil
}

func scanDialogRow(row pgx.Row) (domain.Dialog, error) {
	var d domain.Dialog
	err := row.Scan(&d.ID, &d.Title, &d.Mode, &d.ParentID, &d.TaskID, &d.Subjects, &d.Categories, &d.Pinned, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return domain.Dialog{}, err
	}
	return d, nil
}

func scanMessage(rows pgx.Rows) (domain.DialogMessage, error) {
	var m domain.DialogMessage
	var toolCalls []byte
	var toolCallID, name *string
	err := rows.Scan(
		&m.ID, &m.DialogID, &m.Seq, &m.Role, &m.Content,
		&toolCalls, &toolCallID, &name, &m.CreatedAt,
	)
	if err != nil {
		return domain.DialogMessage{}, err
	}
	if len(toolCalls) > 0 {
		m.ToolCalls = toolCalls
	}
	if toolCallID != nil {
		m.ToolCallID = *toolCallID
	}
	if name != nil {
		m.Name = *name
	}
	return m, nil
}

func scanMessageRow(row pgx.Row) (domain.DialogMessage, error) {
	var m domain.DialogMessage
	var toolCalls []byte
	var toolCallID, name *string
	err := row.Scan(
		&m.ID, &m.DialogID, &m.Seq, &m.Role, &m.Content,
		&toolCalls, &toolCallID, &name, &m.CreatedAt,
	)
	if err != nil {
		return domain.DialogMessage{}, err
	}
	if len(toolCalls) > 0 {
		m.ToolCalls = toolCalls
	}
	if toolCallID != nil {
		m.ToolCallID = *toolCallID
	}
	if name != nil {
		m.Name = *name
	}
	return m, nil
}

func nullableJSON(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	return raw
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
