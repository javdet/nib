-- Remove kind and deletable columns from prompt_variables.

ALTER TABLE prompt_variables DROP CONSTRAINT IF EXISTS prompt_variables_kind_check;

ALTER TABLE prompt_variables
    DROP COLUMN IF EXISTS deletable,
    DROP COLUMN IF EXISTS kind;
