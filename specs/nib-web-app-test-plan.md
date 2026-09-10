# NIB web application — comprehensive E2E test plan

Target: the NIB SPA (`frontend/`) served on `http://localhost:5173` by the dev stack, talking to the
Go backend on `/api/v1`. Written for the Playwright suite at the repo root (`playwright.config.ts`,
`tests/`, seed at `tests/seed.spec.ts`).

---

## 1. System under test

Single-page React app with a fixed shell: left sidebar (navigation), top header (global
project / environment / cloud / location selection + mode indicator), main content, and a
right-hand chat panel present on every route, separated by a draggable splitter.

Routes (from `frontend/src/app.tsx`):

| Path | Page |
|---|---|
| `/` | redirect → `/workplace` |
| `/workplace` | plan list ("Plans") |
| `/workplace/:id` | plan detail (summary, DAG, action list, chat) |
| `/incidents`, `/incidents/:id` | incident list / detail (badged **In dev**) |
| `/knowledge` | Knowledge Base — company info + document upload |
| `/plans` | static placeholder heading "Plans" |
| `/rules` | rule files editor |
| `/tools` | tabs: MCP Servers, Categories, Included tools, Executor |
| `/skills` | skill files editor |
| `/variables` | tabs: Variables, Secrets |
| `/system-tools` | developer catalog of built-in agent tools (not linked from the UI) |
| `/projects`, `/projects/:id`, `/dialogs` | redirect → `/workplace` |
| `/task-tracker`, `/mcp-connections`, `/base` | redirect → `/tools` (query string preserved) |

---

## 2. Environment and preconditions

1. Dev stack up: `docker compose -f docker-compose.dev.yml up` (`nib-dev-frontend`,
   `nib-dev-backend`, `nib-dev-postgres`, `nib-dev-kb-mcp`).
2. `baseURL` = `http://localhost:5173` (override with `NIB_E2E_BASE_URL`).
3. There is deliberately **no `webServer` block** — Playwright attaches to an already-running
   dev server; a local `vite build` does not work in this repo.
4. Backend state is *not* reset between runs. Every scenario below must therefore be written to be
   order-independent and must not assert on absolute list contents unless it mocked the list.

## 3. Safety rules — mandatory, read before writing any spec

This suite drives a **real backend with real LLM credentials and a real executor**. Violating these
rules costs money and can start infrastructure changes on the user's machine.

- **S1 — Mock every non-GET `/api/` call by default.** Install `page.route('**/api/**', …)` in a
  fixture that fulfils/aborts anything that is not `GET`, and opt in per test to the specific write
  under test. Anything reaching *Execute action*, *Process plan here*, *Replan all stages* or the
  chat *Send* button otherwise launches an actual sub-agent run.
- **S2 — Never let a test press Execute against the live backend.** Execution is serialised by a
  global lease (`DELETE /api/v1/execution` force-stops the holder); a stray run blocks the operator's
  own work until it finishes or is killed.
- **S3 — Assert against mocked payloads, not live data**, for anything list-, count- or
  status-shaped. Use fixtures (§4) so the same spec passes on an empty and a busy install.
- **S4 — Clean up what you create.** Skills, rules, variables, secrets and MCP servers written
  through a real (unmocked) API call live on the data volume; delete them in `afterEach`. Prefer a
  `e2e-` name prefix so leftovers are identifiable.
- **S5 — No writes to `config.yaml`-backed entities** (projects/environments/clouds/locations)
  against the live backend — those edits rewrite the operator's `backend/config.yaml`. Mock them.
- **S6 — Verify after the fact** for any test that touched a write path:
  `GET /api/v1/dialogs/<id>/messages` count must not have moved.

## 4. Fixtures and helpers to build first

| Helper | Purpose |
|---|---|
| `apiGuard(page)` | Blocks all non-GET `/api/**`; returns an `allow(method, urlGlob)` escape hatch. |
| `mockDialogs(page, dialogs[])` | Fulfils `GET /api/v1/dialogs` (list + pagination + pinned). |
| `mockPlan(page, {dialog, summary, dag, actionPlan, planState})` | Fulfils the whole plan-detail fan-out: `/dialogs/{id}`, `/summary`, `/dag`, `/action-plan`, `/plan-state`, `/children`, `/messages`. |
| `mockSSE(page, events[])` | Fulfils `GET /dialogs/{id}/events` with a canned `text/event-stream` body. |
| `emptyState(page)` | Every list endpoint returns `[]` — drives all "No … yet." assertions. |
| `failWith(page, urlGlob, status)` | Forces 4xx/5xx to exercise error toasts. |

**Match API routes on `url.pathname.startsWith('/api/')`, never on a `**/api/**` glob.** In dev, Vite
serves the app's own source modules, and their paths contain `/api/` too
(`/src/features/projects/api/projects.ts`). A glob-based route intercepts those as well, and aborting
one blanks the entire app — which looks exactly like an application bug. `isApiRequest` in
`tests/fixtures/nib-test.ts` is the shared predicate.

Reference fixtures: a plan in each `PlanStatus` (`draft`, `scheduled`, `in_progress`, `done`,
`reopened`, `rolled_back`), an action plan with ≥2 stages and both `command` and `code` steps, a
message list containing an unanswered `ask_question` tool call, and a `mcp.json` raw body with
comments and an unknown top-level key.

---

## 5. Suites and scenarios

Priority: **P0** smoke / release-blocking, **P1** core, **P2** edge and polish.

### Suite 1 — Application shell and navigation

**1.1 Shell renders on first load (P0)**
1. Navigate to `/`.
2. Expect redirect to `/workplace`.
3. Expect sidebar links visible: Workplace, Incidents (with badge "In dev"), Knowledgebase, Rules,
   Tools, Skills, Variables, plus Help and Collapse.
4. Expect the chat panel heading "Chat" and its empty text
   "Messages will appear here. Select a dialog or send a message to start a new one."
5. Expect no console errors (`page.on('console')` collector, ignore Vite HMR noise).

**1.2 Every nav item routes and highlights (P0)** — for each of the five NAV items: click, assert
URL, assert the page's own heading/description, assert the link carries the active state.

**1.3 Legacy redirects (P1)** — visit `/projects`, `/projects/abc`, `/dialogs` → `/workplace`;
`/task-tracker`, `/mcp-connections`, `/base` → `/tools`. With a query string
(`/base?tab=executor`) the search part must survive the redirect.

**1.4 Unknown route (P2)** — visit `/does-not-exist`. **Confirmed behaviour:** no catch-all route is
declared, so the app renders an empty document — no shell, no message, no way back. Asserted as-is in
`tests/shell/redirects.spec.ts` so that adding a 404 page turns the test red.

**1.5 Sidebar collapse (P1)** — click "Collapse": labels hide, icons remain, tooltips show the label
on hover; expand restores. Assert the state survives a reload if it is persisted.

**1.6 Panel resize (P1)** — drag the "Resize panels" splitter left/right; assert the chat panel width
changes and is clamped at both ends; reload and assert whether the width persists.

**1.7 Deep-link into a lazy route (P1)** — load `/variables` directly: the "Loading..." fallback may
appear, then the page content; assert the final state, never the fallback.

**1.8 Help menu (P2)** — open Help: assert entries incl. "About Nib" showing "Nib Version",
"License", "Source Code"; the version must match `GET /api/v1/version` (mocked).

### Suite 2 — Header: global selection and mode

**2.1 Default selection is "Any" (P1)** — with no selection stored, all four dropdowns read `Any`.

**2.2 Selecting a project (P1)** — mock `GET /projects` with two projects; open the project menu,
pick one; assert the trigger label updates, a check mark marks the chosen row, and
`PUT /api/v1/selection` was sent with the id.

**2.3 Cascade (P1)** — environments/clouds/locations are scoped to the selection; after changing the
project, the dependent menus refresh and a stale child selection is cleared or re-resolved.

**2.4 Selection survives reload (P1)** — reload and assert the labels come back from
`GET /api/v1/selection`.

**2.5 Loading and empty menus (P2)** — while the list request is pending the trigger is disabled;
with an empty list the menu opens with no rows and does not crash.

**2.6 Mode indicator (P1)** — the right-hand pill is read-only and exposes
`aria-label="Mode: <mode>"`; on `/workplace` with no dialog it shows the default (`Discuss`), and on
a plan dialog it reflects that dialog's mode.

### Suite 3 — Workplace: plan list

**3.1 Empty state (P0)** — with `mockDialogs([])`: heading "Plans", the description mentioning
"New plans start in decompose mode", "New plan" button and "No plans yet.".

**3.2 List rendering (P0)** — with a mocked list assert per row: title, formatted date, pin and
delete icon buttons, and that pinned plans render in the pinned group above the rest.

**3.3 Open a plan (P0)** — click a row → URL `/workplace/{id}`, the row is marked active, and the
chat panel binds to that dialog.

**3.4 Create a plan (P0)** — allow `POST /api/v1/dialogs` mocked to return a new id; click
"New plan"; assert the request body carries `mode: "main"`, the app navigates to `/workplace/{newId}`
and the list is refreshed. Never run this unmocked.

**3.5 Search (P1)** — type into "Search plans by name...", submit; assert the query reaches the list
request and the rendered rows change; clearing the field restores the full list.

**3.6 Search with no matches (P1)** — assert the empty-result message, and that clearing recovers.

**3.7 Pin / unpin (P1)** — click the pin icon: `PUT /dialogs/{id}/pin` with `pinned:true`, the row
moves into the pinned group, the icon state flips; unpin reverses it. Assert the row click did **not**
navigate (the icon must stop propagation).

**3.8 Delete with confirmation (P1)** — click delete → confirm dialog appears; Cancel leaves the row;
Confirm sends `DELETE /dialogs/{id}` and removes the row. Deleting the currently open plan clears the
detail/chat binding.

**3.9 Pagination / incremental loading (P1)** — with a long mocked list assert the next page loads
(scroll or "load more") and no row is duplicated.

**3.10 List request fails (P2)** — 500 on `GET /dialogs`: an error state is shown, the shell stays
usable, and a retry path exists.

### Suite 4 — Plan detail: layout, title, metadata

**4.1 Detail loads (P0)** — with `mockPlan(...)`: "Back to list", the title, the status badge,
Progress, and the three collapsible cards Summary, DAG, Action List.

**4.2 Back to list (P0)** — returns to `/workplace` with the list intact.

**4.3 Rename the plan (P1)** — click the title, edit, confirm: `PUT /dialogs/{id}/title`; the header
and the list row both show the new title. Escape cancels; an empty title is rejected.

**4.4 Simplified / Detailed toggle (P1)** — the `role="group"` `aria-label="Plan view"` toggle flips
`aria-pressed`; Detailed reveals the richer action rows. Assert persistence if implemented.

**4.5 Download plan as Markdown (P1)** — click the download icon button; capture the `download`
event; assert the filename and that the body contains the plan title and the action list.

**4.6 Status badge per status (P1)** — one assertion per `PlanStatus` fixture:
DRAFT / SCHEDULED / IN PROGRESS / FINISHED / REOPENED / ROLLED BACK.

**4.7 Change status (P1)** — pick a new status → `PUT /dialogs/{id}/plan-state/status`; the badge and
the list row badge update.

**4.8 Schedule (P1)** — the trigger reads "Select schedule" until a date exists, then the formatted
date. Picking a day sends `PUT /plan-state/schedule` and moves a `draft` plan to SCHEDULED; the time
field stays disabled until a day is chosen; "Clear schedule" sends `scheduledAt: 0` and reverts the
status. **Layout note:** the popover (calendar + time + clear) is taller than a 720px viewport and
its lower half cannot be clicked there — Radix's collision handling does not reclaim the space, so
the spec raises the viewport to 1000px. Worth fixing in the component.

**4.9 Progress bar (P1)** — with an action plan where n of m steps are checked, the bar reflects
n/m; with an empty plan it reads zero and does not divide by zero.

**4.10 Unknown plan id (P2)** — `/workplace/00000000-0000-0000-0000-000000000000` with a 404 mock:
a not-found state, no crash, "Back to list" still works.

### Suite 5 — Summary and DAG

**5.1 Summary empty (P1)** — "No summary yet for this conversation."

**5.2 Summary renders Markdown (P1)** — headings, lists, code and tables from the fixture render as
HTML, not raw text.

**5.3 Edit summary (P1)** — pencil → textarea (`aria-label="Summary"`); Save sends
`PUT /dialogs/{id}/summary`; Cancel and Escape both discard; Cmd/Ctrl+Enter saves; the Save button
reads "Saving..." while in flight.

**5.4 Decomposition chat link (P1)** — visible only when a decompose child exists; clicking it binds
the chat panel to the decompose sub-dialog.

**5.5 Collapse / expand cards (P2)** — each of Summary, DAG and Action List collapses via its header
chevron and keeps its state while navigating within the plan.

**5.6 DAG renders (P1)** — nodes and edges from the fixture appear on the board with stage grouping;
an empty DAG shows the placeholder rather than an empty canvas.

**5.7 DAG interaction (P2)** — **there is no pan/zoom**: the board is a plain `overflow-x-auto`
container and mermaid scales its SVG down to the container width, so even a 12-stage graph does not
overflow. Assert the container owns the scroll (`overflow-x: auto`), that the SVG fits inside it, and
that the document itself never scrolls sideways. Labels shrink with the graph — legibility on a large
DAG is a product question, not something the suite can assert.

### Suite 6 — Action list: planning, editing, ordering

**6.1 Gate before decomposition (P1)** — with no DAG the hint
"Complete decomposition first — a DAG is required before processing the plan." is shown and the
process button is disabled.

**6.2 Process plan (P0, mocked)** — with a DAG and no action plan the hero button "Process plan here"
is shown. **The endpoint depends on the plan's mode:** a `decompose` dialog calls
`POST /dialogs/{id}/plan-fanout` directly, while an orchestrator (`main`) dialog instead posts a chat
message ("Work the whole DAG into a detailed action plan…") and lets its own agent do the planning.
Both paths need a mock. **Never unmocked (S1/S2).**

**6.3 Replan all stages (P1, mocked)** — with an existing action plan the button reads
"Replan all stages" and re-issues the fan-out.

**6.4 Cancel fan-out (P1, mocked)** — Stop sends `DELETE /plan-fanout`; the buttons return to idle.

**6.5 Fan-out progress (P1)** — with a mocked `GET /plan-fanout` the per-stage progress block expands
and shows each stage's state.

**6.6 Step checkboxes (P1)** — toggling a step sends `PUT /action-plan/checks`; the progress bar
updates; the checkbox is labelled by its number + label (`aria-labelledby`).

**6.7 Edit an action (P1)** — the "Edit action" icon opens the "Edit action" dialog; change fields,
save → `PUT /action-plan`; Cancel discards. Validation: an empty required field blocks Save.

**6.8 Action comment (P1)** — "Add comment" opens the "Action comment" dialog; saving sends
`PUT /action-plan/comments` and the icon's label/tooltip flips to "Edit comment"; an existing comment
is pre-filled; clearing it removes the marker.

**6.9 Reorder by drag (P1)** — drag a row within its stage → `PUT /action-plan/reorder` with the new
index; the rendered order matches. Assert a drag across stages is rejected or handled per spec.

**6.10 Drag disabled (P2)** — while an execution is running the drag handle is disabled and reorder
requests are not sent.

**6.11 Rollback section (P1)** — the rollback scope renders its own steps, checkboxes and ordering,
independent of the forward plan.

**6.12 `code` vs `command` steps (P2)** — a `code` step shows the repository chip
(`title="repository: …"`); a `command` step does not; only `code` steps offer Execute.

### Suite 7 — Action execution and the execution lease

All of Suite 7 runs against mocks. Nothing here may reach the real executor.

**7.1 Execute is offered only where it can run (P1)** — a step with nowhere to run has its
"Execute action" button disabled with the explanatory tooltip.

**7.2 Execute an action (P0, mocked)** — **two different paths, confirmed against the app:** a
`command` step's Execute posts a chat message `Execute plan item {number}` to the orchestrator, and
only a `code` step (with an executor configured) calls `POST /action-plan/execute`. Assert exactly
one request on whichever path the step takes. A finished run then offers "Run action again", and a
running one offers "Stop execution" ("Force-stop this execution and free the slot").

**7.3 Lease conflict (P1)** — mock the execute call with the "already running" refusal; assert the
error surfaces naming the holder and that no optimistic running state is left behind.

**7.4 Force-stop (P1, mocked)** — the force-stop button issues `DELETE /api/v1/execution`; the row
returns to idle.

**7.5 Exec run history (P1)** — `GET /action-plan/exec` mocked with past runs: each run's status and
timestamps render; a failed run is visually distinct.

**7.6 Execution status polling / SSE (P1)** — with `mockSSE` emitting a run-finished event the row
flips to finished without a manual reload.

### Suite 8 — Chat panel

**8.1 Empty chat (P0)** — the placeholder text and a disabled Send button with an empty input.

**8.2 Render a transcript (P0)** — user, assistant and tool messages from a mocked `/messages`
render with the right roles; Markdown, code blocks and tables (`table-message`) are formatted.

**8.3 Send a message (P0, mocked)** — type into "Message…", press Send: exactly one POST, the
message appears optimistically, the input clears, and the input is disabled while the turn runs.

**8.4 Keyboard send (P1)** — Enter sends, Shift+Enter inserts a newline and grows the textarea.

**8.5 Stop generating (P1, mocked)** — while a turn is running the Send button is replaced by
"Stop generating"; clicking it cancels and re-enables the input.

**8.6 Attachments (P1, mocked)** — the file input accepts
`image/*,text/*,.md,.txt,.csv,.json,.log,.yaml,.yml`; after upload a chip with the filename appears
with "Remove {filename}"; removing before send deletes the attachment; sending includes the
attachment ids. An image chip shows a thumbnail.

**8.7 Rejected file type (P2)** — `accept` only filters the picker. A file supplied any other way
(drag, a scripted set, a renamed extension) **is uploaded**, and only the backend can refuse it. Test
both halves: the `accept` attribute is exactly
`image/*,text/*,.md,.txt,.csv,.json,.log,.yaml,.yml`, and a backend 415 surfaces to the operator with
no chip left behind.

**8.8 Upload failure (P2)** — 500 on the upload: an error is shown and no chip is left behind.

**8.9 Clarifying question (P0)** — with a transcript ending in an unanswered `ask_question`, the
"Clarifying question" block renders with a "Your answer" field and a Submit button; the main input is
disabled until it is answered. Submitting posts the answer, the button shows "Submitting…", and the
block collapses into an answered-questions summary.

**8.10 Multi-question ask (P1)** — several questions in one call: each gets its own field and all
answers are submitted together, in order.

**8.11 SSE activity stream (P1)** — with `mockSSE` the tool-activity entries appear live in order and
the stream reconnects after being cut without duplicating entries.

**8.12 Chat history menu (P1)** — the "Chat" menu switches between the plan's dialogs (root and
sub-agents); the header mode pill follows the selected dialog.

**8.13 Chat survives navigation (P1)** — navigating from `/workplace/:id` to `/tools` and back keeps
the bound dialog and does not duplicate the transcript.

### Suite 9 — Incidents

**9.1 Empty state (P1)** — "Recent incidents", "New incident", the search field and "No incidents yet."

**9.2 In-dev badge (P2)** — the sidebar item carries the badge "In dev".

**9.3 Create an incident (P1, mocked)** — "New incident" posts a dialog with `mode: "incident"` and
navigates to `/incidents/{id}`.

**9.4 Search (P1)** — "Search incidents by name..." filters the list.

**9.5 Detail (P1)** — an incident detail renders its summary and binds the chat panel in incident
mode.

### Suite 10 — Knowledgebase

**10.1 Page loads (P1)** — "Knowledge Base" with the Company Information card and the Document upload
card.

**10.2 Company information round-trip (P1, mocked)** — fill Company name, Company description,
Version control system, GIT base URL, Git username, Git email, CI/CD system, Task tracker,
Issue project, Wiki, Command messenger; Save sends `PUT /api/v1/company`; a reload shows the saved
values; a success toast appears.

**10.3 Validation (P2)** — an invalid Git email / malformed base URL is rejected before the request.

**10.4 Collection selector (P1)** — collections come from `GET /knowledge/collections`; `default` is
preselected; the chunk count and the "Only one document is kept at a time" note render.

**10.5 View current document (P1)** — opens the stored document for the collection; for a collection
with no file the embedded skeleton is shown instead.

**10.6 Upload & index (P1, mocked)** — pick a `.md` file, press "Upload & index": one
`POST /knowledge/documents`, a progress/disabled state, then the chunk count refreshes. Assert the
warning that this replaces all chunks in the collection is shown before the write.

**10.7 Repositories field (P1)** — accepts comma-separated `owner/repo` and full URLs; assert the
hint text and that a malformed entry is flagged.

**10.8 Generate from repositories (P1, mocked)** — the generate path starts an agent run: must be
mocked; assert the disabled/running state and that exactly one request is sent.

**10.9 Knowledge connection status (P2)** — `GET /knowledge/status` unhealthy: the page shows the
degraded state rather than silently rendering an empty collection list.

### Suite 11 — Rules

**11.1 Empty state (P1)** — "No rules yet." and "Select a rule or add a new one."

**11.2 Create a rule (P1)** — "Add rule" → Name, Description, Body; Save sends
`POST /api/v1/rules`; the file appears in the list as `{name}.md` and is selected. Clean up in
`afterEach` (S4).

**11.3 Duplicate name (P1)** — creating a rule whose name exists surfaces the backend error, not a
generic 500 toast.

**11.4 Invalid name (P2)** — spaces, slashes and path traversal (`../x`) are rejected client- or
server-side; assert the message.

**11.5 Edit and save (P1)** — change the body, Save → `PUT /rules/{name}`; reselecting the rule shows
the new body. Unsaved-change navigation: assert the guard (or its deliberate absence).

**11.6 Delete (P1)** — Delete asks for confirmation via a **native `window.confirm`**, then
`DELETE /rules/{name}`; the list row goes and the editor resets. Switching to another rule with
unsaved changes raises a second native confirm ("You have unsaved changes…"); both need a
`page.on('dialog')` handler.

**11.7 Large body (P2)** — a ~200 KB body saves and reloads without truncation or a frozen editor.

### Suite 12 — Skills

**12.1 List (P1)** — seeded skills render as `{name}.md` rows (e.g. `build-knowledge-base.md`);
selecting one fills Name, Description and Body.

**12.2 Create / edit / delete (P1)** — same round-trips as Rules against `/api/v1/skills`, with
cleanup.

**12.3 Rendered view (P2)** — **not reachable from the UI.** `GET /skills/{name}/rendered` exists on
the backend but nothing in `frontend/src` calls it, so there is no page to drive. Either wire it into
the skills editor as a preview (then test that `{{ .global.X }}` is substituted and a missing
variable surfaces the render error) or drop the route.

**12.4 System skills are invisible (P1)** — `nib-configuration` and `nib-internals` must **not**
appear in the list and must not be creatable under those names.

**12.5 Frontmatter integrity (P2)** — saving a skill preserves its YAML frontmatter (`name`,
`description`) and does not duplicate it into the body.

### Suite 13 — Tools › MCP Servers

**13.1 Tab renders (P0)** — the page description mentioning `mcp.json`, the four tabs
(MCP Servers, Categories, Included tools, Executor), the Servers / Raw JSON sub-tabs, "Add server",
and each configured server row with its URL, Edit and Delete.

**13.2 Add an HTTP server (P1, mocked)** — Add server → Name `e2e-http`, URL
`https://example.com/mcp`, optional headers (Key/Value rows) and Description; Save sends
`POST /mcp/servers`; the row appears.

**13.3 stdio is refused (P0)** — switch Transport to the stdio shape and fill Command `npx` / Args:
saving must fail with the backend's `ErrStdioNotSupported` message. This is the single most
important negative case on this page — stdio and SSE are not supported, only URL-registered
streamable HTTP.

**13.4 Validation (P1)** — empty name, empty URL, a non-URL string, and a duplicate server name are
each rejected with a field-level message.

**13.5 Edit a server (P1)** — change the URL and a header; `PUT /mcp/servers/{name}`; the row
updates. Assert unrelated keys in the file are preserved (verify via the Raw JSON tab).

**13.6 Delete a server (P1)** — this page guards deletion with a **native `window.confirm`**, not the
app's own confirm dialog (as the plan list uses). Playwright auto-dismisses native dialogs, so a test
must register `page.on('dialog', …)` before clicking. Cover both dismiss and accept.

**13.7 Per-server tools (P1)** — the per-row **Tools** button opens a "Tools — {server}" dialog
listing what `GET /mcp/servers/{name}/tools` returned, with the count ("2 tools available"); each
tool's **description sits behind its own disclosure**, not in the list. An unreachable server shows
the transport error plus a Refresh, never an empty list. Every such error goes through
`redactRouteError`, so a `${SECRET}` reference may appear but a resolved value never may — assert the
page contains no bearer-token-shaped string.

**13.8 Raw JSON editor (P1)** — the Raw JSON tab shows the file; editing and saving sends
`PUT /mcp/config/raw`; JSONC comments, key order and unmodelled keys survive the round-trip.

**13.9 Invalid raw JSON (P1)** — saving malformed JSON is rejected with a parse error and the stored
file is unchanged.

**13.10 `${SECRET}` references (P2)** — a value of `${GITHUB_TOKEN}` saves even when no such secret
exists; the failure only surfaces when the server is contacted, and the resolved value is never
rendered.

### Suite 14 — Tools › Categories

**14.1 List categories (P1)** — categories come from the `toolCategories` variable; "Uncategorized"
is present.

**14.2 Edit patterns (P1)** — enter patterns (`kubernetes_get_pods`, `kubernetes_*`), save →
`PUT /tool-categories/{name}/patterns`; "Matched tools" refreshes to the matching set.

**14.3 Glob semantics (P1)** — `*` matches, an unmatched pattern yields an empty "Matched tools",
and a pattern matching everything does not hang the UI.

**14.4 Uncategorized (P1)** — the uncategorized list shrinks as patterns claim tools.

**14.5 Empty catalog (P2)** — with no MCP servers the tab shows "No tools available." rather than a
spinner.

**14.6 Auto-fill warning (P1)** — "Auto-fill with AI" opens a discuss chat that **replaces** the
patterns of every category it assigns tools to. The warning must be on screen before the button is
pressed, and the run itself must be mocked.

### Suite 15 — Tools › Included tools

**15.1 Two-pane layout (P1)** — included vs excluded panes grouped by server, with per-mode selection.

**15.2 Move one tool (P1)** — the per-row button labelled `Include {tool}` / `Exclude {tool}` moves
it and sends `PUT /included-tools/{mode}`.

**15.3 Move all (P1)** — "Include all" / "Exclude all", and the per-server variants
(`Include all {server} tools`), move the whole group in one request.

**15.4 Search (P1)** — "Search by name, server, or description" filters both panes; bulk actions then
apply to the filtered set only (or to everything — assert the intended semantics explicitly).

**15.5 Per-mode isolation (P1)** — excluding a tool in `plan` leaves `execute` unchanged; switching
modes reloads the right list.

**15.6 Persistence (P1)** — reload and assert the choice came back from the server, not from local
state.

**15.7 Distribute warning (P1)** — "Distribute with AI" narrows **every** mode's list, not just the
selected one, and only ever removes tools. Assert the warning renders; mock the run.

### Suite 16 — Tools › Executor

**16.1 Disabled type (P1)** — Type `disabled` hides every runtime field and saving persists it.

**16.2 Local docker (P1)** — Type `local`: Agent (`claude-code` / `codex`), Auth type
(`api_key` / `oauth_token`), API key or OAuth token, Base URL, Image, LLM model and
Webhook base URL are shown; Save sends `PUT /executor/config`.

**16.3 Remote kubernetes, local_config (P1)** — Platform `kubernetes`, Auth `local_config`: Context
name is offered ("Leave empty for current context"); host/token/CA fields are hidden.

**16.4 Remote kubernetes, token (P1)** — Auth `token` reveals host
(`https://cluster.example.com:6443`), token secret and the optional PEM CA certificate; the token
field must reference a secret, never accept a raw pasted credential rendered back in clear text.

**16.5 Kubernetes resources (P2)** — Namespace, Service account, Secret name, CPU/memory limits and
requests, MCP ConfigMap and job TTL round-trip, with numeric/quantity validation
(`1`, `1Gi`, `256Mi`, `3600`).

**16.6 Conditional-field integrity (P1)** — switching Type/Platform/Auth back and forth does not
submit stale hidden fields.

**16.7 Save failure (P2)** — a 400 from the backend renders the field error rather than a bare toast.

### Suite 17 — Variables and Secrets

**17.1 Variables tab (P0)** — the description with `{{ .scope.Name }}`, the tabs Variables/Secrets,
and the seeded rows (`CompanyName`, `TaskTracker`, `IssueProject`, `Wiki`, `Messenger`,
`toolCategories`, and the `host*` set) with their scope.

**17.2 Create a variable (P1)** — Type (String/List), Scope (`global` / `project` / `environment` /
`cloud` / `location`), Scope name for non-global scopes, Name, Description, Value; Save posts to
`/variables`. Clean up.

**17.3 Scope name is required off-global (P1)** — choosing `project` without a scope name blocks Save.

**17.4 List-typed value (P1)** — a `list` variable accepts multiple entries and renders as a list
(`toolCategories` is the reference case).

**17.5 Edit (P1)** — Type and Name are disabled while editing; Value and Description save via
`PUT /variables/{id}`.

**17.6 Built-ins cannot be deleted or renamed (P0)** — `CompanyName`, `TaskTracker`, `IssueProject`,
`Wiki`, `Messenger`, `toolCategories`: no delete affordance (or a refusal), rename disabled,
re-valuing allowed.

**17.7 `host*` variables (P1)** — editing `hostOS` persists an operator override; assert the UI marks
these as detected-at-startup values.

**17.8 Delete a custom variable (P1)** — confirmation → `DELETE /variables/{id}` → row gone.

**17.9 Secrets tab (P0)** — "No secrets added yet." on an empty install; creating a secret stores it
and the **value is never displayed or returned** afterwards — assert the list response body contains
no value field.

**17.10 Secrets without an encryption key (P1)** — with `SECRETS_ENCRYPTION_KEY` unset (mocked error)
create/read fail with an explanatory message rather than a silent no-op.

**17.11 Duplicate names (P2)** — a duplicate variable or secret name is rejected per scope.

### Suite 18 — System tools (developer catalog)

**18.1 Not linked (P2)** — no sidebar entry points at `/system-tools`; it is reachable only by URL.

**18.2 Catalog renders (P1)** — the "N tools" count matches the rows; each row shows name,
description, its mode badges (`always`, or `main`/`decompose`/`plan`/`execute`/`discuss`/`incident`)
and a "Parameters schema" disclosure holding valid JSON schema.

**18.3 Filter (P1)** — "Filter by name, description, or mode..." narrows by each of the three, and an
unmatched query yields an empty result message.

**18.4 Orchestrator-only tools (P1)** — `run_subagent`, `stop_execution` and `execute_action` are
shown as `main`-only.

### Suite 19 — Resilience and error handling

**19.1 Backend down (P0)** — abort every request whose pathname starts with `/api/` (see §4 for why
the glob form is wrong): **confirmed** — every page still renders its shell, heading and navigation,
and no uncaught error reaches the page.

**19.2 401/403 (P1)** — assert the intended handling on a protected route.

**19.3 5xx on a write (P1)** — for one representative write per feature the toast names the failure
and the optimistic UI rolls back.

**19.4 Slow response (P1)** — a delayed list shows "Loading..." and does not retry. Note the plan
list makes **two** requests (the pinned scope and the paged scope), each double-invoked by React
StrictMode in dev — four in total against the dev server, two against a production build.

**19.5 SSE disconnect (P1)** — cutting `/dialogs/{id}/events` mid-stream: the UI reconnects or shows a
stale-stream indicator; it must not silently show a half-finished turn as complete.

**19.6 Concurrent tabs (P2)** — two contexts on the same plan: a write in one is reflected in the
other after its next refresh, without corrupting local state.

**19.7 Long-running write (P2)** — a request held open past the UI timeout leaves the button in a
recoverable state.

### Suite 20 — Accessibility, responsiveness, visual

**20.1 Keyboard navigation (P1)** — Tab reaches every sidebar link, tab trigger, form field and
dialog control in a sensible order; focus rings are visible (`focus-ring`).

**20.2 Dialog semantics (P1)** — "Edit action", "Action comment", confirm-delete and the MCP server
dialog trap focus, close on Escape, and restore focus to the opener.

**20.3 Labels (P1)** — every input has an associated label; icon-only buttons expose `aria-label`
(pin, delete, Remove {filename}, Execute action, Collapse). **Known defect:** collapsing the sidebar
drops the accessible name of every nav link and of the Help and Expand buttons — the label span is
removed and no `aria-label`/`sr-only` replaces it, so a screen-reader user loses the navigation
entirely. Held as `test.fixme` in `tests/system/a11y.spec.ts`. The splitter is a second, smaller
case: its "Resize panels" text is `sr-only` inside `role="separator"`, which takes no name from
content, so it has no accessible name either.

**20.4 Landmarks and headings (P2)** — one `<h1>`/`<h2>` per page and a `header`/`nav`/`main`
structure.

**20.5 Colour contrast (P2)** — measured in-browser (`tests/fixtures/contrast.ts`) and **passing**:
nav labels 12.8:1, muted text 7.9:1, headings 19:1, status pills above AA. Two things make a naive
measurement lie, both worth keeping in the helper: nib stacks translucent surfaces (a 9% link tint
over a 7% glass panel over the shell), so the background must be accumulated with real source-over
alpha rather than flattened at each step — flattening reports the nav labels at 1.04:1, which looks
like a catastrophic defect and is not one. And computed colours come back as `oklch()`/`oklab()`, so
they are parsed by painting into a 1×1 canvas rather than by regex.

**20.6 Theme (P2)** — **nib is dark-only.** `frontend/index.html` hard-codes `class="dark"` on the
root and nothing in the app toggles it, so the light half of the token set in `index.css` is
unreachable. The spec locks that in: the root carries `dark`, the body paints an explicit background
rather than inheriting one, and no theme control exists. Adding a switch should turn it red.

**20.7 Narrow viewport (P2)** — at 1024×768 and 1280×800 no page scrolls horizontally and `body`
never gets `overflow-x: scroll`; wide content (DAG, tables, code blocks) scrolls inside its own
container. Covered for all seven top-level routes plus the plan detail, which is the densest page.

---

## 6. Coverage map

| Area | Suites | P0 scenarios |
|---|---|---|
| Shell, routing | 1, 2 | 1.1, 1.2 |
| Plans (list) | 3 | 3.1, 3.3, 3.4 |
| Plan detail, DAG, actions | 4, 5, 6 | 4.1, 4.2, 6.2 |
| Execution | 7 | 7.2 |
| Chat | 8 | 8.1, 8.2, 8.3, 8.9 |
| Incidents | 9 | — |
| Knowledgebase | 10 | — |
| Rules, Skills | 11, 12 | — |
| Tools (MCP, categories, included, executor) | 13–16 | 13.1, 13.3 |
| Variables, Secrets | 17 | 17.1, 17.6, 17.9 |
| System tools | 18 | — |
| Resilience | 19 | 19.1 |
| A11y / responsive | 20 | — |

## 7. File layout

`✓` exists and passes today (`npx playwright test`); the rest is still to write.

```
tests/
  fixtures/            ✓ nib-test.ts (apiGuard + mockJson/mockApiError/mockSSE)
                       ✓ data.ts (dialog, action plan, transcript fixtures)
                       ✓ mock-api.ts (mockDialogList, mockPlan, mockEmptyWorkspace)
                       ✓ contrast.ts (WCAG measurement over translucent stacks)
  shell/               ✓ navigation.spec.ts  ✓ redirects.spec.ts    selection.spec.ts
  plans/               ✓ list.spec.ts  ✓ detail.spec.ts  ✓ action-plan.spec.ts
                       ✓ schedule.spec.ts  ✓ dag.spec.ts
  chat/                ✓ messaging.spec.ts  ✓ attachments.spec.ts
  incidents/           ✓ incidents.spec.ts
  knowledge/           ✓ knowledge.spec.ts
  authoring/           ✓ rules.spec.ts  ✓ skills.spec.ts
  tools/               ✓ mcp-servers.spec.ts  ✓ mcp-server-tools.spec.ts
                       ✓ categories.spec.ts  ✓ included-tools.spec.ts
                       ✓ executor.spec.ts
  variables/           ✓ variables.spec.ts (variables + secrets)
  system/              ✓ system-tools.spec.ts  ✓ a11y.spec.ts  ✓ resilience.spec.ts
                       ✓ responsive.spec.ts (layout, theme, contrast)
```

Run order is irrelevant by design (S3): every spec mocks or creates what it asserts on.

**The suite runs serially, and a `globalSetup` warms every route first.** One single-threaded Vite
dev server backs every worker and transforms each lazy route's chunk on first request, so parallel
workers queue behind one another. This is not a small effect: spec files that pass in 8s serially
time out wholesale at the default worker count, and the failures read as application defects — blank
chat panel, missing headings — rather than as contention. Even two workers left two tests wedged for
14 minutes each. Two concurrent `playwright test` invocations do the same; run one at a time.

Serial is also *faster* end to end here: **190 tests in 4.1 minutes**, deterministic. So
`playwright.config.ts` carries three deliberate settings: `workers: 1`, a 15s assertion / 60s test
timeout, and `globalSetup: './tests/global-setup.ts'`, which visits every route once (GETs only,
writes blocked) before the run — ~16s that would otherwise be paid inside the tests. Against a
production build (static assets, no transform) none of this is needed: raise the worker count there.

## 8. Assumptions and open questions

1. **No authentication** is assumed — the app is reached directly at `/workplace`. If auth is added,
   Suites 1 and 19.2 need a storage-state fixture.
2. **Fresh state cannot be assumed.** The planner convention of "assume a blank state" is
   incompatible with a shared dev backend, so this plan mandates mocking instead (§3, §4).
3. `/plans` renders only a heading — confirm whether it is dead and should be deleted rather than
   tested beyond 1.2.
4. Persistence of UI-only state (sidebar collapse, splitter width, Simplified/Detailed) is asserted
   as "record actual behaviour" until the intended behaviour is confirmed.
5. Incidents are badged **In dev**; Suite 9 is scoped to what exists today and should be revisited
   when the feature lands.
6. Whether bulk include/exclude applies to the filtered set or the whole catalog (15.4) needs a
   product decision before the assertion is written.
