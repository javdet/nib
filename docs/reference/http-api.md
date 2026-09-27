# HTTP API

Every route the backend serves. All of them sit under `/api/v1`, on
`SERVER_HOST:SERVER_PORT` (default `0.0.0.0:8080`).

Nothing else is exposed: the metrics listener is a separate HTTP server on its
own port, and the SPA is served by nginx, which proxies only the `/api` prefix
here.

## Conventions

- **Content type:** `application/json` for request and response bodies, except
  attachment upload (`multipart/form-data`) and the SSE stream
  (`text/event-stream`).
- **Authentication:** required on every route except `/health`, `/version`,
  `/auth/session` and the agent-runner webhook. See [Authentication](#authentication).
- **Content type:** a route that reads a JSON body refuses any other declared
  type with `415`, including a missing `Content-Type`.
- **CORS:** driven by `NIB_ALLOWED_ORIGINS`; preflight accepts the
  `Content-Type` and `Authorization` headers.
- **Errors:** a JSON body with a message. Service-level sentinel errors are
  mapped to status codes centrally; an unmapped one surfaces as `500`.
- **Write deadline:** routes that run the agent carry a 30-minute response
  write deadline. Routes marked *no deadline* below deliberately carry none,
  because they must answer while an agent run is in progress.

## Authentication

`NIB_API_TOKEN` is accepted in two forms:

- `Authorization: Bearer <token>`, for scripts.
- The `nib_session` cookie, for the browser. It is obtained by posting the token to
  `/auth/session`, and is needed because `EventSource` and `<img>` cannot send a header.

The cookie has these properties:

- It is `HttpOnly`, `SameSite=Strict` and `Path=/api/`, and lasts 30 days.
- It is `Secure` behind `X-Forwarded-Proto: https`.
- It holds a signed expiry, never the token. Rotating the token therefore signs
  every browser out.

A request carrying an `Authorization` header is judged by that header alone.

| Method | Path | Meaning |
|---|---|---|
| `GET` | `/auth/session` | `{"required": bool, "authenticated": bool}`. `required` is `false` only under `NIB_INSECURE_NO_AUTH`. |
| `POST` | `/auth/session` | Body `{"token": "..."}`. `204` with the cookie, or `401`. |
| `DELETE` | `/auth/session` | Expires the cookie. `204`. |

Every route except `/health`, `/version` and the webhook also passes two browser
guards:

- **Cross-origin writes.** A `POST`, `PUT` or `DELETE` whose `Sec-Fetch-Site` says
  `cross-site`, or whose `Origin` matches neither `Host` nor `NIB_ALLOWED_ORIGINS`,
  is refused with `403`. A request with neither header is not a browser's and
  passes.
- **Host allow-list.** With `NIB_ALLOWED_HOSTS` set, any other `Host` is refused
  with `421`.

| Status | Body `error` | Cause |
|---|---|---|
| `401` | `unauthorized` | No valid bearer token or session cookie. |
| `403` | `cross-origin request refused` | A cross-site browser write. |
| `415` | `Content-Type must be application/json` | A JSON route received another declared type. |
| `421` | `host not allowed` | `Host` not in `NIB_ALLOWED_HOSTS`. |

## System

| Method | Path | Meaning |
|---|---|---|
| `GET` | `/health` | Liveness probe. Unauthenticated. |
| `GET` | `/version` | Version string, from `APP_VERSION` or the build. Unauthenticated. |
| `GET` | `/modes` | The six mode names. |
| `GET` | `/system/tools` | Developer catalog of built-in tools. |
| `GET` | `/stats` | Statistics dashboard. Query: `from`, `to`, `bucket` (`hour`, `day`, `week`, `month`). A range is capped at 400 buckets. |

## Execution

*No write deadline.* A force stop is a Docker or Kubernetes call, and it has to
work while the agent loop holding the lease is wedged.

| Method | Path | Meaning |
|---|---|---|
| `GET` | `/execution` | What currently holds the single execution slot. |
| `DELETE` | `/execution` | Force-stop the holder. |
| `POST` | `/agent-runner/webhook` | Result callback from a finished agent container. Requires the per-run token the backend handed that container (`WEBHOOK_AUTH_HEADER`), bound to the body's `chat_id` and `job.name`. The API token and `AGENT_WEBHOOK_TOKEN` itself are refused with 401, even under `NIB_INSECURE_NO_AUTH`. A missing `chat_id` or `job.name` is 400. |

## Selection

The project / environment / cloud / location chosen in the header, which is
rendered into prompts.

| Method | Path |
|---|---|
| `GET` | `/selection` |
| `PUT` | `/selection` |

## Projects, environments, clouds, locations

Backed by the `projects` tree in [the configuration
file](configuration.md#projects); a write rewrites that file.

| Method | Path |
|---|---|
| `GET` `POST` | `/projects` |
| `GET` `PUT` `DELETE` | `/projects/{id}` |
| `GET` `POST` | `/projects/{id}/environments` |
| `GET` `PUT` `DELETE` | `/projects/{id}/environments/{envID}` |
| `GET` `POST` | `/projects/{id}/clouds` |
| `GET` `PUT` `DELETE` | `/projects/{id}/clouds/{cloudID}` |
| `GET` `POST` | `/projects/{id}/clouds/{cloudID}/locations` |
| `GET` `PUT` `DELETE` | `/projects/{id}/clouds/{cloudID}/locations/{locationID}` |

## Knowledge base

| Method | Path | Meaning |
|---|---|---|
| `GET` | `/knowledge/connection` | Current DSN source and reachability. |
| `PUT` | `/knowledge/connection` | Set the knowledge-base DSN. |
| `GET` | `/knowledge/collections` | Collections with their embedding model. |
| `GET` | `/knowledge/documents` | The stored source document for a collection. |
| `POST` | `/knowledge/documents` | Upload a document. Replaces every chunk in the collection. |
| `GET` | `/knowledge/status` | Ingestion status. |
| `GET` | `/knowledge/settings` | The knowledge-base switches, `{"autoUpdate": bool}`. |
| `PUT` | `/knowledge/settings` | Change them. An omitted field is left as it was. `503` when the settings file cannot be used. |

## Chat and dialogs

A dialog is the unit of work: one UUID, one transcript, and a lineage of
sub-agent dialogs parented to it.

| Method | Path | Meaning |
|---|---|---|
| `POST` | `/chat` | Single-shot completion. 30-minute deadline. |
| `GET` `POST` | `/dialogs` | List, create. List query: `page`, `limit`, `scope` (`all`, `pinned`), `mode`, `search`. |
| `POST` | `/dialogs/import` | Create a new plan from a `.nib` file sent as the JSON body, up to 64 MiB. Answers `201` with the new root dialog; `400` for a file this version cannot read, `413` when it is too large. |
| `GET` `DELETE` | `/dialogs/{id}` | Fetch, delete. |
| `GET` | `/dialogs/{id}/export` | The plan as a `.nib` file (`Content-Disposition: attachment`). `400` for a dialog that is not a plan's root. |
| `PUT` | `/dialogs/{id}/title` | Rename. |
| `PUT` | `/dialogs/{id}/categories` | Set tool categories for the lineage. |
| `PUT` | `/dialogs/{id}/pin` | Pin or unpin. |
| `GET` | `/dialogs/{id}/children` | Every dialog this plan spawned. |
| `GET` | `/dialogs/{id}/dag` | The stage graph. |
| `GET` `PUT` | `/dialogs/{id}/summary` | Plan summary. |
| `GET` | `/dialogs/{id}/report` | The finish report. |
| `POST` | `/dialogs/{id}/report` | Start the report agent. Answers `202`; *no deadline*. |
| `GET` | `/dialogs/{id}/events` | SSE stream of agent activity. |
| `GET` | `/dialogs/{id}/messages` | Transcript. |
| `POST` | `/dialogs/{id}/messages` | Send a message. 30-minute deadline. |
| `POST` | `/dialogs/{id}/messages/retry` | Rewind and retry. 30-minute deadline. |
| `POST` | `/dialogs/{id}/tool-results` | Answer a pending `ask_question`. 30-minute deadline. |

### Plan state and action plan

| Method | Path | Meaning |
|---|---|---|
| `GET` | `/dialogs/{id}/plan-state` | Status and schedule. |
| `PUT` | `/dialogs/{id}/plan-state/schedule` | Set the scheduled window. |
| `PUT` | `/dialogs/{id}/plan-state/status` | Set the status. |
| `GET` `PUT` | `/dialogs/{id}/action-plan` | The action plan. A `PUT` clears no side files. |
| `PUT` | `/dialogs/{id}/action-plan/reorder` | Reorder rows. |
| `PUT` | `/dialogs/{id}/action-plan/checks` | Operator checkboxes. |
| `PUT` | `/dialogs/{id}/action-plan/comments` | Operator comments. |
| `POST` | `/dialogs/{id}/action-plan/execute` | Launch one `code` action in an agent-runner container. Body `{"key": "s0.step1"}`. `400` for a non-code row or a disabled executor, `409` while the execution slot is held, `502` when the image cannot be pulled. Other action types are run by the orchestrator. |
| `GET` | `/dialogs/{id}/action-plan/exec` | Per-row execution runs. |
| `GET` | `/dialogs/{id}/action-plan/logs` | Container output so far of one running code action, `{"logs", "truncated"}`. Query `key` (required). A snapshot, polled by the UI. `409` once the run has finished, `400` for a row that is not a container run, `501` on a remote executor, `404` when the run or its container is gone. |

### Stage run

**Execute all** on a stage or on the rollback: the stage's unticked steps, then
its checks, each started once the one before it has finished. One stage run at
a time across every plan.

| Method | Path | Meaning |
|---|---|---|
| `POST` | `/dialogs/{id}/action-plan/stage-run` | Start. Body `{"scope": "stage" \| "rollback", "stage": <0-based index>}` (`stage` is ignored for the rollback). Answers once the first item has started. 30-minute deadline. `409` when a stage run is already active, nothing is left to run, or the execution slot is held. |
| `GET` | `/dialogs/{id}/action-plan/stage-run` | The plan's latest stage run, or `null`. |
| `DELETE` | `/dialogs/{id}/action-plan/stage-run` | Stop it and the item it is on. `409` when none is running. |

### Plan fan-out

Detailed planning of every stage in parallel. *No write deadline* — the fan-out
answers `202` and runs on past the request.

| Method | Path | Meaning |
|---|---|---|
| `POST` | `/dialogs/{id}/plan-fanout` | Start. |
| `GET` | `/dialogs/{id}/plan-fanout` | Progress. |
| `DELETE` | `/dialogs/{id}/plan-fanout` | Cancel. |

### Attachments

| Method | Path |
|---|---|
| `GET` `POST` | `/dialogs/{id}/attachments` |
| `GET` `DELETE` | `/dialogs/{id}/attachments/{attachmentId}` |

## Prompts, rules, skills

File-backed surfaces edited through the API.

| Method | Path | Meaning |
|---|---|---|
| `GET` | `/system-prompts/{name}` | The prompt for a mode. |
| `PUT` | `/system-prompts/{name}` | Override it. |
| `DELETE` | `/system-prompts/{name}` | Reset to the built-in default. |
| `GET` `POST` | `/rules` | List, create. |
| `GET` `PUT` `DELETE` | `/rules/{name}` | Fetch, update, delete. |
| `GET` `POST` | `/skills` | List (`name`, `description`, `access`), create. |
| `GET` `PUT` `DELETE` | `/skills/{name}` | Fetch, update, delete. |
| `GET` | `/skills/{name}/rendered` | The skill with its template variables resolved. |

The built-in system skills `nib-configuration` and `nib-internals` are
deliberately absent from `GET /skills`. The agent reaches them with `get_skill`;
the HTTP API never lists or returns them.

## Variables, secrets, company

| Method | Path | Meaning |
|---|---|---|
| `GET` `POST` | `/variables` | Prompt variables. |
| `GET` `PUT` `DELETE` | `/variables/{id}` | |
| `GET` `POST` | `/secrets` | Encrypted secrets. `GET` returns names and `allowedHosts`, never values. |
| `PUT` `DELETE` | `/secrets/{id}` | A blank `value` keeps the stored one; adding to `allowedHosts` then fails with 400. An absent `allowedHosts` keeps the list. |
| `GET` `PUT` | `/company` | Company profile used in prompts. |

## Tools

| Method | Path | Meaning |
|---|---|---|
| `GET` | `/included-tools/{mode}` | Tools included for one mode. |
| `PUT` | `/included-tools/{mode}` | Replace that list. |
| `GET` | `/mcp/catalog-tools` | The whole discovered catalog. |
| `GET` | `/tool-categories` | Categories with counts. |
| `GET` | `/tool-categories/uncategorized/tools` | Tools no category claims. |
| `GET` | `/tool-categories/{name}/tools` | Tools in one category. |
| `PUT` | `/tool-categories/{name}/patterns` | Replace the category's match patterns. |

## MCP servers

Registered by URL in `mcp.json`. Streamable HTTP only.

| Method | Path | Meaning |
|---|---|---|
| `GET` `POST` | `/mcp/servers` | List, register. |
| `GET` `PUT` `DELETE` | `/mcp/servers/{name}` | |
| `GET` | `/mcp/servers/{name}/tools` | Tools that server exposes. |
| `GET` `PUT` | `/mcp/config/raw` | The raw `mcp.json` bytes. A `PUT` stores them as given — comments and key order survive. |

## MCP connections

Remote servers registered by URL and API token, stored in Postgres rather than `mcp.json`.

| Method | Path | Meaning |
|---|---|---|
| `GET` `POST` | `/mcp/connections` | |
| `PUT` `DELETE` | `/mcp/connections/{id}` | A blank `apiToken` keeps the stored one only while `serverUrl` stays on the same origin; otherwise 400. |
| `GET` | `/mcp/connections/{id}/tools` | |
| `POST` | `/mcp/connections/{id}/tools/{toolName}` | Call one tool directly. |

## Executor

| Method | Path |
|---|---|
| `GET` | `/executor/config` |
| `PUT` | `/executor/config` |

Field meanings: [Executor settings](executor.md).

## See also

- [Built-in agent tools](agent-tools.md)
- [Modes](modes.md)
- [State: the data directory and the database](state.md)
