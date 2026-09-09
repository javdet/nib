-- parent_id = id is legal SQL and reachable through the public API: two of the
-- six CreateDialog call sites take a parent straight from the request with no
-- root resolution. resolveRootDialogID already walks parents with a cycle set
-- and a depth cap, which is the tell that nobody trusted the schema's claim that
-- this cannot happen.
--
-- Depth cannot be enforced by a CHECK -- it is row-local -- so the one level
-- contract is enforced in the service (DialogService.Create rejects a parent
-- that has a parent). This closes the only cycle SQL can close on its own.

ALTER TABLE chat_dialogs
    ADD CONSTRAINT chat_dialogs_no_self_parent
        CHECK (parent_id IS NULL OR parent_id <> id) NOT VALID;

ALTER TABLE chat_dialogs VALIDATE CONSTRAINT chat_dialogs_no_self_parent;

-- pinned is fully derived from pinned_at: SetDialogPinned is the only writer and
-- maintains the pair by construction, but nothing enforced it, and a desync
-- makes a pinned dialog invisible -- ListPinnedDialogs filters on pinned = true
-- and orders by pinned_at.
ALTER TABLE chat_dialogs
    ADD CONSTRAINT chat_dialogs_pinned_consistent
        CHECK (pinned = (pinned_at IS NOT NULL)) NOT VALID;

ALTER TABLE chat_dialogs VALIDATE CONSTRAINT chat_dialogs_pinned_consistent;
