# How-to guides

These are recipes for getting a particular thing done. They assume you already
have a working install and know roughly what you want — if that is not yet true,
start with [the tutorial](../tutorials/getting-started.md).

Each guide starts and stops somewhere useful rather than covering its subject
exhaustively. Where a guide needs a full list of options, it links to
[reference](../reference/) instead of reproducing it.

## Getting an install running

Everything needed to go from nothing to a working nib, and to keep it current.

- [How to run nib with Docker Compose](run-with-docker-compose.md) — a working
  install on one host, from the published images, behind an API token
- [How to upgrade an install](upgrade.md) — moving to a new version, what your
  data volume keeps, and what an upgrade from v0.8.0 needs from you

For Kubernetes, the chart's own documentation is the guide:
[`deploy/helm/nib/README.md`](../../deploy/helm/nib/README.md).

## Pointing nib at your systems

An install with no tools and no credentials can only plan in the abstract.
These three are what make it useful.

- [How to connect an MCP server](connect-an-mcp-server.md) — give the agent
  tools for a system it does not know about
- [How to keep tokens out of `mcp.json`](keep-tokens-out-of-mcp-json.md) —
  reference a credential instead of writing it into a file on disk
- [How to switch the LLM provider](switch-llm-provider.md) — move between
  gateways without silently breaking knowledge search

## Letting nib act

By default nib plans and hands you the plan. This is how you change that, and
how you watch what happens once you do.

- [How to run planned actions in containers](run-actions-in-containers.md) —
  configure the executor so `code` steps actually run, watch their logs, run a
  whole stage with **Execute all**, and ask for a fix from the chat
- [How to share a plan as a `.nib` file](share-a-plan.md) — hand a plan you
  built to another install, with its chat and sub-agent history, to run there
- [How to scrape nib's metrics with Prometheus](scrape-metrics-with-prometheus.md)
  — get the metrics listener into your monitoring stack

## Not here yet

Guides for writing rules, skills and knowledge-base documents, and for
narrowing which tools a mode may use, are not written. In the meantime,
[Knowledge, rules and
skills](../explanation/knowledge-rules-and-skills.md) covers which of the three
to reach for, and [Built-in agent tools](../reference/agent-tools.md) and
[State](../reference/state.md) cover the mechanics.
