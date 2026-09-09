-- Give prompt_variables/prompt_secrets the scope contract the Go service already
-- enforces (service.validateScope / validateScopeName). Without it a direct write
-- can create a row the read paths cannot address unambiguously.

ALTER TABLE prompt_variables
    ADD CONSTRAINT prompt_variables_scope_check
        CHECK (scope IN ('global', 'project', 'environment', 'cloud', 'location')) NOT VALID,
    ADD CONSTRAINT prompt_variables_scope_name_presence
        CHECK ((scope =  'global' AND scope_name =  '')
            OR (scope <> 'global' AND scope_name <> '')) NOT VALID;

ALTER TABLE prompt_variables VALIDATE CONSTRAINT prompt_variables_scope_check;
ALTER TABLE prompt_variables VALIDATE CONSTRAINT prompt_variables_scope_name_presence;

ALTER TABLE prompt_secrets
    ADD CONSTRAINT prompt_secrets_scope_check
        CHECK (scope IN ('global', 'project', 'environment', 'cloud', 'location')) NOT VALID,
    ADD CONSTRAINT prompt_secrets_scope_name_presence
        CHECK ((scope =  'global' AND scope_name =  '')
            OR (scope <> 'global' AND scope_name <> '')) NOT VALID;

ALTER TABLE prompt_secrets VALIDATE CONSTRAINT prompt_secrets_scope_check;
ALTER TABLE prompt_secrets VALIDATE CONSTRAINT prompt_secrets_scope_name_presence;
