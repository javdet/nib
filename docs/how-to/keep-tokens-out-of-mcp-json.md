# How to keep tokens out of `mcp.json`

`mcp.json` sits on the data volume in plain text, is editable from the web
interface, and routinely ends up in a backup. A token written into it literally
is a token in all three places.

Reference it instead. The value is pulled from the encrypted secret store at the
moment the server is called, so the token itself never lands in the file.

## Store the secret

**Variables → Secrets → Add.** Give it a name and the value, in the **Global**
scope — `${NAME}` reads global secrets only.

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

It works in headers and anywhere in the URL.

For a literal `${` in a value, write `$${`.

Saving never resolves a reference, so an entry naming a secret you have not
created yet still saves. The failure surfaces when the server is first
contacted, and it stops only that one server; the rest carry on.

## Where a secret can go

Any server entry can reference any global secret: secrets are not tied to
hosts. Headers and tokens are sent only to the server's own scheme, host and
port, and a redirect to another host is not followed. But anyone who can edit
`mcp.json` can point an entry at a host of their own and have nib send it a
secret, so treat edit access to `mcp.json` as access to every global secret.

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

A list of tools means the token resolved and the server accepted it. A transport error means the token resolved but the server did not accept
it, or could not be reached — and the error will not show the value, because
resolved secret values are stripped from every MCP transport error before it
reaches you. Confirm the secret exists under Variables → Secrets, with exactly
the name the entry uses.

## Rotate one

Edit the value under **Variables → Secrets**. `mcp.json` does not change, and
the new value is used on the next call — the discovery cache is invalidated on
a secret change, so nothing needs restarting.

## See also

- [How to connect an MCP server](connect-an-mcp-server.md)
- [Why secrets never come from the environment](../explanation/secrets-never-come-from-the-environment.md)
- [State: the database](../reference/state.md#configuration)
