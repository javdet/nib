---
name: categorize-tools
description: Sort a supplied list of MCP tools into a supplied list of tool categories by reading each tool's name and description, then write the assignments with update_tool_category. One tool may belong to several categories. Use when the operator asks to auto-fill, categorize, classify or sort tools into tool categories.
---

# Categorize Tools

Read a list of tool categories and a list of tools, decide which categories each tool belongs to, and write the result back with `update_tool_category`.

Both lists arrive in the operator's message. This skill does not discover anything: it never calls `tool_search`, never lists the catalog, and never invents a category.

## Input Parameters

| Parameter | Required | Default | Example |
|-----------|----------|---------|---------|
| `categories` | yes | — | `cloud: Cloud provider APIs`, `kubernetes` |
| `tools` | yes | — | `list_pods (kubernetes-mcp): List pods in a namespace` |

Neither has a default. If the message carries no categories, or no tools, say so in your reply and stop — do not guess either list from the tool catalog.

## Workflow

### Step 1: Assign every tool

Work down the `tools` list. For each tool, read its name and its description and decide which of the given categories it serves.

- **A tool may belong to several categories.** A tool that reads Kubernetes pod logs belongs to `kubernetes` and to `logging`. Put it in both.
- **A tool that fits nothing is left out.** Do not stretch a category to cover it; an unassigned tool costs nothing, a wrong assignment puts an irrelevant tool into the context of every plan that picks the category.
- **Only the given category names are valid.** Categories come from the `toolCategories` variable and cannot be created here — the `category` argument is constrained to the existing names and any other value is rejected.
- Judge from the description, not the prefix. A server prefix is a hint about who provides the tool, not about what it does.

### Step 2: Write the assignments

Make **one `update_tool_category` call per category** that got at least one tool, and emit **all of those calls in a single response** so they are applied in one round.

```json
{
  "toolName": "update_tool_category",
  "arguments": {
    "category": "kubernetes",
    "patterns": ["list_pods", "get_pod_logs", "describe_deployment"],
    "action": "replace"
  }
}
```

Rules:

- `patterns` holds **bare tool names**, exactly as given, without the server in parentheses. A pattern is matched against the tool name alone.
- Use **exact tool names only, never a wildcard**. `action: "replace"` makes the patterns you send the category's whole set, and a `prefix_*` would also claim tools that were never shown to you.
- Always pass `"action": "replace"`.
- Never send an empty `patterns` array — it is rejected. Skip a category that got no tools; leaving it untouched keeps the patterns it already has.
- One call per category. Do not call the same category twice, and do not retry a call that already succeeded.

Each call answers with the category's full pattern set after the edit and how many catalog tools it now matches. Read that number back — a category reporting fewer tools than the patterns you sent means a name was misspelled.

### Step 3: Reply

Render the outcome with `create_table`: one row per category you wrote, the tool names in the second column, the matched count the call reported in the third.

```json
{
  "toolName": "create_table",
  "arguments": {
    "title": "Tools added to categories",
    "columns": ["Category", "Tools", "Tools matched"],
    "rows": [
      ["kubernetes", "list_pods\nget_pod_logs\ndescribe_deployment", "3"],
      ["logging", "get_pod_logs\nquery_loki", "2"]
    ]
  }
}
```

Do not repeat the table as plain text — `create_table` already renders it. After it, in one sentence, name the tools you left uncategorized and why, and name any category that received nothing.
