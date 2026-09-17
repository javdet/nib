# How to scrape nib's metrics with Prometheus

The backend exports Prometheus metrics on a listener of its own — port `9090`,
path `/metrics` by default. It is a separate HTTP server, not a route on the
API, so both the Helm ingress and the frontend nginx (which proxy only `/api`)
leave it unreachable from the public host while an in-cluster scraper still
gets it.

The endpoint is **unauthenticated**. The metric names alone report how many
plans exist, which model is configured and cumulative LLM cost. Treat reaching
it as reaching your topology.

Metric names and meanings: [Metrics](../reference/metrics.md).

## Configure the listener

```yaml
metrics:
  enabled: true
  host: "0.0.0.0"
  port: 9090
  path: /metrics
  refreshSeconds: 30
```

`METRICS_ENABLED`, `METRICS_HOST`, `METRICS_PORT`, `METRICS_PATH` and
`METRICS_REFRESH_SECONDS` override the YAML — the reverse of every other
config block. That is deliberate: the Dockerfile and the Helm deployment both
pass `METRICS_*`, and a `config.yaml` baked into the image or left on a data
volume must not override what the deployment asked for.

The port cannot equal `SERVER_PORT`. The backend refuses to start if it does.

## Scrape it in Kubernetes

With prometheus-operator:

```yaml
backend:
  metrics:
    enabled: true
    port: 9090
    path: /metrics
    service:
      enabled: true          # adds the metrics port to the Service
    serviceMonitor:
      enabled: true
      interval: 30s
```

The `ServiceMonitor` renders only when the cluster has the
`monitoring.coreos.com/v1` API, so on a cluster without the operator the
template quietly creates nothing — which is why enabling it and seeing no
target usually means the operator is missing, not the values.

Without the operator, annotate the pod instead:

```yaml
backend:
  podAnnotations:
    prometheus.io/scrape: "true"
    prometheus.io/port: "9090"
    prometheus.io/path: "/metrics"
```

## Scrape a Compose install

Uncomment `NIB_METRICS_PORT` in `.env` to publish the port to the host, and
point your scraper at `http://<host>:9090/metrics`.

If the scraper runs in the same Compose network, do not publish the port at
all — scrape `backend:9090` directly and leave it off the host.

## Alerts worth having

| What you are checking | Query |
|---|---|
| The API is returning errors | `sum(rate(nib_http_requests_total{code=~"5.."}[5m])) > 0` |
| API latency | `histogram_quantile(0.95, sum by (le,route) (rate(nib_http_request_duration_seconds_bucket{route!="/api/v1/dialogs/{id}/events"}[5m])))` |
| The database is unreachable | `nib_db_up == 0` |
| The connection pool is running out | `nib_db_pool_connections{state="acquired"} / nib_db_pool_max_connections > 0.9` |
| The LLM provider is degrading | `sum(rate(nib_llm_requests_total{outcome="error"}[5m])) / sum(rate(nib_llm_requests_total[5m])) > 0.1` |
| The agent is hitting its round limit | `increase(nib_agent_turns_total{outcome="max_iterations"}[30m]) > 0` |
| An MCP server is broken | `topk(5, sum by (server) (rate(nib_mcp_tool_discovery_errors_total[15m])))` |
| Tools are failing | `sum by (source) (rate(nib_agent_tool_calls_total{outcome="error"}[15m]))` |
| The lease is blocking operators | `increase(nib_execution_lease_rejections_total[15m]) > 3` |
| The UI has diverged from the server | `increase(nib_sse_events_dropped_total[10m]) > 0` |
| The process died mid-run | `increase(nib_stuck_runs_reconciled_total[1h]) > 0` |
| The plan gauges are stale | `time() - nib_plans_refresh_timestamp_seconds > 300` |
| Container-agent cost over a day | `increase(nib_agent_runner_cost_usd_total[24h])` |
| The backend is down | `up{job="nib-backend"} == 0` |

Exclude `/api/v1/dialogs/{id}/events` from latency percentiles, as the first
query does. It is an SSE stream, so its "duration" is how long a browser tab
stayed open.

## If the plan gauges go stale

`nib_plans` is refreshed by a background goroutine every `refreshSeconds`, not
at scrape time: plan status lives in `data/plan_state/{id}.json`, so counting is
one query plus one file read per plan.

Watch `nib_plans_refresh_duration_seconds` if you accumulate a lot of plans. The
real fix in that case is a status column in `chat_dialogs`, which does not exist
yet — raising `refreshSeconds` is the available lever.

## Use the Statistics page instead for cost

Prometheus answers "what is happening now" and retains only what your
Prometheus is configured to retain. For "what did this task cost over a month",
use the Statistics page, backed by Postgres tables kept indefinitely. Two
traps in reading those numbers — `NULL` costs and overlapping token counts —
are in [Metrics → Usage tables](../reference/metrics.md#usage-tables).

## See also

- [Metrics](../reference/metrics.md)
- [Configuration file](../reference/configuration.md#metrics)
