# How to run nib with Docker Compose

Gets a working install on one host from the published images: the web UI, the
backend, the knowledge-base MCP server and a Postgres with `pgvector`.

You need Docker with the Compose plugin, and an API key for an
OpenAI-compatible LLM endpoint.

## Start the stack

```bash
git clone https://github.com/javdet/nib.git
cd nib
make setup
```

`make setup` asks for:

- the LLM provider, one of the [presets](switch-llm-provider.md#provider-presets),
  the model (Enter keeps the preset's) and its API key. For DeepSeek, xAI and
  Kimi it also asks for an OpenAI key, since they serve no `/embeddings` route
- the first project's name (required), its environments (comma-separated), a
  cloud and that cloud's region. The last three are optional

It checks the three published ports (`NIB_HTTP_PORT`, `NIB_BACKEND_PORT`,
`NIB_METRICS_PORT`) before writing anything. When one is in use it asks for
another, suggesting the next free one. Ports held by nib's own running stack
count as free, since compose replaces those containers in place.

It installs OpenSSL if it is missing and generates `NIB_API_TOKEN`,
`SECRETS_ENCRYPTION_KEY` and a Postgres password. It then writes `.env` and
`deploy/compose/config.local.yaml` (selected through `NIB_CONFIG_FILE`), runs
`docker compose up -d --wait` and prints the URL and API token.

Running it again backs `.env` up to `.env.bak.*` and rewrites both files, but
keeps the tokens and passwords already in `.env`. A new encryption key would make
every stored secret unreadable, and a new Postgres password would not open the
existing volume. If the embedding model changes, knowledge search still needs the
[rename](switch-llm-provider.md#rename-the-embedding-model-in-the-database).

To set it up by hand instead:

```bash
cp .env.example .env
```

Set `LLM_API_KEY` and `NIB_API_TOKEN` in `.env`. Nothing else is required to start.
Generate the token with:

```bash
openssl rand -hex 32
```

```bash
docker compose up -d
```

The UI is on <http://localhost:8080>. It asks for the API token once, then keeps
a session cookie for 30 days.

Four containers come up. If one restarts in a loop, `docker compose logs
backend` names the reason. These stop startup outright:

- a missing `LLM_API_KEY` (and `OPENAI_API_KEY`)
- a missing or short `NIB_API_TOKEN`
- a `metrics.port` colliding with the API port
- a `SECRETS_ENCRYPTION_KEY` that is not base64 of 32 bytes
- a `{DATA_DIR}/.webhook-key` that exists but is empty or unreadable — the
  backend generates this key itself on first start and will not replace one it
  cannot read

`NIB_INSECURE_NO_AUTH=true` starts without a token and leaves the API open to
anything that can reach it. Keep it for a machine only you can reach.

The backend runs as the unprivileged user `nib` (uid/gid 10001). Its entrypoint
starts as root only long enough to prepare `nibdata`, hand anything in it not
owned by `nib` over to it, and join the group that owns the mounted Docker
socket. On Docker Desktop that group is `root`, and the log says so with a
warning; it grants no capabilities.

## Choose which ports reach the host

Defaults in `.env`:

| Variable | Default | Reaches |
|---|---|---|
| `NIB_HTTP_PORT` | `8080` | The web UI. This is the one you need. |
| `NIB_BACKEND_PORT` | `8081` | The backend API directly, bypassing the SPA's nginx. |
| `NIB_METRICS_PORT` | `9090` | The Prometheus listener. |

The metrics endpoint is unauthenticated and its metric names alone report plan
counts, the model in use and cumulative LLM cost. If nothing on the host
scrapes it, delete its line from the backend's `ports` in `docker-compose.yml`
so it stays inside the Compose network. Commenting `NIB_METRICS_PORT` out of
`.env` is not enough: the mapping falls back to `9090`. `NIB_BACKEND_PORT` is
guarded by `NIB_API_TOKEN` (scripts send `Authorization: Bearer <token>`), but
if nothing uses it directly, remove that mapping the same way — it falls back
to `8081`.

## Reach it by a hostname

Once the UI is reached by a name rather than `localhost`, set that name in
`.env`:

```bash
NIB_ALLOWED_HOSTS=nib.example.com
```

The API then refuses any other `Host` header with `421`, which blocks DNS
rebinding. Health probes and the agent-runner webhook are exempt. List bare
names, comma-separated, and include `localhost` if you still open it that way:
the SPA's nginx passes `Host` on without its port, and a bare name matches any
port.

Writes from another origin are refused with `403` whatever the auth mode. The
SPA served on `NIB_HTTP_PORT` is same-origin and needs nothing. Only a
browser front end on a different origin needs listing in
`NIB_ALLOWED_ORIGINS`, as `scheme://host[:port]`, comma-separated.

## Enable encrypted secrets

Without a key, secrets can be neither stored nor read back: you cannot keep a
token, which means you cannot reference one from `mcp.json` or select one for
the executor.

```bash
openssl rand -base64 32
```

Put the result in `SECRETS_ENCRYPTION_KEY` in `.env` and restart:

```bash
docker compose up -d
```

Keep that key with your other credentials. Losing it makes every stored secret
unreadable; changing it has the same effect.

## Point it at a different model

The LLM settings live in `deploy/compose/config.yaml`, which is mounted over the
image's own `/app/config.yaml`. Edit the `llm` block there and restart the
backend:

```bash
docker compose restart backend
```

Remember that the key stays in `.env` — never in the YAML. Switching provider
family is more than an edit to `baseURL`: see [How to switch the LLM
provider](switch-llm-provider.md).

## Use an existing Postgres

The bundled `postgres` service is there so a first install needs nothing else.
To use your own instead, it must have the `pgvector` extension available.

Set `knowledge_base.uri` in `deploy/compose/config.yaml` to the DSN. That key,
when present, is the DSN for everything — not just the knowledge base — and the
`DB_*` variables are then ignored. Then drop the bundled service by scaling it
away:

```bash
docker compose up -d --scale postgres=0
```

## Keep your data across restarts

Two named volumes hold everything that must survive:

| Volume | Holds |
|---|---|
| `pgdata` | Postgres: transcripts, variables, secrets, the tool catalog, the knowledge base. |
| `nibdata` | `DATA_DIR`: `mcp.json`, skills, rules, prompt overrides, tool allow lists, plan artifacts. |

`docker compose down` keeps both. `docker compose down -v` deletes them, and
with them every plan, secret and skill on the install.

Back both up together — a `nibdata` restored beside a different `pgdata` leaves
plan artifacts pointing at dialogs that no longer exist.

## Run from source instead

For live reload while changing nib itself:

```bash
docker compose -f docker-compose.dev.yml up
```

The UI is then on <http://localhost:5173>. This stack builds from the working
tree and uses its own container names and volumes, so it does not disturb a
production stack on the same host.

## Next

- [How to connect an MCP server](connect-an-mcp-server.md) — give the agent tools
- [How to run planned actions in containers](run-actions-in-containers.md) — let it execute `code` steps
- [How to upgrade an install](upgrade.md) — what an older install needs before it starts
- [Environment variables](../reference/environment-variables.md) — everything `.env` accepts
