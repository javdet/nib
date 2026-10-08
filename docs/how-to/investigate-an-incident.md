# How to investigate an incident

nib investigates an incident, ongoing or past, and fills in its post-mortem as it goes: what broke,
where, since when, how widespread it is, why, and what to do about it. The corrective actions you
pick become a plan on the change-management track.

## Before you start

The investigation is only as good as the tools it can reach. It reads your systems through **MCP
servers** — metrics, logs, traces, error tracking, deployment history, version control, cluster and
cloud state — so connect the ones you have ([How to connect an MCP server](connect-an-mcp-server.md))
and sort their tools into **tool categories** (**Tools → Categories**). Each Digger gets the tools of
the categories named for its system, plus the MCP tools included for incident mode (**Tools →
Included tools**). A knowledge-base document describing your services and their dependencies is what
the agent uses to decide which systems to examine.

Local commands (`execute_command`, `api_call`) run inside the nib backend, with no `KUBECONFIG` and no
cloud credentials, so they are good for a reproduction against a public endpoint and little else.

## Start an incident

1. Open **Incidents** and press **New incident**.
2. In the chat, describe the problem, or paste the alert or the bug ticket as it is: labels,
   timestamps and error messages are what the agent works from. Attach screenshots if you have them.

The incident commander triages first. It decides whether the incident is still going, sets the
detection and start times, the environment and a severity (`sev1`–`sev4`), and writes the summary.
You watch the post-mortem fill in on the left while it works.

## The investigation

When the commander knows which systems are involved, it calls `run_investigation`. That starts, in the
background:

1. one **Digger** per system, all at once (at most eight), each comparing its system's state before
   and during the incident, looking for what changed, and filing a finding;
2. a **synthesis** agent, which turns the findings into the working hypothesis, the root causes, one
   merged timeline and the corrective actions;
3. an **impact** agent, beside it, which measures who and what was affected.

The progress card lists each agent; **Transcript** opens what it did in the chat panel, and **Stop**
ends the run, keeping whatever was already written. When the run ends its outcome is posted into the
incident's chat. Ask follow-up questions there — or ask for another system to be examined.

Every agent is told to **only read**: never apply, delete, restart, scale or roll anything back. That
rule is in their instructions, not enforced in code, so give nib's MCP servers read-only credentials
where you can.

## Edit the post-mortem

Every section is editable. A field you edit is marked **Edited by you**, and no agent overwrites it
from then on; they propose changes in the chat instead. Click the badge to hand the field back. List
items you add or edit stay whatever an agent later writes.

**Download post-mortem as Markdown** (the document icon) saves it as a file.

## Hand corrective actions to a plan

1. Tick the short-term and long-term actions you want carried out.
2. Press **Create plan**.

A new plan opens, already carrying the incident's summary, root cause and the actions, and its
orchestrator starts decomposing them. The plan shows **From incident …**, and the actions on the
incident show **In plan**, linking to it. Plan and run it as any other plan.

## Close the incident

Press **Close incident** and, if it differs from now, give the time it was resolved. The impact is
assessed once more over the whole incident, then a closure summary is written: what happened, why,
how it was resolved, the final impact, the follow-ups and the lessons learned. The incident is
marked **closed** only when the summary is written; if that fails, it stays open and you can press
the button again. A closed post-mortem can still be edited by you, never by an agent.

## Tuning

`agent.incidentConcurrency`, `incidentDiggerMaxIterations`, `incidentStepMaxIterations` and
`incidentTimeoutMinutes` in `config.yaml` bound a run — see
[Configuration](../reference/configuration.md#agent). The tools and routes involved are in
[Built-in agent tools](../reference/agent-tools.md#incidents) and the
[HTTP API](../reference/http-api.md#incidents).
