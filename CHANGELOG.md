# Changelog

All notable changes to Nib are documented in this file.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project uses the version in the root `VERSION` file.

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
