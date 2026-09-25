# How to switch the LLM provider

Any OpenAI-compatible endpoint works. The move is four settings and, if the
embedding model's name changes, one SQL statement — without which knowledge
search silently returns nothing.

Completions and embeddings are configured separately. `llm.baseURL` serves
completions; the `llm.embeddings` block serves embeddings and falls back to
`llm.baseURL` when it is not set. That split is what makes a provider serving
only chat completions usable — several of them serve no `/embeddings` route at
all.

Changing the provider here has no effect on the executor's agent containers,
which are configured separately. See [Executor
settings](../reference/executor.md).

## Where the settings live

In the `llm` block of the config file — `deploy/compose/config.yaml` under
Docker Compose, `backend.config.llm` in the Helm values. The API key is
`LLM_API_KEY` in the environment and never goes in the file.

A non-empty key in the YAML overrides the matching environment variable. To
drive a value per environment, remove the key from the file.

## To move to the OpenAI platform

```yaml
llm:
  baseURL: https://api.openai.com/v1
  model: gpt-5.6
  embeddingModel: text-embedding-3-small
  api: responses
  reasoningEffort: medium
  timeoutSeconds: 300
  httpReferer: ""
  appTitle: ""
```

Use `api: responses`. GPT-5.6 rejects function tools on `/v1/chat/completions`
unless reasoning is off, and the agent loop always sends tools — so on `chat`
you would be running a reasoning model with its reasoning disabled.

OpenAI model ids carry no provider prefix. If you are arriving from OpenRouter,
the embedding model's name changes, so do the
[rename](#rename-the-embedding-model-in-the-database).

## To move to OpenRouter

```yaml
llm:
  baseURL: https://openrouter.ai/api/v1
  model: anthropic/claude-opus-4.8
  embeddingModel: openai/text-embedding-3-small
  api: chat
  reasoningEffort: ""
  httpReferer: https://github.com/javdet/nib
  appTitle: nib
```

Three things that bite:

- **Clear `reasoningEffort`.** It is forwarded as `reasoning_effort` on every
  request whatever the model is, and only an empty value is left out of the
  payload.
- **Use `api: chat`.** OpenRouter's `/v1/responses` route exists — it answers
  401 on a bad key, where an unknown path returns 404 — but reasoning replay
  there has not been verified against a working key.
- **Model ids are `provider/model` slugs**, including the embedding model.

## Provider presets

Each block below goes in the `llm` section. All of them use `api: chat` and an
empty `reasoningEffort`, for the reasons under
[What these presets have in common](#what-these-presets-have-in-common).

Model ids move faster than this page does. Treat the ones here as the shape of
the setting and check the provider's own model list before committing.

### Google Gemini

Serves both endpoints, so no separate embeddings host is needed — only a width.

```yaml
llm:
  baseURL: https://generativelanguage.googleapis.com/v1beta/openai/
  model: gemini-2.5-pro
  api: chat
  reasoningEffort: ""
  embeddings:
    model: gemini-embedding-001
    dimensions: 1536
```

`gemini-embedding-001` returns 3072 dimensions by default and `kb_chunks.embedding`
is `vector(1536)`, so `dimensions` is not optional here.

### Alibaba Qwen (DashScope)

Also serves both. Use `dashscope.aliyuncs.com` inside mainland China.

```yaml
llm:
  baseURL: https://dashscope-intl.aliyuncs.com/compatible-mode/v1
  model: qwen-max
  api: chat
  reasoningEffort: ""
  embeddings:
    model: text-embedding-v4
    dimensions: 1536
```

`text-embedding-v4` supports 1536; `text-embedding-v3` does **not** — its
largest width is 1024, which this schema cannot store.

### DeepSeek, xAI Grok, Moonshot Kimi

None of these serves an `/embeddings` route. Completions come from them and
embeddings from somewhere else, which is what the `llm.embeddings` block is for.
`LLM_EMBEDDINGS_API_KEY` holds the second provider's key.

```yaml
llm:
  # DeepSeek: https://api.deepseek.com/v1          (deepseek-chat, deepseek-reasoner)
  # xAI:      https://api.x.ai/v1                  (grok-4)
  # Moonshot: https://api.moonshot.ai/v1           (kimi-k2-0905-preview)
  baseURL: https://api.deepseek.com/v1
  model: deepseek-chat
  api: chat
  reasoningEffort: ""
  embeddings:
    baseURL: https://api.openai.com/v1
    model: text-embedding-3-small
```

Any embeddings host works as long as it returns 1536-wide vectors. Gemini's
endpoint with `dimensions: 1536` is the alternative if you would rather not hold
an OpenAI key.

Whichever you choose, the embedding model's **name** is what knowledge search
matches on, so changing it means the
[rename](#rename-the-embedding-model-in-the-database) below.

### What these presets have in common

- **`api: chat`.** None of these providers serves `/v1/responses`. That costs
  reasoning replay, which is an OpenAI-only feature — see
  [LLM endpoints and reasoning replay](../explanation/llm-endpoints.md).
- **`reasoningEffort: ""`.** The parameter is sent on every request whatever the
  model is. Grok-4 and Kimi reject it outright; Gemini accepts only
  `none`/`low`/`medium`/`high`, not `minimal` or `xhigh`. An empty value omits
  it. Note that an *absent* key defers to `LLM_REASONING_EFFORT`, so write the
  empty string explicitly rather than deleting the line.
- **The `dimensions` setting must match `kb-mcp`.** See
  [Update the knowledge-base container too](#update-the-knowledge-base-container-too).

## Rename the embedding model in the database

Do this whenever `llm.embeddingModel` changes its **name**, even if the
underlying model is the same. Knowledge search compares
`llm.embeddingModel` against `kb_collections.embedding_model` as an exact
string: a mismatch returns no results, with no error anywhere.

Moving to OpenRouter:

```sql
update kb_collections set embedding_model = 'openai/text-embedding-3-small'
where embedding_model = 'text-embedding-3-small';
```

Moving to the OpenAI platform, the same rename in reverse.

The stored vectors stay valid — OpenRouter proxies the same OpenAI model, so
only the label differs. **Do not re-ingest.**

If the underlying model is changing, not just its name, that is not a rename:
the existing vectors are meaningless against the new one and every collection
has to be re-ingested.

### A note on width

`kb_chunks.embedding` is `vector(1536)` and `kb_collections` carries a
constraint to match, so every collection is 1536-wide. A model with a different
native width has to be asked for 1536 through `llm.embeddings.dimensions`;
providers implement that as Matryoshka truncation, which is only meaningful
when the model was trained for it.

nib does not slice vectors locally. If the endpoint ignores `dimensions` and
answers at its own width, the request fails naming both numbers rather than
storing something that would never match.

## Update the knowledge-base container too

`kb-mcp` embeds queries itself and has its own copy of these settings:

- Docker Compose — `KB_EMBEDDINGS_BASE_URL` in `.env`, which becomes
  `EMBEDDINGS_BASE_URL` in the container.
- Helm — `kbMcp.embeddings.baseURL`.

**If you set `llm.embeddings.dimensions`, set the container's width to match.**
`KB_EMBEDDINGS_DIMENSIONS` in `.env` becomes `EMBEDDINGS_DIMENSIONS` in the
container (`kbMcp.embeddings.dimensions` in Helm). The backend ingests the
chunks and `kb-mcp` embeds the queries that search them: at different widths the
comparison matches nothing, and, like the model-name mismatch below, it reports
no error anywhere.

Its `-provider openrouter` flag is only the CLI's name for a generic
OpenAI-compatible `/embeddings` client. Leave it as is and point the base URL
wherever you need; it is not a claim about OpenRouter.

## Apply and verify

```bash
docker compose restart backend kb-mcp
```

Then, in order:

1. Send anything in a **discuss** chat. A reply means completions work.
2. Ask it to search the knowledge base. Results mean the embedding model name
   matches; an empty result on a collection you know has content means the
   rename above is still outstanding.

Unit tests cover request shape and input ordering but cannot catch a schema
rejection from a real API. To exercise a full two-round tool loop including
reasoning replay against the live provider:

```bash
docker run --rm -e NIB_LIVE_OPENAI=1 -e LLM_API_KEY -v "$PWD/backend":/app -w /app \
  golang:1.26 go test ./internal/llm -run TestLiveResponses -v
```

It is skipped by default because it spends tokens. `LLM_MODEL` overrides the
model it exercises. A model may legitimately return no reasoning items on a
question it considers trivial, which is why the test uses `high` effort and a
multi-step prompt.

## Providers that will not work

**Anything that is not OpenAI-compatible.** nib speaks `/v1/chat/completions`
and `/v1/embeddings` and has no provider-specific code. A model behind a wire
format of its own needs a gateway in front of it; OpenRouter is the usual one.

A provider that serves completions but **not** embeddings is no longer a
problem: point `llm.embeddings` at something that does. See
[Provider presets](#provider-presets).

## See also

- [Configuration file](../reference/configuration.md#llm)
- [LLM endpoints and reasoning replay](../explanation/llm-endpoints.md) — why `llm.api` exists
