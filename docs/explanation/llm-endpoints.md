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

## The asymmetry with embeddings

Whatever `llm.api` says, embeddings always go to `/v1/embeddings`. The setting
governs completions only.

It is the seam where the single-endpoint design shows. One `baseURL` serves
both, so a provider has to implement both — which is why Anthropic cannot be
used directly, and why a provider switch that looks like a one-line change can
silently break knowledge search if the embedding model's *name* changes. The
vectors are fine; the label no longer matches what the collections recorded.

Separating the completion endpoint from the embedding endpoint would be the
prerequisite for supporting providers that serve only one of the two. It is a
genuine limitation rather than an oversight, and it has not been needed badly
enough to justify the second set of settings.

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
- [About the architecture](architecture.md#what-nib-talks-to) — why there is one endpoint
