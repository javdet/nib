ALTER TABLE chat_dialogs
    ADD COLUMN IF NOT EXISTS parent_id uuid REFERENCES chat_dialogs (id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_chat_dialogs_parent ON chat_dialogs (parent_id);
