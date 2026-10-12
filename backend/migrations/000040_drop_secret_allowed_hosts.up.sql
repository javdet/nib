-- Per-secret allowed hosts (000039) are withdrawn: a ${NAME} reference in
-- mcp.json resolves for any server again.
ALTER TABLE prompt_secrets
    DROP COLUMN IF EXISTS allowed_hosts;
