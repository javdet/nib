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

## Upgrading from v0.8.2: run state moves into Postgres

Plan status, the per-row run, exec and notes records, stage runs, fan-out
records, sub-agent pauses, code fixes and incident runs move from files under
`DATA_DIR` into the `run_state` table. The execution lease becomes a row, locks
become Postgres advisory locks, and the SSE stream travels over
`LISTEN/NOTIFY`. Nothing needs doing by hand, but:

1. **Stop the old backend before the new one starts.** The first boot imports
   every `plan_state/`, `plan_fanout/`, `subagents/`, `code_fixes/`,
   `incident_runs/` and `action_plans/{id}.{runs,exec,notes,stagerun}.json` file
   and renames it `*.migrated`. An old process still running would keep writing
   files nothing reads any more. Compose and the chart's `Recreate` strategy
   already work this way.
2. **Let running actions finish first**, as for any upgrade: anything still
   `running` when the old process stops is closed by the new one's first sweep.
3. **Budget connections.** Each backend process opens a second pool of up to
   24 connections for locks and the event listener (`NIB_COORDINATION_POOL_SIZE`
   changes it). The default `max_connections` of 100 leaves plenty for one.
4. **Back up `pgdata` before the first start**: from now on it holds the run
   state too. The `*.migrated` files stay on the volume as a record and can be
   deleted once you are satisfied.
5. **A downgrade loses run state** recorded after the upgrade: an older release
   reads only the files.

## Upgrading from v0.8.2: `execute_command` becomes the command tools

`execute_command` is gone. Five tools replace it: `run_command`, `wait_job`,
`read_output`, `cancel_job` and `wait_until` (see
[Agent tools](../reference/agent-tools.md#command-tools)). Most installs need
nothing from you, but check these:

1. **Allow lists migrate themselves.** At the first start, a
   `{DATA_DIR}/tools/{mode}.json` that lists `execute_command` gets the five
   tools in its place. If you had removed `execute_command` from a mode, that
   mode does not get them. Incident Diggers get them in code.
2. **Commands now run as a sandbox user.** Each command runs as a uid of its
   own from 20000 up, in an empty home directory that is deleted afterwards.
   `DATA_DIR` is made `0700` at startup. A rule or skill that expected a command
   to read files under `DATA_DIR`, `/home/nib`, or a CLI login kept there no
   longer works.
3. **Kubernetes `securityContext`.** Isolation needs the entrypoint to start as
   root and keep `SETUID`, `SETGID` and `KILL`. If your values drop `ALL`
   capabilities, add those three back. If they set `runAsUser`, commands run as
   the backend's own user, as before, and the backend warns at boot. Set
   `commands.isolation: required` to refuse commands instead.
4. **Commands can be long.** `run_command` accepts `timeout_seconds` up to an
   hour, and the agent waits with `wait_job`. Every command is recorded in the
   new `command_runs` table (migration `000041`), and its output is kept in
   `{DATA_DIR}/command_logs` for `commands.logRetentionDays` (default 7).
5. **The image runs `tini` as PID 1.** A custom `ENTRYPOINT` override should
   keep it, or add `init: true` in Compose.

## Upgrading from v0.8.0

The release after v0.8.0 changes how nib authenticates and which user it runs
as. Go through this list before the first start,
not after.

1. **Set `NIB_API_TOKEN`.** The backend now refuses to start without a token of
   at least 32 characters (`openssl rand -hex 32`), unless
   `NIB_INSECURE_NO_AUTH=true`. `git pull` brings the `docker-compose.yml` that
   passes it through from `.env`. The UI asks for the token once; scripts send
   `Authorization: Bearer <token>`. Under Helm, set `secrets.apiToken`, or the
   render fails.
2. **Let running actions finish.** The agent-runner webhook now takes a token
   signed for each run, not `AGENT_WEBHOOK_TOKEN` itself, so a container
   launched by v0.8.0 cannot report back to the new backend. The boot sweep
   closes its record, but the result is lost. `AGENT_WEBHOOK_TOKEN` is
   now an optional signing key: leave it as it is, or empty to have the backend
   generate `{DATA_DIR}/.webhook-key` — and keep that file with the volume.
   An empty `AGENT_WEBHOOK_TOKEN` no longer opens the webhook.
3. **Remove `WEBHOOK_AUTH_HEADER` from the Kubernetes agent Secret**
   (`agentSecretName`). The backend sets it on every Job and overrides it
   anyway.
4. **Replace OAuth MCP connections.** The OAuth flow is gone, and with it the
   `ATLASSIAN_CLIENT_ID`, `ATLASSIAN_CLIENT_SECRET`, `ATLASSIAN_MCP_URL`,
   `OAUTH_CALLBACK_BASE_URL` and `FRONTEND_BASE_URL` variables. An existing
   OAuth connection keeps its access token until it expires; nothing refreshes
   it. Re-add the server with a static token.
5. **Expect the backend to run as `nib` (uid/gid 10001)**, not root. On start,
   the entrypoint hands every file in `DATA_DIR` not owned by `nib` over to it,
   so the first start on a large volume takes longer and a bind-mounted data
   directory on the host changes owner. The mounted `config.yaml` has to be
   readable by uid 10001. A pod started as another user through a
   `securityContext` skips the entrypoint's root step entirely: give the
   volume an `fsGroup` or `chown` it yourself, or the backend cannot write it.
6. **Expect narrower agent tools.** `execute_command` (since replaced by the
   [command tools](#upgrading-from-v082-execute_command-becomes-the-command-tools))
   and `api_call` now run as
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
