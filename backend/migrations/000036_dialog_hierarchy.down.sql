ALTER TABLE chat_dialogs
    DROP CONSTRAINT IF EXISTS chat_dialogs_pinned_consistent,
    DROP CONSTRAINT IF EXISTS chat_dialogs_no_self_parent;
