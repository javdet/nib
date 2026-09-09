DROP TRIGGER IF EXISTS chat_attachments_set_updated_at ON chat_attachments;
ALTER TABLE chat_attachments DROP COLUMN IF EXISTS updated_at;

ALTER TABLE mcp_servers
    DROP COLUMN IF EXISTS last_seen_at,
    DROP COLUMN IF EXISTS first_seen_at;

ALTER TABLE mcp_tools DROP COLUMN IF EXISTS first_seen_at;
ALTER TABLE mcp_tools RENAME COLUMN last_seen_at TO discovered_at;

DROP TRIGGER IF EXISTS prompt_secrets_set_updated_at   ON prompt_secrets;
DROP TRIGGER IF EXISTS prompt_variables_set_updated_at ON prompt_variables;
DROP TRIGGER IF EXISTS mcp_connections_set_updated_at  ON mcp_connections;
DROP TRIGGER IF EXISTS chat_dialogs_set_updated_at     ON chat_dialogs;

DROP FUNCTION IF EXISTS set_updated_at();
