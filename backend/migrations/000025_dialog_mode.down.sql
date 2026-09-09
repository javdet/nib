DROP VIEW IF EXISTS plan_dialogs;

ALTER TABLE chat_dialogs
    DROP CONSTRAINT IF EXISTS chat_dialogs_mode_check;

ALTER TABLE chat_dialogs ALTER COLUMN mode SET DEFAULT '';
