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
- `.env` / environment: secrets only (`LLM_API_KEY`, `SECRETS_ENCRYPTION_KEY`, `EXECUTOR_*`,
  `AGENT_WEBHOOK_TOKEN`, `NIB_API_TOKEN`), plus the auth switches `NIB_INSECURE_NO_AUTH`,
  `NIB_ALLOWED_ORIGINS` and `NIB_ALLOWED_HOSTS`. Never put keys in `config.yaml`.
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
2. Token connections in Postgres (URL plus API token), added at **MCP → Servers**. There is
   no OAuth flow; a server that needs OAuth has to be reached through a static token. Moving a
   connection's URL to another origin (scheme, host or port) requires a new token
   (`ErrMCPTokenRequired`); a blank token keeps the stored one only on the same origin.

A header, or the URL's path, query or userinfo, in `mcp.json` may reference `${NAME}`, expanded
from the encrypted secret store at call time only — never from process environment. Without
`SECRETS_ENCRYPTION_KEY` every `${NAME}` stays unresolved.

**A secret is sent only to a host on its own allowed-hosts list** (`prompt_secrets.allowed_hosts`,
set per secret at **Variables → Secrets**). A reference for a server whose URL host is not on the
list fails with `secret is not allowed for this host`. A secret with an empty list cannot be used
from `mcp.json` at all. The host must be written literally: `${NAME}` in the URL's scheme, host or
port is refused. Adding a host requires entering the secret's value again; removing one does not.
MCP clients add headers and tokens only to requests for the server's own origin, and refuse a
redirect to another one.

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
- Each skill has an `access` frontmatter key, set per skill on the Skills page: `enabled` (the
  default, also for a missing or unknown value) lists it in the catalog; `explicit` leaves it out
  and `get_skill` serves it only once an operator message in the root dialog carries `/name`
  (a bare mention does not count); `disabled` hides it from the catalog, refuses it in `get_skill`
  and drops it from the chat's `/` menu. `get_skill` is the gate, since every mode carries it.
  The Knowledge Base, Distribute tools and Categorize tools buttons open their message with
  `/name`, so they survive `explicit` but fail on `disabled`.
- **System skills** (`nib-configuration`, `nib-internals`) are served from the binary. They cannot
  be created, edited, renamed or deleted, do not appear in the Skills page or the HTTP API, and are
  visible only to the agent. They are always `enabled`.
- Rules live in `{DATA_DIR}/rules`, same format, listed in the `plan` and `discuss` prompts and
  loaded with `get_rule`. **Discuss mode alone can write them**, with `write_rule` (name,
  description, body) — it composes the frontmatter and replaces the whole file, so an edit means
  reading the rule first and passing the merged body. Every other mode has the tool withdrawn
  whatever its allow list says (`enforceModeToolLimits`), so a planner cannot rewrite the
  guardrails it is planning against.
- The knowledge base is pgvector chunks searched by `knowledge_search`; the file last uploaded to a
  collection is kept at `{DATA_DIR}/knowledgebase/{collection}.md` and read back with
  `get_kb_document`. `update_kb` is the write half and replaces a collection **in full** — every
  chunk is deleted, the text re-embedded and the file rewritten — so a caller reads, merges and
  sends the whole document. Both tools are in the `discuss` allow list only.
- **Auto-update.** Finishing a plan also folds what it established into the collection named after
  the plan's project. The switch is `autoUpdate` in `{DATA_DIR}/knowledge.json`
  (`knowledge_base.settingsFile`), edited at **Knowledge Base → Automatic updates** and **on by
  default**, including on an install that upgrades into it. A plan whose project is `any`, or whose
  project name is not a usable collection name, is skipped rather than written to `default`.

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
`curl`. Both run with only `PATH`, `HOME` and `LANG` from the backend's environment, so neither
sees the backend's keys and tokens. They also do not see `HTTP(S)_PROXY`, `KUBECONFIG` or other CLI
configuration.

`api_call` accepts only the curl options listed in its tool description
(`internal/service/api_call_guard.go`) and exactly one `http://` or `https://` URL. It refuses:

- reading a value from a file (`-d @file`, `-H @file`, `--data-urlencode name@file`, `-b file`,
  `-K`);
- writing output anywhere but `-o /dev/null`;
- `--unix-socket`, proxies, `--resolve`/`--connect-to`, and following redirects (`-L`). The agent
  has to read `Location` with `-i` and call that URL itself.

The host is resolved before curl runs. A host with any loopback, link-local (`169.254.0.0/16`,
cloud metadata) or other metadata address among its answers is refused, and curl is pinned to the
vetted address with `--connect-to`, so a second DNS answer cannot redirect it. Private
(RFC 1918) addresses are allowed, since reaching internal APIs is the point. `execute_command` has
none of these limits: it can still run `curl` itself.

The backend image runs as `nib` (uid/gid 10001), not root. `docker-entrypoint.sh` starts as root
only long enough to seed `DATA_DIR`, `chown` whatever in it is not `nib:nib` (a volume from an older
release is root-owned), and join the gid that owns the Docker socket. It then drops to `nib` with
`setpriv --no-new-privs` and no capabilities. On Docker Desktop that gid is 0, so nib joins the
root group, which the entrypoint logs as a warning. A container started as another user (a
Kubernetes `securityContext`) skips all of this.

### API authentication

Every `/api/v1` route except `/health`, `/version`, `/auth/session` and the agent-runner webhook
requires `NIB_API_TOKEN` (at least 32 characters). Scripts send `Authorization: Bearer <token>`.
The web UI asks for the token once and posts it to `/auth/session`, which sets an HttpOnly
`SameSite=Strict` cookie holding a signed expiry, never the token. So rotating the token signs
every browser out.

**The backend refuses to start without a token** unless `NIB_INSECURE_NO_AUTH=true`, which opens
the API to anyone who can reach it. A configured token always wins over that switch.
`cmd/toolcatalog` loads the same config and needs neither, since it serves nothing.

Two browser guards apply whatever the auth mode:

- A cross-site `POST`/`PUT`/`DELETE` is refused with 403 (`http.CrossOriginProtection`, trusting
  `NIB_ALLOWED_ORIGINS`).
- A JSON route refuses any `Content-Type` other than `application/json` with 415.

`NIB_ALLOWED_HOSTS`, when set, refuses any other `Host` with 421 to block DNS rebinding. The
machine endpoints skip both guards, since probes and the agent-runner come from other hosts.

The webhook takes neither the API token nor the signing key. It takes the **per-run token**
handed to each agent-runner container: `HMAC-SHA256(key, chat_id + job_name)`
(`internal/webhookauth`), checked against the `chat_id` and `job.name` in the body. So a
container can report its own run and no other, and a replayed delivery is a no-op thanks to the
per-job dedup. The key is `AGENT_WEBHOOK_TOKEN` when set, otherwise generated once into
`{DATA_DIR}/.webhook-key` (0600), and it never leaves the backend. There is no insecure bypass:
`NIB_INSECURE_NO_AUTH` does not open the webhook, and the backend refuses to boot when it can
neither read nor create the key. Changing the key orphans any run still in flight.

### Secrets

`prompt_secrets` in Postgres, AES-encrypted with `SECRETS_ENCRYPTION_KEY`, managed at
**Variables → Secrets** and read by the agent with `get_secrets`. Without the key secrets can be
neither written nor read. Values are redacted from every MCP transport error. Each secret carries
the hosts `mcp.json` may send it to (see *MCP servers* above).

### Executor — where planned actions actually run

The executor launches an agent-runner container per action; it is **operator-configured, never
detected**, at **Executor**, and persisted under `DATA_DIR`.

- Type: `disabled` (nothing is ever launched), `local`, `remote`. While it is `disabled`, a code
  action's *Execute action* button is not refused — pressing it opens **Executor**
  (`/tools?tab=executor`), since enabling the executor is the only thing that makes the row runnable.
- Remote platform: `docker`, `kubernetes` (auth `local_config` — kubeconfig, falling back to
  in-cluster — or `token`), `kubefoundry`.
- Agent image: `claude-code` or `codex`; auth `api_key` or `oauth_token`, from `EXECUTOR_*`.
- On `local`, the configured image is pulled when the host's Docker daemon does not already
  have it, and reused as-is when it does — so a moving tag keeps whatever was pulled first,
  and `docker pull` is still the way to refresh one. Remote Kubernetes leaves the pull to the
  kubelet's `imagePullPolicy`. A registry that refuses the pull fails the launch with the
  registry's own reason rather than a bare "No such image".
- Credentials are **selected by secret name**, never fixed: the LLM key is `tokenSecretName`, the
  git API token is `gitTokenSecretName`, both picked in the Executor page from **Variables →
  Secrets**. There is no `EXECUTOR_GIT_API_TOKEN` fallback — a blank selection is refused at launch
  with `ErrExecutorGitTokenSecretRequired`, not at save time, so an executor can be configured
  before its secrets exist. Remote Kubernetes is the exception: the agent Job takes its credentials
  from the operator-managed Secret in `agentSecretName` (`envFrom`), so the git token may be blank.
- The runner reports back over the webhook with a token bound to its own run, set as
  `WEBHOOK_AUTH_HEADER` by the backend on both local containers and Kubernetes Jobs (injected after
  rendering, so a `job.yaml.tmpl` override cannot drop it, and it beats the same name in
  `agentSecretName`). The Job spec therefore carries that one run's token in plain text.
- On `local`, a running code action's row in the plan view offers its container logs, polled every
  10 seconds while the modal is open and never stored. They are readable **only while the run is
  running** and **only on `local`**: remote Kubernetes answers that logs are local-only, and a
  finished run's account is in its execute chat instead. A force stop removes the container, so its
  logs go with it.

Git provider: `GIT_PROVIDER` is derived from the free-text `VersionControlSystem` company variable
by `executor.NormalizeGitProvider` — anything containing "gitlab" is GitLab, everything else
including blank is GitHub. The agent container branches on it: GitHub clones as
`x-access-token:<token>` and opens PRs with `gh`, GitLab clones as `oauth2:<token>` and opens MRs
with `glab`. The host comes from `REPO_URL` rather than a hardcoded `github.com`, so self-hosted
GitLab and GitHub Enterprise work; an ssh/scp `REPO_URL` is refused, since a token cannot
authenticate it.

Three things launch such a container: the operator pressing *Execute action* on a `code` step, the
`run_executor` tool in `execute` mode, and `run_subagent` with `name: "code"` — the orchestrator's
way to make a code change the plan does not contain, typically a correction to what a code action
produced. All three run the same image; only the last two are reachable from a chat, and a fix takes
the single execution slot like an action does.

That container is a **different machine from the backend**, with its own toolchain. What is on the
backend's PATH says nothing about what the executor can run, and vice versa.

### LLM provider

`llm.baseURL`, `llm.model`, `llm.embeddingModel`, `llm.api` (`chat` vs `responses`),
`llm.reasoningEffort`, `llm.timeoutSeconds` in `config.yaml`; the key comes from `LLM_API_KEY`
(fallback `OPENAI_API_KEY`). Any OpenAI-compatible endpoint works — see
`docs/how-to/switch-llm-provider.md`, which carries a ready-made block per provider.

**Completions and embeddings are configured separately.** The `llm.embeddings` block has its own
`baseURL`, `model` and `dimensions` (key: `LLM_EMBEDDINGS_API_KEY`), each falling back to the
matching `llm.*` value, so omitting the block keeps one provider serving both. It exists because
DeepSeek, xAI Grok and Moonshot Kimi serve chat completions and **no `/embeddings` route at all**;
without a second host, pointing `llm.baseURL` at one of them breaks knowledge search and the tool
catalog. `kb_chunks.embedding` is `vector(1536)`, so a model that is not natively that wide
(Gemini 3072, Qwen 1024) needs `llm.embeddings.dimensions: 1536`; an endpoint that ignores the
parameter fails the request rather than storing vectors that would never match. The `kb-mcp`
container embeds the *queries* and must agree on both model and width
(`KB_EMBEDDINGS_BASE_URL`, `KB_EMBEDDINGS_DIMENSIONS`).

`/v1/responses` is OpenAI-only; everything else needs `api: chat`, which costs reasoning replay.
`reasoningEffort` is sent on every request whatever the model is — Grok and Kimi reject it, Gemini
accepts only `none`/`low`/`medium`/`high` — so those presets clear it. An explicit
`reasoningEffort: ""` in the file overrides `LLM_REASONING_EFFORT`; deleting the key defers to it.

The `chat` path is deliberately tolerant of the dialects gateways emit: tool calls missing `type`
or `id`, arguments sent as an object rather than a string, `reasoning_content` (captured, never
replayed — DeepSeek rejects it coming back), and `finish_reason`/`refusal`, where a `length`
truncation is an error rather than a short answer. Tool schemas and tool names are rewritten onto
the subset every provider accepts before they are sent, since a strict validator rejects the whole
request rather than the offending tool; dispatch keeps the server's own spelling on the route.

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
- **MCP tools** — discovered concurrently from Postgres token connections and `data/mcp.json` servers, cached
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
  beside the fan-out's answer delivery.
- **plan** and **execute** start *asynchronously* and return "started": a fan-out can run for an
  hour and an action for half of one, both longer than the HTTP write deadline. They report into the
  orchestrator's chat through the existing named-message and SSE machinery.
- **code** ([code_fix.go](backend/internal/service/code_fix.go)) is the odd one out: not an agent of
  nib's own but an agent-runner container, launched with `StartCodeFix` and reporting through the
  webhook. It is the chat's way to a code change with no plan row behind it — the operator reads what
  a code action produced, finds it wrong, and says so. `branch` is what decides where the fix lands:
  naming the branch a code action pushed to continues it, because `agent-entrypoint.sh` checks an
  existing remote branch out and reuses the pull request already open on it; omitting it starts
  `nib/fix-{dialog}`. `run_executor` cannot serve this — its request carries no chat id, so its
  result has nowhere to land, which is also why `actionAgentAllowSet` withdraws it.

A stage planner cannot suspend the way decompose does — its turn holds state that is not on disk,
and the wave it belongs to is waiting on its goroutine — so `report_blocker`
([plan_fanout_question.go](backend/internal/service/plan_fanout_question.go)) **blocks inside the
call**: the question is posted into the orchestrator's chat naming the stage that raised it, and the
answer comes back as the tool's own result, in the same turn. Only that stage waits; its siblings
keep planning, and the wave ends when every stage in it has finished. Questions are serialised per
plan by a one-slot desk, because `findPendingAskQuestion` renders only the newest unanswered ask and
a second question posted beside the first would strand it. A waiting stage leaves the *run* running,
so `ErrFanoutInProgress` still refuses a second fan-out and `ReconcileStuckRuns` still closes the run
after a restart — the waiter is a channel and dies with the process. The run's timeout is a budget of
*working* time, charged by a watchdog that stops while any stage waits, and an unanswered wait
returns the agent's own stated assumption so the plan never gains a gap.

Only the orchestrator sees `run_subagent`, `stop_execution` and `execute_action`. Two guards keep it
that way: the tools are registered only when `toolBinding.planID == toolBinding.dialogID` — true
only on a root turn — and `stripSubagentTools` removes them from every sub-agent's allow set
whatever an operator's edit to a mode list says.

**One execution at a time**, across every plan, held by the lease in
[execution_lease.go](backend/internal/service/execution_lease.go) and covering action sub-agents,
code-action containers and chat-requested code fixes. A fix's lease carries `Fix: true` and no row
key, which is what keeps the force-stop and expiry paths from writing an outcome against a plan row
that does not exist. A second request is refused with a sentence naming what holds
it rather than queued, `agent.actionExecConcurrency` is clamped to 1, and `DELETE /api/v1/execution`
force-stops the holder — cancelling a sub-agent's context, or stopping a container through
`executor.StopAction`. That route takes no write deadline on purpose: it has to work while the agent
loop holding the lease is wedged. `ReconcileStuckRuns` sweeps records a previous process left
`running` at boot, without which a restart would block execution permanently.

**Execute all** on a stage header (or the rollback's) is a stage run
([stage_run.go](backend/internal/service/stage_run.go), `POST/GET/DELETE
/dialogs/{id}/action-plan/stage-run`): the stage's unticked steps, then its checks, each started
only after the one before it has finished. It bypasses the orchestrator on purpose — an action's
result is posted into the plan chat without starting a turn, so a model would have nothing to wake
it for the next item. Every terminal path of an item (sub-agent goroutine, webhook, lease expiry)
ends in `advanceStageRun`, called only *after* the lease is released, or the next item would be
refused as busy; it is idempotent, keyed on `Current` plus the attempt it launched from. The run
stops on the first item that does not end `done`, on a busy slot, a force stop, a reorder within the
stage, a change in the stage's shape (title and item counts — rewording an item does not count), a
replaced plan and a restart. One stage run at a time across every plan, like the lease.

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

When that run ends it chains a second one, `StartKBUpdateAgent`
([kb_agent.go](backend/internal/service/kb_agent.go)): `execute` mode under the `execute_kb`
overlay, a literal three-tool catalog of `get_action_list`, `get_kb_document` and `update_kb`,
seeded with the report the first agent just wrote. Chained rather than run beside it because the
report is already the account of what the plan did, and deriving it twice would only disagree.

It is off when `autoUpdate` is off, and skipped when the plan has no usable collection. Which
collection that is comes from `data/plan_selection/{root}.json`, written on the plan's first turn:
the header selection is global and in-memory, so reading it at Finish would answer with whatever
the operator has selected by then. `data/kb_updates/{root}.json` records the run and is what stops
a second press of Finish repeating it — a *failed* run writes none and stays retryable, while a run
that judged the plan taught the knowledge base nothing writes one with `"changed": false`.

Unlike the report agent it holds a lock on the collection for the whole run, because `update_kb` is
a read-merge-overwrite: two plans finishing against one project would otherwise each publish a
document built from the text before the other started. It blocks rather than skipping, since no
request is held open. It takes no execution lease — it rewrites nib's own knowledge base, never a
managed system — and `actionAgentAllowSet` withholds `update_kb` for the same reason it withholds
`set_report`.

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
`data/plan_state/{id}.json`, `data/plan_fanout/{id}.json`, `data/subagents/{id}.json`,
`data/code_fixes/{id}.json`.

An action plan carries side files beside it, all keyed by the same dialog id and by the positional
row key of an action (`s0.step1`, `rollback.2`): `.checks.json` (the operator's checkboxes),
`.comments.json` (their comments), `.runs.json` (the execute dialog per row), `.exec.json` (the last
run's status) and `.notes.json` (what that run reported). Every one of them is positional, so all of
them are remapped together whenever a stage is rewritten, reordered or replaced — `create` clears
them, the HTTP `PUT` clears none. `.stagerun.json` (the latest "execute all") is the exception: it
is never remapped, because a structural edit stops the run instead; `create` clears it and
`ReconcileStuckRuns` stops one a restart stranded.

`.notes.json` is the exception to "artifacts are for the operator": it is the executors' own memory.
A finished action sub-agent's final message is recorded there and handed to the sub-agents that run
the later actions through `get_action_list`, which is how a resource id one action creates reaches
the action that needs it. It is deliberately unreachable from the HTTP API — its accessors are
unexported for exactly that reason — and a successful run's text is kept out of the plan chat, which
shows only that the action is done plus a link to the transcript. A failure, a question or a
cancellation still says why in the chat.

`code_fixes/{root}.json` maps a chat-requested code fix's own dialog id to its run. It exists
because such a fix has no action row to be recorded on, and the agent-runner webhook needs something
durable to recognise it by: the container outlives the process that launched it, so `ReconcileStuckRuns`
sweeps a fix a restart stranded exactly as it sweeps an action. A webhook delivery whose dialog matches
neither `.runs.json` nor this file is a `run_executor` container and is left alone.

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

The store alone is not a boundary: secret values are write-only everywhere else, and whoever can
edit mcp.json could point a server at their own host and read any secret back out. So each secret
is bound to hosts (`allowed_hosts`), and `resolver.lookup` checks the server URL's literal host
against them *before* decrypting (`mcpconfig/hosts.go`; `serverHost` refuses a reference in the
scheme/host/port). The list is guarded like the value: `SecretService.Update` refuses a new host
without the value (`ErrSecretHostsNeedValue`), because the API that edits mcp.json also edits
secret metadata. The same rule applies to token connections, which need a new token to change
origin. And the round trippers in `mcpclient` add credentials only for the endpoint's origin, with
`CheckRedirect` refusing cross-origin hops — a `RoundTripper` runs again on every redirect, so it
would otherwise re-add the `Authorization` that net/http strips. Existing secrets were bound once at
upgrade to the hosts already referencing them (`cmd/nib/secret_hosts.go`, recorded in
`schema_migrations`); nothing binds a host automatically after that.

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
Finished agents call back into `POST /api/v1/agent-runner/webhook`, authenticated with a per-run
token signed by the key in `executor.Secrets.WebhookKey`. The container images live in
`agent-runner/`.

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
  [.claude/skills/go/SKILL.md](.claude/skills/go/SKILL.md).
- TS/React: tabs, single quotes, no semicolons, kebab-case filenames, function components,
  `handle*`/`is*`/`use*` naming.
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
