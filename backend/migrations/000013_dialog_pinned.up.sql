ALTER TABLE chat_dialogs
    ADD COLUMN IF NOT EXISTS pinned boolean NOT NULL DEFAULT false;

ALTER TABLE chat_dialogs
    ADD COLUMN IF NOT EXISTS pinned_at timestamptz;
