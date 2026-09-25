# Getting started: your first plan

In this tutorial we will install nib on our own machine and take one
infrastructure task from a single sentence to a reviewed action plan with a
rollback.

We will not execute anything. Execution acts on real systems, and we want a
plan we can read first.

Allow about twenty minutes, most of it waiting for images to download.

## What we need before we start

Three things:

- **Docker**, with the Compose plugin. Type `docker compose version` in a
  terminal. If it prints a version, we have it.
- **An OpenAI platform API key.** The stack as shipped sends every completion
  and every embedding to `api.openai.com`, so this tutorial will spend a small
  amount of credit there. (nib works with other OpenAI-compatible providers
  too, but that takes a config change we skip here.)
- **`openssl`**, to generate two random keys. Type `openssl version`; macOS and
  most Linux distributions already have it.

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

## Step 3 — Put our keys in it

We need three values in `.env`: our LLM key, a token that guards nib's API, and
a key for nib's secret store. First, generate the two random ones:

```bash
openssl rand -hex 32
openssl rand -base64 32
```

Each command prints one line. The first is our **API token** — keep it
somewhere we can copy it from, because we will paste it into the browser in
step 7. The second is the **encryption key** for stored secrets.

Open `.env` in an editor. Near the middle we will find this block:

```
LLM_API_KEY=
OPENAI_API_KEY=
```

Put our key after the first `=`, with no quotes and no spaces:

```
LLM_API_KEY=sk-our-key-goes-here
```

A little further down are two more empty lines. Put the output of the first
`openssl` command after `NIB_API_TOKEN=` and the output of the second after
`SECRETS_ENCRYPTION_KEY=`:

```
NIB_API_TOKEN=3f9c...the-64-characters-openssl-printed
SECRETS_ENCRYPTION_KEY=q2Xw...the-line-ending-in-=
```

Save the file and close the editor.

Those are the only three values we set. Everything else in `.env` — including
the empty `AGENT_WEBHOOK_TOKEN`, which nib generates for itself — already has a
working default.

Notice that the backend will not start without `NIB_API_TOKEN`: every API
route, and so the web interface, is locked behind it.

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

If `nib-backend` says `Restarting` instead, the most likely reason is a value we
typed in step 3. Run `docker compose logs backend` and look at the last line. It
will say one of:

- `config: llm API key is required (set LLM_API_KEY or OPENAI_API_KEY)` — the
  LLM key did not take
- `config: NIB_API_TOKEN is required ...` or `config: NIB_API_TOKEN must be at
  least 32 characters` — the token is missing or was cut short

Fix `.env`, then run `docker compose up -d` again.

On Docker Desktop the same logs also show `entrypoint: warning:
/var/run/docker.sock is owned by gid 0, joining the root group to reach it`.
That is expected, and not what stops a backend: nib drops root before it
starts, and this line only says which group it needs to reach Docker.

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

The version is whatever `.env` names in `APP_VERSION`.

Those two routes are open to anyone. Everything else needs the token. Ask for
the list of projects without it:

```bash
curl http://localhost:8081/api/v1/projects
```

```
{"error":"unauthorized"}
```

Now with it, replacing `OUR-TOKEN` with the first `openssl` output from step 3:

```bash
curl -s -o /dev/null -w '%{http_code}\n' -H "Authorization: Bearer OUR-TOKEN" http://localhost:8081/api/v1/projects
```

```
200
```

Notice that this is port **8081**, the backend's own port. The web interface is
on a different one, which we open next.

## Step 7 — Open the web interface

In a browser, go to <http://localhost:8080>.

A **Sign in** card asks for the API token this server was started with. Paste
the token from step 3 and press **Sign in**. The browser keeps a session cookie
for 30 days, so we will not be asked again on this machine.

We land on **Workplace**, which is the list of plans. It is empty — we have not
made one yet. Down the left is the sidebar: Workplace, Incidents,
Knowledgebase, Rules, Tools, Skills, Variables, and at the bottom Statistics
and Help. **Help → About Nib** shows the version we just saw in the terminal,
and **Help → Sign out** is how we would leave.

## Step 8 — Tell nib who we are

Before planning, we will fill in one variable, so the plan is written for a
company rather than for nobody.

Click **Variables** in the sidebar. We will see a list of variables, some of
them empty. Type `CompanyName` in the search box and press **Search**. Click the
pencil next to it, type a company name — anything will do, `Acme` is fine — and
save.

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

Within a few seconds the chat starts moving. While the agent works, the chat
shows a short word that changes from turn to turn — *Thinking*, *Planning*,
*Reasoning* — rather than the model's own narration, which nib strips out; only
answers and questions reach the chat. nib has launched its **decompose**
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

Under the **Action List** heading on the plan page is a button, **Process plan
here**. Click it. It sends this into the chat for us:

```
Work the whole DAG into a detailed action plan: plan every stage, then the rollback.
```

The orchestrator launches one planning agent per stage. They run in parallel, and
the chat says the run has started rather than making us wait — each stage
reports back into this chat as it lands.

This takes a few minutes. Notice each stage arriving separately rather than all
at once.

## Step 13 — Read the action list

When the stages have landed, the plan page shows an **Action List**: numbered
steps, grouped by stage, each with the commands or changes it involves, checks
to confirm it worked, and — at the end — a **rollback** list derived from all of
them.

Scroll to a step and notice:

- a checkbox, for marking a step done by hand
- an **Add comment** button, for our own notes
- an **Execute action** button (a play icon) on every step and check
- on each stage header, an **Execute all** button, which would run the stage's
  unticked items one after another

We will not press any of them. An Execute button hands its step to an agent
that acts for real, and our details were invented.

The icon in a step's top-left corner shows its type when we hover over it. A
`code` step — a change to a repository — is different: it runs in a separate
agent container, and a new install has no executor configured to launch one. So
on a code step the Execute button's tooltip reads *Executor is disabled — open
executor settings to enable it*, and pressing it would open **Tools →
Executor** rather than run anything. Setting one up is [How to run planned
actions in containers](../how-to/run-actions-in-containers.md).

At the top of the page are **Finish** and **Cancel**. Finish is for when the
work has been done: it ticks every item, closes the plan and has an agent write
a **Report** card just below the Summary. It also folds what the plan
established into the knowledge-base collection named after the plan's project.
Our plan has done nothing yet, so we leave both alone.

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

We installed nib behind an API token, signed in, gave it one sentence, answered
its questions, and got back a staged plan with an action list and a rollback —
without connecting it to any real infrastructure.

Where to go next:

- [How to connect an MCP server](../how-to/connect-an-mcp-server.md) — give the
  agent real tools, so it plans against what is actually there
- [How a plan is produced](../explanation/how-a-plan-is-produced.md) — what
  those five chats were
- [How to run nib with Docker Compose](../how-to/run-with-docker-compose.md) —
  the install we just did, with the options we skipped
