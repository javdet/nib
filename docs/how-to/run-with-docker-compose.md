# How to run nib with Docker Compose

Gets a working install on one host from the published images: the web UI, the
backend, the knowledge-base MCP server and a Postgres with `pgvector`.

You need Docker with the Compose plugin, and an API key for an
OpenAI-compatible LLM endpoint.

## Start the stack

```bash
git clone https://github.com/javdet/nib.git
cd nib
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
backend` names the reason. Three things stop startup outright:

- a bad `LLM_API_KEY`
- a missing or short `NIB_API_TOKEN`
- a `metrics.port` colliding with the API port

`NIB_INSECURE_NO_AUTH=true` starts without a token and leaves the API open to
anything that can reach it. Keep it for a machine only you can reach.

## Choose which ports reach the host

Defaults in `.env`:

| Variable | Default | Reaches |
|---|---|---|
| `NIB_HTTP_PORT` | `8080` | The web UI. This is the one you need. |
| `NIB_BACKEND_PORT` | `8081` | The backend API directly, bypassing the SPA's nginx. |
| `NIB_METRICS_PORT` | `9090` | The Prometheus listener. |

The metrics endpoint is unauthenticated and its metric names alone report plan
counts, the model in use and cumulative LLM cost. If nothing on the host
scrapes it, comment `NIB_METRICS_PORT` out so it stays inside the Compose
network. `NIB_BACKEND_PORT` is guarded by `NIB_API_TOKEN` (scripts send
`Authorization: Bearer <token>`), but if nothing uses it directly, comment it out
too.

## Enable encrypted secrets

Without a key, nib runs read-only for secrets: you cannot store a token, which
means you cannot reference one from `mcp.json` or select one for the executor.

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
- [Environment variables](../reference/environment-variables.md) — everything `.env` accepts
