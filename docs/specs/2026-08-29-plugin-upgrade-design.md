# Plugin Upgrade — Tool Coverage, Panel Observability, Cancellation

**Date:** 2026-08-29
**Scope:** `plugin/`, `internal/bridge/`, `internal/tools/`. The cluster and prompts packages are read but not changed.
**Status:** design, implemented. See `docs/plans/2026-08-29-plugin-upgrade-plan.md` for the task breakdown and what remains.

## Why

The server had been through an architecture pass; the plugin had not. A survey of
`plugin/src` and `internal/bridge` found the gaps clustered into four kinds, and they are
addressed in that order of weight:

1. **Tools that cannot express what designers actually build.** Responsive layout, rich
   text, icons, and component APIs each had a hole big enough to stop the job.
2. **Silent wrong answers.** Two handlers returned "nothing found" where the honest answer
   was "I did not look", and one tool could never run at all.
3. **A panel that shows nothing.** `AI is working…` made a hung tool and a slow tool look
   identical; the only way to tell them apart was the server's stderr.
4. **Work that cannot be stopped or bounded.** No cancellation, and no ceiling on a
   response size.

Network authentication stays **out of scope**, deliberately — see "Rejected: pairing" below.

## Baseline

| Metric | Before (`a6aa5b7`) | After |
|---|---|---|
| Tools | 63 | 76 |
| `internal/tools/testdata/tools_schema.json` sha256 | — | `cf8b3518b7caa920932b19be341b1b4ec75fece9eaf69e73262461f4afcfb844` |
| Plugin tests | 352 across 13 files | 599 across 27 files |
| Plugin source modules (non-test) | 21 | 33 |
| `go test ./...` | PASS, 7 packages | PASS, 7 packages |
| `gofmt -l .` | clean | clean |

Unlike the 2026-08-28 plan, the golden snapshot is **expected** to change here: this work
adds tools. It is regenerated deliberately, once per tool-adding task, and the diff is read
tool-by-tool before it is accepted.

## What was wrong

### Auto-layout could not say "fill container"

`applyAutoLayout` (`plugin/src/write-helpers.ts`) set `layoutMode`, padding, `itemSpacing`,
alignment, `primaryAxisSizingMode`/`counterAxisSizingMode` and `layoutWrap`. The two sizing
modes are Figma's older spelling and cover `FIXED` and `AUTO` (hug) only. `FILL` — a child
taking the space its parent offers — has no expression in them at all. Neither do
`minWidth`/`maxWidth`, which are what make `HUG` and `FILL` behave responsively rather than
collapsing.

`set_auto_layout` also gated on `node.type !== "FRAME"`, rejecting `COMPONENT`,
`COMPONENT_SET` and `INSTANCE`, all of which carry auto layout.

### Two silent wrong answers and one dead tool

`manifest.json` declares `documentAccess: "dynamic-page"`, under which only the current page
is in memory. `search_nodes` walked `figma.currentPage` and nothing else, so on a
multi-page file it answered "no matches" for every node on every other page. That is not a
wrong result the caller can see — it is indistinguishable from a correct one.

`create_connector` guards itself with `figma.editorType !== "figjam"`, but `manifest.json`
listed `editorType: ["figma", "dev"]`. The plugin could not be opened in FigJam, so the
guard could never pass and the tool could never run.

`batch_execute_pipeline` dispatched to write handlers that each call `figma.commitUndo()`.
A twenty-step pipeline left twenty undo checkpoints, so reversing it meant twenty Ctrl+Z,
each landing on a state no one asked for.

### The panel could not be diagnosed from

`App.svelte` tracked in-flight work as a `Set<string>` of request ids and rendered one
banner: `AI is working…`. The request payload arriving over the WebSocket already carries
`type` and `requestId`; neither reached the screen.

### Nothing bounded the work

`Bridge.Send` timed out and returned, but told the plugin nothing, so a long scan ran to
completion for an answer already discarded — holding the single WebSocket against the next
request. `get_document` serialized every node on a page with no ceiling.

## Design

### Tool surface: 13 additions, grouped by what they unblock

| Group | Tools | Unblocks |
|---|---|---|
| Layout | `set_layout_grids`; `set_auto_layout` extended | Responsive layout, grid guides |
| Text | `set_text_ranges`; `set_text` extended | A bold word, a link, a bulleted list |
| Vector | `create_vector`, `boolean_operation`, `flatten_nodes`, `outline_stroke` | Icons |
| Components | `combine_as_variants`, `manage_component_properties` | Building a design system, not just reading one |
| Assets | `get_image_bytes`; `import_image` extended | Shipping the asset a build needs |
| Human-in-the-loop | `set_selection` | "Look at what I just made" |
| Durability | `save_version_checkpoint`, `manage_plugin_data`, `set_codegen_result` | State that outlives the session |

Two read-side serializer additions carry the same weight as a tool. `styledSegments`
reports per-range text styling, which the node-level fields could only describe as `mixed` —
"something varies here" with no way to learn what, so bold and links were lost on the way to
code. `componentProperties` reports what a component exposes; before it, the only way to
learn a property's name was to place an instance and inspect it.

### Cancellation is advisory

`Bridge.cancelRequest` writes a `cancel_request` frame when the caller's context is
cancelled or the request's budget expires. The plugin records the id and long loops check
it between units of work — between pages, between pipeline steps, inside the two recursive
scans.

The frame is advisory by construction: a handler that never checks simply finishes, and its
response is dropped on the server as "a request that is already gone". The alternative —
making every handler cancellation-aware before the feature works at all — trades a large
diff for the same behaviour, and leaves a new long loop that forgets to check *broken*
rather than merely slow.

A cancelled pipeline rolls back regardless of `stop_on_error`. That flag is about
tolerating a step that failed on its own terms; a cancelled run has no terms left, and a
half-built pipeline standing is worse than none.

### Guard modes live in the UI

`off` / `confirm` / `read-only`, defaulting to `off` — the behaviour every existing user
has. Turning a guard on is opt-in, because a dialog in front of work the user asked for is
only welcome when they asked for the dialog too.

The gate sits in `App.svelte`, before the request is forwarded to the plugin core, for one
reason: approving needs a dialog and only that side has one. There is no trust boundary
between the UI and the core — both are the plugin — so a second gate in the core would buy
nothing.

Classification lives in `plugin/src/tool-classes.ts`, deliberately free of any `figma`
dependency so the UI bundle does not pull in the whole write surface to learn a set of
names. `tool-classes.test.ts` pins the lists against the real handler maps, so a tool cannot
be added without being classified — it caught `get_image_bytes` during this work.

A pipeline is classified by its worst step. It runs as one unit inside the core, so the UI
is the last point at which it can be stopped.

### One undo checkpoint per pipeline, and therefore a write queue

Figma offers no way to suspend `commitUndo`, so `withSingleUndoCheckpoint` swallows the
handlers' calls and makes one at the end, restoring the original function reference in a
`finally`.

That swap is global for the length of the pipeline, which creates a second problem:
`figma.ui.onmessage` is async and the server can have several requests in flight, so a plain
write landing in that window would have *its* checkpoint swallowed too — joining the
pipeline's undo step, or being rolled back with it. `enqueueWrite` serialises mutating
requests. Reads are not queued: they change nothing, and putting a long `get_document` ahead
of every write would make the queue the slowest thing in the plugin.

### Version skew is compared on major.minor

The plugin is imported by hand from a release zip; the server refreshes itself on every
`npx @latest`. The two drift by a patch routinely and that means nothing. A major or minor
gap is where the tool surface moved.

An unreadable version on either side — a dev build, or a plugin old enough not to announce
one — stays silent rather than guessing a direction, which would warn every contributor
running from source.

The plugin also announces its handler list, so a tool it lacks is refused with a remedy
before anything is written, rather than reaching it and coming back `Unknown request type`.
A plugin that announces nothing is given the benefit of the doubt: it predates the
mechanism, and refusing its every call would break a setup that works.

### Serialization budget

`makeBudget(maxNodes, maxDepth)` is shared across the whole walk rather than applied per
branch, so the cost of an answer is bounded by the answer and not by the shape of the tree.
The walk is sequential rather than `Promise.all` because the budget must be spent in tree
order — otherwise which nodes survive depends on which promises settle first, and a
truncated answer is not reproducible.

A node whose children were withheld still reports `childCount` and `childrenOmitted`, and
the tree carries `truncated`. A short answer must never be mistaken for a whole one.

## Rejected

### Pairing token

`networkAccess.allowedDomains` is `["*"]` and there is no authentication, so any process
that can open a WebSocket to `127.0.0.1:1994` can drive the open Figma file. A six-digit
pairing code was designed and rejected: it is paid for in the smooth-connect experience,
which is the plugin's advantage over REST-API-based Figma MCP servers.

The risk is unchanged and recorded here rather than closed. If it is revisited, the
least disruptive shape is a code required for destructive tools only, not for the
connection.

### Live Dev Mode codegen

The obvious design — `codegen.on("generate")` → WebSocket → server → ask the MCP client for
a completion — needs two things: a plugin-initiated request direction through the bridge,
which does not exist, and **MCP sampling**, which is optional in the protocol and absent
from the clients this server targets. Building a second protocol direction for a feature no
client can run is not worth the maintenance surface.

The flow is inverted instead. The client generates code with the repository in front of it
and attaches it with `set_codegen_result`; the provider serves what is stored. It is written
to *shared* plugin data, so it travels with the file and every teammate's Dev Mode shows it,
not only the machine that generated it. Lookup walks node → the instance's main component →
its component set → ancestors, because a designer clicks the instance, not the component
that defines it, and often a layer inside it.

**The cost:** the code does not update when the design changes. It is regenerated by calling
the tool again.
