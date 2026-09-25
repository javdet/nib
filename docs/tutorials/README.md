# Tutorial

A tutorial is a lesson. It takes you through one complete piece of work from
beginning to end, and everything in it is chosen so that it will work — no
options, no branches, nothing left to judgement.

There is one tutorial, and it is the right first thing to read about nib.

## [Getting started: your first plan](getting-started.md)

We install nib with Docker Compose, give it one sentence describing an
infrastructure task, answer the questions it asks back, and end with a staged
plan: a summary, a diagram of the stages, a numbered action list with checks,
and a rollback.

On the way we lock the API behind a token, sign in to the web interface with it,
and see — without pressing them — the buttons that would execute a step, run a
whole stage, or finish the plan with a report.

It takes about twenty minutes, most of that waiting for container images. It
needs Docker, `openssl` and an OpenAI platform API key — nothing else, and in
particular no real infrastructure to point at. We stop before executing
anything.

## After the tutorial

Once a plan exists and you want nib pointed at something real, the [how-to
guides](../how-to/) pick up from there — connecting tool servers, configuring
the executor, and the rest.

If instead you finish the tutorial wondering *why* one sentence became five
chats, that question is answered in [How a plan is
produced](../explanation/how-a-plan-is-produced.md).
