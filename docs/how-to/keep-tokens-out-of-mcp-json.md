# How to keep tokens out of `mcp.json`

`mcp.json` sits on the data volume in plain text, is editable from the web
interface, and routinely ends up in a backup. A token written into it literally
is a token in all three places.

Reference it instead. The value is pulled from the encrypted secret store at the
moment the server is called, so the token itself never lands in the file.

## Store the secret

**Variables → Secrets → Add.** Give it a name, the value, and under **Allowed
hosts** the host of every server that may receive it — `grafana-mcp.internal`
for the example below, one per line, without scheme or port.

A secret with no allowed hosts cannot be used from `mcp.json` at all. That is
the right state for a secret meant for something else, such as the executor's
git token.

Entries are bare hostnames or IP addresses, compared exactly with the host of
the server's URL — no wildcards, so `*.internal` is refused and
`grafana-mcp.internal` does not cover `api.internal`.

On an install that upgraded from a release before allowed hosts existed, each
secret the `mcp.json` of the time referenced was bound once, at the first start,
to the hosts of the servers referencing it. Nothing binds a host automatically
after that: a server added later, or a secret the file did not reference then,
needs its host added here by hand.

This needs `SECRETS_ENCRYPTION_KEY` set. Without it, secrets can be neither
written nor read, and every `${NAME}` stays unresolved. If you have not set one
yet:

```bash
openssl rand -base64 32
```

Put it in `.env` as `SECRETS_ENCRYPTION_KEY` and restart the backend. Keep the
key with your other credentials — losing or changing it makes every stored
secret unreadable.

## Reference it

Any value in an MCP server entry may use `${NAME}`:

```json
{
  "mcpServers": {
    "grafana": {
      "url": "https://grafana-mcp.internal/mcp",
      "headers": { "Authorization": "Bearer ${GRAFANA_TOKEN}" }
    }
  }
}
```

It works in headers and in the URL's path, query string and userinfo. It does
not work in the URL's scheme, host or port: the host is what the secret is
bound to, so it has to be written literally.

For a literal `${` in a value, write `$${`.

Saving never resolves a reference, so an entry naming a secret you have not
created yet still saves. The failure surfaces when the server is first
contacted, and it stops only that one server; the rest carry on.

## Why the host has to be allowed

Secret values are never shown back, but anyone who can edit `mcp.json` could
otherwise point a server at a host of their own, reference any secret, and have
nib send it there on the next discovery. The allowed hosts are what stop that:
a secret goes only to a host listed on the secret itself.

So the list is guarded like the value. **Adding a host means entering the value
again**; removing one does not. A server that answers with a redirect to
another host is not followed either, and no header or token is ever sent to a
host other than the server's own.

## What you cannot reference

**Only the encrypted secret store is a source.** The backend's own process
environment deliberately is not.

So an entry in `mcp.json` cannot name `LLM_API_KEY`, the database password, or
any other variable the backend runs with. This is not an oversight to work
around: it is what stops an edit to `mcp.json` from reading the backend's own
credentials back out through any URL or header the agent then calls. See [Why
secrets never come from the
environment](../explanation/secrets-never-come-from-the-environment.md).

## Check that it resolved

**Tools → MCP Servers → the server → Tools.**

A list of tools means the token resolved and the server accepted it. `secret
is not allowed for this host: ${NAME} may not be sent to host …` means the
server's host is missing from the secret's allowed hosts. `variable reference
in server URL host` means a `${NAME}` sits in the URL's scheme, host or port. A transport error means the token resolved but the server did not accept
it, or could not be reached — and the error will not show the value, because
resolved secret values are stripped from every MCP transport error before it
reaches you. Confirm the secret exists under Variables → Secrets, with exactly
the name the entry uses.

## Rotate one

Edit the value under **Variables → Secrets**; the allowed hosts are kept.
`mcp.json` does not change, and
the new value is used on the next call — the discovery cache is invalidated on
a secret change, so nothing needs restarting.

## See also

- [How to connect an MCP server](connect-an-mcp-server.md)
- [Why secrets never come from the environment](../explanation/secrets-never-come-from-the-environment.md)
- [State: the database](../reference/state.md#configuration)
