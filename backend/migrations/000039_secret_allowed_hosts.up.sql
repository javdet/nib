-- The hosts a secret may be sent to through a ${NAME} reference in mcp.json.
-- Secret values are write-only everywhere else, so without a binding an edit
-- pointing a server at another host would read any secret back out. Empty
-- means none: a secret nobody bound cannot leave through mcp.json at all.
--
-- Secrets that the mcp.json in place at upgrade already references are bound
-- to those servers' hosts once, at boot (cmd/nib/secret_hosts.go), since the
-- file is not something SQL can read.
ALTER TABLE prompt_secrets
    ADD COLUMN allowed_hosts text[] NOT NULL DEFAULT '{}';
