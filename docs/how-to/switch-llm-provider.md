# How to switch the LLM provider

Any OpenAI-compatible endpoint works. The move is four settings and, if the
embedding model's name changes, one SQL statement — without which knowledge
search silently returns nothing.

The backend uses **one** base URL for both chat completions and embeddings. A
provider that serves only one of the two cannot be used on its own.

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
the embedding model's name changes, so do the [rename](#rename-the-embedding-
model-in-the-database).

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

If you are genuinely changing to a different embedding model, that is not a
rename: the existing vectors are meaningless against the new one and every
collection has to be re-ingested.

## Update the knowledge-base container too

`kb-mcp` embeds queries itself and has its own copy of these settings:

- Docker Compose — `KB_EMBEDDINGS_BASE_URL` in `.env`, which becomes
  `EMBEDDINGS_BASE_URL` in the container.
- Helm — `kbMcp.embeddings.baseURL`.

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

**Anthropic directly.** There is no Anthropic provider, and one base URL serves
both chat and embeddings — pointing it at Anthropic would send `/embeddings`
there too, which Anthropic does not offer, breaking knowledge search and the
tool catalog. Claude through OpenRouter has no such problem.

Any provider that serves completions but not embeddings fails the same way.

## See also

- [Configuration file](../reference/configuration.md#llm)
- [LLM endpoints and reasoning replay](../explanation/llm-endpoints.md) — why `llm.api` exists
