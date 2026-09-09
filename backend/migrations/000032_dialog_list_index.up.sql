-- The dialog list is the hottest read in the app and had nothing behind it:
--   WHERE parent_id IS NULL AND mode IN (...) AND pinned = false
--   ORDER BY updated_at DESC LIMIT $1 OFFSET $2
-- The only index on chat_dialogs was idx_chat_dialogs_parent (parent_id), which
-- cannot serve the sort, so every page load was a full scan plus a top-N sort.
--
-- The partial predicate matches the parent_id IS NULL filter exactly and keeps
-- the index to root dialogs, which is a small fraction of the table once
-- fan-outs have created their stage and execute children.

CREATE INDEX idx_chat_dialogs_root_updated
    ON chat_dialogs (updated_at DESC)
    WHERE parent_id IS NULL;
