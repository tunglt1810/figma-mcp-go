# Walkthrough — Layered packages, reliability, observability

**Date:** 2026-08-28
**Spec:** `docs/specs/2026-08-28-go-backend-architecture-design.md`
**Plan:** `docs/plans/2026-08-28-go-backend-architecture-plan.md`

This records what actually shipped and why, including where the plan was wrong.
Read the spec for the reasoning that led here. Read this for what the code looks
like now.

## What changed, in one paragraph

`internal/` used to be one flat package of 21 files. It held the plugin
WebSocket, the leader/follower cluster, the tool table and Figma's domain rules.
It is now four packages with one-way dependencies, enforced by `make
deps-check`. Tool registration and argument checking each had two code paths.
Now each has one. On top of the new structure, five reliability bugs and three
observability gaps were fixed.

## The shape now

```
cmd ─┬─> tools ──> figma
     ├─> cluster ──> bridge
     └─> prompts
```

| Package | Code | Tests | Holds |
|---|---:|---:|---|
| `internal/bridge` | 615 | 652 | One WebSocket to the plugin, request/response matching, timeout budgets, the OS clipboard write the plugin asks for |
| `internal/cluster` | 616 | 1064 | Leader election, `/ping` and `/rpc`, routing a call to the bridge or to the leader |
| `internal/figma` | 125 | 59 | Node IDs, hex colours, reactions, constraints, blend modes |
| `internal/tools` | 2121 | 3110 | The 63-tool table, schema generation, argument checking, handlers |
| `internal/prompts` | 1014 | 76 | Unchanged |

Three placement decisions worth knowing:

- **`clipboard.go` lives in `bridge`.** Its only caller is `readLoop`, when it
  handles the `copy_to_clipboard` message. The plugin sends that message by
  itself. No tool produces it.
- **`timeout.go` lives in `bridge`,** and exports `FollowerTimeoutFor` for
  `cluster`, which already imports `bridge`. A separate package would have added
  a package without removing an edge.
- **`ValidateRPC` went to `tools`, not `figma`.** It is a `specRegistry`
  lookup, so it belongs with the table. `ValidNodeID` and similar functions are
  Figma's rules, and they hold whether or not a tool table exists.

`deps-check` blocks ten edges and also catches indirect ones: adding an import
of `cluster` to `tools` reports both `tools -> cluster` and `tools -> bridge`.

Each edge is checked twice: over the production import graph, and again with
`go list -deps -test`. The first check cannot see a test-only import, and a test
is a likely way for coupling to sneak in. Eight of the ten edges are checked both
ways. `tools -> cluster` and `tools -> bridge` are checked for production only,
because `internal/tools/leader_rpc_test.go` crosses them on purpose. It drives
the leader's `/rpc` with the real `Check` and a real `cluster.Leader`. That is
the only way to test the "checked exactly once" rule at that entry point from
outside. Those two are the only test-side crossings in the tree.

The target starts by checking that it can still see the known `cluster ->
bridge` edge. It reports a violation when a `grep` succeeds. Without that first
check, a wrong module path, a renamed package, or a `go list` that errors would
print "layering holds" while checking nothing.

## The three seams

**`tools.Sender`** returns `(any, error)`. This keeps `tools` from importing
`bridge`, and it removes a duplicated branch. At three places in the tool layer,
`resp.Error != ""` used to sit next to `err != nil`, and both built the same
`mcp.NewToolResultError`. `cluster.Node.Send` turns a plugin error into a Go
error at the boundary.

**`tools.Check`** normalises node IDs, then validates against the spec table. It
has two callers: the handlers that `tools` builds, and the leader's `/rpc`.
`cluster` does not import `tools`. Instead it declares `type Guard`, and `cmd`
passes `Check` to `NewNode`, which passes it to `NewLeader` on every promotion.
It must be passed through `Node`, not straight to `NewLeader`, because `cmd`
does not build the `Leader`. `Node.BecomeLeader` does, at any point during a
takeover.

The rule to keep when changing this: **every call is checked exactly once before
it reaches the plugin.** The check used to live in `Node.Send`, the last point
all calls pass through. It now lives at the only entry point. Two existing tests
cover both directions: `TestSpecRegistry_MatchesRegisteredTools` and
`TestSpecRegistry_CoversEveryTool`.

**One registration loop.** `toolSpec` gained a `Custom func(Sender)
customHandler` field, so the two tools that do work in Go are declared in the
table like the rest. `RegisterTools` is a loop over `allSpecs()`. Deleted:
`tools_read.go`, eleven `registerXTools`, `registerSpecs`, `registerCustom`,
`specHandler`. The group list exists in one place. So a group cannot reach
clients without rules, or have rules without reaching clients.

Side effect: `registerCustom` used to validate and then call `Node.Send`, which
validated again. `export_frames_to_pdf` and `export_screenshots` no longer check
twice.

## The reliability fixes

**A cancelled call no longer closes the shared socket.** `Bridge.Send` passed
the caller's context to `conn.Write`. During a write, `coder/websocket`
registers `context.AfterFunc(ctx, c.close)` (`conn.go:171`, `write.go:276`). So
a cancel that arrived while the write was stuck on a full socket buffer dropped
the connection for every other request in flight. The write now uses a context
that never cancels.

> The first attempt was a write deadline. It was wrong for exactly the same
> reason: it fires the same `AfterFunc`, so the cause just changes from "the
> caller hung up" to "ten seconds passed". No deadline is safe here. The
> keepalive clears a stuck write, because it already drops a peer that stopped
> answering.

**`Close` has a time limit.** `Conn.Close` runs the handshake with a 5s budget
(`close.go:199`), then waits for goroutines for up to 15s (`close.go:231`). So a
plugin that disappeared without a close frame delayed process exit. The graceful
close now runs on its own goroutine, with a 1s `closeGrace`.

**A handover no longer looks like a missing plugin.** The leader dies, a
follower notices within 3–5s and binds the port, and the plugin reconnects 1.5s
later (`RECONNECT_DELAY_MS` in `plugin/src/ui/App.svelte`). `Send` now waits up
to `connectGrace` (2s) on a channel that `HandleUpgrade` closes. Before, it
answered "plugin not connected" for a plugin that was about to come back.

**`RoleUnknown` reports itself.** It used to fall through to the follower
branch, post to a port nobody held, and show up as `connection refused`. Along
with this, `Election.retryUntilSettled` checks again every 200ms while the role
is Unknown, so a startup race settles well inside one 3–5s monitor tick.

**Two HTTP timeouts, and only two, on purpose.**

```go
ReadHeaderTimeout: l.readHeaderTimeout,  // 5s
IdleTimeout:       60 * time.Second,
```

`ReadTimeout` and `WriteTimeout` stay zero, because both are wrong for this
server. A `WriteTimeout` would cap `/rpc`, where a `batch_execute_pipeline`
response can rightly take up to `MaxToolTimeout`. A `ReadTimeout` would cap
reading the 32 MB body. `ReadHeaderTimeout` is safe on every path: when
`ReadTimeout` is zero, `net/http` resets the read deadline to zero after the
headers. `/rpc` reads through a 32 MB `http.MaxBytesReader`.

> The WebSocket is *not* the reason, although an earlier version of this
> document said so. `net/http` clears the deadline itself when a handler hijacks
> (`server.go`, `hijackLocked` → `rwc.SetDeadline(time.Time{})`), so the plugin
> socket would survive either timeout. Cross-review caught the mistake.

## Observability

`log/slog`, with no logging package. `cmd` calls `slog.SetDefault` once. Each
package gets its logger through a small function:

```go
func log() *slog.Logger { return slog.Default().With("component", "bridge") }
```

It is a function, not a package variable. A package variable is set before
`main` installs the handler, so it would keep the stock handler and ignore the
configured level.

`FIGMA_MCP_LOG` takes `debug`, `info`, `warn` or `error`. Any unknown value
means `info`, because a typo should not silence the server. Tool parameters (the
user's text, colours and names) now appear only at `debug`. `info` shows the tool
name, node count and `paramBytes`. `/ping` returns `role`, `connected`,
`pending` and `uptimeSeconds`, as well as `status` and `version`.

## Found in cross-review, after the work

**A `Custom` handler could reach the plugin without a check.**
`exportScreenshotItem` calls `get_screenshot` once per item, with params it
builds itself. So checking the arguments `export_screenshots` was called with
says nothing about those calls. On `main` this was covered by accident:
`Node.Send` validated by the name of the tool actually being sent, so the inner
call was checked against `getScreenshotSpec`. Removing that as "double
validation" was right for `export_frames_to_pdf` (same tool name, no params). It
was wrong for `export_screenshots`, where it was the only check the inner call
ever had. A per-item `format` of `GIF`, on a path whose extension implies no
format, reached the plugin.

The fix gives `Custom` handlers a `checkedSender`. It runs `Check` under the name
of whatever tool each call names. **The rule is about Sender calls, not about
entry points.** The spec's wording, which said otherwise, is what hid this bug.

Since then, `get_screenshot` is no longer a tool of its own. It is the handler
that `export_screenshots` calls per item, so there is no spec for the
`checkedSender` to apply. Instead, `export_screenshots` validates the item schema
itself, one level deeper than a `paramSpec` enum can reach.

## The follow-up round

A second cross-review, after the work had settled. Four of these fixes were on
its list. It found the last three along the way, and they are older than any of
this work.

**`HandleUpgrade` froze the bridge during a close handshake.** It closed the old
connection while holding `b.mu`. A peer that is alive at the TCP level but not
answering (laptop asleep, Figma reloading its UI) makes the library wait for its
whole handshake budget. Under the lock, that blocked every `Send`,
`IsConnected`, `Pending` and `MarshalJSON` in the process: 5.0007s measured. The
close moved out of the lock and onto its own goroutine, through the same
`closeBounded` that `Close` already used. It cannot stay on the current
goroutine either, because that would delay the new connection's `readLoop`,
which is the reconnect the user is waiting for.

**A caller could not give up waiting for the write slot.** Writes use a context
that never cancels, so a write stuck on a full socket buffer holds the slot
until the keepalive clears it. `b.wmu` was a `sync.Mutex`, which cannot be
cancelled. So every other caller waited that whole time, whatever its own
deadline. It is now a one-slot channel with `lockWrite(ctx)`. A caller that
gives up drops only the *wait*, never the write, so the shared connection is not
touched. This does not fix head-of-line blocking, and cannot: one socket means
one write at a time. The pending entry and its timer are now registered after
the slot is taken, so a request no longer spends its budget waiting in line.

> A connection-scoped context was the obvious alternative, and was rejected.
> Every path that would cancel it already closes the socket, and closing the
> socket is what frees a stuck write. So it gains nothing, and adds a field that
> must stay in step with `b.conn`.

**The keepalive comment described a mechanism that does not exist.** It said the
ping stayed off the bridge's write lock because control frames skip the data
path. They do not: `Ping` goes through `writeControl` → `writeFrame` and takes
the same `c.writeFrameMu` as a data message (`write.go:231`, `:244`). The real
reason to stay off the lock is the opposite. The keepalive is what *clears* a
stuck write, so waiting on the lock that write holds would leave it stuck behind
the very problem it exists to fix.

**`deps-check` could not see a test-only import.** It also reported a violation
when a `grep` succeeded, so a wrong module path or a failing `go list` printed
"layering holds" while checking nothing. Both are fixed. See the note under
*The shape now*.

**The server-info reply could block the connection it served.** It was written
on the read goroutine. That is the one goroutine that must be inside `conn.Read`
for the library to handle anything the peer sends: `handleControl` is only
called from `reader` (`read.go:289`, `:368`). So a reply stuck behind another
write stopped the connection from being read at all. Pings went unanswered, the
keepalive dropped a healthy plugin, and a close frame from the plugin went
unnoticed. The reply now runs on its own goroutine. The wait for the write slot
has a time limit, and the write itself still cannot be cancelled.

**A failed ping does not prove the peer is gone.** `writeControl` waits at most
5s for the frame lock (`write.go:232`). So a large send that a healthy plugin is
still reading makes the ping fail, and the keepalive used to drop the connection
on that first failure. A plugin that has sent us something since the last tick
is clearly alive, so the failure is forgiven. The limit is
`keepaliveForgiveness` (3), because the keepalive is also the only thing that
clears a stuck write. The worst case grows from one ping round to three, about a
minute with production defaults.

**`TestBridgeSend_Timeout` tested the caller's deadline, not the bridge's
timer.** It passed a 50ms context, so it ran the `ctx.Done()` branch and would
have passed whatever the timer did. It was renamed to match what it tests. The
branch its old name promised now has its own test: `toolTimeout` is a field on
`Bridge`, so a test can drive the timer without waiting 30 seconds.

## What the plan got wrong

Recorded here because a later reader needs the reasoning, not just the outcome.

1. **The write-deadline fix.** Described above. The plan specified a 10s write
   timeout. It failed its own test after exactly 10s.
2. **Two tests that proved nothing.** The first test written for the
   registration loop passed against the old code, because `registerCustom`
   already validated. It could not tell before from after. The same was true of
   the first cancellation test: the risky window only opens while a write is
   blocked, and a small payload on loopback never blocks. The first was fixed by
   registering against a bare `Sender`. The second was fixed by filling the
   socket buffer with an 8 MB frame, sent to a client that never reads. That test
   now fails in 0.22s against the old code and passes against the new code.
3. **`Close()` did not leave the read loop blocked.** The spec's original claim
   did not hold up after reading the library: `Conn.Close` runs the handshake
   and then `waitGoroutines`, so the loop does exit. The real cost was shutdown
   delay, and that is what got fixed. `context.Background()` in `readLoop` is a
   smell, not a bug, and was left alone.
4. **The test suite did not get faster.** The plan expected that removing about
   30 real TCP dials to a dead port would cut the 6.666s baseline. It now takes
   6.97s. The dials did go away, but the 2s `connectGrace`, added to every test
   without a plugin, used up the saving. The original slowness was always the
   keepalive tests in `bridge`.

## Verification

The golden snapshot `internal/tools/testdata/tools_schema.json` did not change
at any point. Its sha256 is
`8914d70197487e6a53e8b4e4b9edc83df7f667ed553914793573cc3bfad1d874`, the same
value recorded before the first commit. 63 tools and no schema change, so no MCP
client sees any difference.

```
make deps-check   layering holds
go test ./...     all 7 packages pass
bun test          352 pass, 0 fail
make fmt-check    clean
go vet ./...      clean
```

## Left alone

`makeHandler` (`internal/tools/handlers.go`) has no caller in production code,
only its own test. It is dead code that existed before this work, and removing
it is a separate decision.
