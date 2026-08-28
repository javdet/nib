-- Persisted chat dialogs and message history for multi-turn conversations.
-- chat_dialog_messages is used instead of chat_messages to avoid colliding with
-- toolchain's chat_sessions/chat_messages tables on the shared Postgres instance.

CREATE TABLE chat_dialogs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    title text NOT NULL DEFAULT '',
    mode text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE chat_dialog_messages (
    id bigserial PRIMARY KEY,
    dialog_id uuid NOT NULL REFERENCES chat_dialogs (id) ON DELETE CASCADE,
    seq integer NOT NULL,
    role text NOT NULL CHECK (role IN ('system', 'user', 'assistant', 'tool')),
    content text NOT NULL DEFAULT '',
    tool_calls jsonb,
    tool_call_id text,
    name text,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chat_dialog_messages_dialog_seq_unique UNIQUE (dialog_id, seq)
);

CREATE INDEX idx_chat_dialog_messages_dialog ON chat_dialog_messages (dialog_id, seq);
