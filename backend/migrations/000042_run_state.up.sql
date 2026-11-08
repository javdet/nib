-- Run state: what a plan or an incident is doing right now, as opposed to what
-- it is. One row per (kind, dialog), the document stored as JSONB in exactly
-- the shape the per-dialog JSON file under DATA_DIR used to hold:
--
--   plan_state    {DATA_DIR}/plan_state/{id}.json
--   plan_fanout   {DATA_DIR}/plan_fanout/{id}.json
--   subagents     {DATA_DIR}/subagents/{id}.json
--   code_fixes    {DATA_DIR}/code_fixes/{id}.json
--   incident_run  {DATA_DIR}/incident_runs/{id}.json
--   action_exec   {DATA_DIR}/action_plans/{id}.exec.json
--   action_runs   {DATA_DIR}/action_plans/{id}.runs.json
--   action_notes  {DATA_DIR}/action_plans/{id}.notes.json
--   stage_run     {DATA_DIR}/action_plans/{id}.stagerun.json
--
-- It moved here so that it is backed up with the transcripts it refers to, and
-- so that more than one backend process can read and write it. The backend
-- imports any of those files still on disk at boot (renaming each to
-- *.migrated), so an upgrade carries the state over.
--
-- A hard reference to chat_dialogs, unlike the statistics tables: run state is
-- meaningless without its dialog, and the files it replaced were simply left
-- behind when a dialog was deleted.
CREATE TABLE run_state (
    kind       text NOT NULL,
    dialog_id  uuid NOT NULL REFERENCES chat_dialogs(id) ON DELETE CASCADE,
    data       jsonb NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (kind, dialog_id)
);

-- The primary key leads with kind, which serves the per-kind sweeps; the
-- cascade from chat_dialogs needs one by dialog.
CREATE INDEX idx_run_state_dialog ON run_state (dialog_id);
