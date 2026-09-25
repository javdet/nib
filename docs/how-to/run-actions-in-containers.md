# How to run planned actions in containers

Until the executor is configured, **Execute action** is greyed out on every
`code` step: nib will plan the work and then hand it to you. Configuring the
executor is what lets it clone the repository, make the change and open a pull
request itself.

The executor is operator-configured and never detected. Nothing turns itself on
because Docker happens to be reachable.

Every field is in **Settings → Executor**, and full field meanings are in
[Executor settings](../reference/executor.md).

## Decide where containers run

| Type | Containers run | Choose it when |
|---|---|---|
| **Disabled** | nowhere | You want plans only, executed by hand. |
| **Local (Docker socket)** | on the backend host's Docker daemon | nib runs on a host whose Docker daemon you are willing to hand to an agent. |
| **Remote** | on a remote platform, currently Kubernetes | nib runs in a cluster, or you want the agent isolated from nib's own host. |

Mounting the host Docker socket into the backend gives whatever runs in those
containers control of that daemon. On a shared host, prefer Remote.

The agent container is **a different machine from the backend**, with its own
toolchain. What is on the backend's `PATH` says nothing about what the agent can
run, and the reverse.

## Set up a local Docker executor

1. Make sure the backend can reach a Docker daemon. In the shipped Compose
   stack, the socket is already mounted.
2. **Settings → Executor → Type → Local**.
3. **Image** — leave the default `javdet/nib-agent:latest` unless you publish
   your own.
4. **Agent** — `claude-code` or `codex`.
5. Set `EXECUTOR_LLM_API_KEY` and `EXECUTOR_LLM_MODEL` in `.env`, and restart
   the backend. These are the agent's own credentials, not the backend's: the
   container speaks the Anthropic Messages API, so a key that works for nib's
   own provider will usually not work here.
6. **Git API token** — see below.
7. Save.

## Select the git API token

This is the step that catches upgrades. The token the container clones, pushes
and opens the pull request with is **selected by name** from Variables →
Secrets. There is no environment-variable fallback.

1. **Variables → Secrets → Add** — store the token.
2. **Settings → Executor → Git API token** — select it.

A blank selection is accepted at save time and refused when an action is
launched, with `executor git API token secret is not configured; select it in
executor settings`. So an install upgrading from a release before this field
existed looks fine until the first `code` action fails.

Remote Kubernetes is the exception: its Jobs take credentials from the Agent
Secret, so the git API token may be left blank there.

### Point it at GitLab instead of GitHub

Not an executor setting. **Knowledge Base → Version control system**, matched
as a case-insensitive substring:

| Value | Provider | Clones as | Opens |
|---|---|---|---|
| anything containing `gitlab` | GitLab | `oauth2` | merge request, via `glab` |
| anything else, blank included | GitHub | `x-access-token` | pull request, via `gh` |

The host comes from the repository URL, so self-hosted GitLab and GitHub
Enterprise work. The repository URL must be `http(s)` — an SSH address cannot be
authenticated with a token.

## Set up a remote Kubernetes executor

Each `code` action becomes a Kubernetes Job.

1. **Settings → Executor → Type → Remote**, **Platform → Kubernetes**.
2. **Cluster access:**
   - *Local Config* — uses `~/.kube/config` or `KUBECONFIG`, optionally a named
     context, and falls back to the in-cluster service account mount when no
     kubeconfig is present. This is what you want for nib running inside the
     target cluster.
   - *Token* — set **Host** to the API server URL and store a bearer token in
     Variables → Secrets, then select it.
3. **Image** — the agent image, pullable from the cluster.
4. **Agent Secret Name** — an existing Secret in the target namespace, mounted
   with `envFrom`. It must carry:
   - `GITHUB_TOKEN`, or equivalently `GITLAB_TOKEN` or `GIT_TOKEN` — supply any
     one and the container derives the others
   - `ANTHROPIC_API_KEY` or `CLAUDE_CODE_OAUTH_TOKEN`

   `WEBHOOK_AUTH_HEADER` does not belong here: the backend adds it to every Job,
   with a token that authenticates that run only.
5. **Webhook base URL** — must be reachable **from inside the cluster**.
   `http://localhost:8080` works only if the backend is in the same pod network.
   `/api/v1/agent-runner/webhook` is appended for you.
6. Optionally set namespace, service account, resource limits and requests, an
   MCP ConfigMap name, and the Job TTL.

To change what the Job looks like, drop your own template at
`{DATA_DIR}/job.yaml.tmpl`; it overrides the one compiled into the backend.

## Run one action

Open a plan, find a `code` step, press **Execute action**.

Only one execution runs at a time, across every plan. A second request is
refused with a sentence naming what holds the slot rather than being queued —
see [One execution at a
time](../explanation/one-execution-at-a-time.md).

When the Job finishes, the agent posts its result to the webhook and the plan
chat updates. No further backend configuration is needed for that callback.

## When an action will not start

| Message | Fix |
|---|---|
| `executor is disabled; choose an executor type in executor settings` | Type is still Disabled. |
| `executor git API token secret is not configured…` | Select the secret under Settings → Executor. |
| `executor secrets are not fully configured…` | One of `EXECUTOR_LLM_API_KEY`, the git token selection, or the model is missing. |
| `executor image is required` | Set an image. |
| `kubernetes host is required` / `kubernetes token secret is required` | Token auth needs both. |
| `repository URL is required` / `base branch is required` | The action itself is incomplete; fix it in the plan. |

If the container starts and then fails, the plan chat carries what it reported.
The image's own environment contract, agent-type selection and webhook payload
are documented in
[`agent-runner/universal-agent/README.md`](../../agent-runner/universal-agent/README.md).

## See also

- [Executor settings](../reference/executor.md) — every field and its default
- [One execution at a time](../explanation/one-execution-at-a-time.md)
