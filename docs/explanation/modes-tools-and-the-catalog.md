# About modes, tools and the catalog

You register an MCP server, its tools appear under Tools, and the agent still
does not use one of them. Or you remove a tool from a mode and it turns up in a
transcript anyway. Both are the same subject: there are several layers between
"a server exposes a tool" and "this turn is offered that tool", and they answer
to different concerns.

This page is about why those layers exist. The layers themselves, and which tool
is where, are in [Built-in agent tools](../reference/agent-tools.md).

## Why not give the agent everything

The obvious design is to hand the model every tool and let it choose. Two things
break.

**Attention.** A tool catalog is part of every request. A hundred tools is a
hundred descriptions competing with the actual task, and a model given fifty
plausible options picks worse than one given five.

**Authority.** A mode that exists to investigate should not be able to change
things. That is not a prompt-level wish — a prompt asking a model not to use a
tool is a request, not a constraint. If `execute` should not rewrite the rules it
is judged against, the tool has to be absent from the request.

So a turn's catalog is narrowed, twice over: by what the mode is *for*, and by
what the operator has chosen to include.

## Two narrowings, for two different reasons

**Allow lists** are about capability. They are the answer to "what is this mode
permitted to do", they ship with the image, and they are per-mode. `plan` may
write an action plan; `discuss` may not. `discuss` may rewrite a rule; `plan`
may not, because plan mode must read its guardrails, never edit the ones it is
being judged against.

**Included tools** are about noise. Every tool a server exposes is switched on
for every mode as soon as it is indexed — a new server is usable immediately,
with no configuration. Taking tools back out is how you keep the catalog small
once you have several servers.

The two are easy to confuse because both end up as "the tool is not offered".
The difference matters when you want it back: an included-tools removal is a
click, an allow-list restriction is often enforced in code and cannot be
clicked away.

## Removing is not losing

Narrowing would be a much worse trade if a removed tool were unreachable. It is
not: everything outside the offered set stays findable through `tool_search`,
backed by pgvector embeddings of every tool description in the catalog.

This is the design's quiet load-bearing piece. It means the default set can be
small without the agent being stuck when it needs something unusual — the model
searches, finds the tool, and calls it. "Included" is better read as *offered up
front* than as *permitted*.

It also explains why the tool catalog lives in Postgres with embeddings at all,
which otherwise looks like over-engineering for a list of a few hundred strings.

## Reconciliation, and the ledger that makes it possible

When a server is added or removed, the per-mode included lists have to be
brought back into step: new tools on, departed tools off.

That needs a distinction the lists themselves cannot make. A tool absent from
`main`'s included list is either one nobody has ever offered — so it should be
added — or one an operator deliberately took out — so it must stay out. The
lists look identical in both cases.

A ledger of names already reconciled is what separates them. It is why an
install missing that file treats the whole catalog as new and switches
everything on: without the ledger, every exclusion looks like a tool that has
never been seen.

This is a general property of reconciliation: you cannot distinguish "not yet
done" from "deliberately undone" without recording what you have done.

## Why some restrictions are in code

Seeding never removes a name from a list already on a data volume. That rule
exists so an operator's edit is never silently reverted by an upgrade — and it
means a capability *withdrawn* from a mode in a new release stays in the file on
every existing install.

So a withdrawal has to be enforced somewhere that an upgrade reaches. The
orchestrator's tools are stripped in code for exactly this reason, and the
discuss-only tools are guarded both at registration and by a pass over the allow
set.

The result is a rule worth internalising: **the JSON files are the floor, not
the ceiling.** Adding a name to a mode's list may do nothing. Removing one
always works.

## The per-plan third layer

`plan` dialogs get one more source: every tool in the categories chosen during
decomposition. Decomposition works out what systems the task touches, and the
categories it picks travel to every agent in that plan.

This is the part that adapts to the task rather than to configuration. A plan
about Kubernetes gets the Kubernetes tools whether or not they were in the
default set — the alternative being either a permanently wide catalog or an
operator predicting each task's needs in advance.

Categories are matched by patterns, which means they are only as good as the
patterns. A tool nothing claims is uncategorized, invisible to this mechanism,
and reachable only through search.

## What this costs

**Predicting what the agent can do takes three lookups**, not one: the mode's
allow list, the mode's included list, and — for a plan — the chosen categories.
The developer catalog endpoint exists because that calculation is genuinely hard
to do by hand.

**"Why didn't it use that tool?" has several answers.** Not in the allow list;
removed from included; its service is not configured; or it simply was not
chosen. Only the last is a prompt problem, and it is the one people assume
first.

## See also

- [Built-in agent tools](../reference/agent-tools.md) — the matrix, and what each tool does
- [How to connect an MCP server](../how-to/connect-an-mcp-server.md)
- [Modes](../reference/modes.md)
