-- A safety net, not the mechanism: every hand-written updated_at bump in the Go
-- code is correct today (checked against every UPDATE on every table that has
-- the column). The risk is the SQL nobody has written yet. One function and a
-- trigger per table removes the class permanently, and now() is transaction
-- start, so it matches what the application already writes.

CREATE FUNCTION set_updated_at() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$;

CREATE TRIGGER chat_dialogs_set_updated_at     BEFORE UPDATE ON chat_dialogs
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER mcp_connections_set_updated_at  BEFORE UPDATE ON mcp_connections
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER prompt_variables_set_updated_at BEFORE UPDATE ON prompt_variables
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER prompt_secrets_set_updated_at   BEFORE UPDATE ON prompt_secrets
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- mcp_tools.discovered_at was misnamed: the upsert sets it to now() on conflict,
-- so it has always been last_seen_at and the real first-discovery time was
-- destroyed on every reindex. Renaming it makes the existing writer correct and
-- adds the column that was actually missing.
ALTER TABLE mcp_tools RENAME COLUMN discovered_at TO last_seen_at;
ALTER TABLE mcp_tools ADD COLUMN first_seen_at timestamptz NOT NULL DEFAULT now();

-- mcp_servers had no timestamps at all, yet it is upserted on every reindex:
-- there was no way to tell when a catalog server was discovered or last seen.
ALTER TABLE mcp_servers
    ADD COLUMN first_seen_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN last_seen_at  timestamptz NOT NULL DEFAULT now();

-- chat_attachments.message_id is mutated when an upload is linked to a message,
-- but the table had created_at only.
ALTER TABLE chat_attachments ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();
CREATE TRIGGER chat_attachments_set_updated_at BEFORE UPDATE ON chat_attachments
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- kb_chunks and kb_collections are deliberately left alone: chunks are
-- delete-then-reinsert and collections are insert-only, so neither is ever
-- updated and created_at is the whole truth.
