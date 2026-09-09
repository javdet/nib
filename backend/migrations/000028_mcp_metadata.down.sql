ALTER TABLE mcp_connections
    DROP CONSTRAINT IF EXISTS mcp_connections_status_check,
    ALTER COLUMN status SET DEFAULT 'disconnected',
    DROP CONSTRAINT IF EXISTS mcp_connections_metadata_object,
    ALTER COLUMN metadata DROP NOT NULL,
    ALTER COLUMN metadata SET DEFAULT '{}';
