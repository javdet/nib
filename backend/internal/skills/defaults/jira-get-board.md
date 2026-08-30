---
name: jira-get-board
description: Show the user's Jira board as a table with columns "Selected for Development", "On Hold", "In Progress" and "To Review". Each cell holds only the issue key and summary, rendered with the create_table tool. Use when the user asks to see their board, get my board, or show tasks across in-progress statuses.
---

# Jira Get Board

Fetch the user's active Jira issues and render them as a board-style table (one column per status), just like the Jira board. Each card shows only the issue key and summary.

The columns, left to right, are:

1. `Selected for Development`
2. `On Hold`
3. `In Progress`
4. `To Review`

## Input Parameters

If the user specifies a project and assignee, use them for searching. If not, use the defaults.
Don't ask! Just use default values if they are not provided.

| Parameter | Required | Default | Example |
|-----------|----------|---------|---------|
| `project` | yes | `{{ .global.JiraProject }}` | `{{ .global.JiraProject }}`, `PROJ` |
| `assignee` | yes | `{{ .global.Email }}` | `{{ .global.Email }}` |

## Workflow

### Step 1: Fetch the issues

Call the `jira_search` MCP tool. Query all four statuses at once and order by status so cards are easy to group:

```json
{
  "toolName": "jira_search",
  "arguments": {
    "jql": "project = {{ .global.JiraProject }} AND assignee = \"{{ .global.Email }}\" AND status IN (\"Selected for Development\", \"On Hold\", \"In Progress\", \"To Review\") ORDER BY status ASC",
    "fields": "summary,status",
    "limit": 50
  }
}
```

Field mapping from each issue in the response:
- key ← `issue.key` (e.g. `{{ .global.JiraProject }}-1939`)
- summary ← `issue.summary`
- status ← `issue.status.name` (used only to place the card in the right column)

### Step 2: Group issues by status

Bucket every issue into its column by `status.name`:

- `Selected for Development`
- `On Hold`
- `In Progress`
- `To Review`

Ignore any status that is not one of the four columns.

### Step 3: Render the board with `create_table`

Call the `create_table` tool. Use the four statuses as columns. Build one row per card slot: row `i` holds the `i`-th issue of each column, and an empty string `""` where a column has no more issues. Each non-empty cell contains **only** the issue key and its summary, on two lines:

```
{{ .global.JiraProject }}-1939
Flutter unified deploy job
```

Example call for a board with two "In Progress" issues, one "To Review" issue and nothing else:

```json
{
  "toolName": "create_table",
  "arguments": {
    "title": "My Jira board",
    "columns": ["Selected for Development", "On Hold", "In Progress", "To Review"],
    "rows": [
      ["", "", "{{ .global.JiraProject }}-1939\nFlutter unified deploy job", "{{ .global.JiraProject }}-2011\nRotate staging secrets"],
      ["", "", "{{ .global.JiraProject }}-1975\nMigrate CI runners", ""]
    ]
  }
}
```

### Step 4: Reply

Do not repeat the table as plain text in your reply — the `create_table` tool already renders it to the user. If no issues match, tell the user their board is empty for those statuses.
