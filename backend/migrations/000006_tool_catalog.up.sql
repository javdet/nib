-- MCP tool catalog: categories, servers, tools with full-text search (tsvector + GIN).

CREATE TABLE tool_categories (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL UNIQUE,
    description text NOT NULL DEFAULT ''
);

CREATE TABLE mcp_servers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL UNIQUE,
    description text NOT NULL DEFAULT '',
    url text NOT NULL,
    fts tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('simple', coalesce(name, '')), 'A') ||
        setweight(to_tsvector('english', coalesce(description, '')), 'B')
    ) STORED
);

CREATE INDEX idx_mcp_servers_fts ON mcp_servers USING GIN (fts);

CREATE TABLE mcp_server_categories (
    server_id uuid NOT NULL REFERENCES mcp_servers (id) ON DELETE CASCADE,
    category_id uuid NOT NULL REFERENCES tool_categories (id) ON DELETE CASCADE,
    PRIMARY KEY (server_id, category_id)
);

CREATE TABLE mcp_tools (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id uuid NOT NULL REFERENCES mcp_servers (id) ON DELETE CASCADE,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    input_schema jsonb NOT NULL DEFAULT '{}'::jsonb,
    discovered_at timestamptz NOT NULL DEFAULT now(),
    fts tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('simple', coalesce(name, '')), 'A') ||
        setweight(to_tsvector('english', coalesce(description, '')), 'B')
    ) STORED,
    CONSTRAINT mcp_tools_server_name_unique UNIQUE (server_id, name)
);

CREATE INDEX idx_mcp_tools_fts ON mcp_tools USING GIN (fts);
CREATE INDEX idx_mcp_tools_server_id ON mcp_tools (server_id);
