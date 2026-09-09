-- Remove scope_name from prompt variables and secrets.
--
-- This rollback is lossy by construction, and it could not run at all before the
-- deletes below were added: 000021 exists precisely so several entities can hold
-- the same (scope, name), and re-adding UNIQUE (scope, name) over rows that use
-- that fails on the first duplicate -- which is exactly when a rollback would be
-- wanted. Collapsing to one row per (scope, name) is the only way back; the
-- lowest (scope_name, id) survives so the choice is at least deterministic.

DELETE FROM prompt_secrets s
WHERE EXISTS (
    SELECT 1 FROM prompt_secrets keep
    WHERE keep.scope = s.scope AND keep.name = s.name
      AND (keep.scope_name, keep.id) < (s.scope_name, s.id)
);

ALTER TABLE prompt_secrets
    DROP CONSTRAINT IF EXISTS prompt_secrets_scope_scope_name_name_unique;

ALTER TABLE prompt_secrets
    ADD CONSTRAINT prompt_secrets_scope_name_unique UNIQUE (scope, name);

ALTER TABLE prompt_secrets
    DROP COLUMN IF EXISTS scope_name;

DELETE FROM prompt_variables v
WHERE EXISTS (
    SELECT 1 FROM prompt_variables keep
    WHERE keep.scope = v.scope AND keep.name = v.name
      AND (keep.scope_name, keep.id) < (v.scope_name, v.id)
);

ALTER TABLE prompt_variables
    DROP CONSTRAINT IF EXISTS prompt_variables_scope_scope_name_name_unique;

ALTER TABLE prompt_variables
    ADD CONSTRAINT prompt_variables_scope_name_unique UNIQUE (scope, name);

ALTER TABLE prompt_variables
    DROP COLUMN IF EXISTS scope_name;
