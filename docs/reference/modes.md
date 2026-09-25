# Modes

A mode is the role a chat runs in. It fixes the system prompt, the tools on
offer, and whether the chat carries an action plan.

The list is fixed in the backend. `GET /api/v1/modes` returns it.

## The six modes

| Mode | Carries a plan | Started by | Role |
|---|:-:|---|---|
| `main` | yes | the **New plan** button | The orchestrator. Does no work itself; launches the specialists and relays their results. |
| `decompose` | yes | `main`, as a sub-agent | Breaks a task into stages, writes the summary, the DAG and the cross-stage contract, and picks the tool categories. |
| `plan` | yes | `main`, as a sub-agent | Works each stage out into elementary steps; one agent per stage, plus one that derives the rollback. |
| `execute` | yes | `main`, as a sub-agent | Carries out one action of the plan. |
| `discuss` | no | the operator | Free-form conversation. The only mode that can change nib's own configuration. |
| `incident` | no | the operator | Root-cause analysis of incidents. In development. |

`main` is the default mode a dialog gets when none is asked for, and it matches
the `chat_dialogs.mode` column default.

**Carries a plan** means dialogs in that mode own an action plan, a DAG, a
summary and plan state. The four plan modes are a whitelist, kept in step with
the `plan_dialogs` view in the database.

## Sub-agents

`main` launches the other modes with the `run_subagent` tool. The choices are
generated from a registry in the backend, so this list is exactly what the tool
offers.

| Name | Parameters | Blocking | Behaviour |
|---|---|:-:|---|
| `decompose` | `task` (required) | yes | Runs to completion inside the orchestrator's call. May come back asking for a decision, which the orchestrator relays. |
| `plan` | `stages`, `rollback` | no | One planning agent per stage in parallel, wave by wave, then one that derives the rollback. A stage that needs a decision asks in the chat under its own name and waits for you while its siblings carry on. Naming stages replans only those. Returns as soon as the run starts. |
| `execute` | `item` (required), `rerun` | no | Carries out one action by the number the operator sees. A `code` action goes to a container and opens a pull request; anything else runs in an agent that posts its result into the chat. Returns as soon as the work starts. |
| `code` | `task` (required), `repository` (required), `branch`, `pr_title` | no | An ad-hoc code change, outside the action plan: the coding agent gets a container, commits and opens a pull request. Naming the branch an earlier code action pushed to lands the change on top of it, in the pull request already open; omitting it starts a branch of its own. Returns as soon as the container starts. |

Every sub-agent is parented directly to the root dialog, so listing the root's
children returns everything a plan spawned.

Two more `execute`-mode agents run without `run_subagent`, both started by
**Finish**: the report agent (`execute_report` overlay, catalog `get_action_list`
and `set_report`) and, when it ends and `autoUpdate` is on, the knowledge-base
update agent (`execute_kb` overlay, catalog `get_action_list`,
`get_kb_document` and `update_kb`). Neither takes the execution slot.

`code` is the one launch that is not an agent of nib's own: the work happens in
an agent-runner container, which reports through the webhook rather than by
finishing a turn. It takes the single execution slot like a code action does, and
because it has no plan row to be recorded on, its run is recorded in
`data/code_fixes/{root}.json` instead. See [The
executor](executor.md#three-entry-points).

### Orchestrator-only tools

`run_subagent`, `stop_execution` and `execute_action` are removed from every
sub-agent's tool set in code, whatever a mode's allow list says. The
restriction cannot be lifted by editing a list.

Why each is orchestrator-only: [How a plan is
produced](../explanation/how-a-plan-is-produced.md).

## System prompts

Each mode has a Markdown prompt compiled into the backend. The backend refuses
to start if a mode has no prompt.

| Prompt | Used by |
|---|---|
| `main.md` | `main` |
| `decompose.md` | `decompose` |
| `decompose_subagent.md` | the decompose sub-agent |
| `plan.md` | `plan` |
| `plan_stage.md` | one stage planner in a fan-out |
| `rollback_stage.md` | the rollback agent |
| `execute.md` | `execute` |
| `execute_action.md` | one action executor |
| `execute_report.md` | the finish-report agent |
| `execute_kb.md` | the knowledge-base update agent chained after the report |
| `discuss.md` | `discuss` |
| `incident.md` | `incident` |

**Only `discuss` is editable.** `PUT` and `DELETE` on
`/api/v1/system-prompts/{name}` accept `discuss` and reject every other name.
The override is stored as `{DATA_DIR}/prompts/discuss.md`; deleting it restores
the built-in.

Prompts are Go templates, rendered against the prompt variables and the current
selection. See [State](state.md#prompts-rules-and-skills).

## Tool access

Each mode has a default allow list of built-in tools, and a separate included
list of MCP tools that can be narrowed per mode in the UI.

Full matrix: [Built-in agent tools](agent-tools.md#default-allow-lists).

Two modes own tools that no allow list can grant to another:

- **`discuss`** owns `write_rule`, `update_tool_category` and
  `update_included_tools`.
- **`main`** owns the three orchestrator tools above.

## Adding a mode

A mode is not configuration. A new one needs an entry in the backend's mode
list, a compiled-in system prompt, a compiled-in allow list, and frontend
support. There is no way to add one from the UI or the config file.

## See also

- [Built-in agent tools](agent-tools.md)
- [How a plan is produced](../explanation/how-a-plan-is-produced.md)
- [Modes, tools and the catalog](../explanation/modes-tools-and-the-catalog.md)
