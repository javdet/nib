---
name: jira-todo-tasks
description: Retrieve todo tasks from Jira filtered by project, assignee, and "Selected for Development" status. Returns a JSON array. Use when the user asks for their Jira backlog, todo list, selected tasks, or tasks ready for development.
---

# Jira Todo Tasks

Fetch Jira issues with status "Selected for Development" using the Atlassian MCP server.

## Input Parameters

If the user specifies a project and assigne, use them for searching. If not, use the default.
Don't ask! Just use default values if the not provided 

| Parameter | Required | Default | Example |
|-----------|----------|---------|---------|
| `project` | yes | — | `{{ .global.JiraProject }}`, `PROJ` |
| `assignee` | yes | — | `{{ .global.Email }}` |

## Workflow

### Step 1: Build and execute the JQL query

Call the `jira_search` MCP tool:

```json
{
  "toolName": "jira_search",
  "arguments": {
    "jql": "project = {{ .global.JiraProject }} AND status = \"Selected for Development\" AND assignee = \"{{ .global.Email }}\"",
    "fields": "summary,priority,duedate",
    "limit": 20
  }
}
```

### Step 2: Format the response

Transform the raw response into a JSON array with this exact structure:

```json
[
  {
    "Key": "{{ .global.JiraProject }}-1939",
    "Summary": "Flutter unified deploy job",
    "Priority": "Medium",
    "Due Date": "2026-05-01"
  }
]
```

Field mapping from the API response:
- `Key` ← `issue.key`
- `Summary` ← `issue.summary`
- `Priority` ← `issue.priority.name`
- `Due Date` ← `issue.duedate` (use `null` if not set)

### Step 3: Return the result

Present the JSON array to the user. If the result is empty, inform the user that no tasks match the filter.
