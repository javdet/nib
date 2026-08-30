package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ repository.SecretRepository = (*SecretRepo)(nil)

// SecretRepo implements SecretRepository using PostgreSQL.
type SecretRepo struct {
	pool *pgxpool.Pool
}

func NewSecretRepo(pool *pgxpool.Pool) *SecretRepo {
	return &SecretRepo{pool: pool}
}

const secretColumns = `id, scope, scope_name, name, description, created_at, updated_at`

func (r *SecretRepo) List(ctx context.Context) ([]domain.PromptSecret, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+secretColumns+`
		 FROM prompt_secrets
		 ORDER BY scope ASC, name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list prompt secrets: %w", err)
	}
	defer rows.Close()

	var secrets []domain.PromptSecret
	for rows.Next() {
		s, err := scanSecret(rows)
		if err != nil {
			return nil, fmt.Errorf("scan prompt secret: %w", err)
		}
		secrets = append(secrets, s)
	}
	return secrets, rows.Err()
}

func (r *SecretRepo) GetByID(ctx context.Context, id uuid.UUID) (domain.PromptSecret, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+secretColumns+`
		 FROM prompt_secrets
		 WHERE id = $1`, id)

	s, err := scanSecretRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.PromptSecret{}, repository.ErrNotFound
		}
		return domain.PromptSecret{}, fmt.Errorf("get prompt secret: %w", err)
	}
	return s, nil
}

func (r *SecretRepo) GetByName(ctx context.Context, scope, name string) (domain.PromptSecret, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+secretColumns+`
		 FROM prompt_secrets
		 WHERE scope = $1 AND name = $2`, scope, name)

	s, err := scanSecretRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.PromptSecret{}, repository.ErrNotFound
		}
		return domain.PromptSecret{}, fmt.Errorf("get prompt secret by name: %w", err)
	}
	return s, nil
}

func (r *SecretRepo) GetEncrypted(ctx context.Context, id uuid.UUID) ([]byte, error) {
	var encrypted []byte
	err := r.pool.QueryRow(ctx,
		`SELECT value_encrypted FROM prompt_secrets WHERE id = $1`, id).Scan(&encrypted)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("get encrypted secret: %w", err)
	}
	return encrypted, nil
}

func (r *SecretRepo) GetEncryptedByName(ctx context.Context, scope, name string) ([]byte, error) {
	var encrypted []byte
	err := r.pool.QueryRow(ctx,
		`SELECT value_encrypted FROM prompt_secrets WHERE scope = $1 AND name = $2`,
		scope, name).Scan(&encrypted)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("get encrypted secret by name: %w", err)
	}
	return encrypted, nil
}

func (r *SecretRepo) Create(ctx context.Context, s domain.PromptSecret, encrypted []byte) (domain.PromptSecret, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO prompt_secrets (scope, scope_name, name, description, value_encrypted)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING `+secretColumns,
		s.Scope, s.ScopeName, s.Name, s.Description, encrypted)

	result, err := scanSecretRow(row)
	if err != nil {
		if isUniqueViolation(err) {
			return domain.PromptSecret{}, repository.ErrAlreadyExists
		}
		return domain.PromptSecret{}, fmt.Errorf("insert prompt secret: %w", err)
	}
	return result, nil
}

func (r *SecretRepo) Update(ctx context.Context, id uuid.UUID, s domain.PromptSecret, encrypted []byte) (domain.PromptSecret, error) {
	row := r.pool.QueryRow(ctx,
		`UPDATE prompt_secrets
		 SET scope = $2, scope_name = $3, name = $4, description = $5, value_encrypted = $6, updated_at = now()
		 WHERE id = $1
		 RETURNING `+secretColumns,
		id, s.Scope, s.ScopeName, s.Name, s.Description, encrypted)

	result, err := scanSecretRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.PromptSecret{}, repository.ErrNotFound
		}
		if isUniqueViolation(err) {
			return domain.PromptSecret{}, repository.ErrAlreadyExists
		}
		return domain.PromptSecret{}, fmt.Errorf("update prompt secret: %w", err)
	}
	return result, nil
}

func (r *SecretRepo) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM prompt_secrets WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete prompt secret: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func scanSecret(rows pgx.Rows) (domain.PromptSecret, error) {
	var s domain.PromptSecret
	err := rows.Scan(&s.ID, &s.Scope, &s.ScopeName, &s.Name, &s.Description, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return domain.PromptSecret{}, err
	}
	return s, nil
}

func scanSecretRow(row pgx.Row) (domain.PromptSecret, error) {
	var s domain.PromptSecret
	err := row.Scan(&s.ID, &s.Scope, &s.ScopeName, &s.Name, &s.Description, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return domain.PromptSecret{}, err
	}
	return s, nil
}
