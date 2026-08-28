# Go Backend Architecture — Layered Packages, Reliability, Observability

**Date:** 2026-08-28
**Scope:** `internal/`, `cmd/`, `Makefile`, `.github/workflows/ci.yml`. The Figma plugin is read but not changed.
**Status:** design, approved section by section. Implementation plan to follow.

## Why

Three goals, chosen in that order of weight:

1. **Extensibility and testability.** Adding a tool or writing a test should touch one bounded piece of code.
2. **Runtime reliability.** A cancelled request must not take the server down with it.
3. **Observability.** A failure on a user's machine should be diagnosable from stderr.

Network security is explicitly **out of scope**. The `--ip` flag can still open the port to
the network with no authentication, and this design does not change that.

Everything in the 2026-08-15 architecture review has shipped. Nothing below repeats it.

## Baseline (2026-08-28)

| Metric | Value |
|---|---|
| `internal/testdata/tools_schema.json` | sha256 `8914d70197487e6a53e8b4e4b9edc83df7f667ed553914793573cc3bfad1d874` |
| Tools | 63 |
| `go test ./...` | PASS — `internal` 6.666s, `internal/prompts` cached |
| `gofmt -l .` | clean |
| `go vet ./...` | clean |

The golden snapshot hash is the load-bearing number. Steps 1 and 2 below must not change
it; steps 3 and 4 change behaviour but still must not change it.

## Current shape and what is wrong with it

`internal/` is a single flat package of 21 non-test files. It holds the WebSocket bridge,
the leader/follower/election cluster, the declarative tool table, Figma domain validation,
and OS-level side effects (clipboard, writing PNG and PDF files). No boundary separates
them; the only thing keeping the layers apart is convention.

Three concrete consequences:

**The tool group list exists twice.** `specGroups()` (`toolspec.go:422`) feeds validation.
Eleven `registerXTools` functions feed registration. A group added to one and not the other
produces a tool that runs without validation, or a spec that never reaches a client.

**Validation runs a different number of times per path.** `specHandler` does not validate —
`Node.Send` does. `registerCustom` validates and then calls `Node.Send`, which validates
again. So `export_frames_to_pdf` and `save_screenshots` validate twice while every other
tool validates once.

**Every tool result checks two error channels that mean the same thing.** `resp.Error != ""`
appears six times; at the three sites inside the tool layer it builds the same
`mcp.NewToolResultError` as the `err != nil` branch immediately above it.

## Package layout

Five packages, cut along the seams that already exist.

| Package | Files moved in | Responsibility |
|---|---|---|
| `internal/bridge` | `bridge.go`, `timeout.go`, `clipboard.go`, `BridgeRequest`/`BridgeResponse` from `types.go` | One WebSocket to the plugin, matching `requestId`, timeout budgets |
| `internal/cluster` | `node.go`, `leader.go`, `follower.go`, `election.go`, `RPCRequest`/`RPCResponse`/`Role` from `types.go` | Electing a leader, routing a call to the bridge or to `/rpc` |
| `internal/figma` | `schema.go` minus `ValidateRPC` | Figma domain rules: node ID, hex colour, reaction, constraint, blend mode |
| `internal/tools` | `toolspec.go` (plus `ValidateRPC`), `tools.go`, the eleven `tools_*.go` files | The tool table, schema generation, validation, handlers |
| `internal/prompts` | unchanged | MCP prompts |

Dependency direction, one way:

```
cmd ─┬─> tools ──> figma
     ├─> cluster ──> bridge
     └─> prompts
```

`tools` does not import `cluster`: it declares the `Sender` interface and `cluster.Node`
satisfies it. `cluster` does not import `tools`: `cmd` injects the validation function into
`NewLeader`.

Three placement decisions that are not obvious:

**`clipboard.go` belongs to `bridge`, not `tools`.** Its only caller is `bridge.readLoop`
handling the `copy_to_clipboard` message, which the plugin sends on its own initiative —
no tool call produces it. A separate package for 40 lines with one caller would be waste.

**`timeout.go` belongs to `bridge`,** with `FollowerTimeoutFor` exported for `cluster`.
`cluster` already imports `bridge`, so this adds no edge. A separate `internal/timeouts`
package would add a package without removing an edge.

**`ValidateRPC` follows `tools`, not `figma`.** It is a `specRegistry` lookup, so it belongs
with the table. `ValidNodeID`, `ValidHexColor` and `validateReaction` are Figma's rules and
hold whether or not a tool table exists, so they belong to `figma`.

## Seams

### `tools.Sender`

```go
type Sender interface {
	Send(ctx context.Context, tool string, nodeIDs []string, params map[string]any) (any, error)
}
```

Returning `any` rather than `BridgeResponse` is what keeps `tools` from importing `bridge`.

It also collapses the two error channels. `cluster.Node.Send` turns a plugin-reported error
into a Go `error` at the boundary, so the tool layer has one branch instead of two. The tool
layer loses the ability to tell "the plugin reported an error" from "the plugin could not be
reached" — it already could not tell them apart, and the distinction survives in `cluster`,
where the logging happens. No dedicated error type until something needs `errors.As`.

### `tools.Check`

```go
func Check(tool string, nodeIDs []string, params map[string]any) ([]string, map[string]any, error)
```

Normalises node IDs, then validates against `specRegistry`. That order matters: the hyphen
format an LLM emits has to be accepted, not rejected by the very validation that exists to
tolerate it.

Two call sites:

- The tool registration layer in `tools` — every local tool call passes through it.
- The leader's `/rpc` handler — the boundary where another process's input arrives.
  `cluster` declares `type Guard func(string, []string, map[string]any) ([]string, map[string]any, error)`.

  The guard has to be threaded, because `cmd` does not build the `Leader`: `Node.BecomeLeader`
  does, and it can do so at any time during a takeover. So `NewNode` takes the guard, stores
  it, and passes it to `NewLeader` on every promotion — `cmd` supplies `tools.Check` once at
  `NewNode`.

`Node.Send` stops normalising and stops validating; it routes and nothing else.

**The invariant this must preserve:** every call validates exactly once before reaching the
plugin. P1-5 in the previous review was a violation of it, fixed then by moving validation
into `Node.Send` — the last point of convergence. Moving it to the registration layer
preserves it via the only point of entry: no tool reaches the plugin except through a
handler that `tools` built. Two existing tests already pin both directions and need no
change: `TestSpecRegistry_MatchesRegisteredTools` and `TestSpecRegistry_CoversEveryTool`
(`toolspec_wire_test.go:177,191`).

Consequence: `registerCustom`'s double validation disappears.

**Correction, recorded after cross-review:** "the only point of entry" is the wrong frame,
and dropping the double validation on the strength of it caused a regression. A `Custom`
handler can reach the plugin under a *different* tool's name — `save_screenshots` calls
`get_screenshot` once per item with params it builds itself — and checking the arguments it
was invoked with says nothing about those. `Node.Send` had been covering that by accident,
because it validated by the name actually being sent. The invariant is therefore about
**Sender calls, not entry points**, and `Custom` handlers receive a `checkedSender` that
applies `Check` under the name of whatever tool each call names.

### One registration loop

`toolSpec` gains a field:

```go
// Custom, when set, replaces the default forwarder. It takes the Sender so the
// tool can do work in Go around its plugin calls.
Custom func(Sender) customHandler
```

It takes a `Sender` rather than a `customHandler` because specs are package-level variables
and cannot capture the sender at declaration time.

```go
func RegisterTools(s *server.MCPServer, sender Sender) {
	for _, spec := range allSpecs() {
		s.AddTool(buildTool(spec), handlerFor(sender, spec))
	}
}
```

`handlerFor` calls `Check`, then branches to `spec.Custom` if present and to the plain
forwarder otherwise. `registerReadTools`, `registerWriteTools`, the eleven `registerXTools`,
`registerSpecs` and `registerCustom` are all deleted.

`allSpecs()` is `specGroups()` (`toolspec.go:422`) renamed and flattened; `specRegistry` keeps
being built from it. Registration and validation then read the same list, so the group list
exists in exactly one place.

After this, `Node.Send` is about twelve lines and a tool test needs only a fake `Sender`.

## Test migration

Checking which symbols each test file touches shows the split is cleaner than the initial
estimate: **no test needs a newly exported symbol and none needs an `export_test.go`.** Every
test sits in the same package as what it touches, except `leader_test.go`, which uses
`Bridge` — already exported.

| Test today | Destination | Kind |
|---|---|---|
| `bridge_test.go`, `timeout_test.go`, `clipboard_test.go` | `bridge` | move as-is |
| `follower_test.go`, `leader_test.go`, `election_test.go`, `helpers_test.go` | `cluster` | move as-is |
| `tools_test.go`, `tools_schema_test.go`, `tools_golden_test.go`, `tools_read_export_test.go`, `tools_plugin_test.go`, `toolspec_wire_test.go` | `tools` | move as-is |
| `prompts/*_test.go` | unchanged | — |
| `schema_test.go` | split | see below |
| `node_test.go` | split | see below |
| `types_test.go` | split | see below |
| `tools_handler_test.go` | `tools`, helper reworked | see below |

**`schema_test.go` (1384 lines) splits at line 62, in one cut.** Lines 10–61
(`TestValidNodeID`, `TestNormalizeNodeID`) are pure domain rules and go to
`figma/schema_test.go`. Line 62 onward is 55 `TestValidateRPC_*` functions, all of which
need `specRegistry`, and they go to `tools/validate_test.go`. No test straddles the cut.

**`node_test.go` splits along the seam.** Routing and normalisation tests stay in `cluster`.
`TestNodeSend_RejectsInvalidArgsBeforeReachingPlugin` and `fakeSender` move to `tools`,
because validation moved there. The comment at `node_test.go:194` explaining the P1-5 history
moves with them and is rewritten for its new home: it documents an invariant, not a line of
code, and it is the most expensive thing in the file to lose.

**`types_test.go` splits by type.** `BridgeRequest`/`BridgeResponse` round-trip tests go to
`bridge`; `RPCRequest`/`RPCResponse`/`Role` tests go to `cluster`.

**`tools_handler_test.go`:** `newTestServer` returns `(*server.MCPServer, *fakeSender)`
instead of `(*server.MCPServer, *Node)`. It then has the same shape as `newWireTestServer`
(`toolspec_wire_test.go:19`) and the two merge. This is an unplanned win: today
`newTestServer` builds a real `Node` on port 19940 and each of roughly thirty tool calls
makes a real TCP dial to a dead port and waits for the failure. With a fake sender there is
no syscall at all, which should account for a good part of the `internal` package's 6.666s.

Three paths need hand edits:

- `internal/testdata/tools_schema.json` (70.5 KB) `git mv` to `internal/tools/testdata/`.
  Contents must not change by one byte.
- `tools_plugin_test.go:47`: `filepath.Join("..", "..", "plugin", "src")`.
- `freePort` is used only by `election_test`, `leader_test` and `node_test`, all of which
  land in `cluster`, so `helpers_test.go` goes to `cluster`. No `testutil` package needed.

One ripple: `callTool` (`tools_handler_test.go:28`) relies on "an Unknown-role node makes
every tool return `IsError=true`". Reliability item 4 changes the message for that state, so
any assertion on the message text will fail — correctly, and the assertion gets updated.

## Reliability

The plugin already reconnects on its own: `plugin/src/ui/App.svelte:51` schedules
`connect()` again after `RECONNECT_DELAY_MS = 1500`. Nothing on the plugin side needs
changing.

### bridge

**1. `conn.Write` must not receive the caller's context.** Verified in the library source,
`conn.go:171`:

```go
func (c *Conn) setupWriteTimeout(ctx context.Context) {
	stop := context.AfterFunc(ctx, func() {
		c.clearWriteTimeout()
		c.close()
	})
```

`write.go:276` calls it on every write. Cancelling the context passed to `Write` therefore
closes **the whole connection**, not just that write. `Bridge.Send` passes the MCP request's
context straight in (`bridge.go:310`), so one client cancelling mid-flight kills the shared
socket for every request in the air.

Fix: write with a dedicated context carrying a short deadline — 10s, enough for 100 MB over
loopback — while the caller's context keeps guarding the `select` below, as it does today.

**2. `Close` gets a bounded handshake (minor).** An earlier draft of this design claimed
`Close()` leaves the read loop blocked because `readLoop` holds `context.Background()`
(`bridge.go:181`). Reading the library disproves it: `Conn.Close` performs the close
handshake and then calls `waitGoroutines`, so the read loop does exit. `context.Background()`
there is a smell, not a defect.

The real cost is smaller and is about shutdown latency. `waitCloseHandshake` allows 5s
(`close.go:199`) and `waitGoroutines` allows 15s (`close.go:231`). A plugin that vanished
without sending a close frame — laptop asleep, network dropped — makes `Bridge.Close()` sit
there, and `Close` is on the shutdown path (`main.go:50-55` → `node.Stop` → `leader.Stop` →
`bridge.Close`). The process then takes seconds to exit after its MCP client has already let
go.

Fix: run the graceful close on its own goroutine and wait at most `closeGrace` (1s, overridable
in tests) before returning. The close frame is still sent in the normal case; an unresponsive
peer no longer holds up shutdown.

This item is minor and severable, like item 3.

**3. Handover window (optional).** The leader dies, a follower notices after 3–5s
(`Election.monitor` jitter), binds the port, and the plugin reconnects 1.5s later — up to
about 6.5s during which `Bridge.Send` answers `"plugin not connected"`. Proposal: when
`conn == nil`, wait up to 2s for a connection rather than failing immediately. That absorbs
the plugin's 1.5s reconnect entirely, and the message on expiry says a handover is in
progress rather than implying the plugin was never opened.

This item is severable — dropping it changes nothing else in the design.

### cluster

**4. `RoleUnknown` must not fall through to `follower.Send`.** Today (`node.go:165-168`) the
Unknown role has a nil leader, so the call falls to the follower branch, sends HTTP to a port
nobody is listening on, and the user reads `connection refused`. It should return an error
naming the actual state. Alongside it, `determineRole` should retry with a short backoff when
it finds "port taken but no healthy leader" instead of logging and waiting for the next 3–5s
tick (`election.go:66`).

**5. Timeouts on `http.Server`** at `leader.go:61` — and only two of them:

```go
srv := &http.Server{
	Handler:           mux,
	ReadHeaderTimeout: 5 * time.Second,
	IdleTimeout:       60 * time.Second,
	// No ReadTimeout/WriteTimeout: a WriteTimeout would cap a /rpc response
	// that is allowed to take MaxToolTimeout, and a ReadTimeout would cap
	// reading a 32 MB body.
}
```

`ReadHeaderTimeout` is safe because `net/http` restores the read deadline to the zero time
after the headers are read when `ReadTimeout` is zero.

**Correction, recorded after cross-review:** an earlier draft justified leaving both at zero
by claiming the hijacked WebSocket would inherit the deadline, because `coder/websocket`'s
`hijack.go` only locates the `Hijacker`. That is the wrong reason. `net/http` clears the
deadline itself — `hijackLocked` calls `rwc.SetDeadline(time.Time{})` before handing the
connection over — so the WebSocket is safe either way. The decision stands on the two
reasons above instead.

Optional alongside it: wrap `io.ReadAll(r.Body)` (`leader.go:113`) in `http.MaxBytesReader`.
`/rpc` currently reads without a limit.

## Observability

**6. `log/slog`, with no logging package.** `cmd` calls `slog.SetDefault` once with a
`slog.NewTextHandler` on stderr; each package keeps
`var log = slog.Default().With("component", "bridge")`. Stderr stays because stdout carries
the MCP protocol. The level comes from `FIGMA_MCP_LOG`, defaulting to `info`. The five global
loggers disappear without a sixth package appearing.

**7. Parameters stop being logged at info.** `bridge.go:306` and `follower.go:35` print the
whole `params` map on every call — that is the user's design content. At info: tool name,
node count, params size in bytes. The full map only at `debug`.

**8. `/ping` reports state:** `role`, `connected`, `pending` and `uptime` alongside the
existing `status` and `version`. `Bridge.MarshalJSON` (`bridge.go:398`) already computes the
middle two. `Follower.Ping` only reads the status code, so extra fields are backwards
compatible.

## User-visible changes

Four, all worth a changelog entry since the package ships on npm:

1. The error for "no leader yet" changes from `connection refused` to a message naming the
   state.
2. Log format changes from `[bridge] → req-120000-1 …` to slog text
   (`time=… level=INFO component=bridge …`). Anyone tailing stderr or grepping for the
   `[bridge]` prefix sees something different.
3. Parameters no longer appear in the default log output.
4. `/ping` returns more fields.

## Execution order

**Step 0 — baseline.** Recorded above. Verify `make fmt-check`, `go vet ./...` and
`make test-go` are clean before touching anything.

**Step 1 — the three seams, no files moved.** Add `Sender`, `Check`, `toolSpec.Custom` and
the single registration loop; shrink `Node.Send`; delete the thirteen registration
functions. Still one `package internal`.

Semantics first, movement second: every compile error in this step is then about the
semantic change rather than about imports, and step 2 becomes purely mechanical.

Verify: `make test-go` green; `TestToolSchemas_Golden` green **without** `-update`;
`TestSpecRegistry_MatchesRegisteredTools` and `TestSpecRegistry_CoversEveryTool` green;
golden sha matches step 0.

**Step 2 — the split.** `git mv` into the five packages, fix imports, split
`schema_test.go` / `node_test.go` / `types_test.go`, merge `newTestServer` with
`newWireTestServer`, `git mv` `testdata`, fix the plugin source path.  No logic changes.

Verify: `make test-go` green; golden sha still matches step 0; `git diff` on
`tools_schema.json` shows a rename and no content change.

Add a `deps-check` Makefile target, because this is what separates this approach from
merely tidying one package: **the compiler rejects cycles, not directions.** Forbidden edges:
`tools` may not reach `cluster` or `bridge`; `cluster` may not reach `tools` or `figma`
(it stops normalising and validating, so it has no reason to); `bridge` may not reach
`cluster`, `tools` or `figma`; `figma` may not reach any internal package. Wire it into
`make test` and add a CI step next to `vet`.

**Step 3 — the five reliability items, each with a failing test first.**

| Item | Test written first, must be red against today's code |
|---|---|
| 1. Context must not kill the socket | Two concurrent `Send`s; cancel the first's context; the second still receives its response and `IsConnected()` is still true |
| 2. `Close` is bounded | With a client that never reads, `Close()` returns well inside 2s instead of sitting on the library's 5s handshake budget |
| 3. Wait for reconnect | Plugin connects after 500ms; `Send` succeeds instead of returning `"plugin not connected"` immediately |
| 4. `RoleUnknown` | A node in the Unknown role returns an error naming the role, and the fake sender is never called |
| 5. `http.Server` timeouts | `ReadHeaderTimeout` non-zero, `WriteTimeout` zero; and a WebSocket outlives the `ReadHeaderTimeout` mark |

Items 2 and 3 are the severable ones. Item 5 needs `ReadHeaderTimeout` overridable in tests, following
the pattern `pingInterval`/`pingTimeout` already set (`bridge.go:55`), so the test runs in
half a second rather than six.

**Step 4 — the three observability items.** For item 7: capture slog output into a buffer,
call a tool with a distinctive parameter value, assert the value is absent at info and
present at debug. For item 8: assert `/ping` carries `role`, `connected`, `pending` and
`uptime`.

**Step 5 — documentation.** Changelog entry for the four user-visible changes, the log format
change in particular.

**Estimate:** step 1 half a day, step 2 half a day, step 3 three quarters of a day, steps 4
and 5 half a day. About 2.25 days.

## Out of scope, noted

`makeHandler` (`internal/tools.go:32`) has no caller in production code — only
`tools_handler_test.go:57` uses it. It is pre-existing dead code, not something this work
creates, and it is left alone unless asked.
