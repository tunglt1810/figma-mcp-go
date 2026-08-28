# Walkthrough — Layered packages, reliability, observability

**Date:** 2026-08-28
**Spec:** `docs/specs/2026-08-28-go-backend-architecture-design.md`
**Plan:** `docs/plans/2026-08-28-go-backend-architecture-plan.md`

This is the record of what actually landed and why, including the places the
plan turned out to be wrong. Read the spec for the reasoning that led here; read
this for what the code looks like now.

## What changed, in one paragraph

`internal/` was one flat package of 21 files holding the plugin WebSocket, the
leader/follower cluster, the tool table and Figma's domain rules. It is now four
packages with a one-way dependency direction that `make deps-check` enforces.
Tool registration and argument checking, which existed on two paths each, now
exist on one. Five reliability defects and three observability gaps were fixed
on top of the settled structure.

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

- **`clipboard.go` lives in `bridge`.** Its only caller is `readLoop` handling
  the `copy_to_clipboard` message, which the plugin sends on its own initiative.
  No tool produces it.
- **`timeout.go` lives in `bridge`,** with `FollowerTimeoutFor` exported for
  `cluster`, which already imports `bridge`. A separate package would have added
  a package without removing an edge.
- **`ValidateRPC` went with `tools`, not `figma`.** It is a `specRegistry`
  lookup, so it belongs with the table. `ValidNodeID` and friends are Figma's
  rules and hold whether or not a tool table exists.

`deps-check` blocks ten edges and catches transitive ones: adding an import of
`cluster` to `tools` reports both `tools -> cluster` and `tools -> bridge`.

Each edge is checked twice, over the production import graph and again with
`go list -deps -test`, since a test-only import is invisible to the first and is
a plausible way for the coupling to arrive. Eight of the ten are checked both
ways. `tools -> cluster` and `tools -> bridge` are production-only, because
`internal/tools/leader_rpc_test.go` crosses them on purpose: it drives the
leader's `/rpc` with the real `Check` and a real `cluster.Leader`, which is the
only way to pin the "checked exactly once" invariant at that entry point from
outside it. Those two are the only test-side crossings in the tree.

The target starts by asserting it can still see the known `cluster -> bridge`
edge. It reports a violation when a `grep` succeeds, so without that assertion a
wrong module path, a renamed package or a `go list` that errors would print
"layering holds" while checking nothing.

## The three seams

**`tools.Sender`** returns `(any, error)`. That is what keeps `tools` from
importing `bridge`, and it collapses a duplicated branch: `resp.Error != ""`
used to sit next to `err != nil` at three sites in the tool layer, both building
the same `mcp.NewToolResultError`. `cluster.Node.Send` turns a plugin-reported
error into a Go error at the boundary.

**`tools.Check`** normalises node IDs and then validates against the spec table.
Two call sites: the handlers `tools` builds, and the leader's `/rpc`. `cluster`
does not import `tools` — it declares `type Guard` and `cmd` supplies `Check` at
`NewNode`, which passes it to `NewLeader` on every promotion. It has to be
threaded rather than handed straight to `NewLeader`, because `cmd` does not
build the `Leader`: `Node.BecomeLeader` does, at any point during a takeover.

The invariant to preserve when touching this: **every call is checked exactly
once before reaching the plugin.** It used to live in `Node.Send`, the last
point of convergence. It now lives at the only point of entry. Two existing
tests pin both directions — `TestSpecRegistry_MatchesRegisteredTools` and
`TestSpecRegistry_CoversEveryTool`.

**One registration loop.** `toolSpec` gained a `Custom func(Sender) customHandler`
field, so the two tools that do work in Go declare it in the table like everyone
else. `RegisterTools` is a loop over `allSpecs()`. Deleted: `tools_read.go`,
eleven `registerXTools`, `registerSpecs`, `registerCustom`, `specHandler`. The
group list exists in one place, so a group cannot reach clients without rules or
have rules without reaching clients.

Side effect: `registerCustom` used to validate and then call `Node.Send`, which
validated again. `export_frames_to_pdf` and `save_screenshots` no longer check
twice.

## The reliability fixes

**A cancelled call no longer closes the shared socket.** `Bridge.Send` passed
the caller's context to `conn.Write`. For the duration of a write
`coder/websocket` registers `context.AfterFunc(ctx, c.close)` (`conn.go:171`,
`write.go:276`), so a cancel landing while the write was parked on a full socket
buffer dropped the connection for every other in-flight request. The write now
uses a context that never cancels.

> A write deadline was the first attempt and was wrong for exactly the same
> reason — it fires the same `AfterFunc`, moving the cause from "the caller hung
> up" to "ten seconds passed". There is no safe deadline here. A parked write is
> resolved by the keepalive, which already drops a peer that stopped answering.

**`Close` is bounded.** `Conn.Close` runs the handshake with a 5s budget
(`close.go:199`) and then waits on goroutines for up to 15s (`close.go:231`). A
plugin that vanished without a close frame therefore delayed process exit. The
graceful close now runs on its own goroutine with a 1s `closeGrace`.

**A handover no longer looks like an absent plugin.** The leader dies, a
follower notices in 3–5s, binds the port, and the plugin reconnects 1.5s later
(`RECONNECT_DELAY_MS` in `plugin/src/ui/App.svelte`). `Send` now waits up to
`connectGrace` (2s) on a channel that `HandleUpgrade` closes, instead of
answering "plugin not connected" for a plugin that is on its way back.

**`RoleUnknown` reports itself.** It used to fall through to the follower
branch, post to a port nobody held, and surface as `connection refused`.
Alongside it, `Election.retryUntilSettled` re-checks every 200ms while the role
is Unknown, so a startup race settles well inside one 3–5s monitor tick.

**Two HTTP timeouts, and deliberately only two.**

```go
ReadHeaderTimeout: l.readHeaderTimeout,  // 5s
IdleTimeout:       60 * time.Second,
```

`ReadTimeout` and `WriteTimeout` stay zero because both are wrong for what this
server carries: a `WriteTimeout` would cap `/rpc`, where a
`batch_execute_pipeline` response legitimately takes up to `MaxToolTimeout`, and
a `ReadTimeout` would cap reading the 32 MB body. `ReadHeaderTimeout` is safe on
every path — `net/http` restores the read deadline to the zero time after the
headers when `ReadTimeout` is zero. `/rpc` reads through a 32 MB
`http.MaxBytesReader`.

> The WebSocket is *not* the reason, though an earlier version of this document
> said it was. `net/http` clears the deadline itself when a handler hijacks
> (`server.go`, `hijackLocked` → `rwc.SetDeadline(time.Time{})`), so the plugin
> socket would survive either timeout. Cross-review caught the claim.

## Observability

`log/slog`, no logging package. `cmd` calls `slog.SetDefault` once; each package
resolves its logger through a small function:

```go
func log() *slog.Logger { return slog.Default().With("component", "bridge") }
```

A function rather than a package variable, because a package variable is
initialised before `main` installs the handler and would capture the stock one,
ignoring the configured level.

`FIGMA_MCP_LOG` takes `debug`, `info`, `warn`, `error`; anything unrecognised is
`info`, since a typo should not silence the server. Tool parameters — the user's
text, colours and names — now appear only at `debug`; `info` carries the tool
name, node count and `paramBytes`. `/ping` returns `role`, `connected`,
`pending` and `uptimeSeconds` alongside `status` and `version`.

## Found in cross-review, after the work

**A `Custom` handler could reach the plugin unchecked.** `saveScreenshotItem`
calls `get_screenshot` once per item with params it builds itself, so checking
the arguments `save_screenshots` was invoked with says nothing about them. On
`main` this was covered by accident: `Node.Send` validated by the tool name
actually being sent, so the inner call met `getScreenshotSpec`. Removing that as
"double validation" was right for `export_frames_to_pdf` — same tool name, no
params — and wrong for `save_screenshots`, where it was the only check the inner
call ever had. A per-item `format` of `GIF` on a path whose extension implies
nothing reached the plugin.

Fixed by handing `Custom` handlers a `checkedSender`, which applies `Check` under
the name of whatever tool a call actually names. **The invariant is about Sender
calls, not about entry points** — that phrasing in the spec is what hid this.

## The follow-up round

A second cross-review, off the settled work. Four of these were on its list; the
last three it found on the way and are older than any of this work.

**`HandleUpgrade` froze the bridge across a close handshake.** It closed the
displaced connection while holding `b.mu`. A peer alive at TCP level but not
answering — laptop asleep, Figma reloading its UI — makes the library spend its
whole handshake budget, and under the lock that stalls every `Send`,
`IsConnected`, `Pending` and `MarshalJSON` in the process: 5.0007s measured. The
close moved off the lock and onto its own goroutine, through the same
`closeBounded` that `Close` already used. It cannot stay on this goroutine
either — that would delay the new connection's `readLoop`, which is the
reconnect the user is waiting for.

**A caller could not give up on the write slot.** Writes go out under a context
that never cancels, so a write parked on a full socket buffer holds the slot
until the keepalive clears it. `b.wmu` was a `sync.Mutex`, which consults
nothing, so every other caller waited out that whole window regardless of its
own deadline. It is now a one-slot channel with `lockWrite(ctx)`: what the
caller abandons is the *wait*, never the write, so the shared connection is
untouched. This does not fix head-of-line blocking and cannot — one socket, one
write at a time. Registration of the pending entry and its timer moved to after
the slot is acquired, so a request no longer spends its budget queueing.

> A connection-scoped context was the obvious alternative and was rejected:
> every path that would cancel it already closes the socket, which is itself
> what frees a parked write, so it buys nothing and adds a field that has to
> stay in step with `b.conn`.

**The keepalive comment described a mechanism that does not exist.** It claimed
the ping stayed off the bridge's write lock because control frames bypass the
data path. They do not: `Ping` goes through `writeControl` → `writeFrame` and
takes the same `c.writeFrameMu` as a data message (`write.go:231`, `:244`). The
reason to stay off the lock is the opposite one — the keepalive is what *clears*
a parked write, so waiting on the lock that write holds would park it behind the
problem it exists to resolve.

**`deps-check` could not see a test-only import,** and reported a violation by a
`grep` succeeding, so a wrong module path or a `go list` that errored printed
"layering holds" while checking nothing. Both fixed; see the note under *The
shape now*.

**The server-info reply could starve the connection it served.** It was written
on the read goroutine, which is the one goroutine that has to be inside
`conn.Read` for the library to process anything the peer sends — `handleControl`
is only reached from `reader` (`read.go:289`, `:368`). A reply parked behind
another write therefore stopped the connection being read at all: pings went
unanswered and the keepalive dropped a plugin that was perfectly healthy, and a
close frame from the plugin went unnoticed. The reply now runs on its own
goroutine, with the wait for the write slot bounded and the write itself still
uncancellable.

**A failed ping is not proof the peer is gone.** `writeControl` caps its own
wait for the frame lock at 5s (`write.go:232`), so a large send still draining
to a healthy plugin fails the ping while the plugin is fine — and the keepalive
dropped the connection on that first failure. A plugin that has sent us
something since the last tick is demonstrably alive, so the failure is forgiven.
Bounded at `keepaliveForgiveness` (3), because the keepalive is also the only
thing that clears a parked write; the worst case moves from one ping round to
three, about a minute with production defaults.

**`TestBridgeSend_Timeout` tested the caller's deadline, not the bridge's
timer.** It passed a 50ms context, so it exercised the `ctx.Done()` branch and
would have been green whatever the timer did. Renamed to what it does, and the
branch it promised now has its own test: `toolTimeout` is indirected on the
`Bridge` so a test can drive the timer without sitting through 30 seconds.

## What the plan got wrong

Recorded because the reasoning, not just the outcome, is what a later reader
needs.

1. **The write-deadline fix.** Described above. The plan specified a 10s write
   timeout; it failed its own test after exactly 10s.
2. **Two tests that proved nothing.** The first test written for the
   registration loop passed against the old code, because `registerCustom`
   already validated — it could not tell before from after. Same for the first
   cancellation test: the dangerous window is only open while a write is
   blocked, and a small payload on loopback never blocks. Fixed by registering
   against a bare `Sender` in the first case, and by filling the socket buffer
   with an 8 MB frame against a client that never reads in the second. That one
   now fails in 0.22s against the old code and passes against the new.
3. **`Close()` was not leaving the read loop blocked.** The spec's original
   claim did not survive reading the library: `Conn.Close` runs the handshake
   and then `waitGoroutines`, so the loop does exit. The real cost was shutdown
   latency, which is what got fixed. `context.Background()` in `readLoop` is a
   smell, not a defect, and was left alone.
4. **The suite did not get faster.** The plan expected dropping ~30 real TCP
   dials to a dead port to cut the 6.666s baseline. Wall clock is now 6.97s. The
   dials did go away, but the 2s `connectGrace` added to every test without a
   plugin ate the saving, and the original slowness was always the keepalive
   tests in `bridge`.

## Verification

The golden snapshot `internal/tools/testdata/tools_schema.json` is unchanged
through all of it — sha256 `8914d70197487e6a53e8b4e4b9edc83df7f667ed553914793573cc3bfad1d874`,
the same value recorded before the first commit. 63 tools, no schema change, so
no MCP client sees anything different.

```
make deps-check   layering holds
go test ./...     all 7 packages pass
bun test          352 pass, 0 fail
make fmt-check    clean
go vet ./...      clean
```

## Left alone

`makeHandler` (`internal/tools/handlers.go`) has no caller in production code —
only its own test. It is pre-existing dead code, not something this work
created, and removing it is a separate decision.
