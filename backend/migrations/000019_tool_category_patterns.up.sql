-- Tool-level category assignment via patterns (exact name or prefix*).
-- Idempotent: the toolcatalog integration tests apply this file directly to a dev database.

CREATE TABLE IF NOT EXISTS tool_category_patterns (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    category_id uuid NOT NULL REFERENCES tool_categories (id) ON DELETE CASCADE,
    pattern text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT tool_category_patterns_category_pattern_unique UNIQUE (category_id, pattern)
);

CREATE INDEX IF NOT EXISTS idx_tool_category_patterns_category_id ON tool_category_patterns (category_id);

CREATE TABLE IF NOT EXISTS mcp_tool_categories (
    tool_id uuid NOT NULL REFERENCES mcp_tools (id) ON DELETE CASCADE,
    category_id uuid NOT NULL REFERENCES tool_categories (id) ON DELETE CASCADE,
    PRIMARY KEY (tool_id, category_id)
);

CREATE INDEX IF NOT EXISTS idx_mcp_tool_categories_category_id ON mcp_tool_categories (category_id);

DROP TABLE IF EXISTS mcp_server_categories;
