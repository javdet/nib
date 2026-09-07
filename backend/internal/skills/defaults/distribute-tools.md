---
name: distribute-tools
description: Narrow the included MCP tools of every chat mode by applying the per-mode distribution policy below to a supplied tool list, writing the result with update_included_tools. Use when the operator asks to distribute, spread, auto-fill or re-scope included tools across modes.
---

# Distribute Tools

Read a list of modes and a list of MCP tools, decide which tools each mode must **not** carry,
and write the removals with `update_included_tools`.

Both lists arrive in the operator's message. This skill does not discover anything: it never
calls `tool_search`, never lists the catalog, and never invents a mode.

## Input Parameters

| Parameter | Required | Default | Example |
|-----------|----------|---------|---------|
| `modes` | yes | — | `main`, `execute` |
| `tools` | yes | — | `k8s_delete_pod (kubernetes-mcp): Delete a pod in a namespace` |

Neither has a default. If the message carries no modes, or no tools, say so in your reply and
stop — do not guess either list.

## Workflow

### Step 1: Apply the policy

Every tool is included in every mode until it is taken out, so the question for each mode is
only *what must go*.

| Mode | Rule |
|------|------|
| `main` | Remove every tool that modifies or deletes |
| `decompose` | Remove every tool that modifies or deletes |
| `plan` | Remove every tool that modifies or deletes |
| `execute` | Remove every version control tool |
| `discuss` | Remove every tool that modifies or deletes |
| `incident` | Leave untouched — make no call for this mode |

Skip any mode in the table that is not in the supplied `modes` list, and make no call for a mode
the table does not name.

**Modifies or deletes** means the tool changes a live system: it creates, updates, patches,
applies, scales, restarts, drains, rotates, deletes, destroys or otherwise writes. A read-only
tool — `get_`, `list_`, `describe_`, `search_`, `query_`, `logs`, `status` — never counts,
however dangerous the system behind it is.

**Version control** means commits, branches, pushes, tags, pull and merge requests, reviews,
releases and repository administration. Reading a file out of a repository is not version
control: `execute` still has to read the code it is changing.

Judge from the description, not from the server prefix — a prefix says who provides the tool,
not what it does. When a tool is genuinely ambiguous, remove it: an over-removed tool is still
reachable through `tool_search`, while a wrongly kept one is loaded into the context of every
turn the mode runs.

### Step 2: Write the removals

Make **one `update_included_tools` call per mode** that has something to remove, and emit **all
of those calls in a single response** so they are applied in one round.

```json
{
  "toolName": "update_included_tools",
  "arguments": {
    "mode": "execute",
    "tools": ["github_create_pull_request", "github_merge_pull_request", "gitlab_create_branch"]
  }
}
```

Rules:

- `tools` holds **bare tool names**, exactly as given, without the server in parentheses.
- Use **exact tool names only, never a wildcard** — the argument is a list of names, not patterns.
- Never send an empty `tools` array — it is rejected. Skip a mode with nothing to remove.
- One call per mode. Do not call the same mode twice, and do not retry a call that already
  succeeded.
- The tool only removes. Nothing here can put a tool back, so a name you are unsure of costs the
  operator a click on the Tools → Included tools page.

Each call answers with how many names it removed, how many were not in the list, and how many
tools the mode still carries. Read those numbers back — names reported as *not in the list* were
either already excluded or misspelled.

### Step 3: Reply

Render the outcome with `create_table`: one row per mode you wrote, the tool names in the second
column, the remaining count the call reported in the third.

```json
{
  "toolName": "create_table",
  "arguments": {
    "title": "Tools removed from modes",
    "columns": ["Mode", "Tools removed", "Tools remaining"],
    "rows": [
      ["execute", "github_create_pull_request\ngithub_merge_pull_request", "184"],
      ["discuss", "k8s_delete_pod\nk8s_scale_deployment", "171"]
    ]
  }
}
```

Do not repeat the table as plain text — `create_table` already renders it. After it, in one
sentence, name any tool you found ambiguous and which way you decided, and name any mode that
needed no change.
