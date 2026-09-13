# Changelog

All notable changes to Nib are documented in this file.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project uses the version in the root `VERSION` file.

## [Unreleased]

### Added
- **Statistics page.** A new **Statistics** entry in the sidebar, next to Help, showing task metrics broken down by status, token usage and cost as both figures and charts, over a selectable range (24 hours to 12 months). Also covers LLM calls and failures, average call duration, a per-model / per-mode / per-operation breakdown, the most expensive tasks, and container agent runs.
- **Token usage is now recorded.** Prompt, cached, completion and reasoning tokens were previously returned by the provider and thrown away — `llm.AssistantMessage` did not carry them. Every LLM call now writes a row to the new `llm_usage` table with its dialog, plan, mode, model, tokens, duration and outcome.
- **Cost is now stored where the provider reports it.** nib still carries no rate card, so cost is recorded only where a gateway prices the call (OpenRouter returns `usage.cost`; the OpenAI platform does not) and from the agent-runner webhook, which already reported `total_cost_usd`. An unreported cost is stored as NULL and shown as a dash, never as `$0.00`.
- **Plan status history.** Status changes now append to `plan_status_transitions`, so status over time can be charted. Plan status itself still lives in `data/plan_state/{id}.json` and keeps only the current value, so the history starts accumulating from this release; the current breakdown remains complete.
- A response the provider cuts short (`incomplete`, i.e. it hit the output-token cap) now reports its token usage alongside the error instead of returning empty. Those tokens were generated and billed, and would otherwise have been invisible.
- `GET /api/v1/stats?from=&to=&bucket=` returns the whole dashboard in one response. `bucket` is one of `hour`, `day`, `week`, `month`, and a range is capped at 400 buckets.

### Changed
- The agent-runner webhook now parses the `input_tokens` and `output_tokens` the container was already sending and had been silently discarding, and persists each finished run to `agent_run_usage`. A retried delivery no longer risks double-counting cost: the insert sits behind the existing job-name de-duplication and a unique index enforces it in the database.
- Usage rows deliberately carry no foreign key to `chat_dialogs`. Statistics are kept indefinitely, and `ON DELETE CASCADE` would erase the record of what a task cost as soon as its dialog was deleted.

## [v0.8.0] - 2026-09-12

### Added
- GitLab is now supported end to end by the agent container: it installs `glab` alongside `gh`, clones GitLab repositories as `oauth2:<token>`, and opens merge requests instead of pull requests. The provider comes from the **Version control system** field on the Knowledge Base page — anything containing "gitlab" means GitLab, anything else including blank means GitHub. The repository host is now derived from the repository URL rather than assumed to be `github.com`, so self-hosted GitLab and GitHub Enterprise work too.
- Settings → Executor gained a **Git API token** field that selects which entry in Variables → Secrets the agent container clones, pushes and opens the pull/merge request with, matching how the LLM key and Kubernetes token are already configured.

### Changed
- **Breaking:** the git API token is no longer read from a secret hardcoded to the name `EXECUTOR_GIT_API_TOKEN`. Open Settings → Executor and select the secret once; until then `code` actions refuse to start with "executor git API token secret is not configured; select it in executor settings". Remote Kubernetes executors are unaffected — their Jobs still take credentials from the Agent Secret.
- **Breaking:** the `EXECUTOR_GIT_API_TOKEN` environment variable (and the `secrets.executorGitApiToken` Helm value) has been removed. It only ever fed the `run_executor` tool and became a second, invisible source of truth for a credential that is now selected in the UI.
- The agent container accepts the git token as `GITHUB_TOKEN`, `GITLAB_TOKEN` or `GIT_TOKEN` and derives the rest, so existing Kubernetes Agent Secrets keep working unchanged.
- Plan, decompose and rollback prompts now say "pull request (merge request on GitLab)" rather than naming `gh pr create`, so a GitLab install is not told to run a GitHub-only command.

### Removed
- `agent-runner/claude-code-agent/`, an unreferenced GitHub-only copy of the agent image that no build or compose stack used; `agent-runner/universal-agent` is the only published `nib-agent` image.

## [v0.7.10] - 2026-09-10

### Added
- New `write_rule` tool lets discuss mode create or replace rule files in the rules directory, so guardrails can be written from chat instead of edited on disk; other modes still cannot write rules.
- A background sweeper now removes attachment files on disk whose database rows were deleted (dialog delete, or a retry that rewinds messages), and expires uploads that were never sent after 24 hours.

### Changed
- Database migrations are now checksummed and applied under a Postgres advisory lock, preventing two instances (e.g. overlapping pods during a rolling restart) from applying the same migration concurrently, and logging a warning if an already-applied migration file was edited afterward instead of silently ignoring it.

### Fixed
- Startup no longer fails if the plan-ownership migration marker was already recorded (e.g. after a crash or an overlapping pod during the same migration run).

## [v0.7.9] - 2026-09-07

### Added
- Two built-in system skills, `nib-configuration` and `nib-internals`, ship nib's own documentation to the agent; they are read-only, not written to the data volume, and not listed by the skills API.
- New `update_included_tools` tool removes specific tools from a mode's included list while keeping them reachable via search, and a new `distribute-tools` skill applies per-mode tool policies across multiple modes at once.
- Mode prompts now include a live snapshot of the host environment (runtime, distro, kernel, shell, working directory, available binaries), so the agent's answers reflect what it can actually run on this install.

### Changed
- The knowledge-base's OpenAI-compatible embeddings endpoint is now configured via `KB_EMBEDDINGS_BASE_URL` instead of `KB_OPENROUTER_BASE_URL`; update `.env` and any compose overrides accordingly.
- Re-planning a subset of fan-out stages no longer drops the stages that weren't replanned — the plan view now carries them forward from the previous run (dimmed, with an explanation on hover) instead of losing them.

## [v0.7.8] - 2026-09-04

### Added
- Action plans can now declare a `categories` property to scope which tool categories an action requires; unknown categories are dropped and logged, and each action's tool access during execution is restricted to its declared categories.
- The backend now exposes a Prometheus metrics endpoint on port `9090`, configurable via environment variables and YAML settings, with execution and performance metrics recorded across backend services.
- The included-tools panel gained "move all" and "move all from server" controls, so entire filtered lists or a whole server's tools can be included or excluded in one click instead of moving tools one at a time.

## [v0.7.7] - 2026-09-04

### Added
- MCP tools are now switched on for every mode as soon as their server is indexed, and switched off automatically when the server is deleted, preventing orphaned entries.
- Added a `get_dag` action so the agent can retrieve and display a plan's DAG stages and their summaries.
- Skill suggestions in the chat panel: typing `/` lists skills and applies the selected one.
- Actions in a plan now run as dedicated `execute_action` subagents, with configurable concurrency, iteration limits, and timeout.
- Running actions can now be stopped from the action row or by asking in chat, including their underlying Kubernetes jobs or local containers; only one execution runs at a time to prevent conflicts.
- Plan mode now derives a rollback plan from all stage subagents through a dedicated rollback subagent and an `update_rollback_plan` tool.
- Rules now carry a description via frontmatter; Plan mode lists every rule with its description and loads the ones it needs with `get_rule`.

### Changed
- Plan fan-out timeout raised from 45 to 60 minutes.
- Skill categories (`included`/`searchable`) have been removed: every skill is now listed at the end of the Main and Discuss system prompts, and the agent loads the one it needs with `get_skill`.
- The workplace plan metadata table now has a single "Tool categories" field instead of separate "Components" and "Rules" fields, and dialog metadata no longer tracks `subjects`.
- The DAG board now renders with the Neucha font and updated styling.

### Fixed
- `system_tools` API now returns an empty array instead of `null` for a tool with no modes.
