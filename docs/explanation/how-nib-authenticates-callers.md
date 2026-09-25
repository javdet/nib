# About how nib authenticates callers

nib's API can change infrastructure: it can start an action, launch a container
with credentials in it, and rewrite the file that decides which servers receive
which secrets. For a long time it trusted the network to keep strangers out.
It no longer does, and the shape of what replaced that trust is narrower than
"add a login" suggests.

This page is about why authentication looks the way it does — one token, a
cookie that is not the token, two guards that apply even with auth turned off,
and a webhook that takes none of it. Setting it up is in [How to run with
Docker Compose](../how-to/run-with-docker-compose.md); the variables are in
[Environment variables](../reference/environment-variables.md).

## One token, and failing closed

Every `/api/v1` route except health, version, the login endpoint and the
agent-runner webhook requires `NIB_API_TOKEN`. There are no users, no roles and
no passwords — one shared secret, at least 32 characters long.

That is deliberately the smallest thing that closes the hole. nib is an
operator's tool with a handful of people behind it, and the question it has to
answer is "is this caller one of ours?", not "which of ours, and what may they
do?". Anything finer belongs in an authenticating proxy that already knows your
people; nib does not try to become an identity provider.

**The backend refuses to start without a token.** Running open takes
`NIB_INSECURE_NO_AUTH=true`, spelled out. An empty token without that switch
does not mean "no auth" — it means nothing authenticates. A forgotten variable
is the likeliest way to end up without a token, an install upgraded from a
release that had no authentication among them, and an API that quietly served
open in that case would look from the outside exactly like one that was
protected.
A configured token always wins over the switch, so leaving the switch on by
mistake cannot open an install that has a token.

## The cookie is not the token

Scripts send `Authorization: Bearer <token>`. The browser cannot, everywhere it
needs to: `EventSource`, which carries the live activity stream, and an `<img>`
tag have no way to set a header. So the web UI asks for the token once, posts it
to the login endpoint, and receives an HttpOnly, `SameSite=Strict` cookie.

The cookie holds an expiry and a signature over it, with a key derived from the
token — never the token itself, and nothing stored on the server. Two
consequences follow, both intended. A stolen cookie is a time-limited session,
not the credential scripts use. And rotating the token signs every browser out,
because every existing signature was made with the old key. There is no session
table to purge, because there are no sessions to keep.

## Guards that apply whatever the auth mode

Authentication answers "who sent this?". It does not answer "did they mean
to?" — and a browser that holds a valid cookie will send it with a request some
other page asked it to make. Two guards close that, and they run even when auth
is off, since an open install is exactly the one a hostile page can reach.

**Cross-site writes are refused.** A `POST`, `PUT` or `DELETE` a browser sent
from another origin gets 403, judged by the browser's own `Sec-Fetch-Site` or
`Origin` headers. CORS is not a substitute: it hides the *response* from the
other page, but the request has already run. `NIB_ALLOWED_ORIGINS` names any
other origin you do want to allow. A request with neither header — curl, the
agent-runner — is not a browser's, and passes.

**JSON routes accept only JSON.** A body declared as anything other than
`application/json` gets 415. That looks pedantic and is load-bearing: a browser
sends `text/plain`, form and multipart bodies across sites without a preflight,
and decoding them as JSON anyway is what used to make every write route a
cross-site target.

A third guard is opt-in. `NIB_ALLOWED_HOSTS`, when set, refuses any other
`Host` header with 421. That is what stops DNS rebinding — a page whose own
hostname is made to resolve to your nib, so that the browser treats it as same
origin and both guards above see nothing wrong. The machine endpoints skip the
browser guards, since probes and agent containers call from other hosts.

## The webhook takes a token per run

The agent-runner container reports its result to a webhook, and it cannot be
given the API token: the agent inside runs arbitrary tooling and can read its
own environment. A shared webhook token has the same problem one level down —
any container could forge the result of any other run.

So each container is handed a token derived from a key that never leaves the
backend, bound to that run's chat and job name. The webhook checks it against
the chat and job the delivery claims to be about. A container can vouch for its
own run and nothing else; a replayed delivery is absorbed by the per-job
deduplication that already existed for retries.

The key is `AGENT_WEBHOOK_TOKEN` when set, and otherwise generated once into a
file on the data volume. Generating it rather than requiring it means a new
install needs one less secret; keeping it on the volume means a restart does
not invalidate runs still in flight. That is also the price of changing it:
every run started under the old key can no longer report. For the same reason
an empty or unreadable key file is a boot error, not a cue to generate a new
one.

There is no insecure bypass here. `NIB_INSECURE_NO_AUTH` opens the API, not the
webhook, because the webhook is how a run's outcome gets written into a plan,
and an unauthenticated one would let anything on the network declare an action
done.

On Kubernetes the Job spec carries its run's token in plain text. That is
acceptable because it is worth exactly one run's report; it is the reason the
token is derived per run rather than shared.

## What this does not protect

**Anyone with the token is everyone.** It opens every route, including the ones
that edit `mcp.json`, choose which secret the executor gets, and run commands on
the backend's host. The other guards in nib — [secrets bound to
hosts](secrets-never-come-from-the-environment.md#host-binding), the scrubbed
environment of the [backend's own tools](modes-tools-and-the-catalog.md#the-backends-own-hands) —
exist precisely because the token alone is a coarse boundary.

**The metrics listener is not behind it.** It is on its own port, unreachable
through the ingress and the frontend nginx, which is the protection it has.

## See also

- [About the architecture](architecture.md#what-this-architecture-cannot-do)
- [About why secrets never come from the environment](secrets-never-come-from-the-environment.md)
- [HTTP API](../reference/http-api.md)
- [Environment variables](../reference/environment-variables.md)
