ALTER TABLE chat_dialogs
    ADD COLUMN IF NOT EXISTS task_id text;
