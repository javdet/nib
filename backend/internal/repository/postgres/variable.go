package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ repository.VariableRepository = (*VariableRepo)(nil)

// VariableRepo implements VariableRepository using PostgreSQL.
type VariableRepo struct {
	pool *pgxpool.Pool
}

func NewVariableRepo(pool *pgxpool.Pool) *VariableRepo {
	return &VariableRepo{pool: pool}
}

const variableColumns = `id, scope, scope_name, name, description, value, kind, deletable, created_at, updated_at`

func (r *VariableRepo) List(ctx context.Context) ([]domain.PromptVariable, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+variableColumns+`
		 FROM prompt_variables
		 ORDER BY scope ASC, name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list prompt variables: %w", err)
	}
	defer rows.Close()

	var vars []domain.PromptVariable
	for rows.Next() {
		v, err := scanVariable(rows)
		if err != nil {
			return nil, fmt.Errorf("scan prompt variable: %w", err)
		}
		vars = append(vars, v)
	}
	return vars, rows.Err()
}

func (r *VariableRepo) GetByID(ctx context.Context, id uuid.UUID) (domain.PromptVariable, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+variableColumns+`
		 FROM prompt_variables
		 WHERE id = $1`, id)

	v, err := scanVariableRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.PromptVariable{}, repository.ErrNotFound
		}
		return domain.PromptVariable{}, fmt.Errorf("get prompt variable: %w", err)
	}
	return v, nil
}

func (r *VariableRepo) Create(ctx context.Context, v domain.PromptVariable) (domain.PromptVariable, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO prompt_variables (scope, scope_name, name, description, value, kind, deletable)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING `+variableColumns,
		v.Scope, v.ScopeName, v.Name, v.Description, v.Value, v.Kind, v.Deletable)

	result, err := scanVariableRow(row)
	if err != nil {
		if isUniqueViolation(err) {
			return domain.PromptVariable{}, repository.ErrAlreadyExists
		}
		return domain.PromptVariable{}, fmt.Errorf("insert prompt variable: %w", err)
	}
	return result, nil
}

func (r *VariableRepo) Update(ctx context.Context, id uuid.UUID, v domain.PromptVariable) (domain.PromptVariable, error) {
	row := r.pool.QueryRow(ctx,
		`UPDATE prompt_variables
		 SET scope = $2, scope_name = $3, name = $4, description = $5, value = $6, kind = $7, deletable = $8, updated_at = now()
		 WHERE id = $1
		 RETURNING `+variableColumns,
		id, v.Scope, v.ScopeName, v.Name, v.Description, v.Value, v.Kind, v.Deletable)

	result, err := scanVariableRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.PromptVariable{}, repository.ErrNotFound
		}
		if isUniqueViolation(err) {
			return domain.PromptVariable{}, repository.ErrAlreadyExists
		}
		return domain.PromptVariable{}, fmt.Errorf("update prompt variable: %w", err)
	}
	return result, nil
}

func (r *VariableRepo) EnsureExists(ctx context.Context, v domain.PromptVariable) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO prompt_variables (scope, scope_name, name, description, value, kind, deletable)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (scope, scope_name, name) DO NOTHING`,
		v.Scope, v.ScopeName, v.Name, v.Description, v.Value, v.Kind, v.Deletable)
	if err != nil {
		return fmt.Errorf("ensure prompt variable: %w", err)
	}
	return nil
}

func (r *VariableRepo) EnsureBuiltin(ctx context.Context, v domain.PromptVariable) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO prompt_variables (scope, scope_name, name, description, value, kind, deletable)
		 VALUES ($1, $2, $3, $4, $5, $6, false)
		 ON CONFLICT (scope, scope_name, name)
		 DO UPDATE SET kind = EXCLUDED.kind, deletable = false, updated_at = now()`,
		v.Scope, v.ScopeName, v.Name, v.Description, v.Value, v.Kind)
	if err != nil {
		return fmt.Errorf("ensure builtin prompt variable: %w", err)
	}
	return nil
}

func (r *VariableRepo) Upsert(ctx context.Context, v domain.PromptVariable) (domain.PromptVariable, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO prompt_variables (scope, scope_name, name, description, value, kind, deletable)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (scope, scope_name, name)
		 DO UPDATE SET value = EXCLUDED.value, updated_at = now()
		 RETURNING `+variableColumns,
		v.Scope, v.ScopeName, v.Name, v.Description, v.Value, v.Kind, v.Deletable)

	result, err := scanVariableRow(row)
	if err != nil {
		return domain.PromptVariable{}, fmt.Errorf("upsert prompt variable: %w", err)
	}
	return result, nil
}

func (r *VariableRepo) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM prompt_variables WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete prompt variable: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *VariableRepo) LoadAll(ctx context.Context) (map[string]map[string]any, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT scope, name, value, kind FROM prompt_variables`)
	if err != nil {
		return nil, fmt.Errorf("load prompt variables: %w", err)
	}
	defer rows.Close()

	out := make(map[string]map[string]any)
	for rows.Next() {
		var scope, name, value, kind string
		if err := rows.Scan(&scope, &name, &value, &kind); err != nil {
			return nil, fmt.Errorf("scan prompt variable: %w", err)
		}
		if _, ok := out[scope]; !ok {
			out[scope] = make(map[string]any)
		}
		out[scope][name] = parseVariableValue(kind, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate prompt variables: %w", err)
	}
	return out, nil
}

func parseVariableValue(kind, value string) any {
	if kind == "list" {
		var items []string
		if err := json.Unmarshal([]byte(value), &items); err != nil {
			return []string{}
		}
		return items
	}
	return value
}

func scanVariable(rows pgx.Rows) (domain.PromptVariable, error) {
	var v domain.PromptVariable
	err := rows.Scan(&v.ID, &v.Scope, &v.ScopeName, &v.Name, &v.Description, &v.Value, &v.Kind, &v.Deletable, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return domain.PromptVariable{}, err
	}
	return v, nil
}

func scanVariableRow(row pgx.Row) (domain.PromptVariable, error) {
	var v domain.PromptVariable
	err := row.Scan(&v.ID, &v.Scope, &v.ScopeName, &v.Name, &v.Description, &v.Value, &v.Kind, &v.Deletable, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return domain.PromptVariable{}, err
	}
	return v, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
