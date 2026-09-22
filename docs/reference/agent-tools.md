# Built-in agent tools

The tools compiled into the backend. They are distinct from MCP tools, which
arrive from registered servers and are named by those servers.

A turn's catalog is the union of:

- the built-ins named in `{DATA_DIR}/tools/{mode}.json`
- the MCP tools named in `{DATA_DIR}/tools/mcp-included.json` for that mode
- for `plan` dialogs, every tool in the categories chosen during decomposition

Everything else stays reachable through `tool_search`.

`GET /api/v1/system/tools` returns this catalog at runtime, including per-mode
availability.

## Availability

Three things decide whether a tool is offered on a given turn.

1. **The mode's allow list** — the name must appear in `{mode}.json`.
2. **A binding condition** — some tools need a plan, a stage, or a root turn.
   These are enforced in code and cannot be granted by editing an allow list.
3. **A configured service** — a tool whose service is not wired is not offered.
   `run_executor` is absent while the executor is disabled, for example.

Where a tool is *mode-owned* or *root-only* below, adding its name to another
mode's allow list has no effect: the restriction is enforced twice, in the
registration condition and again by a pass that strips the name from every
other mode's allow set.

## Default allow lists

Shipped with the image and seeded onto the data volume at first start.

| Tool | main | decompose | plan | execute | discuss | incident |
|---|:-:|:-:|:-:|:-:|:-:|:-:|
| `knowledge_search` | ● | ● | ● | ● | ● | ● |
| `get_skill` | ● | ● | ● | ● | ● | ● |
| `tool_search` | | ● | ● | ● | ● | ● |
| `ask_question` | ● | ● | ● | | | |
| `get_dag` | ● | ● | | | | |
| `get_action_list` | ● | | | ● | | |
| `run_subagent` | ● | | | | | |
| `stop_execution` | ● | | | | | |
| `create_summary` | | ● | | | | |
| `create_dag` | | ● | | | | |
| `create_plan_contract` | | ● | | | | |
| `set_category` | | ● | | | | |
| `get_rule` | | | ● | | ● | |
| `list_variables` | | | ● | ● | ● | ● |
| `create_action_plan` | | | ● | | | |
| `update_action_plan` | | | ● | | | |
| `update_rollback_plan` | | | ● | | | |
| `get_secrets` | | | | ● | | |
| `api_call` | | | | ● | | |
| `execute_command` | | | | ● | | |
| `run_executor` | | | | ● | | |
| `get_kb_document` | | | | | ● | |
| `update_kb` | | | | | ● | |
| `write_rule` | | | | | ● | |
| `update_tool_category` | | | | | ● | |
| `update_included_tools` | | | | | ● | |
| `create_table` | | | | | ● | ● |

`chat_name` is not in any allow list: it is offered on every turn that has a
dialog. `set_report` and `execute_action` are in no allow list either — see
[Never in an allow list](#never-in-an-allow-list).

Seeding never removes a name from a list already on disk. A capability
withdrawn from a mode in a later release is enforced in code as well.

## Reading and searching

### `knowledge_search`

Embeds a natural-language query and returns the closest `kb_chunks` rows from
the named collection, by cosine similarity in pgvector.

**Requires:** the knowledge service.

### `tool_search`

Searches the catalog of MCP tools by free-text query and optional category.
Returns each match with its server, name, description and per-tool categories
(`null` when uncategorized). Matches servers as well when the scope says so.

### `get_skill`

Loads the full instructions of a skill by name.

It also reaches the two read-only system skills, `nib-configuration` and
`nib-internals`, which the HTTP API does not list.

### `get_rule`

Loads the full text of a rule by name.

### `get_kb_document`

Returns the full knowledge-base source document for a collection, rather than
the roughly 500-character chunks `knowledge_search` returns.

### `list_variables`

Lists the prompt template variables configured in the system.

### `get_secrets`

Lists the names of existing secrets. Values are never returned.

## Plan artifacts

Each of these is bound to the **plan**, not to the transcript of the dialog that
calls it — a sub-agent writing a summary writes the root plan's summary.

### `create_summary`

Saves a plan summary for rendering in the web interface.

### `create_dag`

Saves a Mermaid `flowchart TD` diagram as Markdown for the plan.

### `create_plan_contract`

Saves the cross-stage contract: every identifier, secret path, endpoint,
namespace and version that more than one stage has to agree on.

### `get_dag`

Returns the stage-level plan: the summary, the stages of the DAG, and the
contract.

### `create_action_plan`

Saves a complete action plan. Clears the operator's checkboxes, comments and
per-row run records, because the rows they were keyed to are gone.

### `update_action_plan`

Saves a single stage, leaving the other stages and the rollback untouched.

**Binding:** when the caller is a fan-out sub-agent, it is locked to that
agent's one stage.

### `update_rollback_plan`

Saves the rollback list, leaving every stage untouched.

### `get_action_list`

Returns the action plan bound to the conversation, including what previous
actions reported — which is how an identifier created by one action reaches the
action that needs it.

### `set_category`

Records which tool categories the task will need during planning. Chosen during
decomposition, inherited by every sub-agent of the plan.

## Acting

### `execute_command`

Runs a simple command locally on the backend host.

### `api_call`

Runs `curl` locally on the backend host with the given flags.

### `run_executor`

Launches an autonomous coding agent in a separate container.

**Requires:** the executor service — absent while the executor type is
Disabled. Takes the single execution slot. See [Executor
settings](executor.md).

### `execute_action`

Carries out one action of the plan, named by the number the operator sees
beside it.

**Root-only.** Deleted from the allow set of any sub-agent.

### `stop_execution`

Force-stops the execution running right now.

**Root-only.** Bound to no plan, like the lease it releases.

### `run_subagent`

Launches one of the specialists as a sub-agent. The `name` parameter is an enum
generated from the sub-agent registry, so the list of specialists is whatever the
registry holds; the arguments are one flat object, and each specialist reads only
the ones named for it.

`code` is the one specialist that is not a mode of nib's own: it hands `task`,
`repository`, `branch` and `pr_title` to the coding agent in a container, which
commits and opens a pull request, and reports back into the chat when it
finishes. It is how a code change asked for in the chat — typically a correction
to what a code action produced — is carried out, and it takes the single
execution slot.

**Root-only.**

## Talking to the operator

### `ask_question`

Asks one or more clarifying questions, each with optional suggested answers.
The operator may pick one or type a free-form answer.

**Requires:** a dialog.

### `report_blocker`

Puts one question to the operator and **waits for the answer**, which comes back
as the call's own result. The question is posted into the plan's chat under the
name of the stage that raised it, and only one is pending there at a time: a
second stage asking meanwhile queues behind the first. The other stages keep
planning throughout. If no answer comes back — the run was stopped, or the wait
timed out — the call says so and the agent proceeds under the `assumption` it
stated, so the plan never gains a gap.

**Binding:** stage-scoped. Only a fan-out sub-agent has a stage to ask under, and
only it lacks `ask_question` — a fan-out turn cannot suspend and be resumed.

### `create_table`

Renders a formatted table in the chat. The same table is not to be repeated as
plain text in the reply.

### `chat_name`

Assigns a display name to the conversation. With a `task_id`, the title is
stored as `<task_id>: <chat_name>` and the id is persisted on the dialog.

Offered on every turn that has a dialog; not in any allow list.

## Changing nib's own configuration

All four are **discuss-mode-owned**.

### `write_rule`

Creates a rule, or replaces one that exists, under the rules directory.

### `update_kb`

Overwrites the knowledge-base document for a collection: every existing chunk
is deleted first.

### `update_tool_category`

Replaces the match patterns of an existing tool category.

### `update_included_tools`

Takes named MCP tools out of one mode's included list. The tools stay reachable
through `tool_search`. It only ever removes.

## Never in an allow list

### `set_report`

Saves the closing report of a finished plan.

The report agent is handed a literal two-tool catalog, and the name is deleted
from the action agents' allow set as well.

## See also

- [Modes](modes.md)
- [Modes, tools and the catalog](../explanation/modes-tools-and-the-catalog.md) — why a discovered tool may not be offered
- [HTTP API](http-api.md#tools)
