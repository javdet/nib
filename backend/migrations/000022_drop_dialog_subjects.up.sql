-- Rules are no longer matched by subject name: the plan prompt lists the rule
-- catalog and the planner loads what it needs with get_rule.

ALTER TABLE chat_dialogs
    DROP COLUMN IF EXISTS subjects;
