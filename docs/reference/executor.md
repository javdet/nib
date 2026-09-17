# Executor settings

The executor launches single-use agent containers, one per `code` action. It is
configured in the web interface under **Settings → Executor**, stored as a JSON
file on the data volume, and served by `GET`/`PUT /api/v1/executor/config`.

It is not part of [the configuration file](configuration.md). Three of its
inputs come from the environment, and two from the encrypted secret store.

## `type`

- **Accepted:** `disabled`, `local`, `remote`
- **Default:** `disabled`

| Value | Meaning |
|---|---|
| `disabled` | No container is ever launched. No other executor field is shown, and **Execute action** is greyed out on `code` steps. |
| `local` | Containers run on the backend host's Docker daemon, through the mounted Docker socket. |
| `remote` | Containers run on a remote platform. |

- **Error when acting while disabled:** `executor is disabled; choose an
  executor type in executor settings`
- **Error on an unknown value:** `invalid executor type`

## `platform`

Used only when `type` is `remote`.

- **Accepted:** `kubernetes`, `docker`, `kubefoundry`
- **Errors:** `invalid executor platform`; `executor platform is not available
  yet` for a platform listed in the UI but not yet implemented

## `agent`

Which agent runtime the container runs.

- **Accepted:** `claude-code`, `codex`
- **Errors:** `invalid executor agent`; `executor agent is not available yet`

## `image`

- **Type:** string
- **Default:** `javdet/nib-agent:latest`
- **Required** once the executor is enabled
- **Error:** `executor image is required`

## `llmModel`, `authType`, `tokenSecretName`, `baseURL`

| Field | Meaning |
|---|---|
| `llmModel` | Model the agent container runs. Falls back to `EXECUTOR_LLM_MODEL`. |
| `authType` | `api_key` or `oauth_token`. |
| `tokenSecretName` | Name of the secret holding an OAuth token, when `authType` is `oauth_token`. |
| `baseURL` | Provider base URL for the agent container. |

The agent container speaks the Anthropic Messages API and authenticates with
`EXECUTOR_LLM_API_KEY` or a subscription token. It is configured entirely
separately from the backend's own LLM provider: changing one has no effect on
the other, and a key that works for one may not work for the other.

**Error when the set is incomplete:** `executor secrets are not fully configured
(EXECUTOR_LLM_API_KEY, a git API token secret selected in executor settings, and
an LLM model in settings or EXECUTOR_LLM_MODEL)`

## `gitTokenSecretName`

- **Type:** string — the *name* of an entry in Variables → Secrets
- **No environment fallback.**

The token the agent container clones, pushes and opens the pull or merge
request with. It is a selection, not a fixed name: an install upgrading from a
release before this field existed must pick the secret once before the next
`code` action will start.

A blank selection is refused only when an action is launched, so the executor
can be configured before its secrets exist.

The remote Kubernetes executor is the exception: its Jobs take credentials from
the Agent Secret, so this may be left blank there.

### Which provider the token is used against

Not an executor field. It comes from **Version control system** on the
Knowledge Base page, matched case-insensitively as a substring:

| Setting contains | Provider | Clone user | Opens |
|---|---|---|---|
| `gitlab` | GitLab | `oauth2` | merge request, via `glab` |
| anything else, blank included | GitHub | `x-access-token` | pull request, via `gh` |

The host comes from the repository URL, so self-hosted GitLab and GitHub
Enterprise work. `REPO_URL` must be `http(s)` — an SSH address cannot be
authenticated with a token.

## Kubernetes fields

Used when `type` is `remote` and `platform` is `kubernetes`. Pressing **Execute
action** on a `code` step creates a Kubernetes Job from a template compiled into
the backend, overridable with `{DATA_DIR}/job.yaml.tmpl`.

### `kubernetesAuthMode`

- **Accepted:** `local_config`, `token`
- **Error:** `invalid kubernetes auth mode`

| Value | Meaning |
|---|---|
| `local_config` | Uses `~/.kube/config` or `KUBECONFIG`, optionally a named context. Falls back to the in-cluster service account mount when no kubeconfig is present. |
| `token` | Uses `kubernetesHost` plus a bearer token from the secret store. |

### Required fields

| Field | Required when | Error |
|---|---|---|
| `kubernetesHost` | `kubernetesAuthMode` is `token` | `kubernetes host is required` |
| `kubernetesTokenSecretName` | `kubernetesAuthMode` is `token` | `kubernetes token secret is required` |
| `agentSecretName` | always, for this platform | — |
| `webhookBaseURL` | always, for this platform | — |

### `agentSecretName`

An existing Secret in the target namespace, mounted with `envFrom`. It must
carry the agent's credentials:

- `GITHUB_TOKEN`, or equivalently `GITLAB_TOKEN` or `GIT_TOKEN` — supply any
  one; the container derives the others from it
- `ANTHROPIC_API_KEY` or `CLAUDE_CODE_OAUTH_TOKEN`
- optionally `WEBHOOK_AUTH_HEADER`, for example
  `Authorization: Bearer <AGENT_WEBHOOK_TOKEN>`

### `webhookBaseURL`

Where the finished Job posts its result. `/api/v1/agent-runner/webhook` is
appended automatically.

It must be reachable **from inside the cluster**. `http://localhost:8080` works
only if the backend runs in the same pod network.

### Optional Kubernetes fields

| Field | Default |
|---|---|
| `kubernetesContext` | — (kubeconfig's current context) |
| `kubernetesCACert` | — |
| `kubernetesInsecureSkipTLSVerify` | `false` |
| `namespace` | `default` |
| `serviceAccount` | `default` |
| `agentLimitCPU` | `1` |
| `agentLimitMemory` | `1Gi` |
| `agentRequestCPU` | `1` |
| `agentRequestMemory` | `256Mi` |
| `agentMCPConfig` | — (name of a ConfigMap holding the agent's `mcp.json`) |
| `jobTTLSeconds` | `3600` |

## Run inputs

Validated when an action is launched, not when the config is saved.

| Error | Meaning |
|---|---|
| `repository URL is required` | The action carries no repository. |
| `base branch is required` | The action carries no base branch. |
| `task prompt is required` | The action carries no prompt. |
| `invalid executor json` | The stored config file could not be parsed. |

## Two entry points

| Entry point | Driven by | Notes |
|---|---|---|
| `run_executor` | the LLM tool of the same name | Available in `execute` mode when the executor is configured. |
| **Execute action** | the operator, on a `code` step | Fixed tool surface and timeout constants. |

Both take the single execution slot. See [One execution at a
time](../explanation/one-execution-at-a-time.md).

## The container's own contract

The image's environment variables, agent-type selection, MCP config and webhook
payload are documented with the image itself, in
[`agent-runner/universal-agent/README.md`](../../agent-runner/universal-agent/README.md).

## See also

- [Run planned actions in containers](../how-to/run-actions-in-containers.md)
- [Environment variables](environment-variables.md#backend--executor)
