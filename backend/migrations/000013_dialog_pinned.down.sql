ALTER TABLE chat_dialogs
    DROP COLUMN IF EXISTS pinned_at;

ALTER TABLE chat_dialogs
    DROP COLUMN IF EXISTS pinned;
