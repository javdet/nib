-- The duplicate rows deleted above are not recoverable.
ALTER TABLE mcp_connections
    DROP CONSTRAINT IF EXISTS mcp_connections_type_name_key;
