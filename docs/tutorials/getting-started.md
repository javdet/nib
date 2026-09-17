# Getting started: your first plan

In this tutorial we will install nib on our own machine and take one
infrastructure task from a single sentence to a reviewed action plan with a
rollback.

We will not execute anything. Execution needs a container runtime and
credentials of its own, and we want a plan we can read first.

Allow about twenty minutes, most of it waiting for images to download.

## What we need before we start

Two things:

- **Docker**, with the Compose plugin. Type `docker compose version` in a
  terminal. If it prints a version, we have it.
- **An API key for an OpenAI-compatible LLM endpoint** — an OpenAI platform key
  or an OpenRouter key. nib sends every completion and every embedding there,
  so this tutorial will spend a small amount of credit.

We do not need Go, Node, Postgres or Kubernetes. Everything runs in containers.

## Step 1 — Get the files

In a terminal:

```bash
git clone https://github.com/javdet/nib.git
cd nib
```

We now have a directory called `nib`. Everything else in this tutorial happens
inside it.

## Step 2 — Make our own settings file

```bash
cp .env.example .env
```

This copies the example settings to `.env`, which is the file nib actually
reads. Nothing has changed yet — we have only made ourselves a copy to edit.

## Step 3 — Put our key in it

Open `.env` in an editor. Near the middle we will find this block:

```
LLM_API_KEY=
OPENAI_API_KEY=
```

Put our key after the first `=`, with no quotes and no spaces:

```
LLM_API_KEY=sk-our-key-goes-here
```

Save the file and close the editor.

This is the only value we have to set. Everything else in `.env` already has a
working default.

## Step 4 — Check what we are about to start

Before starting anything, we can see what the stack contains:

```bash
docker compose config --services
```

The output is exactly four lines:

```
postgres
kb-mcp
backend
frontend
```

Those are the four containers: a database, a knowledge-base search server, the
backend, and the web interface. Notice that there are only four — nib needs
nothing else.

## Step 5 — Start it

```bash
docker compose up -d
```

The first run downloads four images, which takes a few minutes. The `-d` means
the containers keep running after we get our prompt back.

When it finishes, check what is running:

```bash
docker compose ps
```

We should see all four, each `Up`:

```
NAME            SERVICE     STATUS
nib-backend     backend     Up 2 minutes (healthy)
nib-frontend    frontend    Up 2 minutes (healthy)
nib-kb-mcp      kb-mcp      Up 2 minutes
nib-postgres    postgres    Up 2 minutes (healthy)
```

If `nib-backend` says `Restarting` instead, the most likely reason is the key we
typed in step 3. Run `docker compose logs backend` and look at the last line: it
will say `config: llm API key is required (set LLM_API_KEY or OPENAI_API_KEY)`
if the key did not take. Fix `.env`, then run `docker compose up -d` again.

## Step 6 — Confirm the backend is answering

```bash
curl http://localhost:8081/api/v1/health
```

The output is:

```
{"status":"ok"}
```

And to see which version we installed:

```bash
curl http://localhost:8081/api/v1/version
```

```
{"version":"v0.8.0"}
```

Notice that this is port **8081**, the backend's own port. The web interface is
on a different one, which we open next.

## Step 7 — Open the web interface

In a browser, go to <http://localhost:8080>.

We land on **Workplace**, which is the list of plans. It is empty — we have not
made one yet. Down the left is the sidebar: Workplace, Incidents, Knowledge
Base, Rules, Tools, Skills, Variables, Statistics. At the bottom of the sidebar
is the version we just saw in the terminal.

## Step 8 — Tell nib who we are

Before planning, we will fill in one variable, so the plan is written for a
company rather than for nobody.

Click **Variables** in the sidebar. We will see a list of variables, some of
them empty. Find the one called `CompanyName`, click it, type a company name —
anything will do, `Acme` is fine — and save.

Notice that the variable list does not let us delete `CompanyName`. A handful of
variables are built in and can only be re-valued, because the shipped prompts
and skills reference them.

## Step 9 — Start a plan

Click **Workplace** in the sidebar, then the **New plan** button.

A chat opens. In the message box, type one concrete task and send it:

```
Rotate the TLS certificate on our staging ingress before it expires.
```

Notice the mode shown in the top right: **Main**. This is the orchestrator, and
it is the only mode a plan can start in. It will not do the work itself — it
launches a specialist and relays what comes back. More on that in [How a plan is
produced](../explanation/how-a-plan-is-produced.md).

## Step 10 — Answer its questions

Within a few seconds the chat starts moving. nib has launched its **decompose**
specialist, and that specialist will almost certainly come back with questions —
which ingress controller, which cluster, where the certificate comes from.

The questions appear in our chat with suggested answers we can click, and a box
where we can type our own. Answer them. If we do not have a real staging
cluster, we can invent details freely; nib is planning, not connecting to
anything.

Notice that the chat pauses and waits. The specialist is suspended mid-thought
until we answer, then carries on from where it stopped.

We may be asked a second round of questions. Answer those too.

## Step 11 — Read the summary and the diagram

When decomposition finishes, the plan page fills in above the chat:

- a **Summary** — a paragraph describing the change in our own terms
- a **DAG** — a diagram of the stages, drawn as boxes with arrows, showing which
  stages depend on which

Click through the diagram. Notice that the stages are still coarse: "obtain the
certificate", "update the secret", "verify". Nothing is a command yet. This is
what decomposition produces — the shape of the work, not the work.

## Step 12 — Ask for the detailed plan

Back in the chat, type:

```
Now plan this out in detail.
```

The orchestrator launches one planning agent per stage. They run in parallel, and
the chat says the run has started rather than making us wait — each stage
reports back into this chat as it lands.

This takes a few minutes. Notice each stage arriving separately rather than all
at once.

## Step 13 — Read the action list

When the stages have landed, the plan page shows an **Action list**: numbered
steps, grouped by stage, each with the commands or changes it involves, checks
to confirm it worked, and — at the end — a **rollback** list derived from all of
them.

Scroll to a step and notice:

- a checkbox, for marking a step done by hand
- a comment field, for our own notes
- on some steps, an **Execute action** button, greyed out

It is greyed out because we have not configured an executor. That is deliberate:
until we set one up, nib plans work and hands the plan to us. Setting one up is
[How to run planned actions in
containers](../how-to/run-actions-in-containers.md).

We now have what we came for: one sentence turned into a reviewed, numbered plan
with checks and a rollback.

## Step 14 — Tidy up

Our plan is saved. We can close the browser and come back to it later.

To stop the containers without losing anything:

```bash
docker compose stop
```

`docker compose start` brings them back with every plan intact.

To remove the containers but keep our data:

```bash
docker compose down
```

Do **not** add `-v` to that command unless we want the plan we just made, and
everything else on the install, deleted.

## What we did

We installed nib, gave it one sentence, answered its questions, and got back a
staged plan with an action list and a rollback — without connecting it to any
real infrastructure.

Where to go next:

- [How to connect an MCP server](../how-to/connect-an-mcp-server.md) — give the
  agent real tools, so it plans against what is actually there
- [How a plan is produced](../explanation/how-a-plan-is-produced.md) — what
  those five chats were
- [How to run nib with Docker Compose](../how-to/run-with-docker-compose.md) —
  the install we just did, with the options we skipped
