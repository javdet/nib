# Diátaxis documentation plan — nib

Durable record of diataxis-docs runs in this repo. Kept at the repo root, outside
`docs/`, so no site generator renders it.

- **Run 1** — 2026-09-17. Full run, all four quadrants. **Completed.** 24 pages
  written under `docs/`; `README.md`, `docs/llm-providers.md` and
  `docs/metrics.md` split; `docs/metrics.md` translated from Russian.

## Audience

Answered by the user, run 1:

- **Operators** — DevOps, platform engineers and SREs who install nib, point it
  at an LLM, connect MCP servers and run plans against their infrastructure.
- **Agent authors** — people who write rules, skills and knowledge-base
  documents for their own install without touching Go or React.
- **Developers without an infrastructure background** — this one sets the
  register. Nothing assumes prior Docker, Kubernetes, Postgres or Prometheus
  knowledge: terms are either explained on first use or linked out.

Not an audience for this set: contributors to nib's own code (`CLAUDE.md` serves
them and is immovable — see constraint 1) and evaluators comparing tools.

**Goals**, run 1: the how-to quadrant was scoped to *install and configure*. The
authoring guides were deferred — see [Not created](#not-created).

## Output format and location

| Item | Finding |
|---|---|
| Docs tooling | None. No `mkdocs.yml`, `conf.py` or `docusaurus.config.*`. Bare `docs/` directory. |
| Format | GitHub-flavoured Markdown under `docs/`. |
| Diagrams | Mermaid in fenced ```mermaid blocks. Both C4 diagrams verified to parse and render with the repo's own `mermaid@11` in headless Chrome, 2026-09-17. |
| Autodoc | None. No `typedoc.json`, no Sphinx, no rustdoc. Reference is hand-written. |

## Standing decisions and constraints

These bind every run. The closing phase re-checks the output against each one.

1. **`CLAUDE.md` is off-limits — it must not be moved, split or trimmed.** Its
   `## ` sections are served *verbatim* to the nib agent as the read-only system
   skills `nib-configuration` and `nib-internals`
   ([internal/skills/system.go](backend/internal/skills/system.go)), and
   [backend/internal/nibdocs/docs/repo-guide.md](backend/internal/nibdocs/docs/repo-guide.md)
   is a copy kept in step by `make sync-docs` and guarded by
   `TestRepoGuideMatchesRepoRoot`. It is a *source*, never a destination.
2. **Every path that is split away leaves a pointer stub behind.** `CLAUDE.md`,
   `backend/config.yaml` and `deploy/helm/nib/README.md` link to
   `docs/metrics.md` and `docs/llm-providers.md`, and constraint 1 forbids
   editing the first. Those two paths, and `docs/getting-started/README.md`,
   survive as pointer stubs.
3. `docs/img/` is untouched.
4. Docs are written in **English**.
5. Component-local READMEs stay with their component
   (`deploy/helm/nib/README.md`, `agent-runner/universal-agent/README.md`). They
   are linked, never copied.
6. Facts are taken from code, not from the older notes in `docs/`. Where the two
   disagree, the code wins and the note is labelled historical.

## The documentation set

### Reference

**Approved scope:** the operator-facing surface — `config.yaml`, environment
variables, the HTTP API, the built-in tool catalog, modes, executor settings,
metrics, the CLI, and where state lives. **Out of scope:** Go
package/function-level internals (no autodoc, and `nib-internals` already serves
that from `CLAUDE.md`) and React component APIs (no component contract exists;
`vitest` covers `lib/` helpers only).

| Page | User need | Source |
|---|---|---|
| `reference/configuration.md` | "What can I put in `config.yaml`, and what wins over what?" | `internal/config/config.go`, `backend/config.yaml`, `deploy/compose/config.yaml`, former `docs/llm-providers.md` |
| `reference/environment-variables.md` | "Which variable does this, and does it beat the YAML?" | `os.Getenv`/`LookupEnv` across `backend/`, `.env.example`, both compose files, Helm values |
| `reference/http-api.md` | "What endpoints does the backend expose?" | `internal/handler/router.go` |
| `reference/agent-tools.md` | "Which tools can the agent call, and in which modes?" | `internal/mode/defaults/*.json`, `service/local_tool_registry.go`, `service/system_tools.go` |
| `reference/modes.md` | "What are the six modes and what may each do?" | `internal/mode/mode.go`, `internal/systemprompts/defaults`, `service/subagent_registry.go` |
| `reference/state.md` | "Where does nib keep things, and what survives a restart?" | `internal/atomicfile`, `internal/filestore`, `backend/migrations/*.up.sql`, `internal/skills/defaults` |
| `reference/executor.md` | "What does each field on Settings → Executor do?" | `internal/executor/{config,types,errors,gitprovider}.go` |
| `reference/metrics.md` | "What is the name and meaning of each metric?" | former `docs/metrics.md` (translated), `internal/metrics` |
| `reference/cli.md` | "What do the three binaries take?" | `cmd/nib`, `cmd/kb`, `cmd/toolcatalog` |

### How-to guides

| Page | Real-world goal | Source |
|---|---|---|
| `how-to/run-with-docker-compose.md` | Get a working install on one host | `docker-compose.yml`, `.env.example` |
| `how-to/switch-llm-provider.md` | Move between gateways without breaking knowledge search | former `docs/llm-providers.md`, `internal/llm`, `internal/config` |
| `how-to/connect-an-mcp-server.md` | Give the agent a new system's tools | `internal/mcpclient`, `internal/mcpconfig`, `tests/tools/mcp-servers.spec.ts` |
| `how-to/keep-tokens-out-of-mcp-json.md` | Reference a credential instead of writing it to disk | `internal/mcpconfig/rawdoc.go`, `internal/crypto` |
| `how-to/run-actions-in-containers.md` | Configure the executor so `code` steps run | `internal/executor`, `tests/tools/executor.spec.ts` |
| `how-to/upgrade.md` | Move to a new version and know what is kept | `CHANGELOG.md`, `VERSION`, seeding behaviour |
| `how-to/scrape-metrics-with-prometheus.md` | Get metrics into an existing monitoring stack | former `docs/metrics.md`, `templates/servicemonitor-backend.yaml` |

The last is not from the approved goal list; it arrived as a destination of the
approved `docs/metrics.md` split.

### Explanation

| Page | Why-question | Source |
|---|---|---|
| `explanation/architecture.md` | "What are the moving parts and what talks to what?" | compose files, Helm templates, `handler.Deps`, `internal/executor`, `internal/mcpclient` |
| `explanation/how-a-plan-is-produced.md` | "Why does one request turn into five chats?" | `CLAUDE.md` Orchestration, `subagent_registry.go`, `subagent_decompose.go` |
| `explanation/one-execution-at-a-time.md` | "Why was my second action refused rather than queued?" | `execution_lease.go`, the concurrency clamp, former `docs/metrics.md` scaling note |
| `explanation/secrets-never-come-from-the-environment.md` | "Why can't `mcp.json` name `LLM_API_KEY`?" | `internal/mcpconfig` resolver, `redactRouteError` |
| `explanation/modes-tools-and-the-catalog.md` | "Why is a tool the server exposes not in the agent's list?" | `mode/allowtools.go`, `includedtools.SyncCatalog`, `mcp-known.json` |
| `explanation/knowledge-rules-and-skills.md` | "Three places to write things down — which do I use?" | `internal/kb`, `internal/rules`, `internal/skills`, `internal/prompttpl` |
| `explanation/llm-endpoints.md` | "Why does `llm.api` exist?" | former `docs/llm-providers.md`, `internal/llm` |

`knowledge-rules-and-skills.md` is what serves the **agent authors** audience in
run 1, since their how-to guides were deferred.

### Tutorial

| Page | User need | Source |
|---|---|---|
| `tutorials/getting-started.md` | A newcomer installs nib and takes one task from a sentence to a reviewed action plan | former `docs/getting-started/README.md` outline, `README.md` quick start, `tests/plans/*.spec.ts` |

**Verification status — partially verified (2026-09-17).**

Verified by execution in a disposable clone under the scratchpad, never in the
working tree:

| Step | How |
|---|---|
| 1 — clone | `git clone` of a local copy. |
| 2 — `cp .env.example .env` | Run. |
| 4 — `docker compose config --services` | Run. Output in the tutorial is verbatim. |
| 6 — `GET /api/v1/health` | Run against a healthy install. Output verbatim. |
| 6 — `GET /api/v1/version` | Run. Shape verbatim; the version string is normalized to this repo's `VERSION` (the install queried was v0.7.6). |

**Unverified** — steps 5 and 7 to 14: starting the stack, and every step in the
browser. Reason: `docker-compose.yml` pins container names (`nib-backend`,
`nib-postgres`, …), so a second stack cannot run beside the one already on the
host, and ports 8080/8081/9090 were taken. Starting it would have meant
disrupting a running install. Steps 9 to 13 are additionally unverifiable
without spending real tokens against a funded provider, which the tutorial
protocol excludes.

The `docker compose ps` output in step 5 is real output from an equivalent
healthy install, normalized: uptimes rewritten and non-stock services removed.

**Discharge this marker** by walking the whole tutorial on a clean host with a
funded key, and replacing step 5's table and the UI descriptions with what
actually appears.

## Not created

| Item | Reason | Remedy |
|---|---|---|
| How-to: load a knowledge base | User scoped run 1's how-to guides to install + configure. | Run 2. Sources ready: `internal/kb`, `internal/kbdoc`, `docs/knowledgebase/example.md`, the `build-knowledge-base` skill. |
| How-to: write a rule | As above. | Run 2. Sources: `internal/rules`, `tests/authoring/rules.spec.ts`. |
| How-to: write a skill | As above. Overlaps the scratch note `docs/skiils.md`. | Run 2. Sources: `internal/skills`, `internal/skills/defaults/*.md`, `tests/authoring/skills.spec.ts`. |
| How-to: limit which tools a mode may use | As above. | Run 2. Sources: `internal/includedtools`, tool-category routes, the `distribute-tools` skill. |
| How-to: stop a stuck execution | As above. The explanation page covers the lease; the operator steps were out of scope. | Run 2. Sources: `execution_lease.go`, `DELETE /api/v1/execution`. |
| A Helm deployment how-to under `docs/` | `deploy/helm/nib/README.md` is already a complete guide and lives beside the values it documents. A copy would drift. | If `docs/` becomes canonical, move the chart README in and leave a pointer beside `Chart.yaml`. |
| A generated API reference | No OpenAPI spec exists; `reference/http-api.md` is written from the chi router by hand. | Add an OpenAPI document (or generate one from the router) and replace that page with generated output. |
| A frontend component reference | `vitest` covers pure `lib/` helpers only; no component contract is exported. | Publish a component API (or add typedoc) if the SPA ever ships as a library. |
| A contributor guide under `docs/` | `CLAUDE.md` is it, constraint 1 forbids moving it, and contributors are out of audience. | Same as the C4 Component row below. |
| C4 Component diagram | The backend's layering is already documented in `CLAUDE.md` § Backend layering, served to the agent as `nib-internals`; a second copy would drift from a guarded file. Also out of audience. | Resolve constraint 1 first, then move that section here and leave a pointer. |
| C4 Code diagram | No autodoc setup, and code-level detail sits outside a Diátaxis set. | Add `typedoc.json` / publish godoc if a code-level reference is wanted. |
| Rename of `docs/knowledgebase/example.md` | Good reference, badly named. Nothing links it by that name today, but a rename is still a gratuitous break. | Rename and update the links from `reference/state.md` and `explanation/knowledge-rules-and-skills.md`. |
| Disposal of `docs/skiils.md` | Scratch notes, mixed Russian/English, unfinished sentences. Not documentation, and not this run's to delete. | Fold the usable task-tracker ideas into the deferred skills how-to, or delete the file. |

## Parking lot

*(content surfacing in the wrong phase is parked here; must be empty at the end
of a run)*

— empty —
