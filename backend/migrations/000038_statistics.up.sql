-- Persisted usage statistics: LLM calls, container agent runs and plan status
-- history. Nothing recorded any of this before -- token counts were dropped by
-- llm.AssistantMessage, cost existed only as a Prometheus counter and as text in
-- a chat message, and plan status was a single current value in a JSON file with
-- no history at all.

-- One row per LLM call.
--
-- No foreign key to chat_dialogs, unlike every other table here. Statistics are
-- kept indefinitely, and ON DELETE CASCADE would erase the record of what a
-- dialog cost the moment an operator deleted it; ON DELETE SET NULL would keep
-- the row but throw away the attribution that makes it worth keeping. So both
-- ids are soft references: any join back to chat_dialogs must be a LEFT JOIN and
-- must tolerate a dialog that is gone.
--
-- dialog_id and plan_id are also NULL by design for POST /api/v1/chat, which
-- runs an agent loop that belongs to no dialog.
CREATE TABLE llm_usage (
    id                   bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    created_at           timestamptz NOT NULL DEFAULT now(),
    dialog_id            uuid,
    plan_id              uuid,
    mode                 text NOT NULL DEFAULT '',
    operation            text NOT NULL,
    model                text NOT NULL DEFAULT '',
    -- cached_prompt_tokens is a SUBSET of prompt_tokens, and reasoning_tokens a
    -- SUBSET of completion_tokens: that is how both providers report them. They
    -- are "of which" figures, so stacking all four in a chart double-counts.
    prompt_tokens        integer NOT NULL DEFAULT 0,
    cached_prompt_tokens integer NOT NULL DEFAULT 0,
    completion_tokens    integer NOT NULL DEFAULT 0,
    reasoning_tokens     integer NOT NULL DEFAULT 0,
    total_tokens         integer NOT NULL DEFAULT 0,
    -- NULL means "the provider did not price this call", which is not "free".
    -- OpenRouter returns usage.cost; the OpenAI platform never does, and nib
    -- deliberately carries no rate card. Nullable so a pricing table could
    -- backfill later without a schema change.
    --
    -- numeric(18,10), not (12,6): a cheap call costs around 2e-7, and numeric
    -- rounds to scale silently rather than erroring, so scale 6 would store a
    -- whole class of calls as exactly zero. numeric rather than float8 because
    -- these are summed over a year and binary float drifts.
    cost_usd             numeric(18, 10) CHECK (cost_usd >= 0),
    duration_ms          integer NOT NULL DEFAULT 0,
    -- Bounded by the recorder, same vocabulary as the nib_llm_requests_total
    -- outcome label. A failed call stores tokens 0 with a non-success outcome;
    -- it must not read as a free call.
    outcome              text NOT NULL DEFAULT ''
);

CREATE INDEX idx_llm_usage_created_at ON llm_usage (created_at DESC);

-- One row per finished agent-runner container, taken from its webhook. Separate
-- from llm_usage because the container prices itself and reports no per-call
-- breakdown: it is a different grain, not a different source of the same rows.
CREATE TABLE agent_run_usage (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    created_at    timestamptz NOT NULL DEFAULT now(),
    dialog_id     uuid,
    plan_id       uuid,
    job_name      text NOT NULL DEFAULT '',
    session_id    text NOT NULL DEFAULT '',
    status        text NOT NULL DEFAULT '',
    input_tokens  integer NOT NULL DEFAULT 0,
    output_tokens integer NOT NULL DEFAULT 0,
    num_turns     integer NOT NULL DEFAULT 0,
    cost_usd      numeric(18, 10) CHECK (cost_usd >= 0),
    duration_ms   integer NOT NULL DEFAULT 0
);

CREATE INDEX idx_agent_run_usage_created_at ON agent_run_usage (created_at DESC);

-- The container retries its webhook with curl, and AppendAgentResult already
-- ignores a repeat delivery by job name. Enforcing it here too means the money
-- cannot be double-counted just because a future edit moves the insert in front
-- of that check. Partial, because job_name is empty for deliveries that carry no
-- job and those cannot be deduplicated this way.
CREATE UNIQUE INDEX idx_agent_run_usage_job
    ON agent_run_usage (job_name) WHERE job_name <> '';

-- Append-only ledger of plan status moves. The status itself stays in
-- data/plan_state/{dialogID}.json, which holds only the current value, so this
-- is the only place status-over-time can come from.
--
-- No CHECK on the status values: the list already exists in three places
-- (service.actionPlanStatuses, metrics.PlanStatuses and the UI), Go validates
-- every write, and a fourth copy here would mean a migration for each new status.
CREATE TABLE plan_status_transitions (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    created_at  timestamptz NOT NULL DEFAULT now(),
    plan_id     uuid NOT NULL,
    from_status text NOT NULL,
    to_status   text NOT NULL,
    CONSTRAINT plan_status_transitions_moved CHECK (from_status <> to_status)
);

CREATE INDEX idx_plan_status_transitions_created
    ON plan_status_transitions (created_at DESC);
CREATE INDEX idx_plan_status_transitions_plan
    ON plan_status_transitions (plan_id, created_at DESC);

-- Plans-created-over-time had no index behind it: the only created_at-capable
-- one is idx_chat_dialogs_root_updated (000032), and it sorts by updated_at.
-- Same partial predicate as that index -- parent_id IS NULL only, never the mode
-- list, so a mode added later cannot silently fall out of the index -- with mode
-- as the second key so the bucketed group-by stays index-only.
CREATE INDEX idx_chat_dialogs_root_created
    ON chat_dialogs (created_at DESC, mode)
    WHERE parent_id IS NULL;
