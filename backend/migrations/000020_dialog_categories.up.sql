ALTER TABLE chat_dialogs
    ADD COLUMN IF NOT EXISTS categories text[] NOT NULL DEFAULT '{}';
