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

Database-backed tests skip unless a pgvector server is named, one database per package since each
resets its schema: `NIB_POSTGRES_TEST_DSN` (`internal/repository/postgres`), `NIB_SERVICE_TEST_DSN`
(the multi-process tests in `internal/service`) and `NIB_MIGRATIONS_TEST_DSN` (`cmd/nib`). CI runs
them against a `pgvector/pgvector:pg18` service. Locally, start one on a user-defined network and run
the test container on it, never on the host network.

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

Any value in an `mcp.json` server entry (URL or header) may reference `${NAME}`, expanded
from the encrypted secret store at call time only — never from process environment. Without
`SECRETS_ENCRYPTION_KEY` every `${NAME}` stays unresolved.

Any global secret can be referenced from any server; secrets carry no per-host restriction.
MCP clients add headers and tokens only to requests for the server's own origin, and refuse a
redirect to another one.

### Modes, prompts and how to change them

Modes: `main` (orchestrator; routes to sub-agents), `decompose` (task → DAG), `plan` (DAG → action
plan), `execute` (carry out one action), `discuss` (read-only investigation), `incident` (incident
commander; see *Incidents* below). Overlay prompts `plan_stage`, `rollback_stage`, `execute_action`,
`execute_report`, `execute_kb` and `decompose_subagent` are appended to their base mode's prompt. The
incident's sub-agent prompts `incident_digger`, `incident_synthesis`, `incident_impact` and
`incident_closure` stand alone instead, since the `incident` prompt is the orchestrator's.

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

Local commands are **jobs** (`internal/cmdrun`), driven by five tools that replaced
`execute_command`. A `{mode}.json` that still lists `execute_command` gets the five in its place on
the next boot. A list whose operator had removed it does not get them
(`renamedTools` in `internal/mode/allowtools_seed.go`). Diggers get them from `diggerBaseTools`.

- `run_command(script, wait_seconds, timeout_seconds)` runs a bash script. It returns when the script
  ends or after `wait_seconds` (default 30, max 900), with a `job_id` if it is still running. The
  script is killed after `timeout_seconds` (default 600, max 3600).
- `wait_job(job_id, wait_seconds ≤ 900, from_offset)` blocks for one round instead of one round per
  poll.
- `read_output(job_id, offset, limit, grep)` pages through or greps the full log.
- `cancel_job(job_id)` sends SIGTERM, then SIGKILL after 10 s.
- `wait_until(script, until: exit0|match, pattern, interval_seconds, timeout_seconds ≤ 900)` reruns a
  check without the model.

A job never outlives the agent run that started it: it hangs off the tool call's context, so a force
stop, an action timeout, a cancelled investigation or the end of a chat turn kills it. A job is
visible only to the dialog that started it. Each job keeps up to 10 MiB of output in
`{DATA_DIR}/command_logs/{plan}/{job}.log` (swept after `commands.logRetentionDays`, default 7). A
result hands the model at most 32 KiB, its head and tail. Every run is audited in the `command_runs`
table, and the plan page's **Commands** button on an action or check row lists them with their
output and a Cancel. At most 4 jobs run per dialog and 16 in all. A job lives in the process that
started it: Cancel from another process is relayed to that one, and a row whose process is gone is
marked `lost` by the next sweep.

**What the backend image carries** for those commands (`backend/Dockerfile`): `bash`, `curl`, `jq`,
`yq`, `openssl`, `dig`/`nslookup`, `nc`, `telnet`, `ping`, `mtr`, `ip`, `python3`, `ssh`, `gh`,
`glab`, `kubectl`, `helm`, `kustomize` and `ansible-core` (`ansible`, `ansible-playbook`). It
deliberately leaves out `git` (`gh api`/`glab api` read a repository; a working copy is the
executor's job), the AWS CLI, `gcloud`, `doctl` and Terraform/OpenTofu, to keep the image small.
None of them is configured: a job sees no kubeconfig, cloud profile, SSH key or `GH_TOKEN`/
`GITLAB_TOKEN`, so a credential has to be passed explicitly (from `get_secrets`) or the system
reached through MCP. `hostCommands` reports
what is actually on PATH, including tools an operator's own image adds.

**Isolation** is `commands.isolation` in `config.yaml`:

- `auto` (default) runs each job as a uid of its own from `commands.uidStart`
  (20000, `uidCount` 1000) with no supplementary groups, so it cannot reach `DATA_DIR` (made 0700 at
  boot), the backend's `/proc` or the Docker socket. It falls back to the backend's own uid, with a
  boot warning, when the backend can neither run as root nor hold `CAP_SETUID`, `CAP_SETGID` and
  `CAP_KILL`.
- `required` refuses commands instead of falling back.
- `off` always uses the backend's uid.

The job starts in an empty `HOME` under `/tmp/nib-jobs` that is deleted afterwards, with only `PATH`,
`HOME`, `TMPDIR` and `LANG` set, so it sees none of the backend's keys and tokens, `HTTP(S)_PROXY`,
`KUBECONFIG` or other CLI configuration. `api_call` is a fixed `curl` and runs in the same sandbox.

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
(RFC 1918) addresses are allowed, since reaching internal APIs is the point. `run_command` has none
of these limits: it can still run `curl` itself, only from its sandbox.

The backend image runs as `nib` (uid/gid 10001), not root, under `tini` as PID 1, which reaps
whatever a killed job backgrounded. `docker-entrypoint.sh` starts as root only long enough to:

- seed `DATA_DIR`;
- `chown` whatever in it is not `nib:nib` (a volume from an older release is root-owned);
- clear stale job homes;
- join the gid that owns the Docker socket.

It then drops to `nib` with `setpriv --no-new-privs`. It keeps exactly `CAP_SETUID`, `CAP_SETGID` and
`CAP_KILL` as ambient capabilities, with a bounding set of only those three, for the job sandbox.
Every child process drops all three again before it execs (a `setpriv --inh-caps=-all
--ambient-caps=-all` wrapper), since ambient capabilities survive a switch between two non-root
uids. On Docker Desktop the socket's gid is 0, so nib joins the root group, which the entrypoint
logs as a warning. A container started as another user (a Kubernetes `securityContext`) skips all of
this and runs commands unisolated. So does one whose `securityContext` drops the three capabilities
without adding them back.

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
- A private registry is reached with **Private registry** on the Executor page:
  `registryUsername` plus `registryPasswordSecretName`, both or neither. On `local` the login
  goes with the pull and the secret is read only when a pull happens. On remote Kubernetes each
  Job gets its own `{job}-registry` dockerconfigjson Secret in `imagePullSecrets`, owned by the
  Job so it goes with it, which needs nib's cluster identity to create, update and delete
  Secrets in the namespace.
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
- On `local`, **Delete agent containers when a plan is cancelled** (`removeContainersOnPlanCancel`,
  on by default, including for an `executor.json` written before it existed) makes **Cancel plan**
  stop the plan's running container through the execution lease (its row or fix closes as
  cancelled) and force-remove every container the plan launched, running or exited. Containers
  are found by the `io.nib.plan` label stamped at launch, plus the ids the plan's exec and
  `code_fixes` records hold for ones launched before the label existed. A failed removal leaves
  the plan cancelled and is reported beside the status; cancelling again retries it. Remote
  Kubernetes ignores the setting.

Before the agent starts, the container checks that the git token can push with a
`git push --dry-run` to the target branch. A token that cannot push fails the run then, not
after the agent has spent its budget. A clone does not catch this, because a public repository
never asks for credentials. The runner, not the agent, commits, pushes and opens the request,
and the agent is told so. Any step that fails after the agent finishes reports the run as
`failed`, with the runner's reason (the webhook's `error`, for example the remote refusing the
token) shown ahead of the agent's own summary.

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

A request the provider rejects ends the turn with the reason in the chat: nib's own sentence, what
the provider said, and the endpoint, model and reasoning effort the request was sent with — never
the key. A 401 against an endpoint that does not match the key (an OpenAI key sent to
`openrouter.ai`) is the usual cause, and `llm.baseURL` in the mounted `config.yaml` is the only
place the provider is chosen.

**Retries and fallback.** A completion that fails with HTTP 408, 409, 429 or 5xx, or whose
connection drops, is sent again up to `llm.maxRetries` times (default 8, `LLM_MAX_RETRIES`; `0`
never retries). The wait doubles from 1 s to a 60 s cap. A `Retry-After`/`Retry-After-Ms` is
honoured up to `llm.maxRetryDelaySeconds` (120), and a longer one gives the model up. A timeout
(`llm.timeoutSeconds`) is retried once only. The SDK's own retries are off
(`internal/llm/retry.go` owns them), and embeddings keep the SDK's two. `llm.fallbackModel`
(`LLM_FALLBACK_MODEL`) is a second model on the same endpoint and key, tried once the primary has
used up its retries on such a failure, never on a 400/401/404. After a switch it goes first for
five minutes. Retries and switches log at `WARN` and count in `nib_llm_retries_total` /
`nib_llm_fallbacks_total`. An action that still dies can be continued rather than rerun (see
*Orchestration*).

Round budgets are `agent.maxIterations` and the per-sub-agent `stageMaxIterations` /
`actionExecMaxIterations` / `incidentDiggerMaxIterations` / `incidentStepMaxIterations`. Only one execution runs at a time, by policy, and
`agent.actionExecConcurrency` is clamped to 1.

### Sharing a plan: `.nib` files

A plan page's import/export menu offers **Download as Markdown** (read-only),
**Export as .nib file** and **Import plan from .nib file**. The **Plans** page also has an import
button. The API is `GET /api/v1/dialogs/{id}/export` and `POST /api/v1/dialogs/import`. A `.nib`
file is JSON (`"format": "nib-plan"`, `"version": 1`). It carries the title, categories, summary,
DAG, plan contract, action plan with comments, the plan chat, the `decompose` and `plan`
sub-agent transcripts (attachments in base64), and any unanswered decompose question.

It does **not** carry execution state (checks, runs, exec records, notes, stage runs, status,
schedule, fan-out record, report, KB updates, code fixes, `execute` transcripts) or system
prompts. An import renders fresh prompts from the importing install's variables and current
selection. It always creates a **new draft plan** with new ids for every dialog and attachment,
and rewrites the old ids wherever they appear in the imported text. It is all or nothing, and
the body is capped at 64 MiB. MCP servers, secrets, variables, skills, rules and executor settings
are not in the file. A step calling a tool the importer lacks fails when it runs, not at import.
See `docs/how-to/share-a-plan.md`.

### Incidents

An incident is a root `incident` dialog, created by **New incident**. Its deliverable is a
**post-mortem** at `{DATA_DIR}/postmortems/{id}.json`, filled in live while the investigation runs and
rendered on the incident page: status (`ongoing` → `mitigated` → `resolved` → `closed`), severity
`sev1`–`sev4`, environment, started/detected/mitigated/resolved times, summary, working hypothesis,
root causes, impacted areas, timeline, short-term and long-term corrective actions, per-system
findings, impact report and closure summary.

- **The orchestrator** is the incident dialog itself. It triages (ongoing or past, where, since when,
  severity), keeps the post-mortem current with `update_postmortem`, and launches the investigation
  with `run_investigation`. It never changes a system and never launches the plan track's
  sub-agents: `run_subagent`, `execute_action`, `run_executor`, `set_report` and `update_kb` are
  withdrawn from incident mode in code (`enforceModeToolLimits`).
- **`run_investigation`** starts a run in the background and returns `started`. One **Digger** per
  system (at most 8, `agent.incidentConcurrency` at once) examines its system before and during the
  window and files a finding with `report_finding`; then a **synthesis** agent writes the hypothesis,
  root causes, merged timeline and actions while an **impact** agent writes the impacted list and the
  impact report. Each runs in a child `incident` dialog with its own literal catalog. The outcome is
  posted into the incident chat as a named message; it does not start a turn.
- **Read-only is enforced by prompt only.** Diggers and the impact agent hold the command tools
  (`run_command`, `wait_job`, `read_output`, `cancel_job`, `wait_until`), `api_call`, the MCP tools
  included for incident mode and those of their categories. They are told not to mutate anything and
  take no execution lease. Like every command, theirs sees no `KUBECONFIG` or cloud credentials, so
  clusters and clouds are reached through MCP.
- **Operator edits win.** The page edits through `PUT /dialogs/{id}/postmortem` with a patch. A
  scalar field the operator edits is locked against agents until handed back (`locks`), and a list
  item the operator wrote, or an action already in a plan, survives an agent's rewrite of the list.
- **Create plan** hands the ticked actions to the change-management track: one new `main` plan
  seeded with the incident's context, linked both ways (`linkedPlans` in the post-mortem,
  `{DATA_DIR}/plan_origin/{plan}.json` for the plan), with the actions marked `in_plan`.
- **Close incident** runs the impact agent over the final window and then a closure writer; the
  incident is marked `closed` only when the closure summary is written, and agents cannot change a
  closed post-mortem. A failed closure leaves it open to retry.
- The latest run is the incident's `incident_run` document in the `run_state` table; a run whose process
  died is closed as failed
  at boot. Budgets: `agent.incidentConcurrency` (4), `incidentDiggerMaxIterations` (25),
  `incidentStepMaxIterations` (15), `incidentTimeoutMinutes` (30).

### Restarts and graceful shutdown

On SIGTERM (or SIGINT) the backend **drains** before it exits, within `SHUTDOWN_TIMEOUT_SECONDS`
(default 900, env only, since it has to fit inside the deployment's kill timer). The chart's
`backend.terminationGracePeriodSeconds` (900) sets both: the pod's grace period, and the variable 5 s
below it. Compose uses `stop_grace_period` (`NIB_STOP_GRACE_PERIOD`, 905 s).

- **New work is refused** with a 503 and `ErrShuttingDown`: a chat message, a retry, Execute on any
  row, a code fix, a fan-out, a stage run, the report on Finish, an investigation or closure. A
  running stage run stops at its next item for that reason.
- **Work in flight continues**: the running action, code action container or code fix (the
  execution lease), fan-outs, investigations, the report and knowledge-base agents, and chat turns
  already under way. An answer to a pending `ask_question` or `report_blocker` is still accepted,
  since refusing it would strand that work. The HTTP server keeps serving throughout, so the UI
  stays live and a container's webhook can still land.
- An idle backend exits at once. Otherwise it logs what it is waiting for every 30 s. A second
  signal cuts the wait short.
- **20 s before the end** (half the budget when it is under 40 s) whatever is left is cancelled. A
  sub-agent action records itself `cancelled` with "interrupted by a nib restart", which **Continue**
  resumes after the restart. A container is left running, since it outlives the process either way;
  the sweep of dead processes closes its row (see *Running more than one backend*), as it does for
  anything a SIGKILL cut off. Commands of cancelled turns are killed, and their rows are marked
  `lost` by the same sweep.
- `/api/v1/health` keeps answering 200 while draining, so probes do not kill the pod early.

The implementation is `workTracker` in `internal/service/shutdown.go` (`Drain`, `StopInFlight`) and
the shutdown sequence at the end of `cmd/nib/main.go`. A new kind of background work has to either
`begin` (refused when draining) or `adopt` (let through, continuing work already in flight) there,
or a restart cuts it off without waiting.

### Running more than one backend

Run state, the execution lease, every lock and the SSE stream are shared through Postgres, so two
backend processes on one database see one plan, one execution slot and one event stream:

- **Run state** — plan status, the per-row exec, runs and notes records, the stage run, the
  fan-out record, sub-agent pauses, code fixes and incident runs — is the `run_state` table, one
  JSONB document per kind and dialog (migration `000042`). The first boot of this release imports
  the old `{DATA_DIR}` files and renames each to `*.migrated`.
- **The lease** is the single row of `execution_lease`. A sub-agent's expires 90 s after its process's
  last heartbeat; a container's at the action deadline, as before.
- **Locks** are Postgres advisory locks on a second pool (`NIB_COORDINATION_POOL_SIZE`, default 24),
  so each process adds that many connections to the server's budget.
- **Events and control** go over `LISTEN/NOTIFY` (`nib_activity`, `nib_control`). A force stop, a
  fan-out or investigation stop, an answer to a planner's `report_blocker` or a command Cancel that
  lands on another process is relayed to the one holding the run.
- **Liveness**: each process registers in `nib_replicas` and heartbeats every 10 s. Every run
  record names its owner. A boot and a sweep every 60 s close running records whose owner is gone:
  missed heartbeats for 60 s, a clean exit, or replaced by a new process on the same hostname (a
  restarted container). A live process's runs are never touched.

Still per process, so `backend.replicaCount` stays 1 in the chart (with `Recreate`): the
`{DATA_DIR}` volume (DAGs, action plans, reports, post-mortems, skills, rules, `mcp.json`, command
logs) needs a shared `ReadWriteMany` volume, the header selection is held in memory, edits to the
`config.yaml` project tree reach only the process that made them, and MCP discovery caches expire
after 60 s rather than on another process's edit. An upgrade from a release before `run_state` cannot
overlap old and new processes: the old one writes files the new one has already imported.

### What an upgrade does and does not touch

A new image replaces the binary, the embedded prompts, the built-in skills and the built-in allow
lists. It does **not** overwrite anything already on the data volume: prompt overrides, edited or
deleted skills and rules, `mcp.json`, allow lists, executor config, dialog artifacts. New built-in
skills are seeded only on a volume that has never been seeded; new built-in *tool names* are merged
into existing allow lists once each. Database state (dialogs, variables, secrets, MCP connections,
knowledge base, run state) is migrated in place by `backend/migrations` at startup.


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
  conversation, so when it calls `ask_question` it suspends: the pause is recorded in the plan's
  `subagents` run-state document, the orchestrator relays the questions under an `ask_question` of its
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

A non-code action whose run ended `failed`, `cancelled` or `blocked` can be **continued** rather
than rerun ([action_resume.go](backend/internal/service/action_resume.go)): `execute` /
`execute_action` with `resume: true` (and an optional `note`, the operator's word for it) takes the
lease, finds the run's dialog through the plan's runs record, appends a note saying why it stopped and that
the agent must re-check the state of what it already touched, and runs the loop again in that
transcript with a fresh round budget and deadline. The row's **Continue** button sends "Continue
plan item N…" through the chat, as Execute does. It is refused for a finished, running or never-run
row and for a code action, whose container keeps no transcript in nib.

Once a plan has a DAG, relaunching **decompose** is guarded, and once it has an action plan,
relaunching **plan** is guarded too ([subagent_plan_guard.go](backend/internal/service/subagent_plan_guard.go)).
The orchestrator has to pass `operator_request`, the operator's words asking for the change, and
the guard refuses a launch whose quote is not in the operator's latest input. That input is their
last message or their answers to an `ask_question`, never the question's own wording. It exists
because the model tends to treat a failed action as proof the plan is wrong, and replanning a stage
throws away the checks, runs and notes recorded on its rows. The plan page's buttons call
`StartPlanFanout` directly and never pass through this guard. Relaying an answer to a suspended
decompose is let through.

A DAG that changes under an existing action plan rebuilds that plan from scratch
([action_plan_rebuild.go](backend/internal/service/action_plan_rebuild.go)). `create_dag` only marks
it (`rebuildActionPlan` in the `subagents` document) when the new diagram differs from the stored
one in more than layout and an action plan exists; the rebuild runs when the decompose turn
*finishes*, so a decompose still asking the operator never loses the plan under a DAG it has not
settled. Any fan-out of every stage that finds the mark — that one, the plan page's button, or a
`plan` launch with no stages — first deletes the action plan with its rollback, every positional side
file, the stage run and the fan-out record (its answered blockers were about the old DAG), puts an
`in_progress`/`done`/`reopened` plan back to `draft`, and records `rebuilt` on the new run. A stage
replan does not consume the mark. An item of the plan holding the execution lease holds the discard
off, since its result would land on whatever row took its position; the mark then stays until a
later full fan-out finds the plan idle. A code fix does not count, being tied to a branch, not a row.

A stage planner cannot suspend the way decompose does — its turn holds state that is not on disk,
and the wave it belongs to is waiting on its goroutine — so `report_blocker`
([plan_fanout_question.go](backend/internal/service/plan_fanout_question.go)) **blocks inside the
call**: the question is posted into the orchestrator's chat naming the stage that raised it, and the
answer comes back as the tool's own result, in the same turn. Only that stage waits; its siblings
keep planning, and the wave ends when every stage in it has finished. Questions are serialised per
plan by a one-slot desk, because `findPendingAskQuestion` renders only the newest unanswered ask and
a second question posted beside the first would strand it. A waiting stage leaves the *run* running,
so `ErrFanoutInProgress` still refuses a second fan-out and the sweep of dead processes still closes the run
after a restart — the waiter is a channel and dies with the process. The run's timeout is a budget of
*working* time, charged by a watchdog that stops while any stage waits, and an unanswered wait
returns the agent's own stated assumption so the plan never gains a gap.

Only the orchestrator sees `run_subagent`, `stop_execution` and `execute_action`. Two guards keep it
that way: the tools are registered only when `toolBinding.planID == toolBinding.dialogID` — true
only on a root turn — and `stripSubagentTools` removes them from every sub-agent's allow set
whatever an operator's edit to a mode list says.

**One execution at a time**, across every plan and every backend process, held by the lease in
[execution_lease.go](backend/internal/service/execution_lease.go) (the `execution_lease` row) and covering action sub-agents,
code-action containers and chat-requested code fixes. A fix's lease carries `Fix: true` and no row
key, which is what keeps the force-stop and expiry paths from writing an outcome against a plan row
that does not exist. A second request is refused with a sentence naming what holds
it rather than queued, `agent.actionExecConcurrency` is clamped to 1, and `DELETE /api/v1/execution`
force-stops the holder — cancelling a sub-agent's context (relayed over `nib_control` to the process
that owns it), or stopping a container through `executor.StopAction`. That route takes no write
deadline on purpose: it has to work while the agent loop holding the lease is wedged.
`SweepOrphanedRuns` ([exec_reconcile.go](backend/internal/service/exec_reconcile.go)), run at boot
and every 60 s by the `ReplicaCoordinator` ([replicas.go](backend/internal/service/replicas.go)),
closes records left `running` by a process that is gone, without which a restart would block
execution permanently.

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

An **incident** is a second, smaller orchestrator ([incident_run.go](backend/internal/service/incident_run.go)),
deliberately kept off `run_subagent`: its flat schema is shared by every plan specialist and
`resolveOrchestratorRoot` requires `main`. `run_investigation` registers only on an incident root
(`toolBinding.incidentRole == orchestrator`) and runs Diggers, then synthesis and impact, as two waves
modelled on the fan-out, under a plain deadline — nothing in it waits on the operator. A step that
calls `ask_question` anyway is answered once with "state your assumption" and fails on the second.
Every write to the post-mortem goes through `applyPostmortemPatch`
([postmortem.go](backend/internal/service/postmortem.go)), whose per-role field matrix is what keeps
agents running side by side off each other's sections and off the operator's locked ones. The
handoff to a plan ([incident_handoff.go](backend/internal/service/incident_handoff.go)) creates the
`main` root and returns its first message rather than starting a turn: the web interface queues it
with `enqueuePendingMessage`, so the orchestrator's first turn runs under a request like any other.

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

A `Dialog` UUID is the unit of work. Beyond the Postgres transcript, what a plan *is* lives in files
keyed by dialog id, written atomically via `internal/atomicfile`: `data/dags/{id}.md`,
`data/summaries/{id}.txt`, `data/reports/{id}.md`, `data/action_plans/{id}.json` (with
`.checks.json` and `.comments.json` beside it), `data/plan_origin/{id}.json`, and for an incident
`data/postmortems/{id}.json`. Command output is the exception to atomic writes:
`data/command_logs/{root}/{job}.log` is appended while the job runs and swept by age, and its index
is the `command_runs` table, not a file.

What a plan is *doing* is run state, in the `run_state` table (`repository.RunStateRepository`,
[run_state.go](backend/internal/service/run_state.go)), one JSONB document per kind and dialog:
`plan_state`, `plan_fanout`, `subagents`, `code_fixes`, `incident_run`, and per action row
`action_runs` (the execute dialog per row), `action_exec` (the last run's status), `action_notes`
(what that run reported) and `stage_run`. A service without a store set — every unit test — keeps
them in the files they used to be (`fileRunState`). Each read-modify-write runs under the record's
named lock (`lockFor`, [locks.go](backend/internal/service/locks.go)), an advisory lock when a
`DistributedLocker` is set.

The per-row records are keyed by the positional row key of an action (`s0.step1`, `rollback.2`), as
are the checks and comments. Every one of them is positional, so all of them are remapped together
whenever a stage is rewritten, reordered or replaced — `create` clears them, the HTTP `PUT` clears
none. The stage run (the latest "execute all") is the exception: it is never remapped, because a
structural edit stops the run instead; `create` clears it and the sweep stops one whose process
died.

The `.nib` export ([plan_bundle.go](backend/internal/service/plan_bundle.go)) lists what a plan
*is* by hand, so a new per-plan artifact has to be sorted there as plan or run. Leaving it out is
the safe default for run state. Import writes the rows through `repository.DialogImporter` in one
transaction, keeping each row's `created_at`, since `ListChildren` orders by it.

The notes record is the exception to "artifacts are for the operator": it is the executors' own memory.
A finished action sub-agent's final message is recorded there and handed to the sub-agents that run
the later actions through `get_action_list`, which is how a resource id one action creates reaches
the action that needs it. It is deliberately unreachable from the HTTP API — its accessors are
unexported for exactly that reason — and a successful run's text is kept out of the plan chat, which
shows only that the action is done plus a link to the transcript. A failure, a question or a
cancellation still says why in the chat.

The `code_fixes` document maps a chat-requested code fix's own dialog id to its run. It exists
because such a fix has no action row to be recorded on, and the agent-runner webhook needs something
durable to recognise it by: the container outlives the process that launched it, so the sweep closes
a fix whose process died exactly as it closes an action. A webhook delivery whose dialog matches
neither the runs record nor this one is a `run_executor` container and is left alone.

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

Secrets are not bound to hosts: whoever can edit mcp.json can point a server anywhere and have any
global secret sent there. A per-secret allowed-hosts list existed in v0.8.1 and was withdrawn as too
cumbersome to operate (migration `000040` drops the column). Token connections still need a new token
to change origin, and the round trippers in `mcpclient` add credentials only for the endpoint's
origin, with `CheckRedirect` refusing cross-origin hops — a `RoundTripper` runs again on every
redirect, so it would otherwise re-add the `Authorization` that net/http strips.

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
background goroutine, not at scrape time: counting is a query for the plans plus one for their
`plan_state` documents. Full metric list in [docs/metrics.md](docs/metrics.md).

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
  skills, tool allow lists, dialog artifacts). Editing a file there
  changes the running app immediately, and generated artifacts routinely show up as untracked files
  in `git status`.
- Locks, the execution lease, run state and SSE are shared through Postgres, but a second backend
  still needs a shared `DATA_DIR` and more (see *Running more than one backend*), so the chart keeps
  `backend.replicaCount: 1`. A new in-memory map of running work needs a relay over `nib_control`,
  and a new run record an `Owner` the sweep can judge.
- Releases are driven by the root `VERSION` file merged to `main`; keep `.env.example`
  (`NIB_VERSION`, `NIB_KB_VERSION`) and `frontend/package.json` version in step.
