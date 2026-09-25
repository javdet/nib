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

It means the default set can be small without the agent being stuck when it
needs something unusual — the model searches, finds the tool, and calls it.
"Included" is better read as *offered up front* than as *permitted*.

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

The same holds where a distinction does not fall along mode lines at all. The
per-action executors run in `execute` mode, and so do the agents launched when a
plan is finished. A mode list cannot tell them apart, so the executors' set is
narrowed in code: an agent carrying out one row of someone else's plan loses
every tool that could rewrite that plan, suspend the turn, spawn another agent,
close the plan with a report or overwrite a knowledge base collection — and
`run_executor`, whose containers would have no chat to report into. The
finishing agents go the other way: they are handed a literal list of two or
three tools, so an edit to `execute.json` cannot widen them.

The result is a rule: **the JSON files are the floor, not the ceiling.** Adding
a name to a mode's list may do nothing. Removing one always works.

## The per-plan third layer

`plan` dialogs get one more source: every tool in the categories chosen during
decomposition. Decomposition works out what systems the task touches, and the
categories it picks travel to every agent in that plan.

This is the part that adapts to the task rather than to configuration. A plan
about Kubernetes gets the Kubernetes tools whether or not they were in the
default set — the alternative being either a permanently wide catalog or an
operator predicting each task's needs in advance.

An action sub-agent is narrower still. It gets the categories the planner put
on *its* action, not the ones chosen for the plan as a whole: an action that
runs one `kubectl` command has no use for the task tracker a sibling needed, and
a catalog cannot be narrowed once the turn has started.

Categories are matched by patterns, which means they are only as good as the
patterns. A tool nothing claims is uncategorized, invisible to this mechanism,
and reachable only through search.

## The backend's own hands

Two built-in tools do not go through an MCP server at all: `execute_command`
runs one binary on the backend's host, and `api_call` runs `curl`. They are
what lets the agent check a version or hit an internal API without anyone
writing a server for it, and they are also the two tools whose risk lands on
nib itself rather than on the systems it manages. Their limits follow from that.

**Not a shell.** One binary through argv, no pipes, redirects, `&&`, globs or
variable expansion. A shell would turn every argument into a program, and the
arguments are the model's.

**No inherited environment.** Both run with `PATH`, `HOME` and `LANG` only. The
backend's environment holds its own keys, and a child's output goes to the
provider and the transcript, so an inherited environment would let `env` print
them. The cost is that nothing else a CLI might read — proxy settings,
`KUBECONFIG` — comes through either.

**`api_call` is an allow list, not a blocklist.** curl can read any file
(`-d @file`, `-K`), write any file, talk to the Docker socket, and reroute its
own connection. So only listed options pass, every value curl would treat as a
file name is refused, and an unknown option is refused rather than passed — a
blocklist would open up silently with every curl release. It does not follow
redirects, because a redirect is a second request to a host nobody vetted; the
agent reads `Location` and calls that URL itself.

**The host is vetted before curl runs.** The name is resolved, and if *any*
answer is loopback, link-local or a cloud metadata address, the call is
refused — any, not the first, since a name answering with both would otherwise
be a coin toss. curl is then pinned to the address that was checked, so a
second DNS answer cannot move it. Private ranges are allowed on purpose:
reaching internal APIs is what the tool is for.

None of this applies to `execute_command`, which can run `curl` itself. The
guard on `api_call` is what lets it be offered freely; `execute_command` is the
one to take out of a mode you do not trust. And because both run whatever the
model asks with the backend's own identity, the backend does not run as root:
the image holds root only long enough to hand over a root-owned volume and
join the group that owns the Docker socket, then drops to an unprivileged user
with no capabilities and no way to regain them.

## What this costs

**Predicting what the agent can do takes three lookups**, not one: the mode's
allow list, the mode's included list, and — for a plan — the chosen categories.
The developer catalog endpoint exists because that calculation is hard to do by
hand.

**"Why didn't it use that tool?" has several answers.** Not in the allow list;
removed from included; its service is not configured; or it simply was not
chosen. Only the last is a prompt problem, and it is the one people assume
first.

## See also

- [Built-in agent tools](../reference/agent-tools.md) — the matrix, and what each tool does
- [How to connect an MCP server](../how-to/connect-an-mcp-server.md)
- [Modes](../reference/modes.md)
