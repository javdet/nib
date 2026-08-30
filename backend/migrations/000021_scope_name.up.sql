-- Add scope_name to prompt variables and secrets for entity-level scoping.

ALTER TABLE prompt_variables
    ADD COLUMN scope_name text NOT NULL DEFAULT '';

ALTER TABLE prompt_variables
    DROP CONSTRAINT prompt_variables_scope_name_unique;

ALTER TABLE prompt_variables
    ADD CONSTRAINT prompt_variables_scope_scope_name_name_unique
        UNIQUE (scope, scope_name, name);

ALTER TABLE prompt_secrets
    ADD COLUMN scope_name text NOT NULL DEFAULT '';

ALTER TABLE prompt_secrets
    DROP CONSTRAINT prompt_secrets_scope_name_unique;

ALTER TABLE prompt_secrets
    ADD CONSTRAINT prompt_secrets_scope_scope_name_name_unique
        UNIQUE (scope, scope_name, name);
