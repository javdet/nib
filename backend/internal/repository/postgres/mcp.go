package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ repository.MCPRepository = (*MCPRepo)(nil)

type MCPRepo struct {
	pool *pgxpool.Pool
}

func NewMCPRepo(pool *pgxpool.Pool) *MCPRepo {
	return &MCPRepo{pool: pool}
}

func (r *MCPRepo) List(ctx context.Context) ([]domain.MCPConnection, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, type, name, server_url, auth_method,
		        access_token, refresh_token, token_expires_at,
		        api_token, status, metadata, created_at, updated_at
		 FROM mcp_connections
		 ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list mcp connections: %w", err)
	}
	defer rows.Close()

	var conns []domain.MCPConnection
	for rows.Next() {
		c, err := scanConnection(rows)
		if err != nil {
			return nil, fmt.Errorf("scan mcp connection: %w", err)
		}
		conns = append(conns, c)
	}
	return conns, rows.Err()
}

func (r *MCPRepo) GetByID(ctx context.Context, id uuid.UUID) (domain.MCPConnection, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, type, name, server_url, auth_method,
		        access_token, refresh_token, token_expires_at,
		        api_token, status, metadata, created_at, updated_at
		 FROM mcp_connections
		 WHERE id = $1`, id)

	c, err := scanConnectionRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.MCPConnection{}, repository.ErrNotFound
		}
		return domain.MCPConnection{}, fmt.Errorf("get mcp connection: %w", err)
	}
	return c, nil
}

func (r *MCPRepo) Create(ctx context.Context, conn domain.MCPConnection) (domain.MCPConnection, error) {
	meta := conn.Metadata
	if meta == nil {
		meta = json.RawMessage("{}")
	}

	row := r.pool.QueryRow(ctx,
		`INSERT INTO mcp_connections (type, name, server_url, auth_method,
		        access_token, refresh_token, token_expires_at,
		        api_token, status, metadata)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 RETURNING id, type, name, server_url, auth_method,
		           access_token, refresh_token, token_expires_at,
		           api_token, status, metadata, created_at, updated_at`,
		conn.Type, conn.Name, conn.ServerURL, conn.AuthMethod,
		nilIfEmpty(conn.AccessToken), nilIfEmpty(conn.RefreshToken), conn.TokenExpiresAt,
		nilIfEmpty(conn.APIToken), conn.Status, meta)

	result, err := scanConnectionRow(row)
	if err != nil {
		return domain.MCPConnection{}, fmt.Errorf("insert mcp connection: %w", err)
	}
	return result, nil
}

func (r *MCPRepo) Update(ctx context.Context, id uuid.UUID, conn domain.MCPConnection) (domain.MCPConnection, error) {
	meta := conn.Metadata
	if meta == nil {
		meta = json.RawMessage("{}")
	}

	row := r.pool.QueryRow(ctx,
		`UPDATE mcp_connections
		 SET name = $2, server_url = $3, api_token = COALESCE(NULLIF($4, ''), api_token),
		     metadata = $5, updated_at = now()
		 WHERE id = $1
		 RETURNING id, type, name, server_url, auth_method,
		           access_token, refresh_token, token_expires_at,
		           api_token, status, metadata, created_at, updated_at`,
		id, conn.Name, conn.ServerURL, nilIfEmpty(conn.APIToken), meta)

	result, err := scanConnectionRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.MCPConnection{}, repository.ErrNotFound
		}
		return domain.MCPConnection{}, fmt.Errorf("update mcp connection: %w", err)
	}
	return result, nil
}

func (r *MCPRepo) UpdateTokens(ctx context.Context, id uuid.UUID, accessToken, refreshToken string, expiresAt *time.Time) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE mcp_connections
		 SET access_token = $2, refresh_token = $3, token_expires_at = $4,
		     status = 'connected', updated_at = now()
		 WHERE id = $1`,
		id, accessToken, nilIfEmpty(refreshToken), expiresAt)
	if err != nil {
		return fmt.Errorf("update mcp tokens: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *MCPRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE mcp_connections SET status = $2, updated_at = now() WHERE id = $1`,
		id, status)
	if err != nil {
		return fmt.Errorf("update mcp status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *MCPRepo) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM mcp_connections WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete mcp connection: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

// scanConnection reads a full row from pgx.Rows into a domain.MCPConnection.
func scanConnection(rows pgx.Rows) (domain.MCPConnection, error) {
	var c domain.MCPConnection
	var accessToken, refreshToken, apiToken *string
	var meta []byte

	err := rows.Scan(
		&c.ID, &c.Type, &c.Name, &c.ServerURL, &c.AuthMethod,
		&accessToken, &refreshToken, &c.TokenExpiresAt,
		&apiToken, &c.Status, &meta, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return domain.MCPConnection{}, err
	}

	c.AccessToken = derefStr(accessToken)
	c.RefreshToken = derefStr(refreshToken)
	c.APIToken = derefStr(apiToken)
	c.Metadata = meta
	return c, nil
}

// scanConnectionRow reads a single pgx.Row into a domain.MCPConnection.
func scanConnectionRow(row pgx.Row) (domain.MCPConnection, error) {
	var c domain.MCPConnection
	var accessToken, refreshToken, apiToken *string
	var meta []byte

	err := row.Scan(
		&c.ID, &c.Type, &c.Name, &c.ServerURL, &c.AuthMethod,
		&accessToken, &refreshToken, &c.TokenExpiresAt,
		&apiToken, &c.Status, &meta, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return domain.MCPConnection{}, err
	}

	c.AccessToken = derefStr(accessToken)
	c.RefreshToken = derefStr(refreshToken)
	c.APIToken = derefStr(apiToken)
	c.Metadata = meta
	return c, nil
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
