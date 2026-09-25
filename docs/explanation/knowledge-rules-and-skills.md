# About knowledge, rules and skills

nib gives you three places to write things down, and they look alike: all three
are text you author, all three end up in front of the model, and all three
accept Markdown. Choosing wrongly is the most common way to end up with an agent
that knows about your infrastructure and still plans badly.

They differ in *when* the text reaches the model, and that difference is what
should decide where something goes.

## The three, by timing

**Knowledge** is retrieved. It is your documentation, chunked, embedded and
stored as vectors; the agent searches it with a query and gets back the closest
few chunks. It reaches the model only when something the agent is thinking about
resembles it.

**Rules** are injected. During decomposition the agent works out which systems
the task touches; where a rule exists for one of them, its text is added to the
prompt during detailed planning. It reaches the model because the *subject*
matched, not because a search did.

**Skills** are requested. Every enabled skill's name and description are listed
at the end of the `main` and `discuss` prompts, and the agent loads the full text
of one on demand. It reaches the model because the agent decided it needed that
procedure — or because you told it to with a `/name` command.

So: knowledge is *what is true here*, rules are *what we require*, skills are
*how we do a particular thing*.

## Choosing between them

The useful test is what happens when the text is wrong.

**If wrong text means a wrong fact** — the cluster is in `eu-central-1`, the
registry is at this address, this service owns that database — it is knowledge.
Facts are numerous, change independently, and are needed unpredictably. Search
is the right retrieval model, and being occasionally missed is acceptable
because a missing fact usually surfaces as a question.

**If wrong text means a wrong decision** — changes to production go through a
merge request, database migrations are never rolled forward without a rollback,
this region is not used for new work — it is a rule. A rule that is *sometimes*
retrieved is worse than no rule, because you will believe it is in force. That
is why rules are matched by subject and injected rather than searched.

**If wrong text means a wrong procedure** — how to build a knowledge base for a
project, how to categorise tools, how to pull a board from the issue tracker —
it is a skill. Procedures are long, and only one is relevant at a time. Listing
names and loading bodies on demand is what keeps them from crowding out the
task.

The most common mistake is writing a rule as knowledge. It will be found
sometimes, which is worse than never.

## Knowledge that updates itself

Knowledge is the one surface nib writes to on its own. Finishing a plan folds
what it established — a new cluster, a changed endpoint, a version that moved —
into the collection named after the plan's project. The reasoning is that a
knowledge base only you maintain goes stale at exactly the rate you use nib to
change things, and the plan's report is already a written account of those
changes.

The write is deliberately blunt: a collection is replaced **in full** — every
chunk deleted, the whole document re-embedded, the file rewritten. There is no
patch operation: a collection is ingested from one document, the same file an
upload replaces, and keeping that document the source of truth is what lets a
person read, diff and re-upload it. So whoever writes — the update agent, or you in `discuss` mode with
`get_kb_document` and `update_kb` — reads the current document, merges, and
sends the whole thing back. That is also why the agent holds a lock on the
collection while it works: two merges started from the same text would each
erase the other's addition.

Three consequences follow from that bluntness:

- **A plan with no project writes nothing.** Replacing the shared default
  collection because a plan did not say which project it was about would be the
  worst possible target for a full overwrite.
- **The write tool is narrow.** `discuss` is the only mode whose list carries
  `update_kb`; the update agent is handed it in a literal three-tool catalog of
  its own, and the per-action executors — which run in `execute` mode, like the update agent —
  have it withheld in code, since a mode list cannot tell the two apart.
- **It can be switched off.** **Knowledge Base → Automatic updates** is on by
  default. Turn it off if the documents in a collection are curated by hand and
  you would rather nothing merged into them.

## Why rules are write-protected from planners

`discuss` mode can create and replace rules. Every other mode has the tool
withdrawn, whatever its allow list says.

The reason is not that planners would write bad rules. It is that a planner
editing the rules it is planning against has no independent standard left: the
constraint and the thing constrained become the same agent's output. The
guardrail has to come from outside the run it guards.

This is also why writing a rule replaces the whole file rather than patching it:
composing frontmatter and body from parts invites half-written rules, and a
half-written rule is exactly the "sometimes in force" case above. Editing one
means reading it first and passing the merged text back.

## Skills, and the two you cannot touch

Built-in skills are copied out of the image onto the data volume once, and
tracked. After that they are yours: an edit, a rename or a deletion survives
every upgrade, and a skill added in a later release still lands on an existing
install.

That asymmetry is intentional and differs from how prompts work. There is no
default-versus-override split for skills — once seeded, the volume owns them. It
means you never have to fight an upgrade to keep a change, and it means you will
not receive improvements to a built-in you have edited.

Two skills work differently. `nib-configuration` and `nib-internals` are served
from the binary, are not written to the volume, and do not appear in the Skills
page or the HTTP API at all. They are nib's own documentation, offered to the
agent so it can answer questions about itself rather than guessing.

They are deliberately invisible to the API: they are not content an operator
authored, they change with the image, and listing them among your skills would
invite editing something that is regenerated on every release.

## Who decides a skill runs

Each skill has an **Access** setting on the Skills page, stored as the `access`
frontmatter key:

| Access | In the prompt catalog | `get_skill` serves it |
|---|---|---|
| **Enabled** (default) | yes | always |
| **Explicit** | no | only after an operator message in this chat carries `/name` |
| **Disabled** | no | never |

Explicit is for a procedure you want run only when you ask — `/get-jira-task
PROJ-42` — and never picked because a description looked relevant. The agent is
told only that a slash command means "load that skill"; it cannot see the list.
The check covers every message you have sent in the plan or discuss chat, and a
plan's sub-agents share it, so a plan started with `/name` can use the skill in
any of its stages. A bare mention of the name does not count.

The prompt catalog is only the hint; `get_skill` is the gate, so a sub-agent
guessing a name meets the same refusal. System skills are always enabled. The
Knowledge Base, Distribute tools and Categorize tools buttons open their message
with the skill's `/name`, so they still work when you set their skill to
Explicit, and fail when you disable it.

## Where variables fit

All three surfaces are Go templates, rendered against prompt variables and the
current project/environment/cloud/location selection before they reach the
model.

This is what lets one rule serve several environments — the text names
`{{ .builtin.Environment }}` rather than being duplicated per environment. It
is also why a built-in skill referencing a variable you have not filled in fails
to load with an unknown-key error rather than rendering a blank: silently
producing a prompt with a hole in it is the worse failure.

The host variables are a small case of the same idea pointing inward. nib
snapshots its own container at startup — distro, shell, working directory,
which binaries are on `PATH` — and renders it into the prompts, so the agent's
answers reflect what it can actually run. An operator's edit to one of those is
never overwritten, which makes them the place to correct what the agent believes
about its host.

## What none of them do

None of the three grants access. A rule saying "use the staging cluster" does
not give the agent a tool for it; knowledge describing your monitoring stack does
not connect it. Capability comes from MCP servers and the per-mode tool lists,
which is a separate subject — see [Modes, tools and the
catalog](modes-tools-and-the-catalog.md).

The failure looks the same from the chat either way: the agent knows what to do
and does not do it.

## See also

- [The knowledge base](../knowledgebase/example.md) — collections, and what an upload replaces
- [Built-in agent tools](../reference/agent-tools.md) — `get_skill`, `get_rule`, `knowledge_search`
- [State](../reference/state.md#operator-editable) — where each of the three lives on disk
