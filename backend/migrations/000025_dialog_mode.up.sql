-- chat_dialogs.mode defaulted to '' and had no CHECK, and the plan predicate was
-- a blacklist (mode NOT IN ('discuss','incident')). A dialog created with no mode
-- was therefore classified as a plan -- listed, counted, given a planStatus --
-- while mode.IsValid('') is false, so it got no system prompt and no tools.
--
-- 'main' is the backfill because that is the mode a plan starts in, and it is
-- what the blacklist already treated those rows as.

UPDATE chat_dialogs SET mode = 'main' WHERE mode = '';

ALTER TABLE chat_dialogs ALTER COLUMN mode SET DEFAULT 'main';

ALTER TABLE chat_dialogs
    ADD CONSTRAINT chat_dialogs_mode_check
        CHECK (mode IN ('main', 'decompose', 'plan', 'execute', 'discuss', 'incident')) NOT VALID;

ALTER TABLE chat_dialogs VALIDATE CONSTRAINT chat_dialogs_mode_check;

-- One home for "is this a plan", as a whitelist. A mode added later is not a
-- plan until it is named here, instead of becoming one by default.
CREATE OR REPLACE VIEW plan_dialogs AS
SELECT * FROM chat_dialogs
WHERE parent_id IS NULL AND mode IN ('main', 'decompose', 'plan', 'execute');
