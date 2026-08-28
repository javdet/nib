# LLM providers

The backend talks to one OpenAI-compatible endpoint for both chat completions
and embeddings. Everything below is set in the `llm` block of `config.yaml`
(`backend/config.yaml` for docker compose, `backend.config.llm` in the Helm
values); the API key comes from the `LLM_API_KEY` environment variable and never
belongs in the config file.

## Choosing the completion endpoint

`llm.api` selects which endpoint the completions go to. Embeddings always use
`/v1/embeddings` regardless of this setting.

| Value       | Endpoint                | When to use |
|-------------|-------------------------|-------------|
| `chat`      | `/v1/chat/completions`  | Default. Every OpenAI-compatible gateway supports it. |
| `responses` | `/v1/responses`         | Required for GPT-5.6 with tools and reasoning. |

Omitting `llm.api` means `chat`, so configs written before this option existed
keep their old behaviour.

The reason the option exists: OpenAI rejects function tools on
`/v1/chat/completions` for GPT-5.6 unless `reasoning_effort` is `none`. Since
the agent loop always sends tools, GPT-5.6 on `chat` is effectively a
non-reasoning model. `responses` lifts that restriction.

On `responses` the provider also replays reasoning between tool-call rounds. It
sends `store: false` with `include: ["reasoning.encrypted_content"]`, and each
assistant message carries its (provider-encrypted) reasoning traces back into
the next request, so the model continues from its own thinking instead of
restarting it after every tool result. Reasoning is held in memory for the
duration of one turn only — the next turn rebuilds history from Postgres.

## Current setup: OpenAI platform

```yaml
llm:
  baseURL: https://api.openai.com/v1
  model: gpt-5.6            # alias for gpt-5.6-sol; also gpt-5.6-terra, gpt-5.6-luna
  embeddingModel: text-embedding-3-small
  api: responses
  reasoningEffort: medium   # none, minimal, low, medium, high, xhigh
  timeoutSeconds: 300
  httpReferer: ""           # OpenRouter-only attribution headers
  appTitle: ""
```

`reasoningEffort` is sent on every completion; leaving it empty uses the
provider default (`medium` for GPT-5.6).

`LLM_API` and `LLM_REASONING_EFFORT` set the same two values from the
environment, but the YAML wins: as with `model` and `baseURL`, a non-empty key
in the config file overrides whatever the environment says. Remove the key from
the YAML if you want to drive it per environment.

## Switching back to OpenRouter

No code changes are needed, but two settings are coupled to things outside the
config file.

```yaml
llm:
  baseURL: https://openrouter.ai/api/v1
  model: anthropic/claude-opus-4.8    # OpenRouter uses provider/model slugs
  embeddingModel: openai/text-embedding-3-small
  api: chat
  reasoningEffort: ""                 # see below
  httpReferer: https://github.com/javdet/nib
  appTitle: nib
```

**Clear `reasoningEffort`.** It is forwarded as `reasoning_effort` on every
request whatever the model is; only an empty value is omitted from the payload.

**Rename the embedding model in the database.** `knowledge_search` compares
`llm.embeddingModel` against `kb_collections.embedding_model` as an exact
string, and OpenRouter needs the provider-prefixed slug:

```sql
update kb_collections set embedding_model = 'openai/text-embedding-3-small'
where embedding_model = 'text-embedding-3-small';
```

The stored vectors stay valid — OpenRouter proxies the same OpenAI model, so
only the label differs. Re-ingesting is not required. The same rename in
reverse is what the move to the OpenAI platform needed.

The `kb-mcp` container has its own copy of these settings in
`docker-compose.yml` (`OPENROUTER_BASE_URL`) and in the Helm values
(`kbMcp.openrouter.baseURL`). Its `-provider openrouter` flag is just the `kb`
CLI's name for a generic OpenAI-compatible `/embeddings` client, so it points
wherever that base URL says.

## Provider compatibility notes

**OpenRouter `/v1/responses`.** The route exists (it answers 401 on a bad key,
where an unknown path returns 404), but reasoning replay there has not been
verified against a working key. Use `api: chat` until it has been.

**Anthropic directly.** Not supported. There is no Anthropic provider, and the
same `baseURL` serves chat and embeddings — pointing it at Anthropic would send
`/embeddings` there too, which Anthropic does not offer, breaking knowledge
search and the tool catalog. Claude through OpenRouter has no such problem.
Separating the embedding endpoint from the completion endpoint would be the
prerequisite for any provider that does not serve both.

**The executor is configured separately.** The agent-runner runs Claude Code and
speaks the Anthropic Messages API via `ANTHROPIC_BASE_URL`, authenticating with
`EXECUTOR_LLM_API_KEY` (an OpenRouter key) or a Claude.ai subscription token.
Changing the backend provider has no effect on it, and an OpenAI key will not
work there.

## Verifying a provider change

Unit tests cover request shape and input-item ordering. They cannot catch
schema rejections from a real API, so there is an opt-in live test that runs a
two-round tool loop including reasoning replay:

```bash
NIB_LIVE_OPENAI=1 go test ./internal/llm -run TestLiveResponses -v
```

It is skipped by default because it spends tokens. `LLM_MODEL` overrides the
model it exercises. Note that a model may legitimately return no reasoning
items on questions it considers trivial, which is why the test uses `high`
effort and a multi-step prompt.
