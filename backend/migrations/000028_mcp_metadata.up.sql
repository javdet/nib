-- metadata was nullable JSONB with a default, and three states reached readers:
-- SQL NULL (any direct INSERT), JSON null (a request body of "metadata": null --
-- json.RawMessage unmarshals that to the 4 bytes `null`, which is non-nil and so
-- sails past the meta == nil guard in the repository), and {}. Collapse them.

UPDATE mcp_connections SET metadata = '{}'::jsonb
WHERE metadata IS NULL OR jsonb_typeof(metadata) <> 'object';

ALTER TABLE mcp_connections
    ALTER COLUMN metadata SET NOT NULL,
    ALTER COLUMN metadata SET DEFAULT '{}'::jsonb,
    ADD CONSTRAINT mcp_connections_metadata_object
        CHECK (jsonb_typeof(metadata) = 'object');

-- status defaulted to 'disconnected', a state no Go code writes or reads. The
-- live values are connecting/connected/error, and 'connected' is the only one
-- ever compared (service/chat.go, when building the tool routes).
UPDATE mcp_connections SET status = 'error'
WHERE status NOT IN ('connecting', 'connected', 'error');

ALTER TABLE mcp_connections
    ALTER COLUMN status SET DEFAULT 'connecting',
    ADD CONSTRAINT mcp_connections_status_check
        CHECK (status IN ('connecting', 'connected', 'error'));
