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
- **Authentication:** none. Every route except the agent-runner webhook is
  open to anything that can reach the port.
- **CORS:** driven by the allowed-origins list; preflight accepts the
  `Content-Type` and `Authorization` headers.
- **Errors:** a JSON body with a message. Service-level sentinel errors are
  mapped to status codes centrally; an unmapped one surfaces as `500`.
- **Write deadline:** routes that run the agent carry a 30-minute response
  write deadline. Routes marked *no deadline* below deliberately carry none,
  because they must answer while an agent run is in progress.

## System

| Method | Path | Meaning |
|---|---|---|
| `GET` | `/health` | Liveness probe. |
| `GET` | `/version` | Version string, from `APP_VERSION` or the build. |
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
| `POST` | `/agent-runner/webhook` | Result callback from a finished agent container. Requires `Authorization: Bearer $AGENT_WEBHOOK_TOKEN`. |

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

## Chat and dialogs

A dialog is the unit of work: one UUID, one transcript, and a lineage of
sub-agent dialogs parented to it.

| Method | Path | Meaning |
|---|---|---|
| `POST` | `/chat` | Single-shot completion. 30-minute deadline. |
| `GET` `POST` | `/dialogs` | List, create. |
| `GET` `DELETE` | `/dialogs/{id}` | Fetch, delete. |
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
| `POST` | `/dialogs/{id}/action-plan/execute` | Execute one action. |
| `GET` | `/dialogs/{id}/action-plan/exec` | Per-row execution runs. |

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
| `GET` `POST` | `/secrets` | Encrypted secrets. `GET` returns names, never values. |
| `PUT` `DELETE` | `/secrets/{id}` | |
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

## MCP connections (OAuth)

Servers reached through an OAuth handshake rather than a static entry.

| Method | Path | Meaning |
|---|---|---|
| `GET` `POST` | `/mcp/connections` | |
| `GET` | `/mcp/connections/{type}/auth` | Begin the OAuth flow. |
| `GET` | `/mcp/connections/{type}/callback` | OAuth redirect target. |
| `PUT` `DELETE` | `/mcp/connections/{id}` | |
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
