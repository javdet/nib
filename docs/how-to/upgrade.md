# How to upgrade an install

An upgrade replaces the binary and everything compiled into it. It does not
overwrite anything already on your data volume.

Check [`CHANGELOG.md`](../../CHANGELOG.md) before starting: entries marked
**Breaking** are the ones that need a step from you, and they are rare enough
that skipping the check is what usually causes trouble.

## Upgrade a Compose install

```bash
cd nib
git pull
```

Set the new tag in `.env` — `NIB_VERSION` and `NIB_KB_VERSION` both, they share
a tag — then:

```bash
docker compose pull
docker compose up -d
```

Migrations are applied by the backend itself at startup, under a Postgres
advisory lock, so nothing needs running by hand.

Back up `pgdata` and `nibdata` together first. Restoring one against a
mismatched copy of the other leaves plan artifacts pointing at dialogs that no
longer exist.

## Upgrading from v0.8.0

The release after v0.8.0 changes how nib authenticates, which user it runs as
and where a secret may be sent. Go through this list before the first start,
not after.

1. **Set `NIB_API_TOKEN`.** The backend now refuses to start without a token of
   at least 32 characters (`openssl rand -hex 32`), unless
   `NIB_INSECURE_NO_AUTH=true`. `git pull` brings the `docker-compose.yml` that
   passes it through from `.env`. The UI asks for the token once; scripts send
   `Authorization: Bearer <token>`. Under Helm, set `secrets.apiToken`, or the
   render fails.
2. **Let running actions finish.** The agent-runner webhook now takes a token
   signed for each run, not `AGENT_WEBHOOK_TOKEN` itself, so a container
   launched by v0.8.0 cannot report back to the new backend. `ReconcileStuckRuns`
   closes its record at boot, but the result is lost. `AGENT_WEBHOOK_TOKEN` is
   now an optional signing key: leave it as it is, or empty to have the backend
   generate `{DATA_DIR}/.webhook-key` — and keep that file with the volume.
   An empty `AGENT_WEBHOOK_TOKEN` no longer opens the webhook.
3. **Remove `WEBHOOK_AUTH_HEADER` from the Kubernetes agent Secret**
   (`agentSecretName`). The backend sets it on every Job and overrides it
   anyway.
4. **Check your secrets' allowed hosts.** A `${NAME}` in `mcp.json` is now
   resolved only for a server whose host is on that secret's allowed-hosts
   list. At the first start, every secret the current `mcp.json` references is
   bound once to the hosts referencing it; nothing is bound after that. A
   server you add later, or a secret the file did not reference at upgrade,
   fails with `secret is not allowed for this host` until you add the host at
   **Variables → Secrets** — which means entering the value again. See [How to
   keep tokens out of `mcp.json`](keep-tokens-out-of-mcp-json.md).
5. **Replace OAuth MCP connections.** The OAuth flow is gone, and with it the
   `ATLASSIAN_CLIENT_ID`, `ATLASSIAN_CLIENT_SECRET`, `ATLASSIAN_MCP_URL`,
   `OAUTH_CALLBACK_BASE_URL` and `FRONTEND_BASE_URL` variables. An existing
   OAuth connection keeps its access token until it expires; nothing refreshes
   it. Re-add the server with a static token.
6. **Expect the backend to run as `nib` (uid/gid 10001)**, not root. On start,
   the entrypoint hands every file in `DATA_DIR` not owned by `nib` over to it,
   so the first start on a large volume takes longer and a bind-mounted data
   directory on the host changes owner. The mounted `config.yaml` has to be
   readable by uid 10001. A pod started as another user through a
   `securityContext` skips the entrypoint's root step entirely: give the
   volume an `fsGroup` or `chown` it yourself, or the backend cannot write it.
7. **Expect narrower agent tools.** `execute_command` and `api_call` now run as
   `nib` and inherit only `PATH`, `HOME` and `LANG` — no proxies, no
   `KUBECONFIG`, none of the backend's keys. `api_call` also refuses loopback
   and link-local hosts (cloud metadata included), reading from or writing to
   files, and following redirects. A rule or skill that relied on any of these
   needs rewriting.

Also new, but needing nothing from you: the `llm.embeddings` block (omitting it
keeps one provider for both), local executor image pulls, container logs,
**Execute all** on a stage, and code fixes requested from the chat.

## Upgrade a Helm install

Follow the Upgrading section of
[`deploy/helm/nib/README.md`](../../deploy/helm/nib/README.md), which is kept
with the chart.

## What changes, and what does not

| | On upgrade |
|---|---|
| Binary, embedded prompts, built-in skills, built-in tool allow lists | Replaced by the new image. |
| Your prompt override (`prompts/discuss.md`) | Kept, verbatim. It does **not** inherit later changes to the shipped default. |
| Skills you edited, renamed or deleted | Kept as you left them. |
| New built-in skills | Seeded onto a volume that has never been seeded. |
| `mcp.json`, rules, executor config, plan artifacts | Kept. |
| `.webhook-key` (the generated webhook signing key) | Kept. Losing it orphans every agent container still running. |
| Ownership of files in `DATA_DIR` | Handed to `nib` (uid/gid 10001) on every start, when the container starts as root. |
| Tool allow lists | Kept. New built-in tool names are merged in once each; **no name is ever removed**. |
| Dialogs, variables, secrets, MCP connections, knowledge base | Migrated in place at startup. |

Two consequences:

- A **prompt override never updates itself.** If you overrode the discuss
  prompt several releases ago, you are still running that text. Delete the
  override to take the current default, then re-apply your changes.
- A capability **withdrawn** from a mode does not disappear from your allow
  list, because seeding only ever adds. The backend enforces such withdrawals
  in code, so the stale entry is harmless — but the file will not match a fresh
  install's.

## Verify

1. The sidebar shows the new version, as does `GET /api/v1/version`, and the UI
   asks for the API token.
2. `docker compose logs backend` shows the migrations that ran. A warning that
   an already-applied migration file was edited means exactly that, and is
   worth reading.
3. Open a plan and check the action list renders.

## Roll back

Put the previous tag in `.env` and `docker compose up -d`.

Migrations are not reversed by this. An older binary against a newer schema is
only safe when nothing in between changed a table it reads — check the
changelog entries you are stepping back over, and restore the `pgdata` backup
if they did.

## See also

- [State: what an upgrade keeps](../reference/state.md#what-an-upgrade-keeps)
- [`CHANGELOG.md`](../../CHANGELOG.md)
