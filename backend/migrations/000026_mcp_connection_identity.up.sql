-- mcp_connections.name had no UNIQUE and the OAuth callback hard-codes
-- Name = providerType, so re-running the Jira flow left two rows named 'jira',
-- both status='connected'. service/chat.go builds one tool route per connected
-- row, so every tool registered twice and the stale row kept a live refresh
-- token. Nothing looks a connection up by name, which is why this was
-- duplication rather than a wrong answer -- but the duplicates are exactly what
-- feeds the tool router.
--
-- Inspect before applying:
--   SELECT type, name, count(*) FROM mcp_connections GROUP BY 1,2 HAVING count(*) > 1;

-- Newest wins: it holds the tokens from the most recent OAuth exchange.
DELETE FROM mcp_connections c
WHERE EXISTS (
    SELECT 1 FROM mcp_connections newer
    WHERE newer.type = c.type AND newer.name = c.name
      AND (newer.created_at, newer.id) > (c.created_at, c.id)
);

ALTER TABLE mcp_connections
    ADD CONSTRAINT mcp_connections_type_name_key UNIQUE (type, name);
