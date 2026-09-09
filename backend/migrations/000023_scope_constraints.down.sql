ALTER TABLE prompt_secrets
    DROP CONSTRAINT IF EXISTS prompt_secrets_scope_name_presence,
    DROP CONSTRAINT IF EXISTS prompt_secrets_scope_check;

ALTER TABLE prompt_variables
    DROP CONSTRAINT IF EXISTS prompt_variables_scope_name_presence,
    DROP CONSTRAINT IF EXISTS prompt_variables_scope_check;
