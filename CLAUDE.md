# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Nib (Neuro Infrastructure Builder) — an AI agent that decomposes infrastructure tasks, plans them
down to elementary steps, and executes them. Go backend (module `github.com/javdet/nib`) +
React SPA. `README.md` and most of `docs/` are in Russian.

## Toolchain and commands

**Go and bun are not installed on the host — run everything through Docker.**

Default to a throwaway container mounting **this** tree — **never with `--network host`**, since
that would let it reach a live backend on `:8080` and the mounted `data/`:

```bash
docker run --rm -v "$PWD/backend":/app -w /app golang:1.26 go test ./...
```

```bash
docker run --rm -v "$PWD/backend":/app -w /app golang:1.26 go test ./internal/service/... -run TestChatServiceSend -v
```

```bash
docker run --rm -v "$PWD/frontend":/app -w /app oven/bun:1 bun run test
```

Beware of sibling checkouts: `../amotool` and `../toolchain` are separate repos holding older
copies of this project, and their compose stacks use `amotool-*` container names. A
`docker exec amotool-backend …` runs against **their** source, not this one. When this repo's own
dev stack is up (`make dev-up` → `nib-dev-backend`, which runs `go run ./cmd/nib` with `./backend`
mounted at `/app`), `docker exec nib-dev-backend go test ./...` is the fast path.

Frontend also has `bun run lint` (eslint) and `bun run build` (`tsc -b && vite build`); append a
file path to `bun run test` for a single suite. CI (`.github/workflows/build.yml`) runs
`go vet ./...`, `go test -race ./...`, `go build ./...`, then bun lint/test/build, and publishes
images tagged from the root `VERSION` file.

`Makefile` wraps compose (`up`, `dev-up`, `logs`, `psql`) and golang-migrate (`migrate-create`,
`migrate-up`) — the migrate targets need `migrate` installed locally, but the backend applies
`backend/migrations/*.up.sql` itself at startup (tracked in `schema_migrations`), so new migrations
just need to land in that directory.

Two compose stacks, both on bridge networks: the root `docker-compose.yml` (project `nib`,
`nib-*` containers) runs the published `javdet/nib-*` images and reads `.env`/`deploy/compose/`;
`docker-compose.dev.yml` (project `nib-dev`, `nib-dev-*` containers) builds from source with live
reload.

## Configuration split

- `backend/config.yaml` (mounted at `/app/config.yaml`, selected by `-config` flag or `CONFIG_FILE`):
  structure — LLM base URL/model/api, agent `maxIterations`, KB DSN, directories, and the static
  projects/environments/clouds/locations tree.
- `.env` / environment: secrets only (`LLM_API_KEY`, `SECRETS_ENCRYPTION_KEY`, `ATLASSIAN_*`,
  `EXECUTOR_*`, `AGENT_WEBHOOK_TOKEN`). Never put keys in `config.yaml`.
- `DATA_DIR` (default `data/`) is the root for every file-backed store; per-store `dir` settings are
  resolved relative to it unless absolute.
- `knowledge_base.dir` (default `{DATA_DIR}/knowledgebase`) keeps the file last uploaded to each
  collection as `{collection}.md`, written only after the chunks commit. A collection with no file
  there falls back to the skeleton embedded in `internal/kbdoc`, which is never written to disk.
- `llm.embeddingModel` must match `kb_collections.embedding_model` exactly — knowledge search
  compares the strings.
- `llm.api` picks `/v1/chat/completions` vs `/v1/responses`; tools + reasoning need `responses`
  (see `docs/llm-providers.md`).

## Operating and configuring nib

This section is what the nib agent answers operator questions about nib from — it is served
verbatim by the `nib-configuration` system skill (`internal/skills/system.go`), so keep it factual
and keep the paths current. Everything here is about configuring *nib itself*, not the
infrastructure nib manages.

### MCP servers — streamable HTTP only

**A nib MCP server must expose an HTTP endpoint and be registered by URL. `stdio` and SSE are not
supported.** `internal/mcpclient` only ever builds `mcp.StreamableClientTransport`
(`discover.go`, `manager.go`), so:

- An `mcp.json` entry with `command`/`args` instead of `url` is parsed and then skipped — tool
  discovery warns `tool catalog: skip stdio server` (`internal/toolcatalog/indexer.go`) and a tool
  call resolves to `has no MCP route` (`internal/service/chat.go`). Adding such a server through the
  interface is refused with `mcpconfig.ErrStdioNotSupported`.
- There is no SSE transport in the codebase at all. `"transport": "sse"` does nothing.
- `ServerEntry.Transport` (`internal/mcpconfig/types.go`) is **dead** — nothing reads it. Setting
  `"transport": "http"` is not what makes a server work; having a `url` is.
- To use a stdio-only MCP server, put an HTTP bridge in front of it and register the bridge's URL.

Two independent sources feed the tool catalog, merged in `buildToolCatalog`:

1. `{DATA_DIR}/mcp.json` (path from `mcp.file`), Cursor-shaped: `{"mcpServers": {"name": {"url":
   "http://host:port/mcp", "headers": {...}}}}`. JSONC comments are allowed, unknown fields are
   preserved on edit. Editable at **MCP → Config** or by hand; a change invalidates the 60s
   discovery cache.
2. OAuth/token connections in Postgres, added at **MCP → Servers** (Atlassian OAuth needs
   `ATLASSIAN_*`).

Any value in `mcp.json` may reference `${NAME}`, expanded from the encrypted secret store at call
time only — never from process environment. Without `SECRETS_ENCRYPTION_KEY` every `${NAME}` stays
unresolved.

### Modes, prompts and how to change them

Modes: `main` (orchestrator; routes to sub-agents), `decompose` (task → DAG), `plan` (DAG → action
plan), `execute` (carry out one action), `discuss` (read-only investigation), `incident`. Overlay
prompts `plan_stage`, `rollback_stage`, `execute_action`, `execute_report` and `decompose_subagent`
are appended to their base mode's prompt.

Every prompt is compiled into the binary from `internal/systemprompts/defaults/{mode}.md`.
**Only `discuss` is editable at runtime**; its override is `{DATA_DIR}/prompts/discuss.md`
(`prompts.dir`) and is returned verbatim, so an override does not inherit later changes to the
shipped default. Editing any other prompt means rebuilding the image. A prompt is rendered once per
dialog and stored as the dialog's `system` message, so an edit affects new dialogs only.

### Prompt variables

`text/template` with `missingkey=error`, rendered from two namespaces:

- `{{ .global.NAME }}` — rows of the `prompt_variables` table, editable at **Variables**. Built-ins
  (`CompanyName`, `TaskTracker`, `IssueProject`, `Wiki`, `Messenger`, `toolCategories`) cannot be
  deleted or renamed, only re-valued. Non-global scopes (`project`, `environment`, `cloud`,
  `location`) need a scope name.
- `{{ .builtin.Project | .Environment | .Cloud | .Location }}` — the global selection in the header,
  `"any"` when unset.

The `host*` variables (`hostOS`, `hostRuntime`, `hostShell`, `hostWorkingDir`, `hostDataDir`,
`hostUser`, `hostCommands`) describe the container the backend runs in. They are **detected at
startup** by `internal/hostenv` and reconciled into the table: a value nobody has touched is
refreshed on the next boot, and an operator's edit is never overwritten. Edit one to correct or
extend what the agent believes about its host.

### Skills, rules and the knowledge base

- Skills live in `{DATA_DIR}/skills` as `{name}.md` with YAML frontmatter (`name`, `description`);
  the catalog is listed in the `main` and `discuss` prompts and loaded with `get_skill`. Built-ins
  are copied out of the image once per data volume (marker `.seeded-defaults`), so edits and
  deletions survive an upgrade.
- **System skills** (`nib-configuration`, `nib-internals`) are served from the binary. They cannot
  be created, edited, renamed or deleted, do not appear in the Skills page or the HTTP API, and are
  visible only to the agent.
- Rules live in `{DATA_DIR}/rules`, same format, listed in the `plan` and `discuss` prompts and
  loaded with `get_rule`. **Discuss mode alone can write them**, with `write_rule` (name,
  description, body) — it composes the frontmatter and replaces the whole file, so an edit means
  reading the rule first and passing the merged body. Every other mode has the tool withdrawn
  whatever its allow list says (`enforceModeToolLimits`), so a planner cannot rewrite the
  guardrails it is planning against.
- The knowledge base is pgvector chunks searched by `knowledge_search`; the file last uploaded to a
  collection is kept at `{DATA_DIR}/knowledgebase/{collection}.md` and read back with
  `get_kb_document`.

### Tools

Built-in tools are compiled in; which ones a mode may use is `{DATA_DIR}/tools/{mode}.json`
(`{"allow_tools": [...]}`), seeded from the image and reconciled on every boot: a tool added in a
later release is offered once (marker `.seeded-tools`) and a tool the operator removed stays
removed. MCP tools are on by default per mode via `{DATA_DIR}/tools/mcp-included.json`, editable at
**Tools → Included tools**; a tool removed there is still reachable through `tool_search`.
Categories come from the `toolCategories` variable and gate which MCP tools a planned action
carries.

`execute_command` is not a shell: one binary through argv, no pipes, redirects, `&&`, globs or
`$VAR` expansion, 60s timeout, 1 MiB of captured output. `api_call` is the same shape fixed to
`curl`.

### Secrets

`prompt_secrets` in Postgres, AES-encrypted with `SECRETS_ENCRYPTION_KEY`, managed at
**Variables → Secrets** and read by the agent with `get_secrets`. Without the key secrets can be
neither written nor read. Values are redacted from every MCP transport error.

### Executor — where planned actions actually run

The executor launches an agent-runner container per action; it is **operator-configured, never
detected**, at **Executor**, and persisted under `DATA_DIR`.

- Type: `disabled` (nothing is ever launched), `local`, `remote`.
- Remote platform: `docker`, `kubernetes` (auth `local_config` — kubeconfig, falling back to
  in-cluster — or `token`), `kubefoundry`.
- Agent image: `claude-code` or `codex`; auth `api_key` or `oauth_token`, from `EXECUTOR_*`.
- Credentials are **selected by secret name**, never fixed: the LLM key is `tokenSecretName`, the
  git API token is `gitTokenSecretName`, both picked in the Executor page from **Variables →
  Secrets**. There is no `EXECUTOR_GIT_API_TOKEN` fallback — a blank selection is refused at launch
  with `ErrExecutorGitTokenSecretRequired`, not at save time, so an executor can be configured
  before its secrets exist. Remote Kubernetes is the exception: the agent Job takes its credentials
  from the operator-managed Secret in `agentSecretName` (`envFrom`), so the git token may be blank.
- The runner reports back over a webhook guarded by `AGENT_WEBHOOK_TOKEN`.

Git provider: `GIT_PROVIDER` is derived from the free-text `VersionControlSystem` company variable
by `executor.NormalizeGitProvider` — anything containing "gitlab" is GitLab, everything else
including blank is GitHub. The agent container branches on it: GitHub clones as
`x-access-token:<token>` and opens PRs with `gh`, GitLab clones as `oauth2:<token>` and opens MRs
with `glab`. The host comes from `REPO_URL` rather than a hardcoded `github.com`, so self-hosted
GitLab and GitHub Enterprise work; an ssh/scp `REPO_URL` is refused, since a token cannot
authenticate it.

That container is a **different machine from the backend**, with its own toolchain. What is on the
backend's PATH says nothing about what the executor can run, and vice versa.

### LLM provider

`llm.baseURL`, `llm.model`, `llm.embeddingModel`, `llm.api` (`chat` vs `responses`),
`llm.reasoningEffort`, `llm.timeoutSeconds` in `config.yaml`; the key comes from `LLM_API_KEY`
(fallback `OPENAI_API_KEY`). Any OpenAI-compatible endpoint works — see `docs/llm-providers.md`.
Round budgets are `agent.maxIterations` and the per-sub-agent `stageMaxIterations` /
`actionExecMaxIterations`. Only one execution runs at a time, by policy, and
`agent.actionExecConcurrency` is clamped to 1.

### What an upgrade does and does not touch

A new image replaces the binary, the embedded prompts, the built-in skills and the built-in allow
lists. It does **not** overwrite anything already on the data volume: prompt overrides, edited or
deleted skills and rules, `mcp.json`, allow lists, executor config, dialog artifacts. New built-in
skills are seeded only on a volume that has never been seeded; new built-in *tool names* are merged
into existing allow lists once each. Database state (dialogs, variables, secrets, MCP connections,
knowledge base) is migrated in place by `backend/migrations` at startup.


## Architecture

### Backend layering

`backend/cmd/nib/main.go` is the composition root: every dependency is constructed there and
passed into `handler.NewRouter` as a named `handler.Deps` struct. Flow is `handler` (chi) →
`service` → `repository`, with `domain` holding plain structs. A handler takes services, never a
store or a repository — when the HTTP layer needs two collaborators joined, the join belongs in a
service (see `service.IncludedToolsService` and `service.SkillService`, which exist for exactly
that reason). Handlers translate errors through `handleServiceError` in
[respond.go](backend/internal/handler/respond.go) — a new sentinel error needs a case added there or
it degrades to a 500.

Two repository backends coexist: `repository/postgres` (dialogs, variables, secrets, MCP
connections, KB, tool catalog) and `repository/static`, which serves projects/environments/clouds/
locations out of `config.yaml` through a `StoreHolder` that `config.Manager` swaps on write —
editing those entities rewrites `config.yaml`.

### Knowing itself and its host

Two small packages exist so the agent can answer questions about nib rather than guess:

- [internal/hostenv](backend/internal/hostenv) snapshots the process environment once at startup —
  distro, kernel, arch, shell, cwd, uid, Docker/Kubernetes/bare-host, and which of a fixed list of
  CLIs are on PATH. Every probe is a file read, an env lookup or `exec.LookPath`; nothing shells
  out, and a probe that fails leaves its field empty. `service.HostEnvService` reconciles the
  snapshot into the `host*` prompt variables (marker `{DATA_DIR}/.host-env.json`) and the
  `## Environment` block of the `main`/`plan`/`execute`/`discuss` prompts renders them.
- [internal/nibdocs](backend/internal/nibdocs) embeds `docs/repo-guide.md`, a copy of this file,
  and slices it by `## ` heading. `internal/skills/system.go` serves two slices of it as the
  non-editable **system skills** `nib-configuration` and `nib-internals`: listed in `ListAll` for
  the prompt catalog, readable through `GetAny`/`get_skill`, and deliberately unreachable through
  `List`/`Get`, which the HTTP API uses. `make sync-docs` keeps the copy honest and
  `TestRepoGuideMatchesRepoRoot` fails the build when it drifts.

### The agent loop

`service.ChatService` ([chat.go](backend/internal/service/chat.go),
[chat_dialog.go](backend/internal/service/chat_dialog.go)) is the core. A turn resolves the system
prompt, builds a tool catalog, then runs up to `agent.maxIterations` rounds of
completion → tool calls → completion. Dialog transcripts persist per message in Postgres; tool
activity streams to the UI over SSE (`SubscribeActivity` →
[dialog_events.go](backend/internal/handler/dialog_events.go)).

The tool catalog combines:
- **Local Go tools** — registered in order by `localToolRegistrars` in
  [local_tool_registry.go](backend/internal/service/local_tool_registry.go). Adding one means a
  `register*` function plus a `…ToolDef()`/handler pair; it then shows up in the developer catalog
  ([system_tools.go](backend/internal/service/system_tools.go)) automatically.
- **MCP tools** — discovered concurrently from OAuth connections and `data/mcp.json` servers, cached
  60s; `InvalidateMCPToolDiscoveryCache` fires on mcp.json or secret changes (wired in `main.go`).

Which tools a turn sees is the union of `data/tools/{mode}.json` (`allow_tools`, built-ins) and
`data/tools/mcp-included.json[mode]` (MCP), plus — for `plan` dialogs — every tool in the categories
chosen during decomposition. Everything else stays reachable through the `tool_search` local tool,
backed by pgvector embeddings of the tool catalog.

`mcp-included.json` is reconciled with the catalog after every successful reindex
(`Indexer.SetAfterIndex`, wired in `main.go` → `includedtools.SyncCatalog`): a tool the catalog has
never carried before is added to every mode, and one that has left the catalog is removed from every
mode. So MCP tools are on by default and a deleted server leaves no orphan names. The ledger of
names already reconciled is `data/tools/mcp-known.json` — it is the only thing distinguishing a tool
that was never offered from one an operator excluded in the UI, so an install without it treats the
whole catalog as new.

Modes are the fixed list in [mode.go](backend/internal/mode/mode.go): `main`, `decompose`, `plan`,
`execute`, `discuss`, `incident`. A new mode needs an entry there, a prompt in
[systemprompts/defaults](backend/internal/systemprompts/defaults) and an allow list in
[mode/defaults](backend/internal/mode/defaults) — both embedded, and the backend refuses to boot
without the prompt — plus frontend support. `SeedAllowLists` reconciles the embedded lists onto
`{DATA_DIR}/tools` at boot, so an existing volume gains a new mode's list on the next start; it
never takes a name out of a list already there, which is why a capability being *withdrawn* from a
mode has to be enforced in code as well.

### Orchestration

A plan starts in `main`, the orchestrator mode, created only by the New plan button. It does no work
itself: it launches the other modes as sub-agents through the `run_subagent` tool, whose schema is
generated from the registry in
[subagent_registry.go](backend/internal/service/subagent_registry.go).

- **decompose** runs *blocking*, inside the orchestrator's turn
  ([subagent_decompose.go](backend/internal/service/subagent_decompose.go)). Decomposition is a
  conversation, so when it calls `ask_question` it suspends: the pause is recorded in
  `data/subagents/{root}.json`, the orchestrator relays the questions under an `ask_question` of its
  own, and the answers are routed back to the sub-agent's dangling call by
  [subagent_resume.go](backend/internal/service/subagent_resume.go), hooked into `SubmitToolResult`
  beside the fan-out's own resume.
- **plan** and **execute** start *asynchronously* and return "started": a fan-out can run for an
  hour and an action for half of one, both longer than the HTTP write deadline. They report into the
  orchestrator's chat through the existing named-message and SSE machinery.

Only the orchestrator sees `run_subagent`, `stop_execution` and `execute_action`. Two guards keep it
that way: the tools are registered only when `toolBinding.planID == toolBinding.dialogID` — true
only on a root turn — and `stripSubagentTools` removes them from every sub-agent's allow set
whatever an operator's edit to a mode list says.

**One execution at a time**, across every plan, held by the lease in
[execution_lease.go](backend/internal/service/execution_lease.go) and covering both action
sub-agents and code-action containers. A second request is refused with a sentence naming what holds
it rather than queued, `agent.actionExecConcurrency` is clamped to 1, and `DELETE /api/v1/execution`
force-stops the holder — cancelling a sub-agent's context, or stopping a container through
`executor.StopAction`. That route takes no write deadline on purpose: it has to work while the agent
loop holding the lease is wedged. `ReconcileStuckRuns` sweeps records a previous process left
`running` at boot, without which a restart would block execution permanently.

Pressing **Finish** on a plan launches one more sub-agent, the only one the operator starts
directly rather than the orchestrator: `StartReportAgent`
([report_agent.go](backend/internal/service/report_agent.go)) runs in `execute` mode under the
`execute_report` overlay, reads the finished plan with `get_action_list` and writes
`data/reports/{root}.md` with `set_report`. The plan page renders it in a collapsed Report card,
refreshed by the `report_updated` activity event — without which a report written minutes after
the button was pressed would only appear on a reload.

Two things about it are deliberate. It does **not** take the execution lease: it reads the plan and
writes prose, changes no managed system, and holding it to the one-execution rule would mean a plan
finished while a last action was still running silently loses its report; a per-plan claim plus an
"already written" check are what stop it running twice. And `set_report` is in **no** mode allow
list — the report agent is handed a literal two-tool catalog, and `actionAgentAllowSet` deletes the
name as well, because the per-action executors run in `execute` mode too and a mode list cannot
tell the two apart.

System prompts, rules, and skills are markdown files under `data/`, rendered as `text/template`
([prompttpl](backend/internal/prompttpl/render.go), with Helm-style `toYaml`/`nindent`) against
prompt variables from Postgres plus the current selection (project/environment/cloud/location).

Built-in skills live in [internal/skills/defaults](backend/internal/skills/defaults) and are
embedded in the binary; `skills.Service.Seed()` (called from `main.go`) copies each one into the
skills directory at most once per data volume, tracked in `{skillsDir}/.seeded-defaults`. Adding a
built-in is dropping a `{name}.md` there — it seeds on the next start, including on existing
installs — but an operator's edit or deletion of a seeded skill is never undone, and a name already
taken on disk is left alone. Unlike system prompts, seeded skills are then owned by the volume:
there is no default-vs-override split for them.

### Dialog artifacts

A `Dialog` UUID is the unit of work. Beyond the Postgres transcript, artifacts are files keyed by
dialog id, written atomically via `internal/atomicfile`: `data/dags/{id}.md`,
`data/summaries/{id}.txt`, `data/reports/{id}.md`, `data/action_plans/{id}.json`,
`data/plan_state/{id}.json`, `data/plan_fanout/{id}.json`, `data/subagents/{id}.json`.

An action plan carries side files beside it, all keyed by the same dialog id and by the positional
row key of an action (`s0.step1`, `rollback.2`): `.checks.json` (the operator's checkboxes),
`.comments.json` (their comments), `.runs.json` (the execute dialog per row), `.exec.json` (the last
run's status) and `.notes.json` (what that run reported). Every one of them is positional, so all of
them are remapped together whenever a stage is rewritten, reordered or replaced — `create` clears
them, the HTTP `PUT` clears none.

`.notes.json` is the exception to "artifacts are for the operator": it is the executors' own memory.
A finished action sub-agent's final message is recorded there and handed to the sub-agents that run
the later actions through `get_action_list`, which is how a resource id one action creates reaches
the action that needs it. It is deliberately unreachable from the HTTP API — its accessors are
unexported for exactly that reason — and a successful run's text is kept out of the plan chat, which
shows only that the action is done plus a link to the transcript. A failure, a question or a
cancellation still says why in the chat.

They are keyed by the **root** dialog of a lineage, not by the dialog that wrote them: a sub-agent's
transcript is its own while every plan artifact belongs to the plan. That split is `toolBinding`
(`dialogID` for the transcript, `planID` for the plan), and `resolveRootDialogID` is what walks
`parent_id` to find the owner. Every sub-agent — the decompose one, the per-stage planners, the
rollback agent, the per-action executors — is parented straight to the root, so `ListChildren(root)`
is everything a plan spawned, and the categories chosen during decomposition sit on the root for all
of them to inherit.

### Secrets and `${NAME}` expansion

Any value in `data/mcp.json` may reference `${NAME}`; it is expanded only at the moment of the MCP
call, and **only** from encrypted secrets (`SECRETS_ENCRYPTION_KEY`, `crypto/secretbox`). The process
environment is deliberately not a source — the backend runs with LLM, database and executor
credentials, and expanding them here would let an mcp.json edit read them back out through any URL or
header the agent then calls. Do not add an env fallback to `resolver.lookup`.
Expanded values travel with the route as `route.secrets` and are stripped from errors by
`redactRouteError` — any new path that surfaces MCP transport errors must keep going through it.

Saving never resolves a reference, so a `${NAME}` whose secret does not exist yet still saves and the
failure surfaces when the server is contacted. `SetRaw` stores the editor's bytes as given — comments,
key order, and keys this package does not model all survive — and the structured add/edit/delete path
goes through `rawdoc.go`, which rewrites one entry and leaves the rest of the file alone. Both were
lossy before: a save that only changed something outside `ServerEntry` came back as a silent no-op.

### Executor / agent-runner

`internal/executor` launches single-use agent containers. Local → `docker.go` via the host Docker
socket; remote Kubernetes → `kubernetes.go`, rendering the embedded
`templates/job.yaml.tmpl` (overridable with `{DATA_DIR}/job.yaml.tmpl`). Two entry points with
different environment contracts: `Run` (driven by the `run_executor` LLM tool) and `RunAction`
(operator pressing *Execute action* on a `code` step, with fixed tool surface and timeout constants).
Finished agents call back into `POST /api/v1/agent-runner/webhook`, authenticated with
`AGENT_WEBHOOK_TOKEN`. The container images live in `agent-runner/`.

### Observability

`internal/metrics` is a leaf package (it imports no other `internal/*` except `version`) holding a
private `prometheus.Registry` plus a package default in an `atomic.Pointer`, so every recorder is a
no-op until `metrics.Setup` runs in `main.go` — which is what keeps the service tests, none of which
call Setup, working. Deliberately `prometheus/client_golang` rather than the OpenTelemetry the Go
skill asks for: no OTel code or collector exists anywhere, and every call site goes through this
package's own API, so an exporter can be swapped underneath later.

Metrics are served by a **second `http.Server`** on `METRICS_PORT` (9090), never a route on the API
router: the ingress and the frontend nginx both route only `/api` to the backend, so a separate port
is unreachable from the public host. Its `ListenAndServe` error is logged, not fatal.

Two things are load-bearing. The HTTP middleware sits after `middleware.Logger` and reuses the
`WrapResponseWriter` Logger already installed — anything that hides `Flush()` breaks SSE
([dialog_events.go](backend/internal/handler/dialog_events.go) answers a bare 500) and anything that
hides `Unwrap()` breaks `writeDeadline`'s 30-minute budget; both fail silently, so
`middleware_metrics_test.go` guards them. And the plans-by-status gauges are refreshed by a
background goroutine, not at scrape time: plan status lives in `data/plan_state/{id}.json`, so
counting is one query plus one file read per plan. Full metric list in [docs/metrics.md](docs/metrics.md).

### Frontend

Vite + React 19 + react-router + Tailwind v4 + shadcn/Radix, `@/` aliased to `src/`. Feature-sliced:
`src/features/{feature}/{api,components,hooks,lib,pages}`, shared primitives in
`src/components/ui`, all HTTP through the single wrapper in
[api-client.ts](frontend/src/lib/api-client.ts) (base `/api/v1`, throws `ApiError`). Routes are lazy
imports in [app.tsx](frontend/src/app.tsx). The dev server proxies `/api` to a target auto-detected
from the container's default gateway (`API_PROXY_TARGET` overrides). Vitest runs in a `node`
environment and covers pure `lib/` helpers only — there are no component tests.

## Conventions

- Go: clean architecture, interface-driven wiring, wrapped errors (`fmt.Errorf("ctx: %w", err)`),
  `slog` structured logs, table-driven `*_test.go` beside the code. Full guidance in
  [.claude/rules/go.md](.claude/rules/go.md).
- TS/React: tabs, single quotes, no semicolons, kebab-case filenames, function components,
  `handle*`/`is*`/`use*` naming. Full guidance in [.claude/rules/react.md](.claude/rules/react.md).
- Existing code carries short comments explaining *why* a non-obvious decision was made (caching,
  redaction, deliberate omissions) — match that density rather than narrating what the code does.

## Watch out

- `data/` is live application state mounted into the running stack (mcp.json, prompts, rules,
  skills, tool allow lists, dialog artifacts, `plan_fanout/`, `subagents/`). Editing a file there
  changes the running app immediately, and generated artifacts routinely show up as untracked files
  in `git status`.
- Every lock, the SSE broker and the execution lease are process-local, and the chart pins
  `backend.replicaCount: 1`. Horizontal scaling would break all four at once.
- Releases are driven by the root `VERSION` file merged to `main`; keep `.env.example`
  (`NIB_VERSION`, `NIB_KB_VERSION`) and `frontend/package.json` version in step.
