# nib documentation

nib (Neuro Infrastructure Builder) takes an infrastructure task, breaks it into
stages, plans each stage down to elementary steps with checks and a rollback,
and — when you let it — carries those steps out.

These docs are for the people who run it: DevOps, platform engineers and SREs,
and the developers working alongside them who would rather not have to know
Kubernetes to get a plan. Nothing here assumes prior Docker, Postgres or
Prometheus knowledge.

They are in four parts, because four different questions bring people here.

## [Tutorial](tutorials/) — "can you show me?"

One lesson, start to finish. [Getting started: your first
plan](tutorials/getting-started.md) installs nib and turns a single sentence
into a reviewed action plan, in about twenty minutes. It is the right place to
begin if you have not run nib before.

## [How-to guides](how-to/) — "how do I…?"

Recipes for things you actually need to get done: [install with Docker
Compose](how-to/run-with-docker-compose.md), [switch the LLM
provider](how-to/switch-llm-provider.md), [connect an MCP
server](how-to/connect-an-mcp-server.md), [keep tokens out of
`mcp.json`](how-to/keep-tokens-out-of-mcp-json.md), [run actions in
containers](how-to/run-actions-in-containers.md), [scrape
metrics](how-to/scrape-metrics-with-prometheus.md) and
[upgrade](how-to/upgrade.md).

## [Reference](reference/) — "what is…?"

The machinery, described: every configuration key, environment variable, API
route, built-in tool, mode, executor setting and metric, plus where state lives
on disk and in the database.

## [Explanation](explanation/) — "why…?"

The reasoning. [The architecture](explanation/architecture.md) and [how a plan
is produced](explanation/how-a-plan-is-produced.md) are the two that make the
rest of the system legible; the others cover [one execution at a
time](explanation/one-execution-at-a-time.md), [why secrets never come from the
environment](explanation/secrets-never-come-from-the-environment.md), [modes,
tools and the catalog](explanation/modes-tools-and-the-catalog.md), [knowledge,
rules and skills](explanation/knowledge-rules-and-skills.md) and [LLM endpoints
and reasoning replay](explanation/llm-endpoints.md).

## Documentation kept elsewhere

Some of nib's documentation lives beside the thing it documents, so it cannot
drift from it:

| Where | What |
|---|---|
| [`deploy/helm/nib/README.md`](../deploy/helm/nib/README.md) | Installing and configuring the Helm chart. |
| [`agent-runner/universal-agent/README.md`](../agent-runner/universal-agent/README.md) | The agent container's environment contract, auth and webhook payload. |
| [`CHANGELOG.md`](../CHANGELOG.md) | What changed in each release, including breaking changes. |
| [`CLAUDE.md`](../CLAUDE.md) | Contributor guide. It is also served verbatim to the nib agent as its own documentation, so it is edited as a whole and never split. |

And two notes predating this set: [`archdecision.md`](archdecision.md), an early
component sketch that no longer matches what runs, and
[`description.md`](description.md), the project's original scope note.

## Where to start

| If you | Go to |
|---|---|
| have never run nib | [the tutorial](tutorials/getting-started.md) |
| are setting up an install | [How to run nib with Docker Compose](how-to/run-with-docker-compose.md) |
| are wondering what nib is doing | [How a plan is produced](explanation/how-a-plan-is-produced.md) |
| are looking for a setting | [Configuration file](reference/configuration.md) or [Environment variables](reference/environment-variables.md) |
