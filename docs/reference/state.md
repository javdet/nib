# State: the data directory and the database

nib keeps state in two places. Postgres holds transcripts, credentials and the
tool catalog; a single directory on disk holds everything an operator can edit
as a file, plus the artifacts of every plan.

Both must survive a restart. In Docker Compose they are the `pgdata` and
`nibdata` volumes; in the Helm chart, the PostgreSQL StatefulSet and the backend
PVC.

## The data directory

Rooted at `DATA_DIR` (default `data`). Every path below is relative to it
unless a config key overrides it with an absolute path.

Files here are live application state. Editing one changes the running install
immediately.

### Operator-editable

| Path | Holds |
|---|---|
| `mcp.json` | MCP server registrations. Overridable with `mcp.file`. |
| `skills/{name}.md` | One skill each, with `name` and `description` frontmatter. Overridable with `skills.dir`. |
| `skills/.seeded-defaults` | Ledger of which built-in skills have been written. |
| `rules/` | One file per rule. Overridable with `rules.dir`. |
| `prompts/discuss.md` | The discuss-mode prompt override. The only editable prompt. Overridable with `prompts.dir`. |
| `tools/{mode}.json` | Built-in tool allow list for one mode. Overridable with `includedTools.dir`. |
| `tools/mcp-included.json` | MCP tools included per mode. |
| `tools/mcp-known.json` | Ledger of MCP tool names already reconciled. |
| `tools/.seeded-tools` | Ledger of built-in tool names already offered to each allow list. |
| `tools/schemas/{tool}.json` | Parameter schemas of `create_action_plan`, `update_action_plan` and `update_rollback_plan`. Copied from the image when absent; an edited one is kept. |
| `knowledgebase/{collection}.md` | The last document uploaded to each collection. Overridable with `knowledge_base.dir`. |
| `knowledge.json` | Knowledge-base switches; today just `autoUpdate`, on by default. Overridable with `knowledge_base.settingsFile`. |
| `executor.json` | Executor settings, edited at **Tools → Executor**. Overridable with `executor.file`. |
| `job.yaml.tmpl` | Optional override of the Kubernetes Job template. |

The `categories`, `downtime` and `degraded` properties of every action are
stamped onto the plan schemas when they are loaded, so an older copy under
`tools/schemas/` still offers them.

`mcp-known.json` is what separates a tool that was never offered from one an
operator excluded. An install without it treats the entire catalog as new and
switches every tool on.

### Built-in skills

Five skills ship with the image and are written into `skills/` once per data
volume. After that the volume owns them: an edit, rename or deletion is never
undone by an upgrade.

| Skill | Does |
|---|---|
| `build-knowledge-base` | Builds or refreshes a project's knowledge-base document from its repositories, over MCP, and publishes it with `update_kb`. |
| `categorize-tools` | Sorts a supplied list of MCP tools into tool categories and writes the assignments with `update_tool_category`. |
| `distribute-tools` | Narrows every mode's included MCP tools by a per-mode policy, writing the result with `update_included_tools`. |
| `jira-get-board` | Renders the user's Jira board as a table. |
| `jira-todo-tasks` | Returns todo tasks from Jira filtered by project, assignee and status. |

Some reference prompt variables — for example `{{ .global.JiraProject }}` and
`{{ .global.Email }}`. Loading a skill whose variables are unfilled fails with
an unknown-key error.

Two further skills, `nib-configuration` and `nib-internals`, are served from the
binary rather than the volume. They cannot be created, edited, renamed or
deleted, and do not appear in the Skills page or the HTTP API; the agent reads
them with `get_skill`.

### Plan artifacts

Keyed by the UUID of the **root** dialog of a plan, not by the sub-agent that
wrote them. Each is written atomically.

| Path | Holds |
|---|---|
| `dags/{id}.md` | The stage diagram, as Mermaid. |
| `summaries/{id}.txt` | The plan summary. |
| `reports/{id}.md` | The closing report written after **Finish**. |
| `action_plans/{id}.json` | The action plan. |
| `plan_state/{id}.json` | Current status and schedule. |
| `plan_selection/{id}.json` | The project/environment/cloud/location the plan was started under, recorded on its first turn. |
| `plan_contracts/{id}.json` | The cross-stage contract written by `create_plan_contract`. |
| `plan_fanout/{id}.json` | Progress of a detailed-planning run. |
| `subagents/{id}.json` | A suspended sub-agent waiting on an answer. |
| `code_fixes/{id}.json` | Chat-requested code fixes: each fix's own dialog id mapped to its run. The agent-runner webhook recognises a fix by it, and a restart sweeps one left `running`. |
| `kb_updates/{id}.json` | That the plan has been folded into its knowledge base collection, and whether anything changed. A failed run writes none, so **Finish** can retry it. |

### The action plan

`action_plans/{id}.json` is `{"stages": [...], "rollback": [...]}`. A stage has
`number`, `title`, `description`, `steps` and `checks`; a check has `check` and
`expectation`. A step and a rollback entry share one shape:

| Field | Holds |
|---|---|
| `number` | The label the operator sees (`1.2`, `R1`). Derived from position on every write, never taken from input. |
| `type` | `code`, `web`, `curl`, `shell` or `other`. |
| `action` | Markdown describing the change. |
| `command` | Ready-to-run commands of a `shell` or `curl` step, one per line. |
| `repository`, `pr_title` | A `code` step's repository and pull request title. |
| `pr_url` | The pull request a code run opened. |
| `comment` | A note for the operator. |
| `categories` | Tool categories the executing sub-agent is given. Never shown to the operator. |
| `downtime` | What becomes unavailable while the action runs. Rendered as a red label with this text as its tooltip. |
| `degraded` | What gets slower or weaker while the action runs, without failing. An amber label. |

`downtime` and `degraded` are optional and mutually exclusive; when both are
set, the interface shows `downtime`.

A check is executable like a step. It carries no categories of its own and runs
with those of the steps in its stage.

### Action-plan side files

Beside `action_plans/{id}.json`, keyed by the same dialog id and by the
positional row key of an action — `s0.step1`, `rollback.2`.

| File | Holds |
|---|---|
| `{id}.checks.json` | The operator's checkboxes. |
| `{id}.comments.json` | The operator's comments. |
| `{id}.runs.json` | The execute dialog created for each row. |
| `{id}.exec.json` | The last run's status per row. |
| `{id}.notes.json` | What the last run reported. |
| `{id}.stagerun.json` | The latest **Execute all** run: scope, stage, the row it is on, status and why it stopped. |

Because the keys are positional, the first five are remapped together whenever
a stage is rewritten, reordered or replaced. `create_action_plan` clears them; a
`PUT` on the action plan clears none.

`{id}.stagerun.json` is never remapped: a reorder or a change in the stage's
shape stops the run instead. `create_action_plan` clears it, and a restart stops
one left `running`.

`{id}.notes.json` is the executors' own memory, not an operator surface: a
finished action's final message is recorded there and handed to later actions
through `get_action_list`. It is unreachable from the HTTP API.

### Other

| Path | Holds |
|---|---|
| `attachments/` | Uploaded chat attachments. |
| `logs/` | The stderr mirror, when `log.enabled` is true. |
| `.host-env.json` | Marker for the host-environment snapshot reconciled into prompt variables. |
| `.webhook-key` | The key that signs each agent-runner container's webhook token, generated once (mode `0600`) when `AGENT_WEBHOOK_TOKEN` is unset. Changing it orphans runs in flight. |

A background sweeper deletes attachment files whose database rows are gone, and
expires uploads never sent after 24 hours.

## The database

Postgres with the `pgvector` extension. Migrations in `*.up.sql` are applied at
startup under an advisory lock and recorded in `schema_migrations`, so
overlapping pods during a rolling restart cannot apply the same file twice. An
already-applied migration whose file was edited afterwards is logged as a
warning.

### Dialogs

| Table | Holds |
|---|---|
| `chat_dialogs` | One row per dialog: mode, parent, title, categories, pinned flag, task id. |
| `chat_dialog_messages` | The transcript, one row per message. |
| `chat_attachments` | Attachment metadata; the bytes are on disk. |
| `attachment_files_to_delete` | Outbox for files the sweeper still has to remove. |

The `plan_dialogs` view selects the dialogs whose mode carries a plan. It has to
stay in step with the plan-mode whitelist in the backend.

### Configuration

| Table | Holds |
|---|---|
| `prompt_variables` | Prompt template variables, including which are predefined and undeletable. |
| `prompt_secrets` | Encrypted secret values, keyed by name and scope, each with the hosts `mcp.json` may send it to. |

Secret values are encrypted with `SECRETS_ENCRYPTION_KEY` and never returned by
the API.

### Tool catalog

| Table | Holds |
|---|---|
| `mcp_servers` | Registered servers. |
| `mcp_tools` | Every discovered tool, with its embedding. |
| `mcp_connections` | Token-authenticated MCP connections, added through `POST /api/v1/mcp/connections` (there is no UI for them). |
| `tool_categories` | Category names. |
| `tool_category_patterns` | Patterns that assign tools to a category. |
| `mcp_tool_categories`, `mcp_server_categories` | The resulting assignments. |

### Knowledge base

| Table | Holds |
|---|---|
| `kb_collections` | One row per collection, with its `embedding_model` and vector dimension. |
| `kb_chunks` | Chunk text and its vector. |

`kb_collections.embedding_model` must match `llm.embeddingModel` exactly.
Knowledge search compares them as strings.

### Statistics

| Table | Holds |
|---|---|
| `llm_usage` | One row per LLM call: dialog, plan, mode, model, tokens, duration, outcome, and cost where the provider reports one. |
| `agent_run_usage` | One row per finished agent container. |
| `plan_status_transitions` | Every plan status change, appended. |

These tables carry no foreign key to `chat_dialogs`, so their rows outlive the
dialogs they came from and any join to `chat_dialogs` must be a `LEFT JOIN`.

Plan *status* itself lives in `plan_state/{id}.json` and keeps only the current
value; the history in `plan_status_transitions` starts accumulating from the
release that added it.

## What an upgrade keeps

| Kind of state | On upgrade |
|---|---|
| Everything in Postgres | Kept. New migrations are applied. |
| Skills | Kept, including your edits. A new built-in is seeded; one you edited, renamed or deleted stays that way. |
| System prompt override | Kept. Built-in prompts come from the new image. |
| Tool allow lists | Kept. A new mode's list is added, and a built-in tool new to the release is added to existing lists once; no name is ever removed. |
| `mcp.json`, rules, variables, secrets | Kept. |
| Plan artifacts | Kept. |

See [Upgrade an install](../how-to/upgrade.md).

## See also

- [Configuration file](configuration.md#directory-and-file-paths)
- [The knowledge base](../knowledgebase/example.md) — collections, and what an upload replaces
- [Knowledge, rules and skills](../explanation/knowledge-rules-and-skills.md)
