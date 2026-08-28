# universal-agent

Single-use container that runs a headless coding agent against one repository (optional),
opens a pull request with the result, and notifies a webhook on completion.

**Agent runtimes:** `claude-code` (default) or `codex`, selected by `AGENT_TYPE`.

One container = one task. The backend launches this image via Docker/Kubernetes;
the entrypoint handles git clone, branch management, commit, push, PR creation,
and webhook delivery deterministically.

## Build

```bash
cd agent-runner/universal-agent

docker build -t universal-agent:local .

# Pin versions:
docker build -t universal-agent:local \
  --build-arg CLAUDE_CODE_VERSION=2.1.220 \
  --build-arg CODEX_VERSION=0.147.0 \
  --build-arg GH_VERSION=2.97.0 .
```

Multi-arch:

```bash
docker buildx build --platform linux/amd64,linux/arm64 -t universal-agent:local .
```

Expected image size: ~650–750 MB (Debian slim + Claude Code npm + Codex musl binary).

## Runtime selection

| `AGENT_TYPE` | CLI | Default model env |
|---|---|---|
| `claude-code` (default) | `claude` | `ANTHROPIC_MODEL` |
| `codex` | `codex exec` | `OPENAI_MODEL` |

Aliases: `claude` → `claude-code`.

## Shared environment contract

These variables work identically for both agents:

| Variable | Required | Default | Description |
|---|---|---|---|
| `PROMPT` | yes | – | Task instructions |
| `REPO_URL` | no | – | HTTPS clone URL; omit for MCP-only scratch mode |
| `TARGET_BRANCH` | if repo | – | Branch to work on and push |
| `BASE_BRANCH` | no | remote default | Base branch for PR |
| `GITHUB_TOKEN` | if repo | – | Token for clone, push, PR |
| `MCP_CONFIG` | no | – | Inline JSON with `mcpServers` object |
| `MCP_CONFIG_FILE` | no | – | Path to MCP JSON file |
| `SYSTEM_PROMPT_FILE` | no | – | Path to system prompt file |
| `SYSTEM_PROMPT_MODE` | no | `append` | `append` or `replace` |
| `ALLOWED_TOOLS` | no | – | Comma-separated tool list (see translation below) |
| `DISALLOWED_TOOLS` | no | – | Comma-separated deny list |
| `PERMISSION_MODE` | no | `acceptEdits` | Sandbox/permission mode (see translation) |
| `REQUIRE_MCP` | no | `1` | Fail if MCP servers do not connect |
| `TIMEOUT_SECONDS` | no | – | Hard cap on agent run |
| `WEBHOOK_URL` | no | – | POST callback URL on exit |
| `WEBHOOK_AUTH_HEADER` | no | – | e.g. `Authorization: Bearer <token>` |
| `TASK_ID` | no | – | Echoed in webhook |
| `THREAD_ROOT_ID` | no | – | Echoed in webhook |
| `CHAT_ID` | no | – | Echoed in webhook (required by Nib backend) |
| `PR_TITLE` / `PR_BODY` | no | derived | PR metadata |
| `GIT_AUTHOR_NAME` / `GIT_AUTHOR_EMAIL` | no | defaults | Commit author |
| `JOB_NAME`, `POD_NAME`, `POD_NAMESPACE` | no | – | Job metadata in webhook |
| `IMAGE_VERSION` | no | – | Image tag in webhook |

## Claude Code auth (`AGENT_TYPE=claude-code`)

| Variable | Description |
|---|---|
| `ANTHROPIC_API_KEY` | API key (mapped to `ANTHROPIC_AUTH_TOKEN` for OpenRouter) |
| `ANTHROPIC_BASE_URL` | Default `https://openrouter.ai/api` |
| `ANTHROPIC_MODEL` | Default `anthropic/claude-opus-5` |
| `CLAUDE_CODE_OAUTH_TOKEN` | Anthropic subscription auth (clears OpenRouter defaults) |

## Codex auth (`AGENT_TYPE=codex`)

| Variable | Description |
|---|---|
| `OPENAI_API_KEY` or `CODEX_API_KEY` | API key (either works) |
| `OPENAI_BASE_URL` | Default `https://openrouter.ai/api/v1` (Responses API) |
| `OPENAI_MODEL` | Default `openai/gpt-5.3-codex` |
| `CODEX_HOME` | Default `/home/node/.codex` (config + auth root) |

`CLAUDE_CODE_OAUTH_TOKEN` is rejected for codex.

When `OPENAI_BASE_URL` points at a non-OpenAI host, the entrypoint writes a custom
`[model_providers.gateway]` block with `wire_api = "responses"`.

## Codex-specific overrides

| Variable | Description |
|---|---|
| `CODEX_SANDBOX_MODE` | Override sandbox: `read-only`, `workspace-write`, `danger-full-access`, or `bypass` |
| `CODEX_NETWORK_ACCESS` | Default `1`; set `0` to disable network in `workspace-write` mode |
| `CODEX_CONFIG_TOML` | Extra TOML appended to `$CODEX_HOME/config.toml` |
| `CODEX_CONFIG_FILE_PATH` | Path to extra TOML file appended to config |
| `CODEX_EXTRA_ARGS` | Space-separated extra args passed to `codex exec` |

## Tool allow/deny translation (codex)

Codex has no `--allowedTools` flag. The entrypoint translates the shared env vars:

| Input | Codex behavior |
|---|---|
| `mcp__<server>__<tool>` | `enabled_tools = ["<tool>"]` on that MCP server |
| `mcp__<server>__*` | Server fully enabled (no allow-list) |
| Any `mcp__` in `ALLOWED_TOOLS` | Servers not mentioned get `enabled = false` |
| `DISALLOWED_TOOLS` with `mcp__` prefix | `disabled_tools` on the server |
| `Read`, `Write`, `Edit`, `Grep`, `Glob`, `Bash` | Ignored (logged once); sandbox mode governs file/shell access |

## Permission mode translation (codex)

| `PERMISSION_MODE` | Codex flags |
|---|---|
| `plan` | `-s read-only` |
| `acceptEdits`, `default`, unset | `-s workspace-write` + `network_access=true` |
| `bypassPermissions` | `--dangerously-bypass-approvals-and-sandbox` |

Override with `CODEX_SANDBOX_MODE`.

## MCP config

Both agents accept the same Claude Code-shaped JSON:

```json
{
  "mcpServers": {
    "tool-gw": {
      "type": "http",
      "url": "https://mcp.example.com/mcp",
      "headers": {
        "Authorization": "Bearer ${MCP_GW_TOKEN}"
      }
    }
  }
}
```

- **Claude Code:** passed as `--mcp-config` JSON file.
- **Codex:** converted to `[mcp_servers.*]` blocks in `$CODEX_HOME/config.toml`.
  HTTP header values are exported as env vars and referenced via `env_http_headers`
  so secrets never appear literally in the config file.

`REQUIRE_MCP=1` (default) sets `required = true` on each MCP server for codex
(all must connect; stricter than Claude's "at least one connected" check).

## Webhook payload

POST JSON to `WEBHOOK_URL` on container exit (success or failure):

```json
{
  "status": "success",
  "exit_code": 0,
  "agent_type": "claude-code",
  "result": "...",
  "log_tail": "...",
  "task_id": "...",
  "thread_root_id": "...",
  "chat_id": "...",
  "job": { "name": "...", "pod": "...", "namespace": "...", "image_version": "..." },
  "repo": { "url": "...", "base_branch": "...", "target_branch": "...", "pushed": true, "pr_url": "..." },
  "usage": {
    "duration_ms": 12345,
    "num_turns": 3,
    "total_cost_usd": 0.42,
    "input_tokens": 0,
    "output_tokens": 0,
    "session_id": "..."
  }
}
```

- `total_cost_usd` is populated for Claude Code; codex sets it to `0`.
- `input_tokens` / `output_tokens` are populated for codex; Claude sets them to `0`.
- `agent_type` identifies which runtime ran.

## Example: Claude Code (default)

```bash
docker run --rm \
  -e PROMPT="Add a readiness probe to the api Deployment" \
  -e REPO_URL="https://github.com/org/repo.git" \
  -e TARGET_BRANCH="agent/test-123" \
  -e GITHUB_TOKEN="$GITHUB_TOKEN" \
  -e ANTHROPIC_API_KEY="$OPENROUTER_KEY" \
  universal-agent:local
```

## Example: Codex

```bash
docker run --rm \
  -e AGENT_TYPE=codex \
  -e PROMPT="Add a readiness probe to the api Deployment" \
  -e REPO_URL="https://github.com/org/repo.git" \
  -e TARGET_BRANCH="agent/test-123" \
  -e GITHUB_TOKEN="$GITHUB_TOKEN" \
  -e OPENAI_API_KEY="$OPENROUTER_KEY" \
  -e OPENAI_MODEL="openai/gpt-5.3-codex" \
  universal-agent:local
```

## Design notes

- **Debian slim, not Alpine:** full glibc `curl`, GNU coreutils, and Claude Code needs Node.
- **Codex from release tarball:** ~260 MB static musl binary, no npm wrapper overhead.
- **bubblewrap** installed for codex sandbox modes inside the container.
- **Shared entrypoint:** git/PR/webhook logic is agent-agnostic; only auth, MCP prep,
  invocation, and log parsing differ.
