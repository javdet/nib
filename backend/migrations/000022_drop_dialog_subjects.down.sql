ALTER TABLE chat_dialogs
    ADD COLUMN IF NOT EXISTS subjects text[] NOT NULL DEFAULT '{}';
