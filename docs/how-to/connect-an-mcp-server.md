# How to connect an MCP server

Registering a server is how the agent gets tools for a system nib does not know
about — your monitoring stack, your cloud provider, your issue tracker.

**A nib MCP server must expose an HTTP endpoint and be registered by URL.**
`stdio` and SSE are not supported. There is no SSE transport in the codebase at
all, and a `"transport"` field in an entry is dead — nothing reads it. Having a
`url` is what makes a server work.

## Register a server by URL

In the web interface, **Tools → MCP Servers → Add**. Give it a name and the URL
of its streamable-HTTP endpoint, usually ending in `/mcp`.

Or edit `mcp.json` directly — **Tools → Config**, or the file at
`{DATA_DIR}/mcp.json`. It is Cursor-shaped:

```json
{
  "mcpServers": {
    "grafana": {
      "url": "http://grafana-mcp:8000/mcp",
      "headers": { "Authorization": "Bearer ${GRAFANA_TOKEN}" }
    }
  }
}
```

JSONC comments are allowed and unknown fields survive an edit through the UI.

Saving invalidates the 60-second discovery cache, so the server's tools appear
without a restart.

Never write a token into this file literally — see [How to keep tokens out of
`mcp.json`](keep-tokens-out-of-mcp-json.md). A referenced secret must list the
server's host, `grafana-mcp` here, among its allowed hosts.

## Register a server with a static token instead

The second source of servers is a *token connection*: a URL and an API token
stored in Postgres rather than in `mcp.json`. There is no page for these; they
are managed through the API:

```bash
curl -X POST http://localhost:8081/api/v1/mcp/connections \
  -H "Authorization: Bearer $NIB_API_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"type": "custom", "name": "grafana", "serverUrl": "https://grafana-mcp.internal/mcp", "apiToken": "…"}'
```

The token is sent as `Authorization: Bearer`, or as Basic auth for `"type":
"jira"`. Both sources feed the same tool catalog.

There is **no OAuth flow**. A server that needs OAuth has to be reached with a
static token. A connection left over from the OAuth flow an older release had
keeps working with the access token it holds until that expires — nothing
refreshes it — and then has to be re-added as a token connection.

Moving a connection's `serverUrl` to another origin (scheme, host or port)
requires sending a new `apiToken` with it. A blank token keeps the stored one
only while the origin stays the same, so a stored token is never sent to a
server it was not issued for.

## Use a stdio-only server

Put an HTTP bridge in front of it and register the bridge's URL. A `command`/
`args` entry in `mcp.json` is parsed and then skipped: discovery logs `tool
catalog: skip stdio server`, and a call to one of its tools fails with `has no
MCP route`. Adding one through the interface is refused outright.

## Check that it worked

**Tools → MCP Servers → the server → Tools** lists what it exposes.

| What you see | What it means |
|---|---|
| A list of tools | Discovery succeeded; they are already usable. |
| A transport error | nib could not reach the URL. Secret values are stripped from these messages, so a redacted-looking error is expected, not a bug. |
| An empty list | The server answered but exposes nothing. |

Every tool a server exposes is switched **on for every mode** as soon as it is
indexed, so a newly added server is usable immediately. Deleting a server takes
its tools back out of every mode.

## Narrow what the agent may reach

The default of "every tool in every mode" is rarely what you want once you have
more than a couple of servers — a read-only mode should not be holding tools
that change things.

**Tools → Included tools** removes a tool from one mode. It stays reachable
through the agent's `tool_search`, so removing it narrows the default surface
without making it unavailable.

The **Distribute with AI** button on that tab hands the whole catalog to a
discuss chat, which applies a per-mode policy: read-only modes lose the tools
that change things, `execute` loses the version-control ones. It only ever
removes. Putting a tool back is a move you make on the page.

## See also

- [How to keep tokens out of `mcp.json`](keep-tokens-out-of-mcp-json.md)
- [Modes, tools and the catalog](../explanation/modes-tools-and-the-catalog.md) — why a tool may not be offered
- [HTTP API](../reference/http-api.md#mcp-servers)
- [HTTP API: MCP connections](../reference/http-api.md#mcp-connections)
