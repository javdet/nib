# How to share a plan as a `.nib` file

A `.nib` file carries everything that built a plan, so someone on another nib
install can import it and run it against their own systems. It does not carry
anything about how the plan ran on yours.

## Export

1. Open the plan.
2. Click the import/export icon (the two arrows beside **Simplified / Detailed**).
3. Choose **Export as .nib file**.

The file is named after the plan's title. **Download as Markdown** is still in
the same menu. It produces a readable document, which nib cannot import back.

From a script:

```bash
curl -H "Authorization: Bearer $NIB_API_TOKEN" \
  -o plan.nib "https://nib.example.com/api/v1/dialogs/$PLAN_ID/export"
```

## Import

On the **Plans** page, click the upload icon beside **New plan**. Or, on any
open plan, choose **Import plan from .nib file** from the import/export menu.
Either way the file becomes a **new** plan and opens. Nothing already on the
install changes.

```bash
curl -H "Authorization: Bearer $NIB_API_TOKEN" -H "Content-Type: application/json" \
  --data-binary @plan.nib "https://nib.example.com/api/v1/dialogs/import"
```

The imported plan starts as a **draft** with no step ticked, and you can run it
like any plan you made yourself.

## What travels and what does not

| In the file | Left behind |
|---|---|
| Title and tool categories | Status and schedule |
| Summary and DAG | Ticked checkboxes |
| Plan contract (decomposition's shared names) | Execution runs, their notes and container logs |
| Action plan with the operator's comments | **Execute all** runs |
| The plan chat, every message, with attachments | The execute sub-agents' transcripts |
| The decompose and plan sub-agents' transcripts | The finish report and knowledge-base update |
| A decompose question still waiting for an answer | Chat-requested code fixes |

Also left behind: the **system prompts**. Each one is rendered from the
exporting install's variables (company name, wiki, host details), so an import
renders fresh ones from the importing install's variables and its current
project/environment selection. The file records the exporter's selection under
`plan.selection` for information only.

Things the plan *refers to* do not travel either: MCP servers, secrets,
variables, skills, rules and the executor configuration. A plan that calls a
tool the importing install lacks still imports, and that step fails when it
runs. Check the tool categories and the executor before you execute.

## The file format

A `.nib` file is UTF-8 JSON, pretty-printed so it reads and diffs as text:

```json
{
  "format": "nib-plan",
  "version": 1,
  "exportedAt": "2026-09-27T12:00:00Z",
  "nibVersion": "v0.8.1",
  "plan": {
    "id": "…",
    "title": "Upgrade Postgres to 17",
    "mode": "main",
    "categories": ["database"],
    "createdAt": "…",
    "selection": { "project": "billing", "environment": "prod", "cloud": "any", "location": "any" }
  },
  "summary": "…",
  "dag": "```mermaid\ngraph TD\n…\n```",
  "contract": { "shared": [], "stages": {} },
  "actionPlan": { "stages": [], "rollback": [] },
  "comments": { "s0.step1": "check disk space first" },
  "chat": [
    { "role": "user", "content": "…", "createdAt": "…" },
    { "role": "assistant", "content": "", "toolCalls": [], "createdAt": "…" },
    { "role": "tool", "toolCallId": "…", "name": "run_subagent", "content": "…", "createdAt": "…" }
  ],
  "subagents": [
    { "id": "…", "mode": "decompose", "title": "Decompose", "createdAt": "…", "messages": [] }
  ],
  "pauses": []
}
```

- `chat` and each sub-agent's `messages` hold only `user`, `assistant` and
  `tool` rows. An attachment rides on its message as
  `{"id", "filename", "contentType", "data"}`, with `data` in base64.
- Ids are the exporting install's. An import gives every dialog and
  attachment a new id and rewrites each old id wherever it appears in the text.
- A future release bumps `version` only when an older nib could not read the
  file correctly. An older nib refuses a newer version instead of guessing.
  Unknown fields are ignored.
- An import is all or nothing: a file that fails validation, or a write that
  fails halfway, leaves no plan behind.
