# About one execution at a time

You press **Execute action** on a second plan and nib refuses, naming what is
already running. It does not queue the request. On a tool whose whole promise is
automating infrastructure work, a hard limit of one concurrent action looks like
an unfinished feature.

It is a policy, and this page is about why it is one, what it costs, and what
would have to change for it to be lifted.

## The argument for refusing

An action is not a computation. It applies a change to a real system — creates a
cluster, rotates a secret, pushes a branch, runs a migration. Two of them
running at once against infrastructure that nobody has proven to be independent
is how you get a half-applied change nobody can describe.

The model cannot tell you whether two actions are independent. It can tell you
they look independent, which is not the same claim, and the cost of being wrong
is asymmetric: serialised actions are slow, concurrent conflicting actions are
an incident.

So the lease is global — across every plan, not per plan. Two operators working
on two unrelated plans still serialise. That is the conservative reading, and
it is the deliberate one.

## Why refuse rather than queue

A queue would be friendlier. It is also a promise nib is not in a position to
keep.

A queued action would sit behind work that might take half an hour, might fail
halfway, and might change the state its own plan was written against. By the
time it ran, its plan could be stale in ways nobody reviewed. Refusing puts the
decision back where it belongs — you see what is running, and you decide whether
to wait, stop it, or do something else.

The refusal names the holder for exactly that reason. "Busy" would be useless;
"plan X is executing step 4" is something you can act on.

`agent.actionExecConcurrency` exists in the config and is *clamped to one*. A
higher value is accepted, logged, and ignored. That is an unusual choice —
normally you would reject the value — and it is a bet that an operator who set
it to 4 would rather have a working install with a warning than a backend that
will not start.

## What the lease covers

Action sub-agents, code-action containers, and code fixes asked for in the
chat. An agent reasoning its way through a step and a container pushing a
branch are the same kind of risk, so they share the slot — and a fix is a
container pushing a branch like any other, even though no plan row stands
behind it.

That last case is marked on the lease as a fix and carries no row key. The mark
is what keeps the force-stop and expiry paths from writing an outcome against a
plan row that does not exist; they close the fix's own record instead, and
report into the chat that asked for it.

Two agents deliberately stay outside it: the one that writes the report when
you press **Finish**, and the one that then updates the knowledge base. Neither
changes a managed system — one writes prose, the other rewrites nib's own
knowledge base — and holding them to the rule would mean a plan finished while
its last action was still running silently loses its report. What stops them
running twice is a per-plan claim, not the lease.

The force-stop route deliberately carries **no write deadline**. Every other
agent route does, because a request that runs forever is a bug. This one is
different: it exists to be reachable precisely when the agent loop holding the
lease is wedged, which is when a deadline would be most likely to fire.

Cancelling means cancelling a sub-agent's context, or stopping a container
through the runtime. Both leave a record.

## Execute all is not a queue

**Execute all** on a stage looks like the queue this page argues against: press
once, and several actions run. The difference is what each one is started
against. A queued request waits behind *someone else's* work, for a slot, with
a plan that may have gone stale. A stage run starts its next item only when the
previous one of *the same stage* has finished, and only if it finished *done*.

It takes the lease one item at a time, like any other request, and moves on
only after the finished item has released it. It is refused a busy slot like
anyone else, and it stops — rather than waits — when that happens, when an
item fails, or when the stage is restructured under it. And, for the same
reason as the lease, there is one stage run at a time across every plan.

## The failure mode it introduces

A lease held by a process that dies is a lease nobody can release. Without a
sweep, one crash mid-execution would block every execution on the install,
permanently, until someone found the record.

So a reconciliation pass runs at startup over the runs a previous process left
marked running — actions, chat-requested fixes and stage runs alike. The metric `nib_stuck_runs_reconciled_total` above zero after a
restart is that pass doing its job — which also makes it a useful signal that
the process died mid-run rather than shutting down cleanly.

The restart is not the only way a holder vanishes. A container reports back
through a webhook, and a crashed image, a failed pull or a node that went away
means nothing ever will — with no goroutine left in the backend to notice. So a
container's lease is treated as expired once the action deadline has passed —
a little longer than the container's own timeout — the next time anything asks
for the slot. That is the one case where
the lease is released without anyone saying how the run ended.

This is the general shape of the trade: a global lock is simple to reason about
and needs a recovery path for every way the holder can vanish.

## Why it cannot simply be lifted

The lease is process-local. So is the SSE broker, so is the stage-run slot,
so is every other lock, and so
is the background refresh that counts plans by status. The Helm chart pins the
backend to one replica, and that pin is what makes all four correct.

Run two replicas and each gets its own lease. Two actions run at once, neither
knowing about the other — the exact outcome the policy exists to prevent, now
arrived at silently. The SSE broker splits the same way: a browser connected to
pod A never sees events published on pod B.

This also makes three metrics ambiguous under replication.
`nib_execution_lease_held`, `nib_sse_subscribers` and `nib_plans` are
unambiguous today only because there is one process. With several they become
per-pod values that must not be summed.

Lifting the limit is therefore not a config change. It means moving the lease
into Postgres — an advisory lock or a row with a heartbeat — and the broker onto
something shared. Both are ordinary pieces of work; neither has been done,
because one replica has been enough.

## Is one replica a problem?

For availability, mostly not: nib is an operator's tool, not a request-serving
service, and a restart loses at most an in-flight agent turn, which the
reconciliation pass tidies. Plans, transcripts and artifacts are durable.

For throughput, it depends on what you expected. If you thought nib would run
twenty changes across twenty plans at once, it will not, and it will not be made
to by scaling the deployment. If you expect it to plan continuously and execute
one thing at a time under supervision, the limit will not be what you notice.

## See also

- [How a plan is produced](how-a-plan-is-produced.md)
- [How to run planned actions in containers](../how-to/run-actions-in-containers.md)
- [Metrics](../reference/metrics.md#execution)
- [About the architecture](architecture.md#what-this-architecture-cannot-do)
