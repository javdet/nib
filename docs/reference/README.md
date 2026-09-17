# Reference

Descriptions of the machinery: what each setting, route, tool and metric is,
what it defaults to, and what it refuses. These pages are for looking things up
while you work, not for reading through.

The scope is the operator-facing surface. Go package internals are not here —
they are in [`CLAUDE.md`](../../CLAUDE.md), which the nib agent is also served
as its own documentation.

## Settings

Two files and one environment, with different precedence rules between them.

- [Configuration file](configuration.md) — every key of `config.yaml`, its
  default and its validation errors
- [Environment variables](environment-variables.md) — every variable the three
  binaries and the Compose stack read, and which ones beat the YAML
- [Executor settings](executor.md) — the fields under Settings → Executor,
  which live neither in the file nor in the environment

## What the agent can do

Three pages that together answer "why did it do that, or not do that".

- [Modes](modes.md) — the six modes, which carry a plan, and which sub-agents
  the orchestrator can launch
- [Built-in agent tools](agent-tools.md) — every compiled-in tool, and the
  matrix of which modes get which by default
- [Command-line interface](cli.md) — the `nib`, `kb` and `toolcatalog` binaries
  and their flags

## Interfaces and state

- [HTTP API](http-api.md) — every `/api/v1` route
- [State: the data directory and the database](state.md) — where everything is
  kept, and what an upgrade keeps
- [Metrics](metrics.md) — every exported metric, and the usage tables behind
  the Statistics page

## Reference kept elsewhere

- [`agent-runner/universal-agent/README.md`](../../agent-runner/universal-agent/README.md)
  — the agent container's environment contract and webhook payload
- [`deploy/helm/nib/README.md`](../../deploy/helm/nib/README.md) — the chart's
  values
- [`CHANGELOG.md`](../../CHANGELOG.md) — release history, breaking changes
  marked
- [The knowledge base](../knowledgebase/example.md) — collections, the stored
  source document, and the built-in template
