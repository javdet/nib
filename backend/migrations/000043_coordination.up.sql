-- What lets more than one backend process share one database: the execution
-- lease as a row instead of a variable in one process's memory, and a registry
-- of live processes so a boot or a sweep can tell work whose process died from
-- work another process is still doing.

-- One row per running backend process, kept fresh by a heartbeat. A process
-- that stops cleanly deletes its own row; one that dies is recognised by a
-- stale heartbeat, and its runs are closed by whichever process notices.
CREATE TABLE nib_replicas (
    id           text PRIMARY KEY,
    hostname     text NOT NULL DEFAULT '',
    version      text NOT NULL DEFAULT '',
    started_at   timestamptz NOT NULL DEFAULT now(),
    heartbeat_at timestamptz NOT NULL DEFAULT now()
);

-- The one execution allowed at a time, across every plan. At most one row,
-- which the CHECK enforces: acquiring is an INSERT that conflicts on slot, so
-- two processes racing for it cannot both win.
--
-- expires_at is NULL for a lease that never expires on its own. A sub-agent's
-- is extended by its process's heartbeat, so a process that dies loses its
-- lease within a heartbeat window; a code action container's is fixed at the
-- action deadline, since the container outlives the process that launched it.
CREATE TABLE execution_lease (
    slot        smallint PRIMARY KEY DEFAULT 1 CHECK (slot = 1),
    token       uuid NOT NULL,
    owner       text NOT NULL,
    holder      jsonb NOT NULL,
    acquired_at timestamptz NOT NULL DEFAULT now(),
    expires_at  timestamptz
);

-- The process that ran a command. Boot used to mark every running row lost;
-- with several processes it may only mark those of processes that are gone.
ALTER TABLE command_runs ADD COLUMN owner text NOT NULL DEFAULT '';
