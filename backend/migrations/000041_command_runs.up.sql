-- One row per command an agent ran through run_command or wait_until.
--
-- The audit trail of what the model executed on the backend host: the script
-- verbatim, which sandbox user it ran as, and how it ended. The output itself
-- lives on the data volume ({DATA_DIR}/command_logs/{plan}/{id}.log) and is
-- swept after commands.logRetentionDays; the row is kept.
--
-- plan_id and dialog_id are soft references, like the statistics tables: the
-- record of what ran must outlive an operator deleting the plan, and the
-- stateless chat endpoint has no dialog at all (both are NULL for it).
CREATE TABLE command_runs (
    id             uuid PRIMARY KEY,
    plan_id        uuid,
    dialog_id      uuid,
    -- The action-plan row an action executor ran it for ("s0.step1"); empty
    -- for a Digger, a follow-up turn or anything else outside a plan row.
    action_key     text NOT NULL DEFAULT '',
    mode           text NOT NULL DEFAULT '',
    kind           text NOT NULL,
    script         text NOT NULL,
    -- false when the backend could not switch uids and the command ran as the
    -- backend's own user; uid is -1 then.
    isolated       boolean NOT NULL,
    uid            integer NOT NULL,
    status         text NOT NULL,
    exit_code      integer,
    reason         text NOT NULL DEFAULT '',
    output_bytes   bigint NOT NULL DEFAULT 0,
    output_capped  boolean NOT NULL DEFAULT false,
    started_at     timestamptz NOT NULL,
    finished_at    timestamptz,
    duration_ms    bigint NOT NULL DEFAULT 0
);

CREATE INDEX idx_command_runs_plan_started ON command_runs (plan_id, started_at DESC);
-- Boot marks every row a restart stranded as lost, which is a scan by status.
CREATE INDEX idx_command_runs_running ON command_runs (status) WHERE status = 'running';
