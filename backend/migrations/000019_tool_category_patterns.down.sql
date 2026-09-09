-- mcp_server_categories is a fossil: nib never wrote it from any commit. It is
-- recreated here only because toolchain's own catalog does use a table of that
-- name on its own database, so dropping the recreate would make a shared
-- instance strictly worse. It comes back empty either way.
CREATE TABLE mcp_server_categories (
    server_id uuid NOT NULL REFERENCES mcp_servers (id) ON DELETE CASCADE,
    category_id uuid NOT NULL REFERENCES tool_categories (id) ON DELETE CASCADE,
    PRIMARY KEY (server_id, category_id)
);

DROP TABLE IF EXISTS mcp_tool_categories;
DROP TABLE IF EXISTS tool_category_patterns;
