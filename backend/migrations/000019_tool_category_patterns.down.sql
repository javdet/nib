CREATE TABLE mcp_server_categories (
    server_id uuid NOT NULL REFERENCES mcp_servers (id) ON DELETE CASCADE,
    category_id uuid NOT NULL REFERENCES tool_categories (id) ON DELETE CASCADE,
    PRIMARY KEY (server_id, category_id)
);

DROP TABLE IF EXISTS mcp_tool_categories;
DROP TABLE IF EXISTS tool_category_patterns;
