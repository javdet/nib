# Explanation

Why nib is built the way it is: the decisions, the constraints they came from,
and what each one costs. These pages are for reading away from the keyboard —
they will not tell you which button to press, and none of them is needed to use
nib.

They are worth reading when something nib does seems arbitrary. Most of the
time it is not, and knowing the reason changes what you do next.

## The system as a whole

Start here. Between them, these two make the rest of nib legible.

- [About the architecture](architecture.md) — what the moving parts are, why
  there are four containers, and what this shape cannot do
- [About how a plan is produced](how-a-plan-is-produced.md) — why one sentence
  becomes five chats, and how agents that cannot talk to each other still agree

## Deliberate limits

Two places where nib refuses to do something it could, and the reasoning behind
each refusal.

- [About one execution at a time](one-execution-at-a-time.md) — why a second
  action is refused rather than queued, and why scaling the deployment does not
  help
- [About why secrets never come from the
  environment](secrets-never-come-from-the-environment.md) — what an
  environment fallback in `mcp.json` would let anyone do

## Working with the agent

Why the agent has the tools and the context it has, and how to decide what to
give it.

- [About modes, tools and the catalog](modes-tools-and-the-catalog.md) — why a
  tool a server exposes may not be offered, and why removing one is not losing
  it
- [About knowledge, rules and skills](knowledge-rules-and-skills.md) — three
  places to write things down, and how to choose between them
- [About LLM endpoints and reasoning replay](llm-endpoints.md) — what `llm.api`
  really selects, and why choosing wrong fails quietly

## Older notes

Two documents predate this set and are kept as they were written:

- [`archdecision.md`](../archdecision.md) — an early component sketch. It names
  NATS, S3 and a multi-provider LLM abstraction, none of which exists. Read it
  as a record of what was considered.
- [`description.md`](../description.md) — the project's original scope note:
  what nib was meant to do, and the workflow it was imagined around.
