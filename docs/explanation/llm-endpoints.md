# About LLM endpoints and reasoning replay

`llm.api` takes one of two values, `chat` or `responses`, and choosing the wrong
one gives you an install that works — just noticeably worse, with nothing in the
logs to say why.

This page is about what the setting actually selects, and why a configuration
option exists for something that looks like an implementation detail.

## The constraint that forced the option

OpenAI rejects function tools on `/v1/chat/completions` for GPT-5.6 unless
`reasoning_effort` is `none`.

nib's agent loop always sends tools. There is no turn without a catalog — that
is what makes it an agent rather than a chat. So on `chat`, GPT-5.6 is
effectively a non-reasoning model: you are paying for a reasoning model and
getting its reasoning switched off.

`/v1/responses` does not have that restriction. Hence the option.

It defaults to `chat` because every OpenAI-compatible gateway implements that
endpoint, and a default that works everywhere is worth more than a default that
is optimal on one provider. A config written before the option existed keeps its
old behaviour.

## What `responses` also buys

Reasoning replay, which is the more interesting half.

An agent turn is not one request. It is completion → tool calls → completion →
tool calls, up to the round budget. On `chat`, each round is a fresh request
carrying the conversation so far — and the model's *thinking* from the previous
round is not part of that conversation. It restarts its reasoning after every
tool result.

On `responses`, nib asks the provider to include encrypted reasoning traces and
carries them back into the next request. The model continues from its own
thinking rather than re-deriving it.

For a single-round exchange this is worth little. For a ten-round investigation
it is the difference between an agent that builds an argument and one that
starts over ten times, each time with slightly more context and slightly less
room.

The traces are provider-encrypted — nib cannot read them — and held in memory for
one turn only. The next turn rebuilds history from Postgres, without them. So
reasoning replay is a within-turn property, not a memory that accumulates.

## Why this is a setting and not a detection

nib could probe the provider and choose. It does not, and the reason is that the
probe is unreliable in exactly the case that matters.

OpenRouter's `/v1/responses` route *exists*: it answers 401 on a bad key, where
an unknown path returns 404. By any reasonable probe it is supported. Whether
reasoning replay through it behaves correctly has not been verified against a
working key — and a wrong answer here does not fail loudly. It produces slightly
worse plans.

A setting makes the uncertainty visible. Detection would hide it behind a
heuristic that is right most of the time, which for a quality regression with no
error message is the wrong trade.

## Why embeddings have a block of their own

Whatever `llm.api` says, embeddings always go to `/v1/embeddings`. The setting
governs completions only.

They also go to their own host. `llm.embeddings` has its own `baseURL`,
`model` and `dimensions`, and its own key in `LLM_EMBEDDINGS_API_KEY` — the
key comes from the environment like every other one, never from the file. Each
falls back to the matching completion setting, so an install that never sets
the block behaves exactly as it did when one base URL served both.

The block exists because "an OpenAI-compatible provider" turned out not to mean
one thing. DeepSeek, xAI and Moonshot serve chat completions and no
`/embeddings` route at all. Under a single base URL they were not
misconfigurable, they were unusable: knowledge search and the tool catalog both
embed, so pointing `llm.baseURL` at one of them broke two subsystems that have
nothing to do with the model doing the reasoning.

Falling back rather than defaulting is the load-bearing part. The common case
is still one provider for both, and making the block required would have been a
breaking change to every existing config for the benefit of the minority that
needs it.

`dimensions` comes from the other direction. `kb_chunks.embedding` is
`vector(1536)`, which is a schema decision rather than a preference — several
widths for real means a chunk table per width. Gemini's embedding model is
natively 3072 and Qwen's is 1024, and both accept a request for 1536. Without a
way to ask, neither could be used at all.

What nib will not do is truncate locally. A vector sliced by the caller is only
meaningful from a model trained for it, and afterwards it is indistinguishable
from a good one. So a provider that ignores `dimensions` and answers at its own
width fails the request, naming both numbers.

## What the chat path tolerates

`chat` is the endpoint every gateway implements, which in practice means every
gateway implements it slightly differently. The parsing is deliberately more
forgiving than the OpenAI schema:

- **Tool calls without a `type` field.** Several compatibility layers omit it.
  The typed SDK union discriminates on exactly that field, so a nil variant
  used to drop every call in the round — and a round with no tool calls is what
  the agent loops read as the turn's final answer. The failure looked like a
  model that had decided to stop, on an agent whose whole job is tool calls.
- **Tool-call arguments sent as an object** rather than a JSON string, which
  decodes to an empty string without an error. The raw body is re-read when a
  field comes back empty.
- **Missing tool-call ids.** One is synthesized, because every later step pairs
  the result back to the call by id.
- **`finish_reason` and `refusal`,** which were previously not read at all. A
  `length` truncation is now an error rather than a short answer: the loops
  would otherwise persist a plan that stopped mid-stage as a finished one.
- **`reasoning_content` and `reasoning`,** where DeepSeek, Qwen and OpenRouter
  put a plaintext chain of thought. It is captured for the operator but never
  replayed — DeepSeek rejects a request that sends it back. Only the responses
  endpoint replays reasoning, and only the encrypted form it issued itself.

One thing is stripped whatever the endpoint: a `<thinking>` (or `<think>`)
block in the reply's content. The plan and decompose prompts ask the model to
open a turn with one, and DeepSeek and Qwen emit one unprompted. A provider with
a reasoning channel keeps it out of the content; one without leaves it in, where
it would land in the transcript and in the note a finished action hands to the
next. A final answer wrapped entirely in the block is unwrapped rather than
dropped, because an empty answer costs more than a stray tag.

Tool *schemas* and tool *names* are rewritten for the same reason, before they
are sent. A published JSON Schema may carry `$ref`, `$defs`, `anyOf` or
`format`, and a name may carry dots or run past 64 characters; a provider that
validates strictly rejects the whole request rather than the offending tool, so
one awkward MCP server would otherwise take down every turn that offers it.
Dispatch keeps the server's own spelling on the route, since the rewrite has no
inverse.

The one thing the rewrite costs is `create_plan_contract`, whose `stages` is an
open map keyed by stage name. That has no equivalent in the stricter
function-call subsets, so it degrades to a bare object and the model takes the
shape from the tool description instead.

## How to know you chose wrong

There is no error. The signals are indirect:

- On `chat` with a reasoning model and tools, plans are shallower than the model
  should produce, and multi-step investigations lose the thread between tool
  calls.
- On `responses` with a provider that does not really support it, requests fail
  outright — which is at least loud.

Because the failure in the first direction is quiet, it is worth verifying a
provider change deliberately rather than assuming. Unit tests cover request
shape and input ordering but cannot catch a schema rejection from a real API;
there is an opt-in live test for that, described in [How to switch the LLM
provider](../how-to/switch-llm-provider.md#apply-and-verify).

## See also

- [How to switch the LLM provider](../how-to/switch-llm-provider.md)
- [Configuration file](../reference/configuration.md#llmapi)
- [About the architecture](architecture.md#what-nib-talks-to) — what nib talks to
