<p align="center">
  <img src="docs/img/nib-logo-transparent.png" alt="Nib logo" width="400">
</p>

# NIB - Neuro Infrastructure Builder

## Overview
NIB is a tool built for infrastructure work. It lets you decompose any task, lay out the concrete
list of actions needed to complete it, describe the verification checks and the rollback plan, and
then either execute each step or hand the operator the most detailed instructions possible.

The tool is meant to
- Cut the time infrastructure teams spend on planned work
- Improve the predictability and quality of execution
- Become a source of skills

## Quick start

```bash
cp .env.example .env
# set LLM_API_KEY in .env
docker compose up -d
# UI: http://localhost:8080
```

New to nib? [Getting started: your first plan](docs/tutorials/getting-started.md)
walks through the install and turns one sentence into a reviewed action plan.

## Documentation

Full documentation is in [docs/](docs/README.md), in four parts:

- **[Tutorial](docs/tutorials/)** — one lesson, start to finish: install nib and
  produce your first plan.
- **[How-to guides](docs/how-to/)** — install with Compose, switch the LLM
  provider, connect an MCP server, keep tokens out of `mcp.json`, run actions in
  containers, scrape metrics, upgrade.
- **[Reference](docs/reference/)** — every configuration key, environment
  variable, API route, built-in tool, mode, executor setting and metric, and
  where state lives.
- **[Explanation](docs/explanation/)** — the architecture, how a plan is
  produced, and the reasoning behind nib's deliberate limits.

Deployment on Kubernetes is documented with the chart, in
[deploy/helm/nib/README.md](deploy/helm/nib/README.md). The agent container's
own contract is in
[agent-runner/universal-agent/README.md](agent-runner/universal-agent/README.md).

## Core concepts

### Modes
Work happens in several modes (the current mode is shown in the top right of the interface).
* **Main** — the orchestrator, and the only mode you start a plan in: press New plan and describe the task. It does no work itself, launching the specialists below as sub-agents and relaying their results into the same chat.
* **Decompose** — breaks a task into stages.
* **Plan** — works each stage out into elementary steps, one agent per stage in parallel, plus one that derives the rollback.
* **Execute** — carries the steps out. Only one execution runs at a time.
* **Discuss** — free-form conversation, and the only mode whose system prompt you can edit.
* **Incident** — root cause analysis of incidents (in development).

Details: [Modes](docs/reference/modes.md) · [How a plan is produced](docs/explanation/how-a-plan-is-produced.md)

### LLM providers
The backend talks to a single OpenAI-compatible endpoint — for both chat and embeddings.
Choosing a provider and moving between them: [How to switch the LLM provider](docs/how-to/switch-llm-provider.md).

### Knowledge base, rules and skills
Three places to write things down: the pgvector-backed knowledge base the agent searches, rules
describing the principles it must plan by, and skills it loads on demand. Which to use when:
[Knowledge, rules and skills](docs/explanation/knowledge-rules-and-skills.md).

### Tools
Connections to remote MCP servers are supported, registered by URL. Every tool a server exposes is
switched on for every mode as soon as it is indexed; the set can then be narrowed per mode, and
anything taken out stays reachable through search.
See [How to connect an MCP server](docs/how-to/connect-an-mcp-server.md) and
[Modes, tools and the catalog](docs/explanation/modes-tools-and-the-catalog.md).

### Executor
Planned `code` steps can run in single-use agent containers, on a local Docker daemon or as
Kubernetes Jobs. It is off by default:
[How to run planned actions in containers](docs/how-to/run-actions-in-containers.md).

### Metrics
The backend exposes Prometheus metrics on a listener of its own (`:9090/metrics` by default).
See [Metrics](docs/reference/metrics.md) and
[How to scrape them](docs/how-to/scrape-metrics-with-prometheus.md).

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

## Contributing

[CLAUDE.md](CLAUDE.md) is the contributor guide: toolchain, architecture, conventions and the
things to watch out for.
