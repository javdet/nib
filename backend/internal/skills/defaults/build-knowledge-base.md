---
name: build-knowledge-base
description: Build or refresh the knowledge base document for a project from its repositories. Reads the current knowledge base skeleton, examines the repositories over MCP (README.md and CLAUDE.md first, then targeted code search), fills the skeleton in and publishes it with update_kb. Use when the user asks to build, create, fill in, refresh or update the knowledge base for a project from its repos.
category: included
---

# Build Knowledge Base

Turn a project's repositories into its knowledge base document: fetch the current skeleton, read the repositories, fill every section you can support with evidence, and publish the result to the collection named after the project.

The collection name **is** the project name. `update_kb` replaces the collection in full, so what you send has to be the whole document.

## Input Parameters

| Parameter | Required | Default | Example |
|-----------|----------|---------|---------|
| `project` | yes | — | `kiss`, `therapylog` |
| `repositories` | yes | — | `playneta/kiss2-infra`, `https://github.com/playneta/helm-charts` |

Neither has a default. If the user named a project but no repositories, ask for the repository list in your reply and stop — do not guess repository names. If the user named repositories but no project, ask which collection to write to.

## Workflow

### Step 1: Read the current document

Call `get_kb_document` with the project name as the collection:

```json
{
  "toolName": "get_kb_document",
  "arguments": {
    "collection": "kiss"
  }
}
```

Never work from a skeleton you remember — the template ships with the image and changes between releases. Read it here, every time.

The `source` field decides what you are doing:

- `"template"` — nothing has been uploaded to this collection yet. `content` is the empty skeleton. Its headings, their order and the writing rules in the HTML comment at the top are your contract for the rest of this skill.
- `"uploaded"` — this is the project's living document. **Keep every fact it already states.** Fold your findings into it: add what is missing, correct what the repositories contradict, and leave untouched anything the repositories say nothing about — an operator wrote those facts from sources you cannot see. If a section from the current skeleton is absent from the uploaded document, add it.

### Step 2: Find the repository tools

Call `tool_search` for the tools that read repository content — file fetch and code search. With a GitHub MCP server configured these are `get_file_contents` and `search_code`; other providers name them differently, so use what the search returns.

If no repository tools are configured, stop. Tell the operator the knowledge base cannot be built from repositories until an MCP server with repository access is connected.

### Step 3: Read the front door of each repository

For every repository, in this order:

1. `README.md`
2. `CLAUDE.md` (also `AGENTS.md` or `CONTRIBUTING.md` when present)
3. The `docs/` pages the README explicitly points at

These name the stack, the environments, the deploy path and the vocabulary the rest of the repository uses. Read them before searching anything, because they tell you what to search for.

Worked example:

```json
{
  "toolName": "get_file_contents",
  "arguments": {
    "owner": "playneta",
    "repo": "kiss2-infra",
    "path": "README.md"
  }
}
```

### Step 4: Search for what each section needs

Work section by section through the skeleton you got in step 1. For each one, run a targeted code search for the artefacts that hold the answer:

| Skeleton section | What to search for |
|------------------|--------------------|
| Repositories | repository layout in the READMEs, top-level directory names, `CODEOWNERS` |
| Environments | `stage`/`prod` directory names, environment matrices in pipelines, `values-*.yaml` |
| Clouds, Accounts and Regions | provider blocks in `*.tf`, region and zone identifiers, `terragrunt.hcl` inputs |
| Naming and Tagging Conventions | resource `name` and `tags` in `*.tf`, chart `fullnameOverride`, label blocks |
| Traffic Routing | `Ingress` manifests, DNS records in Terraform, CDN origins, load balancer resources |
| Network Layer | VPC and subnet resources, CIDR literals, firewall and security-group rules |
| Compute and Orchestration | `Chart.yaml`, `values.yaml`, `helmfile.yaml`, Argo CD `Application`, node pool resources |
| Application Layer | language manifests (`go.mod`, `package.json`, `pubspec.yaml`), `Dockerfile`, deployment templates |
| Data Layer | env var names (`*_HOST`, `DATABASE_URL`, `REDIS_*`, `KAFKA_*`), migration directories, pooler configs |
| Infrastructure as Code | `*.tf`, `terragrunt.hcl`, `ansible/`, backend/state configuration, module registries |
| CI/CD | `.github/workflows/*.yml`, `.gitlab-ci.yml`, registry hosts, image tag patterns |
| Secrets Management | Vault paths, `SealedSecret`, `ExternalSecret`, secret injector annotations |
| Access and Permissions | IAM policy resources, `Role`/`RoleBinding`, service account definitions |
| Observability | dashboard JSON, `PrometheusRule`, alert receivers and routes, tracing config |
| Incident Management | runbook documents under `docs/`, on-call and escalation notes |
| Backup and Disaster Recovery | backup schedules and retention in IaC, snapshot resources, restore runbooks |

Worked example:

```json
{
  "toolName": "search_code",
  "arguments": {
    "query": "repo:playneta/kiss2-infra path:.github/workflows registry"
  }
}
```

**Keep the context under control.** This is the difference between a skill that finishes and one that drowns:

- Search for the thing you need. Do **not** list the repository tree from the root, and do not walk it directory by directory to see what is there.
- Open a file only when a search hit or a README reference points at it, and prefer the smallest file that answers the question.
- When a listing really is unavoidable, list exactly one directory, not the tree beneath it.
- Skip vendored, generated and lock files, `node_modules`, test fixtures, and anything that is mostly data.
- The moment a section can be stated from what you have read, stop searching for it and move on.
- A section nothing supports is left unknown. Do not keep searching for it, and never fill it from general knowledge of how such systems usually work.

### Step 5: Write the document

Keep the skeleton's headings and their order. Delete the HTML comment block at the top — it is instructions for you, not content — and replace each section's placeholder text with facts.

Follow the writing rules the skeleton states, because they are what makes retrieval work:

- State facts, not prose. Retrieval returns ~500-character chunks by semantic similarity, so **every section must make sense on its own**, without the ones around it. Repeat the project or component name inside a section rather than writing "it" or "the above".
- Spell names out in full at least once: cluster names, namespaces, hostnames, repository URLs, image tags.
- Prefer a pattern plus a worked example over an abstract description.
- Say where a fact is defined — repository path, chart value, Terraform resource.
- **Never put a secret value in the document.** If you find a token, password or key in a repository, record where the secret lives (the Vault path, the sealed secret name) and never the value. Mention the exposure to the operator in your final reply.

For a section the repositories did not support, write exactly:

```
Unknown — not found in the repositories examined.
```

End the document with the repositories you examined and today's date, so the next run knows what the current text is based on:

```
## Sources
Built from: github.com/playneta/kiss2-infra, github.com/playneta/helm-charts.
Last updated: 2026-08-30.
```

### Step 6: Publish with `update_kb`

Send the **complete** document — never a fragment, never a diff, never just the sections you changed. `update_kb` deletes every existing chunk in the collection and replaces it with what you pass.

```json
{
  "toolName": "update_kb",
  "arguments": {
    "collection": "kiss",
    "content": "# Knowledge Base\n\n## Project Overview\n..."
  }
}
```

### Step 7: Reply

Report, briefly:

- the collection written and the `chunk_count` `update_kb` returned;
- which sections you filled;
- which sections are `Unknown`, and what would answer them (a repository not in the list, a cloud console, an operator who knows);
- any secret value you found sitting in a repository.

Do not paste the document back into the chat — the operator can read it on the Knowledge Base page.
