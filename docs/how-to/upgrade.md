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

1. The sidebar shows the new version, as does `GET /api/v1/version`.
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
