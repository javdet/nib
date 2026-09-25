# Command-line interface

Three binaries are built from this repository. Each ships in its own image.

| Binary | Image | Role |
|---|---|---|
| `nib` | `javdet/nib-backend` | The API server and agent loop. |
| `kb` | `javdet/nib-kb` | Knowledge-base ingestion and the `knowledge_search` MCP server. |
| `toolcatalog` | — | Re-indexes the MCP tool catalog. Built from source. |

## `nib`

```
nib [-config <path>]
```

**Flags**

| Flag | Default | Meaning |
|---|---|---|
| `-config` | `config.yaml` | Path to the YAML configuration file. |

With the flag absent, `CONFIG_FILE` is used; with that absent too, `config.yaml`
relative to the working directory.

**Behaviour at startup**

1. Loads and validates the configuration; any validation error is fatal. Refuses
   to start without `NIB_API_TOKEN` unless `NIB_INSECURE_NO_AUTH=true`.
2. Applies `*.up.sql` migrations from `MIGRATIONS_PATH` (default `migrations`),
   under a Postgres advisory lock, recording each in `schema_migrations`.
3. Reconciles the per-mode tool allow lists onto the data volume, and loads or
   generates the webhook signing key (`{DATA_DIR}/.webhook-key`) — fatal when it
   can be neither read nor created.
4. Seeds built-in skills, and reconciles the detected host environment into the
   `host*` prompt variables.
5. Closes executions, fan-outs, code fixes and stage runs a previous process
   left `running`.
6. Serves the API on `SERVER_HOST:SERVER_PORT` and, unless disabled, metrics on
   a second listener.

**Exit codes:** `0` on clean shutdown, `1` on any startup error.

## `kb`

```
kb ingest [flags] [files...]
echo text | kb ingest [flags]
kb serve-mcp [flags]
```

An unknown sub-command, or none, exits `2`.

### `kb ingest`

Chunks the input, embeds each chunk and writes it to pgvector. With no file
arguments it reads standard input.

| Flag | Default | Meaning |
|---|---|---|
| `-collection` | — | Collection name. **Required.** |
| `-model` | — | Embedding model id. **Required.** |
| `-config` | — | YAML config to read `knowledge_base.uri` from. |
| `-dsn` | — | Postgres DSN. Overrides `DATABASE_URL` and `-config`. |
| `-provider` | `openrouter` | `openrouter` (any OpenAI-compatible `/embeddings`) or `google`. |
| `-chunk-size` | package default | Maximum runes per chunk. |
| `-chunk-overlap` | package default | Rune overlap between consecutive chunks. |
| `-embed-batch` | `32` | Maximum texts per embedding request. |
| `-insert-batch` | `256` | Maximum rows per database insert. |
| `-metric` | `cosine` | Distance metric label stored on the collection. |
| `-replace` | `false` | Delete existing chunks for the same `source_uri` before inserting. |
| `-timeout` | `120s` | HTTP timeout for embedding requests. |
| `-embeddings-base-url` | `$EMBEDDINGS_BASE_URL` | OpenAI-compatible API base URL. |
| `-dimensions` | `$EMBEDDINGS_DIMENSIONS`, else `0` | Embedding width to request; `0` omits the parameter. Must match what the collection was ingested with. |
| `-openrouter-base-url` | — | Deprecated alias for `-embeddings-base-url`. |
| `-google-base-url` | `$GOOGLE_API_BASE_URL` | Generative Language API base. |
| `-http-referer` | `$HTTP_REFERER` | OpenRouter attribution header. |
| `-app-title` | `$OPENROUTER_APP_TITLE` | OpenRouter attribution header. |

A DSN must come from `-dsn`, `DATABASE_URL`, or `-config` with
`knowledge_base.uri`; with none of the three the command fails with `set -dsn,
environment DATABASE_URL, or -config with knowledge_base.uri`.

### `kb serve-mcp`

Serves one MCP tool, `knowledge_search`: it embeds the query and runs a
pgvector cosine search.

| Flag | Default | Meaning |
|---|---|---|
| `-config` | — | YAML config to read `knowledge_base.uri` from. |
| `-dsn` | — | Postgres DSN. Overrides `DATABASE_URL` and `-config`. |
| `-provider` | `openrouter` | Must match how the collection was ingested. |
| `-model` | — | When set, every searched collection must use this `embedding_model`; otherwise the collection row supplies it. |
| `-timeout` | `120s` | HTTP timeout for embedding requests. |
| `-transport` | `stdio` | `stdio` or `http` (streamable HTTP). |
| `-http-addr` | `:8081` | Listen address when `-transport=http`. |
| `-http-path` | `/mcp` | Endpoint path when `-transport=http`. |
| `-http-stateless` | `false` | Run the streamable HTTP server without session tracking. |
| `-embeddings-base-url` | `$EMBEDDINGS_BASE_URL` | OpenAI-compatible API base URL. |
| `-dimensions` | `$EMBEDDINGS_DIMENSIONS`, else `0` | Embedding width to request; `0` omits the parameter. Must match what the collection was ingested with. |
| `-openrouter-base-url` | — | Deprecated alias for `-embeddings-base-url`. |
| `-google-base-url` | `$GOOGLE_API_BASE_URL` | Generative Language API base. |
| `-http-referer` | `$HTTP_REFERER` | OpenRouter attribution header. |
| `-app-title` | `$OPENROUTER_APP_TITLE` | OpenRouter attribution header. |

The `kb-mcp` service in Docker Compose runs this sub-command with
`-transport=http` on port 8081. The backend registers it like any other MCP
server — by URL.

`stdio` works for a local shell, never for the backend: nib only speaks
streamable HTTP to MCP servers. See
[Connect an MCP server](../how-to/connect-an-mcp-server.md).

## `toolcatalog`

```
toolcatalog reindex [flags]
```

Discovers every tool the registered MCP servers expose and rewrites the tool
catalog, including its embeddings. The backend does this itself; the binary is
for doing it out of band.

| Flag | Default | Meaning |
|---|---|---|
| `-config` | — | YAML config supplying the default `mcp.json` path, the DSN and the HTTP timeout. |
| `-mcp-json` | `{DATA_DIR}/mcp.json` from `-config` | Path to the Cursor-style `mcp.json`. |
| `-dsn` | — | Postgres DSN. Overrides `DATABASE_URL`, `knowledge_base.uri` and the `DB_*` variables. |
| `-timeout` | `llm.timeoutSeconds`, else `60s` | HTTP timeout for MCP discovery. |

An unknown sub-command, or none, exits `2`.

## See also

- [Environment variables](environment-variables.md)
- [Configuration file](configuration.md)
