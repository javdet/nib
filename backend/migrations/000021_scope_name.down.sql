-- Remove scope_name from prompt variables and secrets.

ALTER TABLE prompt_secrets
    DROP CONSTRAINT IF EXISTS prompt_secrets_scope_scope_name_name_unique;

ALTER TABLE prompt_secrets
    ADD CONSTRAINT prompt_secrets_scope_name_unique UNIQUE (scope, name);

ALTER TABLE prompt_secrets
    DROP COLUMN IF EXISTS scope_name;

ALTER TABLE prompt_variables
    DROP CONSTRAINT IF EXISTS prompt_variables_scope_scope_name_name_unique;

ALTER TABLE prompt_variables
    ADD CONSTRAINT prompt_variables_scope_name_unique UNIQUE (scope, name);

ALTER TABLE prompt_variables
    DROP COLUMN IF EXISTS scope_name;
