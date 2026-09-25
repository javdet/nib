# Environment variables

Every variable the three binaries read, grouped by what it configures. Secrets
live here and nowhere else — they are never read from
[the configuration file](configuration.md).

Where a variable overlaps a YAML key, the **Precedence** column says which one
the backend uses when both are set.

## Backend — LLM provider

| Variable | Default | Precedence | Meaning |
|---|---|---|---|
| `LLM_API_KEY` | — | env only | API key for the completion endpoint, and for embeddings unless `LLM_EMBEDDINGS_API_KEY` is set. **Required.** |
| `OPENAI_API_KEY` | — | env only | Used when `LLM_API_KEY` is empty. |
| `LLM_BASE_URL` | — | YAML wins | Base URL of the OpenAI-compatible API. |
| `LLM_MODEL` | `gpt-4o-mini` | YAML wins | Model id. |
| `OPENAI_MODEL` | — | YAML wins | Used when `LLM_MODEL` is empty. |
| `OPENAI_EMBEDDING_MODEL` | `text-embedding-3-small` | YAML wins | Embedding model. |
| `LLM_API` | `chat` | YAML wins | `chat` or `responses`. |
| `LLM_REASONING_EFFORT` | empty | YAML wins | `none`, `minimal`, `low`, `medium`, `high`, `xhigh`. An explicit `reasoningEffort: ""` in YAML clears it; an absent key defers to it. |
| `LLM_EMBEDDINGS_BASE_URL` | falls back to `LLM_BASE_URL` | YAML wins | Base URL of the `/embeddings` endpoint, when it is not the completion host. |
| `LLM_EMBEDDINGS_API_KEY` | falls back to `LLM_API_KEY` | env only | API key for the embeddings endpoint. |
| `LLM_EMBEDDINGS_MODEL` | falls back to `OPENAI_EMBEDDING_MODEL` | YAML wins | Embedding model. Overwrites `llm.embeddingModel` when set. |
| `LLM_EMBEDDINGS_DIMENSIONS` | `0` (omitted) | YAML wins | Embedding width to request. Must be 1536 for the knowledge base. |
| `HTTP_REFERER` | empty | YAML wins | OpenRouter `HTTP-Referer` header. |
| `OPENROUTER_APP_TITLE` | empty | YAML wins | OpenRouter `X-Title` header. |

Startup fails with `config: llm API key is required (set LLM_API_KEY or
OPENAI_API_KEY)` when neither key is set.

## Backend — server and database

| Variable | Default | Meaning |
|---|---|---|
| `SERVER_HOST` | `0.0.0.0` | API listener bind address. |
| `SERVER_PORT` | `8080` | API listener port. |
| `DB_HOST` | `localhost` | Postgres host. |
| `DB_PORT` | `5432` | Postgres port. |
| `DB_USER` | `nib` | Postgres user. |
| `DB_PASSWORD` | `nib` | Postgres password. |
| `DB_NAME` | `nib` | Database name. |
| `DB_SSLMODE` | `disable` | `sslmode` in the DSN. |
| `MIGRATIONS_PATH` | built-in | Directory of `*.up.sql` migrations to apply at startup. |

The `DB_*` variables are used only when `knowledge_base.uri` is absent from the
config file. When that key is set it supplies the DSN for everything.

## Backend — API authentication

| Variable | Default | Meaning |
|---|---|---|
| `NIB_API_TOKEN` | empty | Token guarding every `/api/v1` route except `/health`, `/version` and the agent-runner webhook. At least 32 characters. |
| `NIB_INSECURE_NO_AUTH` | `false` | `true` runs with no authentication at all. Ignored while `NIB_API_TOKEN` is set. |
| `NIB_ALLOWED_ORIGINS` | `http://localhost:5173` | Comma-separated `scheme://host[:port]` origins allowed to make cross-origin writes, besides the API's own origin. Also the CORS allow-list. |
| `NIB_ALLOWED_HOSTS` | empty | Comma-separated `host` or `host:port` values the API answers to. Empty serves any `Host`. |

The server refuses to start with `config: NIB_API_TOKEN is required` when the
token is empty and `NIB_INSECURE_NO_AUTH` is not `true`. Other startup checks:

- token shorter than 32 characters → `config: NIB_API_TOKEN must be at least 32 characters`
- `NIB_INSECURE_NO_AUTH` not a boolean → `config: NIB_INSECURE_NO_AUTH must be true or false`. A typo
  is an error rather than a silent fallback.
- an origin with a path or no scheme → `config: NIB_ALLOWED_ORIGINS entry ... must be scheme://host[:port]`

Generate a token with `openssl rand -hex 32`. The web UI asks for it once and
exchanges it for an HttpOnly session cookie. See [HTTP API](http-api.md#authentication).

Set `NIB_ALLOWED_HOSTS` to the names the UI is reached by, to block DNS
rebinding. It matters most with `NIB_INSECURE_NO_AUTH`, where the cookie is not
there to stop a rebound page.

## Backend — secrets and storage

| Variable | Default | Meaning |
|---|---|---|
| `SECRETS_ENCRYPTION_KEY` | empty | Base64 of exactly 32 bytes. Enables writing encrypted secrets. |
| `DATA_DIR` | `data` | Root of every file-backed store. |
| `CONFIG_FILE` | `config.yaml` | Config file path, when `-config` is not passed. |
| `APP_VERSION` | build-time value | Version reported by `GET /api/v1/version` and shown in the sidebar. |

`SECRETS_ENCRYPTION_KEY` is validated at startup:

- not base64 → `config: SECRETS_ENCRYPTION_KEY must be base64-encoded`
- wrong length → `config: SECRETS_ENCRYPTION_KEY must decode to 32 bytes, got <n>`

Left empty, the install runs without secret *write* support; existing secrets
stay readable. Generate one with `openssl rand -base64 32`.

## Backend — metrics and logging

| Variable | Default | Precedence | Meaning |
|---|---|---|---|
| `METRICS_ENABLED` | `true` | **env wins** | Serve the metrics listener. |
| `METRICS_HOST` | `0.0.0.0` | **env wins** | Metrics bind address. |
| `METRICS_PORT` | `9090` | **env wins** | Metrics port. Must differ from `SERVER_PORT`. |
| `METRICS_PATH` | `/metrics` | **env wins** | Metrics path. Must start with `/`. |
| `METRICS_REFRESH_SECONDS` | `30` | **env wins** | Recompute interval for plan gauges and the database probe. |
| `LOG_LEVEL` | `info` | **env wins** | `debug`, `info`, `warn`, `error`. |

## Backend — executor

| Variable | Default | Meaning |
|---|---|---|
| `EXECUTOR_LLM_API_KEY` | empty | API key handed to the agent container. |
| `EXECUTOR_LLM_MODEL` | empty | Model the agent container runs. |
| `AGENT_WEBHOOK_TOKEN` | empty | Bearer token authenticating `POST /api/v1/agent-runner/webhook`. Empty rejects every callback unless `NIB_INSECURE_NO_AUTH=true`. |

The git API token is **not** an environment variable. It is selected by name in
Settings → Executor and read from the encrypted secret store — see
[Executor settings](executor.md).

## `kb` binary

The knowledge-base CLI and MCP server. These are read only when the matching
flag is absent.

| Variable | Used by | Meaning |
|---|---|---|
| `DATABASE_URL` | both sub-commands | Postgres DSN. Overridden by `-dsn`. |
| `EMBEDDINGS_BASE_URL` | `-provider openrouter` | Base URL of the OpenAI-compatible `/embeddings` endpoint. |
| `OPENROUTER_BASE_URL` | `-provider openrouter` | Used when `EMBEDDINGS_BASE_URL` is empty. |
| `OPENROUTER_API_KEY` | `-provider openrouter` | **Required** for that provider. |
| `HTTP_REFERER` | `-provider openrouter` | `HTTP-Referer` header. |
| `OPENROUTER_APP_TITLE` | `-provider openrouter` | `X-Title` header. |
| `GEMINI_API_KEY` | `-provider google` | **Required** for that provider, or `GOOGLE_API_KEY`. |
| `GOOGLE_API_KEY` | `-provider google` | Used when `GEMINI_API_KEY` is empty. |
| `GOOGLE_API_BASE_URL` | `-provider google` | Generative Language API base. |

In Docker Compose, `KB_EMBEDDINGS_BASE_URL` in `.env` is what sets
`EMBEDDINGS_BASE_URL` inside the `kb-mcp` container.

`-provider openrouter` is the CLI's name for a generic OpenAI-compatible
`/embeddings` client. It points wherever the base URL says, including the
OpenAI platform.

## `toolcatalog` binary

Reads `DATABASE_URL` for the DSN when `-dsn` is absent.

## Compose-only variables

These are read by `docker-compose.yml` itself, not by any binary. They appear
in `.env`.

| Variable | Default | Meaning |
|---|---|---|
| `NIB_VERSION` | — | Image tag for `nib-backend` and `nib-frontend`. |
| `NIB_KB_VERSION` | — | Image tag for `nib-kb`. |
| `NIB_HTTP_PORT` | `8080` | Host port for the web UI. |
| `NIB_BACKEND_PORT` | `8081` | Host port for the backend API. |
| `NIB_METRICS_PORT` | `9090` | Host port for the metrics listener. |
| `POSTGRES_USER` | `nib` | Bundled Postgres user. |
| `POSTGRES_PASSWORD` | `nib` | Bundled Postgres password. |
| `POSTGRES_DB` | `nib` | Bundled Postgres database. |
| `KB_EMBEDDINGS_BASE_URL` | — | Becomes `EMBEDDINGS_BASE_URL` in `kb-mcp`. |
| `KB_EMBEDDINGS_DIMENSIONS` | — | Becomes `EMBEDDINGS_DIMENSIONS` in `kb-mcp`. Must equal `LLM_EMBEDDINGS_DIMENSIONS`, or queries are embedded at a different width than the chunks they search and match nothing. |
| `API_PROXY_TARGET` | auto-detected | Dev compose only: where the Vite dev server proxies `/api`. |

`/metrics` is unauthenticated and reports plan counts, the model id and
cumulative LLM cost. Publishing `NIB_METRICS_PORT` to the host exposes that to
anything that can reach the host.

## Test-only variables

Read by tests, never by a running install.

| Variable | Meaning |
|---|---|
| `NIB_E2E_API_TOKEN` | The backend's `NIB_API_TOKEN`. The Playwright suite sends it as a bearer token on every request. |
| `NIB_LIVE_OPENAI` | Set to `1` to run the opt-in live provider test. |
| `NIB_MIGRATIONS_TEST_DSN` | DSN for the migration tests. |
| `NIB_TOOLCATALOG_TEST_DSN` | DSN for the tool-catalog tests. |

## See also

- [Configuration file](configuration.md)
- [Command-line interface](cli.md)
- [Run nib with Docker Compose](../how-to/run-with-docker-compose.md)
