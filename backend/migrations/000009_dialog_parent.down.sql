DROP INDEX IF EXISTS idx_chat_dialogs_parent;

ALTER TABLE chat_dialogs
    DROP COLUMN IF EXISTS parent_id;
