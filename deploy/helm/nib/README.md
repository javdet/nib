# NIB Helm Chart

Helm chart for deploying [NIB (Neuro Infrastructure Builder)](https://github.com/javdet/nib) to Kubernetes.

This chart packages the services from `docker-compose.yml` using production container images instead of the dev-mode compose setup (`go run`, `bun dev`).

## Components

| Service    | Image                    | Port | Description                          |
|------------|--------------------------|------|--------------------------------------|
| backend    | `javdet/nib-backend`     | 8080 | Go API server                        |
| frontend   | `javdet/nib-frontend`    | 80   | nginx SPA (proxies `/api/` to backend) |
| kb-mcp     | `javdet/nib-kb`          | 8081 | Knowledge-base MCP server            |
| postgresql | `pgvector/pgvector:pg18` | 5432 | PostgreSQL with pgvector extension   |

## Prerequisites

- Kubernetes 1.21+
- Helm 3.8+
- A default StorageClass (for PVCs) unless you provide `storageClass` values
- `LLM_API_KEY` (or `OPENAI_API_KEY`) for LLM and knowledge-base features
- An API token for the UI and API: `openssl rand -hex 32`

## Quick start

```bash
# Install with bundled PostgreSQL
helm install nib ./deploy/helm/nib \
  --namespace nib --create-namespace \
  --set secrets.llmApiKey="$LLM_API_KEY" \
  --set secrets.apiToken="$(openssl rand -hex 32)"

# Port-forward the UI
kubectl port-forward svc/nib-frontend 8080:80 -n nib
# Open http://localhost:8080
```

## Configuration

### Secrets

By default the chart creates a Secret (`<release>-app`) with these keys:

| Key                      | Value source              | Purpose                        |
|--------------------------|---------------------------|--------------------------------|
| `DB_PASSWORD`            | `secrets.dbPassword`      | PostgreSQL password            |
| `LLM_API_KEY`            | `secrets.llmApiKey`       | LLM / OpenRouter API key       |
| `OPENAI_API_KEY`         | `secrets.openaiApiKey`    | Fallback LLM key               |
| `SECRETS_ENCRYPTION_KEY` | `secrets.secretsEncryptionKey` | Encrypt prompt secrets   |
| `EXECUTOR_*`             | `secrets.executor*`       | Agent-runner executor          |
| `NIB_API_TOKEN`          | `secrets.apiToken`        | API / UI login token (required)|
| `AGENT_WEBHOOK_TOKEN`    | `secrets.agentWebhookToken` | Agent-runner webhook auth    |
| `mcp.json`               | auto-generated            | MCP server bootstrap           |

To use an existing Secret instead:

```yaml
secrets:
  existingSecret: my-nib-secrets
```

The Secret must contain at minimum `DB_PASSWORD`, `LLM_API_KEY` and `NIB_API_TOKEN`.

### API authentication

Every `/api/v1` route except `/health`, `/version` and the agent-runner webhook
requires `NIB_API_TOKEN` (at least 32 characters, `openssl rand -hex 32`). The UI
asks for it once and keeps an HttpOnly session cookie; scripts send
`Authorization: Bearer <token>`. Rendering fails when `secrets.apiToken` is empty
and no `existingSecret` is set, and the backend refuses to start without a token.

```yaml
secrets:
  apiToken: "<openssl rand -hex 32>"
  agentWebhookToken: "<openssl rand -hex 32>"
backend:
  auth:
    allowedHosts: ["nib.example.com"]   # blocks DNS rebinding
    # insecureNoAuth: true              # no authentication at all
```

### External PostgreSQL

Disable the bundled database and point to an external instance with pgvector:

```yaml
postgresql:
  enabled: false

externalDatabase:
  host: postgres.example.com
  port: 5432
  username: nib
  password: "<password>"
  database: nib
  sslmode: require

secrets:
  dbPassword: "<password>"
```

### Ingress

The frontend nginx image proxies `/api/` to the backend, so a single Ingress host is usually sufficient:

```yaml
ingress:
  enabled: true
  className: nginx
  hosts:
    - host: nib.example.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: nib-tls
      hosts:
        - nib.example.com
```

Set `ingress.routeApiToBackend: true` to route `/api` directly to the backend Service (bypassing frontend nginx).

### Metrics

The backend serves Prometheus metrics on a separate port (9090 by default), so they stay off the
ingress -- which routes only `/api` to the backend. The endpoint is unauthenticated, like the rest
of the API.

```yaml
backend:
  metrics:
    enabled: true
    port: 9090
    path: /metrics
    serviceMonitor:
      enabled: true       # requires prometheus-operator
      interval: 30s
```

The ServiceMonitor is only rendered when the cluster actually has the `monitoring.coreos.com/v1`
API, so enabling it without prometheus-operator installed is a no-op rather than an error. For an
annotation-based scraper instead:

```yaml
backend:
  podAnnotations:
    prometheus.io/scrape: "true"
    prometheus.io/port: "9090"
    prometheus.io/path: "/metrics"
```

Metric names and queries: [docs/metrics.md](../../../docs/metrics.md).

### Backend data volume

Runtime state is stored at `/app/data` on a PVC:

- Written at runtime: `dags/`, `summaries/`, `action_plans/`, `plan_state/`, `attachments/`, `logs/`, `mcp.json`, `executor.json`
- Prompts self-seed on first start; rules/skills are managed via the UI

Optional: seed prompts, tools, rules, and skills from a ConfigMap:

```yaml
backend:
  dataSeed:
    configMapName: nib-data-seed
```

### MCP server bootstrap

When `kbMcp.enabled` is true and `backend.mcpServers` is empty, the chart bootstraps `mcp.json` with a `knowledge-base` entry pointing at the in-cluster kb-mcp Service. The file is copied only when absent, so UI edits persist.

Custom MCP servers:

```yaml
backend:
  mcpServers:
    knowledge-base:
      url: http://nib-kb-mcp:8081/mcp
    context7:
      url: http://mcp-context7:8090/mcp
      transport: http
```

### Tokens in mcp.json

Any value in a server entry may reference a secret as `${NAME}`, so credentials
never have to be written into `mcp.json`:

```yaml
backend:
  mcpServers:
    github:
      url: https://api.githubcopilot.com/mcp/
      headers:
        Authorization: Bearer ${MCP_GITHUB_TOKEN}
```

`${NAME}` is expanded only when the backend contacts the server, and the one
place it is looked up is the encrypted secret store: an encrypted secret named
`MCP_GITHUB_TOKEN` (Variables → Secrets in the UI, or `POST /api/v1/secrets`),
which requires `SECRETS_ENCRYPTION_KEY`. The pod's own environment variables are
not a source — otherwise an `mcp.json` edit could name `LLM_API_KEY` or a
database password and send it to an arbitrary URL.

With no secret of that name, that one server fails with an error naming the
missing reference; the other MCP servers keep working. Write `$${` for a literal
`${`.

Store credentials in headers rather than in the URL: transport errors embed the
request URL, and while expanded values are stripped from the backend's own log
lines, an upstream server may echo the URL back.

### Executor / Docker socket

`docker-compose.yml` mounts `/var/run/docker.sock` for the `local` executor (`run_executor` tool). In Kubernetes this is **disabled by default** because:

- Mounting the host Docker socket is a security risk
- The `kubernetes` executor type is not yet implemented in the backend

To enable (not recommended in production):

```yaml
backend:
  dockerSocket:
    enabled: true
```

## Image versions

Backend and frontend image tags default to `Chart.appVersion` (matches the root `VERSION` file, e.g. `v0.7.5`).

```yaml
backend:
  image:
    repository: javdet/nib-backend
    tag: v0.7.5

frontend:
  image:
    repository: javdet/nib-frontend
    tag: v0.7.5
```

Pin the kb-mcp image explicitly:

```yaml
kbMcp:
  image:
    repository: javdet/nib-kb
    tag: "<tag>"
```

## Upgrading

```bash
helm upgrade nib ./deploy/helm/nib \
  --namespace nib \
  --set secrets.llmApiKey="$LLM_API_KEY" \
  --set secrets.apiToken="$NIB_API_TOKEN"
```

An upgrade from a release before API authentication needs `secrets.apiToken`
(or `NIB_API_TOKEN` in the existing Secret); without it the render fails.

Database migrations run automatically in-process on backend startup.

## Uninstall

```bash
helm uninstall nib -n nib
```

PVCs are not deleted automatically. Remove them manually if needed:

```bash
kubectl delete pvc -l app.kubernetes.io/instance=nib -n nib
```

## Development

```bash
# Lint
helm lint deploy/helm/nib --set secrets.apiToken=$(openssl rand -hex 32)

# Render templates
helm template nib deploy/helm/nib --set secrets.apiToken=$(openssl rand -hex 32)

# External database + ingress
helm template nib deploy/helm/nib \
  --set secrets.apiToken=$(openssl rand -hex 32) \
  --set postgresql.enabled=false \
  --set externalDatabase.host=postgres.example.com \
  --set ingress.enabled=true
```
