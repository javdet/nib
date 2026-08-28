-- Chat message attachments (text files and images) stored on disk with metadata in Postgres.

CREATE TABLE chat_attachments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    dialog_id uuid NOT NULL REFERENCES chat_dialogs (id) ON DELETE CASCADE,
    message_id bigint REFERENCES chat_dialog_messages (id) ON DELETE CASCADE,
    filename text NOT NULL,
    content_type text NOT NULL DEFAULT '',
    kind text NOT NULL CHECK (kind IN ('image', 'text')),
    size_bytes bigint NOT NULL DEFAULT 0,
    path text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_chat_attachments_dialog ON chat_attachments (dialog_id);
CREATE INDEX idx_chat_attachments_message ON chat_attachments (message_id);
