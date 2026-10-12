ALTER TABLE prompt_secrets
    ADD COLUMN IF NOT EXISTS allowed_hosts text[] NOT NULL DEFAULT '{}';
