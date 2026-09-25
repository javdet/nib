# About how a plan is produced

Type one sentence into a new plan and you get back five chats, a diagram, a
numbered action list and a rollback. Why five chats? Why does the plan page
sometimes say "started" and go quiet for twenty minutes?

This page is about the orchestration that produces a plan: what runs where, why
the work is split up at all, and which of the splits are load-bearing. It is
not about how to drive it — for that, open a plan and read the chat.

## The problem with one long conversation

The obvious design is a single conversation: describe the task, let the model
think, get a plan. It fails for a boring reason. A real infrastructure change
has five or ten stages, each needing its own research — which cluster, which
version, which secret path — and a single conversation that researches all of
them in sequence either runs out of round budget or produces a plan whose later
stages are visibly thinner than its earlier ones, because the context is full of
the earlier ones.

So nib splits the work. Decomposition decides *what the stages are*; a separate
agent per stage decides *what each stage does*; a further agent derives the
rollback once the stages exist. Each gets a fresh context and its own budget.

The cost of that split is the thing the rest of this page is about:
sub-agents cannot talk to each other, so everything they have to agree on has to
be made explicit somewhere.

## The orchestrator does no work

A plan starts in `main`, which is the only mode the **New plan** button creates.
`main` never researches, never writes a plan, never runs a command. It decides
which specialist a request needs, launches it, relays any question back to you,
and reports each result in the same chat.

That is why you only ever talk to one chat even though five exist: the others
are reachable, but they report into this one.

Three tools belong to the orchestrator alone — `run_subagent`,
`stop_execution` and `execute_action` — and the reasons are each specific:

- **`run_subagent`** — a sub-agent that could spawn sub-agents would have no
  chat to report into. The reporting machinery is rooted at the orchestrator.
- **`stop_execution`** — a sub-agent holding the execution lease would be
  stopping itself.
- **`execute_action`** — a sub-agent given one row must not hand out more work
  while the orchestrator holds the single execution slot on its behalf.

What makes these enforceable is a small thing: a sub-agent runs with its
transcript in one dialog and its plan artifacts in another, and that mismatch is
what marks a turn as *not* the orchestrator's. The tools are registered only
when the two match. A second guard strips them from every sub-agent's tool set
regardless — because tool allow lists live on the data volume and are never
pruned, so an install upgraded from an older release still has stale entries in
its files.

## Blocking and non-blocking, and why it matters

Decomposition runs **inside** the orchestrator's turn. The other two start and
return immediately.

The reason is the HTTP write deadline. An agent route holds its response open
for up to thirty minutes; a fan-out can take an hour and an action half of one.
Neither fits, so both answer "started" and report back through the same named-
message and SSE machinery the orchestrator already uses.

Decomposition fits because it is short — but being synchronous creates its own
problem. Decomposition is a *conversation*: it asks which environment, which
repository, which of two readings of the task you meant. A blocking sub-agent
cannot ask you anything, because you are waiting on the call that contains it.

So it suspends. The pause is written to a file, the orchestrator relays the
questions as a question of its own, and your answers are routed back into the
sub-agent's dangling tool call. It resumes where it stopped. This is the most
complicated part of the system, and it exists entirely to make one blocking
call behave like a conversation.

A stage planner cannot do the same. Its turn holds state that is not on disk,
and the wave it belongs to is waiting on its goroutine, so it blocks inside the
`report_blocker` call instead: the question goes into the chat naming its stage,
and the answer comes back as the call's own result. Only that stage waits. The
stages beside it keep planning, and a second question queues until the first is
answered, because the chat can only show one at a time.

That queue is a one-slot desk per plan rather than a free-for-all because the
chat renders only the newest unanswered question. Two posted side by side would
leave the older one with no way for anyone to answer it — the stage behind it
would wait for an answer that could not arrive.

Blocking inside a call raises a question the suspend design never had to
answer: what does the fan-out's timeout measure? A run's budget is meant to
bound what nib does, not how long you take to reply, so it is charged by a
watchdog that stops the clock while any stage waits on you. A question queued
behind another stage's counts as waiting too. Two further limits keep one
confused planner from holding the chat hostage: a stage may ask only a couple
of times, and a single wait is capped at a couple of hours. Either way the tool
then answers with the assumption the agent stated when it asked, so the stage
is still written — on a stated guess you can see, rather than with a gap.

**What you see:** a plan that goes quiet after "started" is a fan-out or an
action running, and it will report when it lands. A plan that asks you a
question mid-decomposition is a suspended sub-agent, and the plan does not move
until you answer. A question naming a stage is that stage's planner waiting on
you — it is marked "waiting for you" in the stage list, and the rest of the plan
carries on around it.

## Everything belongs to the root

A sub-agent's transcript is its own. Every plan artifact — the summary, the DAG,
the contract, the action plan, the report, the categories chosen during
decomposition — belongs to the **root** dialog, whichever agent wrote it.

This split is what makes the plan a single coherent object rather than a
scattering of per-agent notes. Every sub-agent is parented straight to the root,
so listing the root's children is exactly "everything this plan spawned", and
the tool categories picked during decomposition are inherited by every agent
that follows.

It also means "which dialog is this for?" has two answers at every call site,
and getting them the wrong way round writes a stage planner's summary over the
plan's. That ambiguity is intrinsic to the design; the code carries an explicit
pair of identifiers rather than pretending there is one.

## How sub-agents agree without talking

Two mechanisms, for two different problems.

**Before the fact: the contract.** Decomposition writes a cross-stage contract —
every identifier, secret path, endpoint, namespace and version that more than
one stage has to agree on. Stage planners run in parallel and cannot see each
other's output, so anything shared has to be decided before they start. A stage
planner that invents its own name for something two stages touch produces a plan
that does not compose.

**After the fact: notes.** When an action finishes, its final message is
recorded in a side file and handed to the agents that run later actions. That is
how a resource id created by action 3 reaches action 7. It is the executors' own
memory, deliberately unreachable from the API, and deliberately kept out of the
plan chat — a successful action shows as *done* with a link to its transcript,
because a wall of successful-run prose buries the one failure you need to see. A
failure, a question or a cancellation still says why, in the chat.

Because the notes are fed straight into later agents, what goes into them is
trimmed: a `<thinking>` block the model opened its reply with — which the plan
prompts ask for, and some models emit unprompted — is stripped from the final
answer before it is recorded. Scratch reasoning addressed to nobody would
otherwise become part of every later action's context.

## Execute all goes around the orchestrator

**Execute all** on a stage header (or the rollback's) runs the stage's unticked steps, then its
checks, one after another. It is the one way of running actions that does not
go through the orchestrator, and that is deliberate rather than a shortcut.

An action's result is posted into the plan chat *without* starting a turn —
that is what keeps a finished action from setting the orchestrator talking. The
consequence is that a model asked to "run these five in order" has nothing to
wake it when the first one lands. So the sequencing is done in code: every way
an item can end — the sub-agent finishing, the container's webhook, the lease
expiring — calls the same advance step, which starts the next one.

Two details keep that honest. The advance runs only *after* the execution
lease has been released, or the next item would be refused as busy by the
item that just finished. And it is idempotent, keyed on the item and the
attempt it launched, because a webhook can be delivered twice and a force stop
races the stopped goroutine's own finish.

The run stops rather than carrying on when anything makes "the next item" mean
something you did not ask for: an item that does not end *done*, a busy slot, a
force stop, a reorder within the stage, a change in the stage's shape, a
replaced plan, a restart. Rows are addressed by position, so a stage whose
items have moved would otherwise run a different row under the same number.
Rewording an item does not count as a change of shape; only its title and the
item counts do. Like the lease, only one stage run exists at a time across
every plan.

## Code fixes asked for in the chat

A code action pushes a branch; you read it, and it is wrong. The fix is not a
row of the plan, and replanning a stage to get one would throw away work that
was mostly right. So the orchestrator can launch a `code` sub-agent directly —
the same agent-runner container a code action uses, carrying your instructions
instead of a plan row.

Where the fix lands is decided by the branch. Naming the branch a code action
pushed to continues it: the container checks the existing branch out and adds
to the pull request already open on it. Naming none starts a branch of its own,
deliberately not the plan's, so that a fix meant as a separate change does not
rewrite an action's pull request by accident.

Having no plan row is also what makes a fix awkward to track. The container
outlives the process that launched it and reports back through the webhook, so
the backend needs something durable to recognise it by — a small record per
plan, beside the plan's other artifacts, which the restart sweep treats
exactly like an action's. The existing `run_executor` tool could not stand in:
its request carries no chat id, so a container it starts has nowhere to report,
which is also why it is withdrawn from the action sub-agents.

## The report is the exception

Pressing **Finish** launches one more sub-agent — the only one you start
directly. It reads the finished plan and writes a closing report.

It deliberately does **not** take the execution lease. It changes no managed
system; it reads a plan and writes prose. Holding it to the one-execution rule
would mean a plan finished while a last action was still running silently loses
its report. A per-plan claim and an "already written" check are what stop it
running twice instead.

Its `set_report` tool is in no mode's allow list at all. The per-action
executors run in `execute` mode too, and a mode allow list cannot tell a report
agent from an action agent — so the report agent is handed a literal two-tool
catalog, and the name is deleted from the action agents' set as well. Where a
capability distinction does not fall along mode lines, the mode list is the
wrong instrument.

## Finishing also teaches the knowledge base

When the report agent ends, it chains a second agent that folds what the plan
established back into the knowledge base collection named after the plan's
project. It is chained rather than run beside the report because the report is
already the deduplicated account of what the plan did; deriving it a second
time, independently, would only produce two accounts that disagree.

Which project that is cannot be read at Finish. The header selection is global
and in memory, so by the time you finish a plan it names whatever you selected
last. The plan's selection is therefore recorded on its first turn, and the
update agent reads that. A plan whose project is `any` is skipped rather than
written into a default collection — the write replaces a collection whole, and
overwriting the shared one because a plan named no project is the worse
mistake.

Like the report agent it takes no execution lease: it rewrites nib's own
knowledge base, never a managed system. Unlike the report agent it does take a
lock, on the collection, for the whole run. Its write is read, merge,
overwrite, so two plans finishing against one project would otherwise each
publish a document built from the text before the other started. It waits for
the lock rather than skipping, because nobody is waiting on it.

A run that decides the plan taught nothing new records that and is not
repeated on a second press of Finish; a run that failed records nothing and
stays retryable. The whole mechanism is switched by **Knowledge Base →
Automatic updates**, on by default. Its side of the story is in [Knowledge,
rules and skills](knowledge-rules-and-skills.md#knowledge-that-updates-itself).

## What this design gives up

**Determinism.** Five agents with independent contexts produce a plan whose
consistency rests on the contract and on the prompts, not on a schema. Two runs
of the same task can differ.

**A tidy mental model.** The transcript/plan split, the suspend-and-resume, the
blocking question with its paused clock, the lists that are enforced twice — these are the kind of thing that is obvious once
known and invisible before. This page exists because the design is not
self-evident from the interface.

What it buys is that each stage gets a full context to think in, and that a
change to one stage can be replanned without touching the others.

## See also

- [Modes](../reference/modes.md) — the six modes and the sub-agent registry
- [One execution at a time](one-execution-at-a-time.md) — why only one action runs
- [Modes, tools and the catalog](modes-tools-and-the-catalog.md)
