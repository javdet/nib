# Configuration file

The backend reads one YAML file at startup. It holds structure: which model to
call, how hard the agent may work, where files live, and the static tree of
projects, environments, clouds and locations.

Credentials are not read from this file. They come from the environment — see
[Environment variables](environment-variables.md).

## File selection

The path is resolved in this order, first match wins:

1. the `-config` flag — `nib -config /app/config.yaml`
2. the `CONFIG_FILE` environment variable
3. `config.yaml`, relative to the working directory

A missing or unparseable file is fatal: the backend exits with `load config:
read config file <path>` or `parse config file <path>`.

## Precedence

Two different rules apply, depending on the block.

| Block | Rule |
|---|---|
| `llm` (except the API key), `log.enabled`, `log.file`, directory and file paths, `agent`, `knowledge_base`, projects | **YAML wins.** The environment supplies a starting value; a non-empty key in the file overwrites it. Remove the key from the file to drive the value per environment. |
| `metrics`, `log.level` | **Environment wins.** Each field checks the environment last, so a `METRICS_*` variable set by a Dockerfile or a Helm deployment overrides whatever the file says. |

The API key is never read from YAML.

## `llm`

Settings for the single OpenAI-compatible endpoint used for both chat
completions and embeddings.

### `llm.baseURL`

- **Type:** string
- **Required.** No default.
- **Environment starting value:** `LLM_BASE_URL`
- **Error:** `config: llm.baseURL is required (set llm.baseURL in YAML or LLM_BASE_URL)`

Base URL of the API, without a trailing path. Example:
`https://api.openai.com/v1`.

### `llm.model`

- **Type:** string
- **Required.** Falls back to `gpt-4o-mini` only when neither YAML nor
  environment supplies one.
- **Environment starting value:** `LLM_MODEL`, then `OPENAI_MODEL`
- **Error:** `config: llm.model is required (set llm.model in YAML or LLM_MODEL)`

The model id as the provider spells it. OpenRouter uses `provider/model` slugs.

### `llm.embeddingModel`

- **Type:** string
- **Default:** `text-embedding-3-small`
- **Environment starting value:** `OPENAI_EMBEDDING_MODEL`

The embedding model used when ingesting knowledge-base documents.

This string must match the `embedding_model` column of the `kb_collections`
table exactly. Knowledge search compares the two as strings, and a mismatch
returns no results rather than an error. See
[Switch the LLM provider](../how-to/switch-llm-provider.md) for the rename.

### `llm.api`

- **Type:** string
- **Default:** `chat`
- **Accepted:** `chat`, `responses`
- **Environment starting value:** `LLM_API`
- **Error:** `config: invalid llm.api "<value>" (allowed: chat, responses)`

Selects the completion endpoint. Embeddings always use `/v1/embeddings`
whatever this is set to.

| Value | Endpoint | Notes |
|---|---|---|
| `chat` | `/v1/chat/completions` | Supported by every OpenAI-compatible gateway. |
| `responses` | `/v1/responses` | Required for GPT-5.6 with tools and reasoning together. |

Why the option exists: [LLM endpoints and reasoning
replay](../explanation/llm-endpoints.md).

### `llm.reasoningEffort`

- **Type:** string
- **Default:** empty — the key is omitted from the request and the provider's
  own default applies
- **Accepted:** `none`, `minimal`, `low`, `medium`, `high`, `xhigh`
- **Environment starting value:** `LLM_REASONING_EFFORT`
- **Error:** `config: invalid llm.reasoningEffort "<value>" (allowed: none, minimal, low, medium, high, xhigh)`

Sent as `reasoning_effort` on every completion, whatever the model is. Only an
empty value is omitted, so a non-reasoning model needs this cleared.

### `llm.timeoutSeconds`

- **Type:** integer
- **Default:** `120`
- **Limit:** at least `1`
- **Error:** `config: llm.timeoutSeconds must be at least 1`

HTTP client timeout for one completion request.

### `llm.httpReferer`, `llm.appTitle`

- **Type:** string
- **Default:** empty
- **Environment starting value:** `HTTP_REFERER`, `OPENROUTER_APP_TITLE`

OpenRouter attribution headers. The OpenAI platform ignores them.

## `agent`

Budgets for the agent loop. Each value is a round or concurrency cap, not a
token limit.

| Key | Type | Default | Meaning |
|---|---|---|---|
| `maxIterations` | integer | `10` | Completion → tool-call rounds in one ordinary turn. |
| `planFanoutConcurrency` | integer | `4` | Stage sub-agents planning at the same time during a fan-out. |
| `stageMaxIterations` | integer | `20` | Round budget of one stage sub-agent. It has to cover both the research that raised a question and the planning that follows the answer, since the answer arrives mid-turn. |
| `planFanoutTimeoutMinutes` | integer | `60` | Budget of *working* time for a fan-out run. It does not run down while a stage waits on your answer. |
| `actionExecConcurrency` | integer | `1` | Action sub-agents executing at once. **Clamped to 1.** |
| `actionExecMaxIterations` | integer | `20` | Round budget of one action sub-agent. |
| `actionExecTimeoutMinutes` | integer | `30` | Deadline for one action run. |

A value of `0` or less means the default. A `actionExecConcurrency` above 1 is
accepted, logged as `agent.actionExecConcurrency is clamped to 1; one execution
runs at a time`, and then ignored — see [One execution at a
time](../explanation/one-execution-at-a-time.md).

## `log`

Mirrors everything written to stderr into a file, in append mode. stdout is
unchanged.

| Key | Type | Default | Meaning |
|---|---|---|---|
| `log.enabled` | boolean | `false` | Turn the file mirror on. |
| `log.file` | string | empty | Path of the mirror file. Required when `enabled` is true. |
| `log.level` | string | `info` | `debug`, `info`, `warn`, `error`, case-insensitive. |

- **Errors:** `config: log.file is required when log.enabled is true`;
  `config: invalid log.level "<value>" (allowed: debug, info, warn, error)`
- `LOG_LEVEL` overrides `log.level`.
- The parent directory must already exist. A missing file is created with mode
  `0644`.
- `debug` logs full tool-call arguments and responses.

## `metrics`

The Prometheus listener. It is a second HTTP server on its own port, never a
route on the API router.

| Key | Type | Default | Meaning |
|---|---|---|---|
| `metrics.enabled` | boolean | `true` | Serve metrics at all. |
| `metrics.host` | string | `0.0.0.0` | Listener bind address. |
| `metrics.port` | integer | `9090` | Listener port. |
| `metrics.path` | string | `/metrics` | Path the metrics are served on. |
| `metrics.refreshSeconds` | integer | `30` | How often plans-by-status and the database probe are recomputed. |

- **Errors:** `config: metrics.path must start with /`; `config: metrics.port
  must be between 1 and 65535`; `config: metrics.port <n> is already the API
  port; metrics need a listener of their own`; `config: metrics.refreshSeconds
  must be at least 1`.
- Every field also reads `METRICS_ENABLED`, `METRICS_HOST`, `METRICS_PORT`,
  `METRICS_PATH`, `METRICS_REFRESH_SECONDS`, and the environment wins.
- `metrics.enabled` is the one boolean that distinguishes "absent" from
  "false": omitting it means `true`.

Metric names: [Metrics](metrics.md).

## `knowledge_base`

| Key | Type | Default | Meaning |
|---|---|---|---|
| `knowledge_base.uri` | string | empty | Postgres DSN for the knowledge base. When set, it is also the DSN for everything else. |
| `knowledge_base.dir` | string | `{DATA_DIR}/knowledgebase` | Where the last uploaded document per collection is kept, as `{collection}.md`. |

With `uri` empty the DSN is built from the `DB_*` environment variables
instead. `GET /api/v1/knowledge/connection` reports which of the two is in use.

## Directory and file paths

Each of these is resolved relative to `DATA_DIR` unless it is absolute.

| Key | Default | Holds |
|---|---|---|
| `skills.dir` | `{DATA_DIR}/skills` | One `{name}.md` per skill. |
| `rules.dir` | `{DATA_DIR}/rules` | One file per rule. |
| `prompts.dir` | `{DATA_DIR}/prompts` | System-prompt overrides, one per mode. |
| `includedTools.dir` | `{DATA_DIR}/tools` | `{mode}.json` allow lists, `mcp-included.json`, `mcp-known.json`. |
| `mcp.file` | `{DATA_DIR}/mcp.json` | MCP server registrations. |
| `executor.file` | — | Executor settings file. |

Contents of each: [State](state.md).

## `projects`

A static tree of projects, each with environments and clouds, each cloud with
locations. It is served through the API like a database table, and editing a
project, environment, cloud or location through the UI rewrites this file.

```yaml
projects:
  - name: platform
    description: Shared platform services
    environments:
      - name: prod
        description: Production
    clouds:
      - name: aws
        locations:
          - eu-central-1          # a bare string is a name
          - name: eu-west-1       # or a mapping with a description
            description: Ireland
```

A singular `project:` block is also accepted. It is appended to `projects` when
it has a name that no entry in the array already uses.

Locations accept either form shown above: a plain scalar is taken as the name.

## Worked example

The file shipped for Docker Compose, at `deploy/compose/config.yaml`:

```yaml
llm:
  baseURL: https://api.openai.com/v1
  model: gpt-5.6
  embeddingModel: text-embedding-3-small
  api: responses
  reasoningEffort: medium
  timeoutSeconds: 300
  httpReferer: ""
  appTitle: ""

log:
  enabled: true
  file: /app/data/logs/llmchat.stderr.log
  level: info
```

## See also

- [Environment variables](environment-variables.md) — credentials, and the
  variables that override this file
- [Switch the LLM provider](../how-to/switch-llm-provider.md)
- [Executor settings](executor.md) — configured in the UI, not in this file
