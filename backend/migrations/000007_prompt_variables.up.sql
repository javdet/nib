-- Prompt variables rendered into system prompts via text/template.

CREATE TABLE prompt_variables (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    scope text NOT NULL DEFAULT 'global',
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    value text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT prompt_variables_scope_name_unique UNIQUE (scope, name)
);

CREATE INDEX idx_prompt_variables_scope ON prompt_variables (scope);
