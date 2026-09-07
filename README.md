# NIB - Neuro Infrastructure Builder

## Overview
NIB is a tool built for infrastructure work. It lets you decompose any task, lay out the concrete
list of actions needed to complete it, describe the verification checks and the rollback plan, and
then either execute each step or hand the operator the most detailed instructions possible.

The tool is meant to
- Cut the time infrastructure teams spend on planned work
- Improve the predictability and quality of execution
- Become a source of skills

## Core concepts
### Modes
Work happens in several modes (the current mode is shown in the top right of the interface)
* Main - the orchestrator, and the only mode you start a plan in: press New plan and describe the task. It does no work itself. It decides which specialist the request needs and launches it as a sub-agent, relays any question back to you, and reports each result in the same chat. The three modes below are those specialists — you can reach their transcripts, but you talk to Main.
* Decompose - breaks a task into simple stages. You can either send the task description into the chat or ask the agent to pick up a task from your issue tracker (an issue tracker integration over MCP has to be configured first).
* Plan - works the plan out in detail. The task is broken down into elementary steps (code changes, running commands, working through a web interface). One agent per stage, planning in parallel, then one more that derives the rollback from all of them.
* Execute - carries the steps out. Only one execution runs at a time; you can force-stop the running one from the action row or by asking in the chat.
* Discuss - free-form conversation with the agent. This is the only mode whose system prompt you can edit.
* Incident - deep root cause analysis of incidents (in development)

### LLM providers
The backend talks to a single OpenAI-compatible endpoint - for both chat and embeddings.
Choosing a provider, switching between `/v1/chat/completions` and `/v1/responses`, and
falling back to OpenRouter are described in [docs/llm-providers.md](docs/llm-providers.md).

### Metrics
The backend exposes Prometheus metrics on a listener of its own (`:9090/metrics` by default),
covering HTTP traffic, the Go runtime, plans by status, the agent loop, LLM calls, the execution
lease and agent containers. Metric names, configuration and useful queries are in
[docs/metrics.md](docs/metrics.md).

### Knowledge base
The knowledge base is backed by pgvector.
Each project's document follows a skeleton that ships with the image; the agent reads the current
one with the `get_kb_document` tool and writes the collection back with `update_kb`. The
`build-knowledge-base` skill drives the whole round trip - give it a project name and a list of
repositories and it fills the skeleton in from what the repos say.

### Rules
Rules describe the company's accepted principles for working with the various infrastructure components. While decomposing, the agent works out the problem domain and which systems it will have to deal with. It lists them, and where such rules exist their descriptions are added to the prompt during detailed planning.

### Variables
Variables can be used when writing rules, skills, and the Discuss mode system prompt.
There is a set of predefined variables that cannot be deleted. Filling them in is strongly recommended - they let the agent work more precisely.

### Tools
Connections to remote MCP servers are supported. This can be done through data/mcp.json or the web interface.
Every tool a server exposes is switched on for every mode as soon as it is indexed, so a newly added MCP server
is usable right away; deleting a server takes its tools back out of every mode.
The number of tools available to the agent can then be limited per mode on Tools -> Included tools - a tool taken
out there stays out, and the remaining tools stay reachable through search.
The Distribute with AI button on that tab hands the whole catalog to a discuss chat, which applies the
per-mode policy in the `distribute-tools` skill - read-only modes lose the tools that change things,
execute loses the version control ones. It only ever removes; putting a tool back is a move on the page.
Tools can be sorted into different categories.

Any value in an MCP server entry may use a reference of the form `${NAME}` - for example `"Authorization": "Bearer ${MCP_GITHUB_TOKEN}"`.
Substitution happens only at the moment the MCP server is called, so the token itself never lands in `data/mcp.json`.
The value comes from the encrypted secrets (Variables -> Secrets) and from nowhere else - the backend's own environment
is not a source, so an entry in `mcp.json` cannot name the LLM key, the database password or any other variable the
backend runs with. A name with no secret behind it stops only that one server; the rest carry on. For a literal `${`, use `$${`.

### Templating
Go templates are supported when writing rules, skills, and the Discuss mode system prompt.
Example
```
```

### Skills
Skills are markdown files under `data/skills` (one `{name}.md` per skill, with a `name` and
`description` frontmatter). Every skill is listed at the end of the Main and Discuss system
prompts, and the agent loads the one it needs with the `get_skill` tool. Skills can be written
and edited in the web interface.

A set of skills ships with the image and is seeded into `data/skills` on first start, so a fresh
install already has a working catalog. Seeding is recorded in `data/skills/.seeded-defaults`:
each built-in is written at most once, so a skill you edited, renamed, or deleted stays that way
across restarts and upgrades, while a skill added by a later release still lands on an existing
install. Built-in skills may reference variables (for example `{{ .global.JiraProject }}` and
`{{ .global.Email }}`) - fill those in under Variables, otherwise loading the skill fails with an
unknown-key error.

## Releasing

The application version is defined in the root [`VERSION`](VERSION) file (for example, `v0.7.5`).

To release a new version:

1. Update `VERSION` with the new tag (for example, `v0.7.6`).
2. Merge the change to `main`.
3. GitHub Actions builds and pushes three images — `javdet/nib-backend`,
   `javdet/nib-frontend` and `javdet/nib-kb` (knowledge-base MCP server) — each
   tagged three ways:
   - `<version>` (from `VERSION`, for example `v0.7.6`)
   - `latest`
   - `<git-sha>`

The backend exposes `GET /api/v1/version` and the UI shows the version at the bottom of the sidebar.

### Docker Compose

Production quick start (published images):

```bash
cp .env.example .env
# set LLM_API_KEY in .env
docker compose up -d
# UI: http://localhost:8080
```

Local development (live reload):

```bash
cp .env.example .env
docker compose -f docker-compose.dev.yml up
# UI: http://localhost:5173
```

See `.env.example` for all supported variables.

### Executor type

Settings → Executor → **Type** selects how agent containers are launched:

- **Disabled** (default) — the executor is off. No other executor settings are shown, and **Execute action** is greyed out on `code` steps.
- **Local (Docker socket)** — containers run on the backend host Docker daemon.
- **Remote** — containers run on a remote platform (currently Kubernetes).

### Remote Kubernetes executor

When executor type is **Remote** and platform is **Kubernetes**, pressing **Execute action** on a `code` step creates a Kubernetes Job from `backend/internal/executor/templates/job.yaml.tmpl` (override with `{DATA_DIR}/job.yaml.tmpl` if needed).

Cluster access modes (Settings → Executor):

- **Local Config** — uses `~/.kube/config` (or `KUBECONFIG`), optionally a named context; falls back to the in-cluster service account mount when no kubeconfig is present.
- **Token** — uses the API server URL plus a bearer token stored in Variables → Secrets (`kubernetesTokenSecretName`).

Required Kubernetes settings:

- **Image** — agent container image.
- **Agent Secret Name** — existing Secret in the target namespace mounted via `envFrom`. It must contain agent credentials, for example:
  - `GITHUB_TOKEN`
  - `ANTHROPIC_API_KEY` or `CLAUDE_CODE_OAUTH_TOKEN`
  - optional `WEBHOOK_AUTH_HEADER` (for example `Authorization: Bearer <AGENT_WEBHOOK_TOKEN>`)
- **Webhook base URL** — must be reachable **from inside the cluster** (not `http://localhost:8080` unless the backend runs in the same pod network). The path `/api/v1/agent-runner/webhook` is appended automatically.

Optional advanced settings: namespace, service account, resource limits/requests, MCP ConfigMap name, job TTL, TLS CA / skip-verify for token auth.

When the Job finishes, the agent posts results to the execute chat webhook; no extra backend changes are required for that callback.
