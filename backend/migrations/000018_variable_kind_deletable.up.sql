-- Add kind and deletable columns to prompt_variables.

ALTER TABLE prompt_variables
    ADD COLUMN kind text NOT NULL DEFAULT 'string',
    ADD COLUMN deletable boolean NOT NULL DEFAULT true;

ALTER TABLE prompt_variables
    ADD CONSTRAINT prompt_variables_kind_check CHECK (kind IN ('string', 'list'));
