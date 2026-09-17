# Metrics

The backend exports Prometheus metrics on a listener of its own — by default
port `9090`, path `/metrics`. It is a second HTTP server, never a route on the
API router.

The endpoint is **unauthenticated**, like the rest of the API. The metric names
alone report the number of plans, the model in use and cumulative LLM cost.

Settings: [Configuration file](configuration.md#metrics) and [Environment
variables](environment-variables.md#backend--metrics-and-logging).
To point a scraper at it: [Scrape nib's metrics with
Prometheus](../how-to/scrape-metrics-with-prometheus.md).

## Go and process

The standard `client_golang` collectors: `go_*`, including GC and scheduler
latency, and `process_*` — CPU, RSS, file descriptors,
`process_start_time_seconds`.

Plus `nib_build_info{version,go_version}` and `nib_start_time_seconds`.

## HTTP

| Metric | Type | Labels |
|---|---|---|
| `nib_http_requests_total` | counter | `method`, `route`, `code` |
| `nib_http_request_duration_seconds` | histogram | `method`, `route` |
| `nib_http_response_bytes_total` | counter | `method`, `route` |
| `nib_http_requests_in_flight` | gauge | — |

`route` is the chi route template — `/api/v1/dialogs/{id}/messages` — not the
request path. A request matching no route is labelled `unmatched`; a CORS
preflight is labelled `preflight`.

Duration buckets run to 1800 seconds, because the agent routes (`/chat`,
`/messages`, `/messages/retry`, `/tool-results`) run under a 30-minute write
deadline.

`/api/v1/dialogs/{id}/events` is an SSE stream: its duration is the lifetime of
the subscription and its response size is every event delivered.

## Plans

| Metric | Type | Labels |
|---|---|---|
| `nib_plans` | gauge | `status` |
| `nib_plans_refresh_duration_seconds` | histogram | — |
| `nib_plans_refresh_errors_total` | counter | — |
| `nib_plans_refresh_timestamp_seconds` | gauge | — |
| `nib_plan_status_transitions_total` | counter | `from`, `to` |

Statuses are `draft`, `scheduled`, `in_progress`, `done`, `reopened`,
`rolled_back` and `cancelled`. All seven are published, including zeroes.

Plan status lives in `data/plan_state/{dialogID}.json`, defaulting to `draft`
when the file is absent, so counting is one query plus one file read per plan.
It runs on a background refresh every `refreshSeconds`, not at scrape time.

## Database

`nib_db_up` (0 or 1) and `nib_db_ping_duration_seconds` are the real liveness
signal. `GET /api/v1/health` answers `ok` unconditionally and never touches the
database.

pgx pool: `nib_db_pool_connections{state}`, `nib_db_pool_max_connections`,
`nib_db_pool_acquires_total`, `nib_db_pool_empty_acquires_total`,
`nib_db_pool_canceled_acquires_total`, `nib_db_pool_new_connections_total`,
`nib_db_pool_destroys_total{reason}`, `nib_db_pool_acquire_wait_seconds_total`,
`nib_db_pool_empty_acquire_wait_seconds_total`.

## Agent and LLM

| Metric | Type | Labels |
|---|---|---|
| `nib_agent_turns_total` | counter | `mode`, `outcome` |
| `nib_agent_turn_duration_seconds` | histogram | `mode` |
| `nib_agent_rounds` | histogram | `mode` |
| `nib_agent_tool_failures_total` | counter | `mode` |
| `nib_agent_turns_in_flight` | gauge | `mode` |
| `nib_agent_tool_calls_total` | counter | `source`, `outcome` |
| `nib_agent_local_tool_calls_total` | counter | `tool`, `outcome` |
| `nib_agent_tool_call_duration_seconds` | histogram | `source` |
| `nib_llm_requests_total` | counter | `operation`, `model`, `outcome` |
| `nib_llm_request_duration_seconds` | histogram | `operation`, `model` |
| `nib_llm_tool_calls_returned_total` | counter | `model` |
| `nib_llm_embedding_tokens_total` | counter | `model` |
| `nib_mcp_tool_discovery_duration_seconds` | histogram | — |
| `nib_mcp_tool_discovery_errors_total` | counter | `server` |
| `nib_mcp_tools_discovered` | gauge | `server` |

Turn `outcome` is one of `success`, `error`, `max_iterations`, `tool_failures`,
`awaiting_input`. `nib_agent_rounds` is how much of the `agent.maxIterations`
budget a turn used.

Tool `source` is `local`, `mcp` or `unknown`. Tool *names* appear only in
`nib_agent_local_tool_calls_total`, and only for built-in tools: MCP names come
from operator configuration and an invented name comes from the model, so
neither goes in a label.

Prompt and completion tokens are not exported to Prometheus. They are recorded
per call in the `llm_usage` table instead — see [Usage
tables](#usage-tables). `nib_llm_embedding_tokens_total` is unaffected.

## Execution

| Metric | Type | Labels |
|---|---|---|
| `nib_execution_lease_held` | gauge | — |
| `nib_execution_lease_acquisitions_total` | counter | `kind` |
| `nib_execution_lease_rejections_total` | counter | `kind` |
| `nib_execution_lease_expirations_total` | counter | — |
| `nib_execution_lease_hold_seconds` | histogram | `kind` |
| `nib_execution_force_stops_total` | counter | `kind`, `outcome` |
| `nib_action_exec_runs_started_total` | counter | `kind` |
| `nib_action_exec_runs_finished_total` | counter | `status` |
| `nib_action_exec_run_duration_seconds` | histogram | `status` |
| `nib_action_exec_active` | gauge | — |
| `nib_plan_fanout_runs_started_total` | counter | — |
| `nib_plan_fanout_runs_finished_total` | counter | `status` |
| `nib_plan_fanout_run_duration_seconds` | histogram | `status` |
| `nib_plan_fanout_stages_total` | counter | `kind`, `status` |
| `nib_plan_fanout_blockers_total` | counter | — |
| `nib_plan_fanout_rejections_total` | counter | `reason` |
| `nib_plan_fanout_active` | gauge | — |
| `nib_stuck_runs_reconciled_total` | counter | `kind` |

`nib_execution_lease_rejections_total` counts how often the one-execution-at-a-
time policy refused an operator: a second request is rejected, not queued.

`nib_stuck_runs_reconciled_total` above zero after a start means the previous
process died mid-execution.

## Agent containers

| Metric | Type | Labels |
|---|---|---|
| `nib_executor_runs_total` | counter | `entrypoint`, `type`, `platform`, `outcome` |
| `nib_executor_run_duration_seconds` | histogram | `entrypoint` |
| `nib_executor_stops_total` | counter | `outcome` |
| `nib_agent_runner_webhooks_total` | counter | `outcome` |
| `nib_agent_runner_results_total` | counter | `status`, `pushed` |
| `nib_agent_runner_run_duration_seconds` | histogram | — |
| `nib_agent_runner_turns` | histogram | — |
| `nib_agent_runner_cost_usd_total` | counter | — |

`nib_executor_run_duration_seconds` measures **launching** the container, not
running it: the call returns as soon as the job is created, and the container
reports its result by webhook. The working duration is
`nib_agent_runner_run_duration_seconds`.

The webhook `status` is normalised to `success`, `failed`, `timeout`, `error`
or `other`, because it arrives from a container over the network and any other
string would become a new series.

## SSE

`nib_sse_subscribers`, `nib_sse_events_published_total{kind}`,
`nib_sse_events_dropped_total{kind}`.

A dropped event means the UI has diverged from server state: the broker buffers
32 events per subscriber and discards events for a slow one.

## Usage tables

Prometheus answers "what is happening now" and retains only what the Prometheus
server is configured to retain. Three Postgres tables hold usage indefinitely.

| Table | One row per |
|---|---|
| `llm_usage` | LLM call: dialog, plan, mode, model, tokens (prompt, cached, completion, reasoning), cost, duration, outcome |
| `agent_run_usage` | finished agent container, from the webhook |
| `plan_status_transitions` | plan status change |

`GET /api/v1/stats?from=&to=&bucket=` reads them and drives the Statistics page.
`bucket` is `hour`, `day`, `week` or `month`; a range is capped at 400 buckets
and a wider one is rejected with `400`.

Two ways to misread these rows:

- **`cost_usd` may be `NULL`.** nib carries no rate card and stores only what
  the provider reported: OpenRouter returns `usage.cost`, the OpenAI platform
  does not. `NULL` means unknown, not free, and the interface shows a dash
  rather than `$0.00`.
- **`cached_prompt_tokens` is a subset of `prompt_tokens`**, and
  `reasoning_tokens` is a subset of `completion_tokens`. Adding all four
  double-counts.

Rows deliberately outlive the dialog they came from: there is no foreign key to
`chat_dialogs`, so any join to it must be a `LEFT JOIN`.

Status history accumulates only from the release that introduced it; plans that
existed before have no transitions. The current breakdown is still complete,
because it is counted from the `plan_state` files.

## Multiple replicas

`nib_execution_lease_held`, `nib_sse_subscribers` and `nib_plans` are
process-local. They are unambiguous today because the chart pins one backend
replica; with more they would become per-pod and must not be summed. See [One
execution at a time](../explanation/one-execution-at-a-time.md).

## See also

- [Scrape nib's metrics with Prometheus](../how-to/scrape-metrics-with-prometheus.md)
- [State: the data directory and the database](state.md#statistics)
