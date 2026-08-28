# Go Backend Architecture Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split the flat `internal/` package into four bounded packages with a one-way dependency direction, fold the duplicated tool registration and validation into one path, then fix five reliability items and three observability ones.

**Architecture:** Semantics first, movement second. Tasks 1–4 change how tools are registered and validated while everything is still one package, so every compile error is about the change rather than about imports. Tasks 5–9 then move files into `bridge`, `figma`, `cluster` and `tools`, one package at a time, each move keeping the build green. Tasks 10–18 are behaviour changes on the settled structure.

**Tech Stack:** Go 1.27, `encoding/json/v2`, `log/slog`, `github.com/mark3labs/mcp-go` v0.46.0, `github.com/coder/websocket` v1.8.14, `github.com/pdfcpu/pdfcpu` v0.11.1.

**Spec:** `docs/specs/2026-08-28-go-backend-architecture-design.md`

## Global Constraints

- Go 1.27.0 (`go.mod`). Use `encoding/json/v2`, not `encoding/json` — every existing file already does.
- `internal/testdata/tools_schema.json` must stay byte-identical through the whole plan. Baseline sha256: `8914d70197487e6a53e8b4e4b9edc83df7f667ed553914793573cc3bfad1d874`. It moves to `internal/tools/testdata/` in Task 8; the contents never change.
- 63 tools. No task adds, removes or renames a tool, and no task changes a tool's JSON schema.
- Never run `go test -run TestToolSchemas_Golden -update`. If the golden test fails, the change was wrong, not the snapshot.
- `make fmt-check` and `go vet ./...` must pass before every commit.
- Logs go to **stderr**. Stdout carries the MCP protocol and must stay clean.
- Marshal with `json.Deterministic(true)` wherever map keys reach output — `encoding/json/v2` does not sort keys for free.
- Commit after every task. Work on branch `arch-layered-packages`.

---

## File Structure

**Phase A (Tasks 1–4)** — no files move; `package internal` throughout.

| File | Change |
|---|---|
| `internal/toolspec.go` | gains `Check`, `allSpecs`, `handlerFor`, `Sender`, `toolSpec.Custom`; loses `registerSpecs`, `registerCustom`, `specGroups` |
| `internal/node.go` | `normalizeArgs` and friends move out to `toolspec.go`; `Send` returns `(any, error)`; `NewNode` takes a `Guard` |
| `internal/leader.go` | `NewLeader` takes a `Guard`; `handleRPC` calls it |
| `internal/tools.go` | `RegisterTools` becomes one loop; `renderResponse` takes `(any, error)` |
| `internal/tools_read.go` | deleted — held only `registerReadTools` |
| `internal/tools_read_*.go`, `internal/tools_write*.go` | `registerXTools` deleted; the two custom tools declare `Custom` in their spec |
| `cmd/figma-mcp-go/main.go` | passes `internal.Check` into `NewNode` |

**Phase B (Tasks 5–9)** — the split.

| Package | Holds |
|---|---|
| `internal/bridge` | `bridge.go`, `timeout.go`, `clipboard.go`, `BridgeRequest`/`BridgeResponse` |
| `internal/figma` | node ID, hex colour, reaction, constraint and blend-mode rules |
| `internal/cluster` | `node.go`, `leader.go`, `follower.go`, `election.go`, `RPCRequest`/`RPCResponse`/`Role`, `Guard` |
| `internal/tools` | `toolspec.go`, `tools.go`, the eleven `tools_*.go`, `testdata/` |
| `internal/prompts` | unchanged |

Dependency direction, enforced by `make deps-check` in Task 9:

```
cmd ─┬─> tools ──> figma
     ├─> cluster ──> bridge
     └─> prompts
```

---

## Task 1: Extract `Check`

**Files:**
- Modify: `internal/toolspec.go` (add `Check`; receive the normalize helpers)
- Modify: `internal/node.go:31-109` (move `nodeIDParams`, `normalizeArgs`, `normalizeValue`, `normalizeIDList` out), `internal/node.go:147-169` (`Send` calls `Check`)
- Test: `internal/check_test.go` (new)

**Interfaces:**
- Consumes: `ValidateRPC`, `specRegistry`, `NormalizeNodeID` — all existing.
- Produces: `func Check(tool string, nodeIDs []string, params map[string]any) ([]string, map[string]any, error)`. Tasks 2 and 4 both call it.

- [ ] **Step 1: Write the failing test**

Create `internal/check_test.go`:

```go
package internal

import (
	"strings"
	"testing"
)

// Check is the one place both entry points — an MCP tool call and a follower's
// /rpc post — agree on what a valid call looks like. Normalisation has to come
// first: the hyphen format LLMs emit must be accepted, not rejected by the very
// validation that exists to tolerate it.

func TestCheck_NormalizesBeforeValidating(t *testing.T) {
	ids, params, err := Check("set_text", []string{"4029-12345"}, map[string]any{"text": "hi"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 1 || ids[0] != "4029:12345" {
		t.Errorf("node ID not normalised: %v", ids)
	}
	if params["text"] != "hi" {
		t.Errorf("params mangled: %v", params)
	}
}

func TestCheck_RejectsInvalidArguments(t *testing.T) {
	_, _, err := Check("set_node_properties", []string{"1:1"}, map[string]any{"opacity": 5.0})
	if err == nil {
		t.Fatal("expected an error for opacity 5.0")
	}
	if !strings.Contains(err.Error(), "opacity must be at most 1") {
		t.Errorf("unexpected message: %v", err)
	}
}

func TestCheck_UnknownToolIsNotRejected(t *testing.T) {
	// A tool with no spec has no rules to break. ValidateRPC has always
	// returned "" for one, and Check must not turn that into an error.
	if _, _, err := Check("not_a_tool", nil, nil); err != nil {
		t.Errorf("unknown tool should pass through, got %v", err)
	}
}

func TestCheck_DoesNotMutateCallerArguments(t *testing.T) {
	ids := []string{"4029-12345"}
	if _, _, err := Check("set_text", ids, map[string]any{"text": "hi"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ids[0] != "4029-12345" {
		t.Error("Check mutated the caller's slice")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/ -run TestCheck -v`
Expected: FAIL — `undefined: Check`

- [ ] **Step 3: Move the normalisation helpers into `toolspec.go`**

Cut `nodeIDParams`, `normalizeArgs`, `normalizeValue` and `normalizeIDList` from `internal/node.go` (lines 31–109, comments included) and paste them into `internal/toolspec.go` under a new section header, directly above the `// ── Validation ──` divider:

```go
// ── Normalization ────────────────────────────────────────────────────────────
```

They keep their bodies exactly as they are. They belong with the tool table now, because that is where the arguments are checked.

- [ ] **Step 4: Add `Check` to `toolspec.go`**

Put it at the end of the Validation section, after `validateArrayParam`:

```go
// Check normalizes a tool call's arguments and validates them against the
// tool's spec. Both entry points call it — the handlers this package builds,
// and the leader's /rpc endpoint, which receives another process's input — so
// there is one answer to "is this call valid", not one per path.
func Check(tool string, nodeIDs []string, params map[string]any) ([]string, map[string]any, error) {
	// Normalize first: the hyphen format LLMs emit must be accepted, not
	// rejected by the validation that exists to tolerate it.
	nodeIDs, params = normalizeArgs(nodeIDs, params)
	if msg := ValidateRPC(tool, nodeIDs, params); msg != "" {
		return nil, nil, errors.New(msg)
	}
	return nodeIDs, params, nil
}
```

Add `"errors"` to the imports of `internal/toolspec.go`.

- [ ] **Step 5: Point `Node.Send` at `Check`**

In `internal/node.go`, replace the normalise-then-validate opening of `Send` (currently lines 148–155) with:

```go
	nodeIDs, params, checkErr := Check(tool, nodeIDs, params)
	if checkErr != nil {
		nodeLogger.Printf("tool=%s rejected: %s", tool, checkErr)
		return BridgeResponse{Error: checkErr.Error()}, nil
	}
```

Everything below it stays as it is. `Send` still returns `BridgeResponse` — that changes in Task 3.

- [ ] **Step 6: Run the full suite**

Run: `go test ./... && make fmt-check && go vet ./...`
Expected: PASS, clean, clean. `TestNodeSend_RejectsInvalidArgsBeforeReachingPlugin` still passes — it goes through `Send`, which now delegates.

- [ ] **Step 7: Verify the golden snapshot is untouched**

Run: `shasum -a 256 internal/testdata/tools_schema.json`
Expected: `8914d70197487e6a53e8b4e4b9edc83df7f667ed553914793573cc3bfad1d874`

- [ ] **Step 8: Commit**

```bash
git add internal/check_test.go internal/toolspec.go internal/node.go
git commit -m "refactor: extract Check as the single normalize-and-validate path"
```

---

## Task 2: One registration loop

**Files:**
- Modify: `internal/toolspec.go:418-481` (registry section), `internal/tools.go:18-27`
- Modify: `internal/tools_read_export.go:90-101`, `internal/tools_write.go:41-43`
- Modify: `internal/tools_read_document.go`, `internal/tools_read_styles.go`, `internal/tools_write_components.go`, `internal/tools_write_create.go`, `internal/tools_write_modify.go`, `internal/tools_write_page.go`, `internal/tools_write_prototype.go`, `internal/tools_write_styles.go`, `internal/tools_write_variables.go` — delete the `registerXTools` function at the bottom of each
- Delete: `internal/tools_read.go`
- Test: `internal/toolspec_wire_test.go` (existing tests must stay green)

**Interfaces:**
- Consumes: `Check` from Task 1.
- Produces: `type Sender interface`, `func allSpecs() []toolSpec`, `func handlerFor(sender Sender, spec toolSpec) server.ToolHandlerFunc`, `toolSpec.Custom func(Sender) customHandler`. Task 3 implements `Sender` on `*Node`.

Note on `Sender` in this task: it is declared here and `RegisterTools` takes it, but `*Node` does not satisfy it until Task 3. To keep the build green between the two, `Sender` is declared in this task with the **current** `Node.Send` signature and changed in Task 3:

```go
// Sender carries a tool call to the Figma plugin. The tool layer does not know
// how — a leader writes to its WebSocket, a follower proxies over HTTP.
type Sender interface {
	Send(ctx context.Context, tool string, nodeIDs []string, params map[string]any) (BridgeResponse, error)
}
```

- [ ] **Step 1: Write the failing test**

Append to `internal/toolspec_wire_test.go`:

```go
// A tool declared with Custom runs its own Go code; every other tool forwards.
// Both go through Check first, so neither can reach the plugin with arguments
// the table rejects.
func TestHandlerFor_ChecksBeforeRunningCustomCode(t *testing.T) {
	fake := &fakeSender{}
	s := server.NewMCPServer("test", "0.0.1")
	RegisterTools(s, newNodeWithSender(fake))

	res := callToolResult(t, s, "export_frames_to_pdf", map[string]any{
		"nodeIds":    []any{"not-a-node-id"},
		"outputPath": "out.pdf",
	})

	if !res.IsError {
		t.Fatal("expected an invalid node ID to be rejected")
	}
	if !strings.Contains(res.Text, "colon format") {
		t.Errorf("unexpected message: %s", res.Text)
	}
	if len(fake.calls) != 0 {
		t.Errorf("rejected call still reached the sender: %v", fake.calls)
	}
}
```

Add this helper to `internal/tools_handler_test.go`, below `callTool`:

```go
// toolResult is the part of a tools/call response a test asserts on.
type toolResult struct {
	IsError bool
	Text    string
}

// callToolResult dispatches a tool call and returns the parsed result, so a
// test can assert on the message a rejected call produces. It parses into a
// local struct rather than mcp.CallToolResult, whose Content is an interface
// that will not unmarshal.
func callToolResult(t *testing.T, s *server.MCPServer, name string, args map[string]any) toolResult {
	t.Helper()
	argsJSON, _ := json.Marshal(args)
	msg := fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":%q,"arguments":%s}}`,
		name, argsJSON,
	)
	raw := s.HandleMessage(context.Background(), []byte(msg))
	if raw == nil {
		t.Fatalf("HandleMessage returned nil for tool %q", name)
	}
	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal tools/call response: %v", err)
	}
	var envelope struct {
		Result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(b, &envelope); err != nil {
		t.Fatalf("unmarshal tools/call response: %v", err)
	}
	var texts []string
	for _, c := range envelope.Result.Content {
		texts = append(texts, c.Text)
	}
	return toolResult{IsError: envelope.Result.IsError, Text: strings.Join(texts, "\n")}
}
```

Add `"strings"` to that file's imports if it is not already there.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/ -run TestHandlerFor_ChecksBeforeRunningCustomCode -v`
Expected: FAIL — the call reaches `executeExportFramesToPDF` and fails on something other than the node ID, or `callToolResult` is undefined.

- [ ] **Step 3: Add `Sender`, `Custom` and `allSpecs`**

In `internal/toolspec.go`, add the `Sender` interface shown above to the top of the file, below the imports.

Add the field to `toolSpec`, after `Validate`:

```go
	// Custom, when set, replaces the default forwarder. It takes the Sender
	// because a spec is a package-level variable and cannot capture one at
	// declaration time. The schema and the checking still come from the table,
	// so a tool that does work in Go cannot drift from it either.
	Custom func(Sender) customHandler
```

Replace `specGroups()` (lines 420–436) with:

```go
// allSpecs is every tool the server offers. Registration and validation both
// read this one list, so a tool cannot be in the registry without reaching
// clients, or reach clients without rules.
func allSpecs() []toolSpec {
	groups := [][]toolSpec{
		{batchPipelineSpec},
		exportSpecs,
		readDocumentSpecs,
		readStyleSpecs,
		writeComponentSpecs,
		writeCreateSpecs,
		writeModifySpecs,
		writePageSpecs,
		writePrototypeSpecs,
		writeStyleSpecs,
		writeVariableSpecs,
	}
	all := make([]toolSpec, 0, 64)
	for _, group := range groups {
		all = append(all, group...)
	}
	return all
}
```

Change `buildSpecRegistry` to walk it:

```go
func buildSpecRegistry() map[string]toolSpec {
	registry := map[string]toolSpec{}
	for _, spec := range allSpecs() {
		if _, duplicate := registry[spec.Name]; duplicate {
			panic("duplicate tool spec: " + spec.Name)
		}
		registry[spec.Name] = spec
	}
	return registry
}
```

- [ ] **Step 4: Replace `registerSpecs` and `registerCustom` with `handlerFor`**

Delete both functions (lines 456–479) and put this in their place:

```go
// handlerFor builds a tool's MCP handler: split the arguments, check them, then
// run either the tool's own Go code or the plain forwarder. Both paths check,
// which is what makes "every call is validated exactly once" true by
// construction rather than by remembering to do it.
func handlerFor(sender Sender, spec toolSpec) server.ToolHandlerFunc {
	run := spec.Custom
	if run == nil {
		run = forwarder(spec.Name)
	}
	handle := run(sender)

	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nodeIDs, params := specArgs(spec, req.GetArguments())
		nodeIDs, params, err := Check(spec.Name, nodeIDs, params)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return handle(ctx, nodeIDs, params)
	}
}

// forwarder is the default body: hand the arguments to the plugin and render
// whatever comes back.
func forwarder(tool string) func(Sender) customHandler {
	return func(sender Sender) customHandler {
		return func(ctx context.Context, nodeIDs []string, params map[string]any) (*mcp.CallToolResult, error) {
			resp, err := sender.Send(ctx, tool, nodeIDs, params)
			return renderResponse(resp, err)
		}
	}
}
```

Delete `specHandler` (lines 183–190) — `forwarder` replaces it.

- [ ] **Step 5: Rewrite `RegisterTools`**

In `internal/tools.go`, replace `RegisterTools` (lines 18–22) with:

```go
// RegisterTools registers every table-declared tool on the server.
func RegisterTools(s *server.MCPServer, sender Sender) {
	for _, spec := range allSpecs() {
		s.AddTool(buildTool(spec), handlerFor(sender, spec))
	}
}
```

- [ ] **Step 6: Move the two custom bodies into their specs**

In `internal/tools_read_export.go`, add a `Custom` field to `exportFramesToPDFSpec` (after its `Params`):

```go
	Custom: func(sender Sender) customHandler {
		return func(ctx context.Context, nodeIDs []string, params map[string]any) (*mcp.CallToolResult, error) {
			outputPath, _ := params["outputPath"].(string)
			return executeExportFramesToPDF(ctx, sender, nodeIDs, outputPath)
		}
	},
```

and to `saveScreenshotsSpec` (after its `Validate`):

```go
	Custom: func(sender Sender) customHandler {
		return func(ctx context.Context, _ []string, params map[string]any) (*mcp.CallToolResult, error) {
			return executeSaveScreenshots(ctx, sender, params)
		}
	},
```

Change the two functions' first argument from `node *Node` to `sender Sender`, and the same for `saveScreenshotItem` in `internal/tools.go:133`. Their bodies are unchanged — `sender.Send` has the same signature as `node.Send` does today.

Delete `registerReadExportTools` (lines 90–101).

- [ ] **Step 7: Delete the remaining registration functions**

Delete `registerReadTools` by deleting the whole file `internal/tools_read.go`. Delete `registerWriteTools` and `registerBatchPipelineTool` from `internal/tools_write.go`, leaving only `batchPipelineSpec` and the `server` import if still needed — remove the import if not.

Delete the `registerXTools` function at the bottom of each of: `tools_read_document.go`, `tools_read_styles.go`, `tools_write_components.go`, `tools_write_create.go`, `tools_write_modify.go`, `tools_write_page.go`, `tools_write_prototype.go`, `tools_write_styles.go`, `tools_write_variables.go`. Remove the now-unused `"github.com/mark3labs/mcp-go/server"` import from each.

- [ ] **Step 8: Run the tests**

Run: `go test ./... && make fmt-check && go vet ./...`
Expected: PASS. `TestSpecRegistry_MatchesRegisteredTools` and `TestSpecRegistry_CoversEveryTool` are the ones that matter — they pin that the registry and the registered set are still the same 63 tools.

- [ ] **Step 9: Verify the golden snapshot is untouched**

Run: `go test ./internal/ -run TestToolSchemas_Golden -v && shasum -a 256 internal/testdata/tools_schema.json`
Expected: PASS, and the sha still `8914d70…`. Do not pass `-update`.

- [ ] **Step 10: Commit**

```bash
git add internal/
git rm internal/tools_read.go
git commit -m "refactor: register every tool from one loop over the spec table"
```

---

## Task 3: `Sender` returns `(any, error)`

**Files:**
- Modify: `internal/toolspec.go` (the `Sender` declaration), `internal/node.go:147-169`, `internal/tools.go:39-55,133-191`, `internal/tools_read_export.go:103-153`
- Test: `internal/node_test.go:194-260` (retarget), `internal/tools_handler_test.go:14-22` (helper)

**Interfaces:**
- Produces: `Sender.Send(ctx, tool, nodeIDs, params) (any, error)`; `renderResponse(data any, err error) (*mcp.CallToolResult, error)`; `Node.Send` with the same signature as `Sender.Send`.
- The cluster-internal `sender` interface (`node.go:15`) keeps returning `BridgeResponse` — it is what `Bridge` and `Follower` satisfy. Two interfaces, deliberately: one is the plugin wire, the other is what the tool layer needs.

- [ ] **Step 1: Write the failing test**

Replace `TestNodeSend_RejectsInvalidArgsBeforeReachingPlugin` in `internal/node_test.go` with this, moved to `internal/tools_handler_test.go` (it is now a statement about the tool layer, not about `Node`):

```go
// Validation used to be reachable only from the leader's /rpc handler, i.e.
// only for follower processes. The first process to start is the leader, so for
// most users it never ran and bad arguments went straight to Figma. It then
// lived in Node.Send, the last point of convergence. It now lives at the only
// point of entry — the handlers this package builds — so a rejected call cannot
// reach a sender at all.
func TestToolCall_RejectsInvalidArgsBeforeReachingPlugin(t *testing.T) {
	cases := []struct {
		name    string
		tool    string
		args    map[string]any
		wantMsg string
	}{
		{"opacity out of range", "set_node_properties",
			map[string]any{"nodeIds": []any{"1:1"}, "opacity": 5.0}, "opacity must be at most 1"},
		{"invalid blend mode", "set_node_properties",
			map[string]any{"nodeIds": []any{"1:1"}, "blendMode": "NEON"}, "blendMode must be one of"},
		{"missing node id", "get_node", map[string]any{}, "nodeId is required"},
		{"malformed node id", "set_text",
			map[string]any{"nodeId": "not-an-id", "text": "hi"}, "colon format"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, fake := newTestServer(t)
			res := callToolResult(t, s, tc.tool, tc.args)

			if !res.IsError {
				t.Fatalf("expected %s to be rejected", tc.tool)
			}
			if !strings.Contains(res.Text, tc.wantMsg) {
				t.Errorf("want message containing %q, got %q", tc.wantMsg, res.Text)
			}
			if len(fake.calls) != 0 {
				t.Errorf("a rejected call still reached the sender: %v", fake.calls)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/ -run TestToolCall_RejectsInvalidArgs -v`
Expected: FAIL — `newTestServer` returns `*Node`, not `*fakeSender`, so `fake.calls` does not compile.

- [ ] **Step 3: Change the `Sender` declaration**

In `internal/toolspec.go`:

```go
type Sender interface {
	Send(ctx context.Context, tool string, nodeIDs []string, params map[string]any) (any, error)
}
```

- [ ] **Step 4: Change `Node.Send`**

Replace `Node.Send` in `internal/node.go` entirely. Validation and normalisation are gone — the tool layer did them, and `/rpc` does them in Task 4:

```go
// Send routes a tool call to the plugin: the leader writes to its own bridge,
// a follower proxies to the leader over HTTP. Arguments arrive already
// normalized and checked.
//
// A plugin-reported error and a transport error become the same thing here.
// Every caller already treated them identically; keeping them apart only
// duplicated the branch.
func (n *Node) Send(ctx context.Context, tool string, nodeIDs []string, params map[string]any) (any, error) {
	n.mu.RLock()
	role := n.role
	leader := n.leader
	follower := n.follower
	n.mu.RUnlock()

	nodeLogger.Printf("tool=%s role=%s nodeIDs=%v", tool, n.RoleName(), nodeIDs)

	var (
		resp BridgeResponse
		err  error
	)
	if role == RoleLeader && leader != nil {
		resp, err = leader.GetBridge().Send(ctx, tool, nodeIDs, params)
	} else {
		resp, err = follower.Send(ctx, tool, nodeIDs, params)
	}
	if err != nil {
		return nil, err
	}
	if resp.Error != "" {
		return nil, errors.New(resp.Error)
	}
	return resp.Data, nil
}
```

Add `"errors"` to the imports of `internal/node.go`.

- [ ] **Step 5: Collapse the doubled error branches**

In `internal/tools.go`, `renderResponse` becomes:

```go
// renderResponse converts a sender's answer into an MCP tool result.
func renderResponse(data any, err error) (*mcp.CallToolResult, error) {
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	// Deterministic keeps map keys sorted. encoding/json v1 sorted them for
	// free; v2 does not, and plugin data is mostly maps, so without this the
	// same tool call would come back with its keys shuffled every time.
	text, err := json.Marshal(data, json.Deterministic(true))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshal response: %v", err)), nil
	}
	return mcp.NewToolResultText(string(text)), nil
}
```

In `saveScreenshotItem` (`internal/tools.go:162-173`), replace the `resp, err :=` block and the two error checks with one:

```go
	data, err := sender.Send(ctx, "get_screenshot", []string{item.NodeID}, params)
	if err != nil {
		return saveResult{Index: index, NodeID: item.NodeID, OutputPath: resolvedPath, Error: err.Error()}
	}

	export, err := extractScreenshotExport(data)
```

In `executeExportFramesToPDF` (`internal/tools_read_export.go:116-127`), the same collapse:

```go
	data, err := sender.Send(ctx, "export_frames_to_pdf", nodeIDs, nil)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	pages, err := extractFramePDFs(data)
```

In `internal/tools.go`, change `makeHandler`'s first parameter from `node *Node` to `sender Sender` and its body's `node.Send` to `sender.Send`. It has no caller in production code and is left in place rather than deleted — that is a separate decision, out of this plan's scope.

- [ ] **Step 6: Rework the test fakes**

In `internal/node_test.go`, replace the anonymous-struct `fakeSender` with a named type that satisfies the new `Sender`:

```go
// fakeCall is one recorded call.
type fakeCall struct {
	tool    string
	nodeIDs []string
	params  map[string]any
}

// fakeSender records what the tool layer asked for and never touches the network.
type fakeSender struct {
	calls []fakeCall
	data  any
	err   error
}

func (f *fakeSender) Send(_ context.Context, tool string, nodeIDs []string, params map[string]any) (any, error) {
	f.calls = append(f.calls, fakeCall{tool, nodeIDs, params})
	return f.data, f.err
}
```

`Node`'s own routing tests need a fake on the *other* interface — the one `Follower` satisfies. Add it next to the above:

```go
// fakeBackend stands in for a Follower on the cluster-internal sender
// interface, which speaks BridgeResponse.
type fakeBackend struct {
	calls []fakeCall
	resp  BridgeResponse
	err   error
}

func (f *fakeBackend) Send(_ context.Context, tool string, nodeIDs []string, params map[string]any) (BridgeResponse, error) {
	f.calls = append(f.calls, fakeCall{tool, nodeIDs, params})
	return f.resp, f.err
}
```

Change `newNodeWithSender(s sender) *Node` to take a `*fakeBackend`, and update the routing tests in `node_test.go` that used `fakeSender` to use `fakeBackend`.

In `internal/tools_handler_test.go`, `newTestServer` becomes:

```go
// newTestServer returns an MCPServer with every tool registered against a fake
// sender. No Node, no HTTP: a tool test has no business dialling anything.
func newTestServer(t *testing.T) (*server.MCPServer, *fakeSender) {
	t.Helper()
	s := server.NewMCPServer("test", "0.0.1")
	fake := &fakeSender{}
	RegisterTools(s, fake)
	return s, fake
}
```

Prompts are no longer registered here — `internal/prompts` covers `RegisterAll` in its own `TestRegisterAll_NoPanic`, and the tool tests never read a prompt. Delete `TestRegisterPrompts_Smoke`, whose subject is that duplicate.

In `internal/toolspec_wire_test.go`, `newWireTestServer` now has the same body as `newTestServer` — delete it and call `newTestServer` instead. Replace the Task-2 test's `RegisterTools(s, newNodeWithSender(fake))` with `newTestServer(t)`.

`TestMakeHandler_UnknownNode` and `TestRegisterTools_Smoke` still compile: `*Node` satisfies the new `Sender`.

- [ ] **Step 7: Run the tests**

Run: `go test ./... && make fmt-check && go vet ./...`
Expected: PASS.

- [ ] **Step 8: Verify the golden snapshot and check the suite got faster**

Run: `shasum -a 256 internal/testdata/tools_schema.json && go test ./internal/ -count=1`
Expected: sha unchanged; the `internal` package's elapsed time drops well below the 6.666s baseline, because roughly thirty tool calls no longer dial a dead port.

- [ ] **Step 9: Commit**

```bash
git add internal/
git commit -m "refactor: Sender returns (any, error) and Node.Send only routes"
```

---

## Task 4: Inject the guard into the leader

**Files:**
- Modify: `internal/node.go` (`NewNode`, `BecomeLeader`), `internal/leader.go:25-41,107-147`
- Modify: `cmd/figma-mcp-go/main.go:37`
- Test: `internal/leader_test.go`

**Interfaces:**
- Produces: `type Guard func(tool string, nodeIDs []string, params map[string]any) ([]string, map[string]any, error)`; `NewNode(ip string, port int, version string, guard Guard) *Node`; `NewLeader(ip string, port int, version string, guard Guard) *Leader`.
- `cmd` supplies `Check` as the guard, once, at `NewNode`.

Why threaded rather than passed straight to `NewLeader`: `cmd` does not build the `Leader`. `Node.BecomeLeader` does, and it can do so at any point during a takeover, so the node has to be holding the guard.

- [ ] **Step 1: Write the failing test**

Append to `internal/leader_test.go`:

```go
// /rpc is where another process's input arrives, so it checks arguments itself
// rather than trusting the follower that sent them. It normalizes too: a
// hyphen-format node ID posted here reaches the bridge in colon format.
func TestLeaderRPC_NormalizesAndValidates(t *testing.T) {
	port := freePort(t)
	leader := NewLeader("127.0.0.1", port, "test", Check)
	if err := leader.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(leader.Stop)

	base := "http://127.0.0.1:" + itoa(port)

	// Invalid arguments are rejected with 400 and never reach the bridge.
	body := `{"tool":"set_node_properties","nodeIds":["1:1"],"params":{"opacity":5}}`
	resp, err := http.Post(base+"/rpc", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("want 400 for invalid opacity, got %d", resp.StatusCode)
	}

	// A hyphen-format ID is accepted; with no plugin connected the call fails
	// at the bridge, which is proof it got past the check.
	body = `{"tool":"set_text","nodeIds":["4029-12345"],"params":{"text":"hi"}}`
	resp2, err := http.Post(base+"/rpc", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("want 200 for a normalizable node ID, got %d", resp2.StatusCode)
	}
	var rpcResp RPCResponse
	if err := json.UnmarshalRead(resp2.Body, &rpcResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(rpcResp.Error, "plugin not connected") {
		t.Errorf("want a bridge error, got %q", rpcResp.Error)
	}
}
```

Add `"net/http"`, `"strings"` and `"encoding/json/v2"` to that file's imports if missing.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/ -run TestLeaderRPC_NormalizesAndValidates -v`
Expected: FAIL — `NewLeader` takes three arguments, not four.

- [ ] **Step 3: Declare `Guard` and thread it**

In `internal/leader.go`, above the `Leader` struct:

```go
// Guard checks and normalizes an incoming call. The leader holds one so /rpc
// applies the same rules as a local tool call without this package having to
// know what the rules are.
type Guard func(tool string, nodeIDs []string, params map[string]any) ([]string, map[string]any, error)
```

Add `guard Guard` to the `Leader` struct and to `NewLeader`:

```go
func NewLeader(ip string, port int, version string, guard Guard) *Leader {
	return &Leader{
		ip:      ip,
		port:    port,
		bridge:  NewBridge(version),
		version: version,
		guard:   guard,
	}
}
```

In `internal/node.go`, add `guard Guard` to the `Node` struct, take it in `NewNode`, and pass it on promotion:

```go
func NewNode(ip string, port int, version string, guard Guard) *Node {
	return &Node{
		ip:       ip,
		port:     port,
		role:     RoleUnknown,
		version:  version,
		guard:    guard,
		follower: NewFollower(fmt.Sprintf("http://%s:%d", ip, port)),
	}
}
```

In `BecomeLeader`: `leader := NewLeader(n.ip, n.port, n.version, n.guard)`.

- [ ] **Step 4: Use the guard in `handleRPC`**

Replace the validation block in `internal/leader.go` (lines 127–133) with:

```go
	nodeIDs, params, err := l.guard(req.Tool, req.NodeIDs, req.Params)
	if err != nil {
		leaderLogger.Printf("rpc %s rejected: %s", req.Tool, err)
		l.sendJSON(w, http.StatusBadRequest, RPCResponse{Error: err.Error()})
		return
	}

	resp, err := l.bridge.Send(r.Context(), req.Tool, nodeIDs, params)
```

Delete the old `resp, err := l.bridge.Send(r.Context(), req.Tool, req.NodeIDs, req.Params)` line that followed it.

- [ ] **Step 5: Wire `cmd` and the tests**

In `cmd/figma-mcp-go/main.go`:

```go
	node := internal.NewNode(*ip, *port, version, internal.Check)
```

and `internal.RegisterTools(s, node)` is unchanged — `*Node` satisfies `Sender`.

Update every other `NewNode(...)` and `NewLeader(...)` call in the test files to pass `Check` as the last argument: `node_test.go`, `election_test.go`, `leader_test.go`, `tools_handler_test.go`.

- [ ] **Step 6: Run the tests**

Run: `go test ./... && make fmt-check && go vet ./...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/ cmd/
git commit -m "refactor: inject the argument guard into the leader through Node"
```

---

## Task 5: Extract `internal/bridge`

**Files:**
- Create: `internal/bridge/bridge.go`, `internal/bridge/timeout.go`, `internal/bridge/clipboard.go`, `internal/bridge/types.go`
- Create: `internal/bridge/bridge_test.go`, `internal/bridge/timeout_test.go`, `internal/bridge/clipboard_test.go`, `internal/bridge/types_test.go`
- Delete: the corresponding files under `internal/`
- Modify: every `internal/` file referring to `Bridge`, `BridgeRequest`, `BridgeResponse`, `timeoutFor` or `WriteOSClipboard`

**Interfaces:**
- Produces: package `bridge` exporting `Bridge`, `NewBridge`, `Request`, `Response`, `TimeoutFor`, `FollowerTimeoutFor`, `MaxToolTimeout`, `WriteOSClipboard`.
- Renames: `BridgeRequest` → `bridge.Request`, `BridgeResponse` → `bridge.Response`, `timeoutFor` → `bridge.TimeoutFor`, `followerTimeoutFor` → `bridge.FollowerTimeoutFor`, `maxToolTimeout` → `bridge.MaxToolTimeout`. `bridge.BridgeResponse` would stutter; Go style is `bridge.Response`.

This task and the next three are moves. They are verified by the existing suite plus the golden sha rather than by a new failing test — there is no new behaviour to drive.

- [ ] **Step 1: Move the files**

```bash
mkdir -p internal/bridge
git mv internal/bridge.go internal/bridge/bridge.go
git mv internal/timeout.go internal/bridge/timeout.go
git mv internal/clipboard.go internal/bridge/clipboard.go
git mv internal/bridge_test.go internal/bridge/bridge_test.go
git mv internal/timeout_test.go internal/bridge/timeout_test.go
git mv internal/clipboard_test.go internal/bridge/clipboard_test.go
```

Change the package clause in each to `package bridge`.

- [ ] **Step 2: Split `types.go`**

Create `internal/bridge/types.go` holding the two plugin wire types, taken verbatim from `internal/types.go:1-27` including the `omitzero` comment at the top of the file, with the types renamed:

```go
package bridge

// Numeric and any-typed fields below use omitzero rather than omitempty.
// Under encoding/json/v2 omitempty drops a value that encodes to an empty JSON
// string, object or array — it no longer drops a zero number, and it does drop
// an any field holding "" or an empty slice. omitzero drops exactly the Go zero
// value, which is what the plugin wire format has always meant here.

// Request is sent from the Go server to the Figma plugin over WebSocket.
type Request struct {
	Type      string         `json:"type"`
	RequestID string         `json:"requestId"`
	NodeIDs   []string       `json:"nodeIds,omitempty"`
	Params    map[string]any `json:"params,omitempty"`
}

// Response is received from the Figma plugin over WebSocket.
type Response struct {
	Type      string `json:"type"`
	RequestID string `json:"requestId"`
	Text      string `json:"text,omitempty"`
	Data      any    `json:"data,omitzero"`
	Error     string `json:"error,omitempty"`
	// Progress fields — sent mid-operation for long-running commands
	Progress int    `json:"progress,omitzero"`
	Message  string `json:"message,omitempty"`
}
```

Delete those two types from `internal/types.go`, leaving `RPCRequest`, `RPCResponse` and `Role` behind.

Create `internal/bridge/types_test.go` with the `BridgeRequest`/`BridgeResponse` round-trip tests cut from `internal/types_test.go`, renamed to `Request`/`Response`.

- [ ] **Step 3: Export what leaves the package**

In `internal/bridge/timeout.go`, rename `timeoutFor` → `TimeoutFor`, `followerTimeoutFor` → `FollowerTimeoutFor`, `maxToolTimeout` → `MaxToolTimeout`. `defaultToolTimeout`, `toolTimeouts`, `defaultPingInterval`, `defaultPingTimeout` and `followerGrace` stay unexported.

In `internal/bridge/bridge.go`, rename every `BridgeRequest` to `Request` and `BridgeResponse` to `Response`.

- [ ] **Step 4: Fix the callers in `internal/`**

Add `"github.com/tunglt1810/figma-mcp-go/internal/bridge"` to `internal/node.go`, `internal/leader.go` and `internal/follower.go`, and qualify: `*bridge.Bridge`, `bridge.NewBridge`, `bridge.Response`, `bridge.FollowerTimeoutFor`.

The local identifier `bridge` on `Leader` collides with the package name. Rename the struct field to `b`:

```go
type Leader struct {
	ip      string
	port    int
	b       *bridge.Bridge
	server  *http.Server
	version string
	guard   Guard
}
```

and update `GetBridge`, `Start`, `Stop` and `handleRPC` accordingly.

- [ ] **Step 5: Run the tests**

Run: `go test ./... && make fmt-check && go vet ./...`
Expected: PASS.

- [ ] **Step 6: Verify the golden snapshot**

Run: `shasum -a 256 internal/testdata/tools_schema.json`
Expected: `8914d70197487e6a53e8b4e4b9edc83df7f667ed553914793573cc3bfad1d874`

- [ ] **Step 7: Commit**

```bash
git add -A internal/
git commit -m "refactor: extract internal/bridge"
```

---

## Task 6: Extract `internal/figma`

**Files:**
- Create: `internal/figma/schema.go`, `internal/figma/schema_test.go`
- Modify: `internal/schema.go` (keeps only `ValidateRPC`), `internal/schema_test.go` (loses its first two tests)
- Modify: `internal/toolspec.go` and the `tools_*.go` files that call the validators

**Interfaces:**
- Produces: package `figma` exporting `NormalizeNodeID`, `ValidNodeID`, `ValidHexColor`, `ValidateReaction`, `ValidateConstraintAxes`, `BlendModeNames`.
- `validateReaction`, `validateTriggerType`, `validateActionType`, `validateConstraintAxes` and `blendModeNames` are called from the spec tables, so they cross the boundary and must be exported. `validateTriggerType` and `validateActionType` are only called by `ValidateReaction` and stay unexported.

- [ ] **Step 1: Move the domain rules**

```bash
mkdir -p internal/figma
```

Create `internal/figma/schema.go` as `package figma` holding, verbatim from `internal/schema.go`: `nodeIDPattern`, `NormalizeNodeID`, `ValidNodeID`, `hexColorPattern`, `ValidHexColor`, `validTriggerTypes`, `validActionTypes`, `validateReaction` (renamed `ValidateReaction`), `validateTriggerType`, `validateActionType`, `blendModeNames` (renamed `BlendModeNames`), `validateConstraintAxes` (renamed `ValidateConstraintAxes`). Keep every comment.

`internal/schema.go` is left holding only `ValidateRPC` — it is a `specRegistry` lookup and belongs with the table. Rename the file to `internal/validate.go` for accuracy:

```bash
git mv internal/schema.go internal/validate.go
```

- [ ] **Step 2: Split the test**

Create `internal/figma/schema_test.go` as `package figma` containing `TestValidNodeID` and `TestNormalizeNodeID`, cut from `internal/schema_test.go` lines 10–61 with the file's imports.

Rename what remains:

```bash
git mv internal/schema_test.go internal/validate_test.go
```

`internal/validate_test.go` keeps the 55 `TestValidateRPC_*` functions from line 62 onward.

- [ ] **Step 3: Fix the callers**

Add `"github.com/tunglt1810/figma-mcp-go/internal/figma"` where needed and qualify. The call sites are: `internal/toolspec.go` (`ValidNodeID`, `ValidHexColor` in `validateSpec`; `NormalizeNodeID` in the normalisation helpers), `internal/tools_read_export.go:75` (`ValidNodeID` in `saveScreenshotsSpec.Validate`), and the spec tables that reference `blendModeNames`, `validateReaction` or `validateConstraintAxes` — find them with:

```bash
grep -rn "blendModeNames\|validateReaction\|validateConstraintAxes\|ValidNodeID\|ValidHexColor\|NormalizeNodeID" internal/*.go
```

- [ ] **Step 4: Run the tests**

Run: `go test ./... && make fmt-check && go vet ./...`
Expected: PASS.

- [ ] **Step 5: Verify the golden snapshot**

Run: `shasum -a 256 internal/testdata/tools_schema.json`
Expected: the baseline sha.

- [ ] **Step 6: Commit**

```bash
git add -A internal/
git commit -m "refactor: extract internal/figma"
```

---

## Task 7: Extract `internal/cluster`

**Files:**
- Create: `internal/cluster/{node,leader,follower,election,types}.go` and their tests, plus `internal/cluster/helpers_test.go`
- Delete: the corresponding files under `internal/`
- Modify: `cmd/figma-mcp-go/main.go`

**Interfaces:**
- Produces: package `cluster` exporting `Node`, `NewNode`, `Election`, `NewElection`, `Leader`, `NewLeader`, `Follower`, `NewFollower`, `Guard`, `Role`, `RoleUnknown`, `RoleLeader`, `RoleFollower`, `RPCRequest`, `RPCResponse`.
- `cluster.Node` satisfies the `Sender` interface that still lives in `internal`. It will satisfy `tools.Sender` after Task 8 without any change — an interface is satisfied structurally.

- [ ] **Step 1: Move the files**

```bash
mkdir -p internal/cluster
git mv internal/node.go internal/cluster/node.go
git mv internal/leader.go internal/cluster/leader.go
git mv internal/follower.go internal/cluster/follower.go
git mv internal/election.go internal/cluster/election.go
git mv internal/types.go internal/cluster/types.go
git mv internal/node_test.go internal/cluster/node_test.go
git mv internal/leader_test.go internal/cluster/leader_test.go
git mv internal/follower_test.go internal/cluster/follower_test.go
git mv internal/election_test.go internal/cluster/election_test.go
git mv internal/helpers_test.go internal/cluster/helpers_test.go
git mv internal/types_test.go internal/cluster/types_test.go
```

Change the package clause in each to `package cluster`.

- [ ] **Step 2: Split the leftover tests out of `node_test.go`**

`internal/cluster/node_test.go` keeps the routing and role tests. Move `fakeSender`, `fakeCall` and `TestToolCall_RejectsInvalidArgsBeforeReachingPlugin` — anything that talks about tools rather than routing — back to `internal/tools_handler_test.go`, which is still in `internal` until Task 8. `fakeBackend` and `newNodeWithSender` stay in `cluster`.

`internal/cluster/types_test.go` keeps only the `RPCRequest`/`RPCResponse`/`Role` tests; the `Request`/`Response` ones already went to `bridge` in Task 5.

- [ ] **Step 3: Break the guard import**

`cluster` must not import `internal` for `Check`. It does not have to: `Guard` is declared in `cluster/leader.go` and the concrete function is supplied by the caller. In the tests, pass a local stub rather than `Check`:

```go
// passthroughGuard accepts everything. Tests that care about checking use the
// real one in the tools package; these care about routing.
func passthroughGuard(_ string, nodeIDs []string, params map[string]any) ([]string, map[string]any, error) {
	return nodeIDs, params, nil
}
```

`TestLeaderRPC_NormalizesAndValidates` from Task 4 depends on the real rules, so it moves to `internal/tools_handler_test.go` (and to `internal/tools/` in Task 8), where `Check` is in scope. It builds a `cluster.Leader` with `cluster.NewLeader(ip, port, version, Check)`.

- [ ] **Step 4: Fix the callers**

`internal/toolspec.go` and `internal/tools.go` reference `*Node` only through the `Sender` interface, so they need no import. `cmd/figma-mcp-go/main.go` becomes:

```go
	node := cluster.NewNode(*ip, *port, version, internal.Check)
	election := cluster.NewElection(*ip, *port, node)
```

with `"github.com/tunglt1810/figma-mcp-go/internal/cluster"` imported.

- [ ] **Step 5: Run the tests**

Run: `go test ./... && make fmt-check && go vet ./...`
Expected: PASS.

- [ ] **Step 6: Verify the golden snapshot**

Run: `shasum -a 256 internal/testdata/tools_schema.json`
Expected: the baseline sha.

- [ ] **Step 7: Commit**

```bash
git add -A internal/ cmd/
git commit -m "refactor: extract internal/cluster"
```

---

## Task 8: Extract `internal/tools`

**Files:**
- Move every remaining `internal/*.go` and `internal/testdata/` into `internal/tools/`
- Modify: `cmd/figma-mcp-go/main.go`
- Modify: `internal/tools/tools_plugin_test.go` (plugin source path)

**Interfaces:**
- Produces: package `tools` exporting `RegisterTools`, `Check`, `Sender`, `ValidateRPC`.
- After this task the `internal` root package no longer exists.

- [ ] **Step 1: Move everything that is left**

```bash
mkdir -p internal/tools
git mv internal/toolspec.go internal/tools.go internal/validate.go internal/tools_*.go internal/tools/
git mv internal/testdata internal/tools/testdata
```

`internal/tools/tools.go` and the directory now share a name; rename it for readability:

```bash
git mv internal/tools/tools.go internal/tools/handlers.go
```

Change the package clause in every moved file — including the tests — to `package tools`.

- [ ] **Step 2: Fix the plugin source path**

In `internal/tools/tools_plugin_test.go:47`:

```go
	dir := filepath.Join("..", "..", "plugin", "src")
```

- [ ] **Step 3: Drop the prompts wrapper**

Delete `RegisterPrompts` from `internal/tools/handlers.go` and its `prompts` import. Keeping it would make `tools` depend on `prompts` in production for one line of forwarding.

- [ ] **Step 4: Rewire `cmd`**

`cmd/figma-mcp-go/main.go`:

```go
import (
	// …
	figmamcpgo "github.com/tunglt1810/figma-mcp-go"
	"github.com/tunglt1810/figma-mcp-go/internal/cluster"
	"github.com/tunglt1810/figma-mcp-go/internal/prompts"
	"github.com/tunglt1810/figma-mcp-go/internal/tools"
)

	node := cluster.NewNode(*ip, *port, version, tools.Check)
	election := cluster.NewElection(*ip, *port, node)
	// …
	s := server.NewMCPServer("figma-mcp-go", version)
	tools.RegisterTools(s, node)
	prompts.RegisterAll(s)
```

- [ ] **Step 5: Run the tests**

Run: `go test ./... && make fmt-check && go vet ./...`
Expected: PASS. Four packages plus `cmd` now appear in the output.

- [ ] **Step 6: Verify the snapshot moved without changing**

Run: `shasum -a 256 internal/tools/testdata/tools_schema.json && git diff --stat HEAD -- '*tools_schema.json'`
Expected: the baseline sha `8914d70…`, and a rename with no content change.

- [ ] **Step 7: Commit**

```bash
git add -A internal/ cmd/
git commit -m "refactor: extract internal/tools and retire the internal root package"
```

---

## Task 9: Enforce the dependency direction

**Files:**
- Modify: `Makefile`, `.github/workflows/ci.yml`

**Interfaces:**
- Produces: `make deps-check`, run by `make test` and by CI.

The compiler rejects import cycles, not import directions. Without this the layering is back to being a convention.

- [ ] **Step 1: Add the target**

In `Makefile`, add `deps-check` to the `.PHONY` line and add:

```make
deps-check:
	@fail=0; \
	check() { \
	  if go list -deps ./internal/$$1 2>/dev/null | grep -q "figma-mcp-go/internal/$$2$$"; then \
	    echo "forbidden import: internal/$$1 -> internal/$$2"; \
	    fail=1; \
	  fi; \
	}; \
	check tools cluster; check tools bridge; \
	check cluster tools; check cluster figma; \
	check bridge cluster; check bridge tools; check bridge figma; \
	check figma bridge; check figma cluster; check figma tools; \
	if [ $$fail -eq 0 ]; then echo "deps-check: layering holds"; fi; \
	exit $$fail
```

Change the test target to run it first:

```make
test: deps-check test-go test-ts
```

- [ ] **Step 2: Run it and confirm it passes**

Run: `make deps-check`
Expected: `deps-check: layering holds`

- [ ] **Step 3: Confirm it can fail**

Temporarily add `_ "github.com/tunglt1810/figma-mcp-go/internal/cluster"` to the imports of `internal/tools/handlers.go`, then run `make deps-check`.
Expected: `forbidden import: internal/tools -> internal/cluster`, exit status 1. Remove the import again and re-run to confirm it goes back to passing.

- [ ] **Step 4: Add the CI step**

In `.github/workflows/ci.yml`, in the `verify-go` job, between "Vet Go code" and "Test Go code":

```yaml
      - name: Check package layering
        run: make deps-check
```

- [ ] **Step 5: Commit**

```bash
git add Makefile .github/workflows/ci.yml
git commit -m "build: fail the build on a forbidden internal import"
```

---

## Task 10: A cancelled request must not close the socket

**Files:**
- Modify: `internal/bridge/bridge.go` (`Send`)
- Test: `internal/bridge/bridge_test.go`

**Interfaces:**
- Produces: no new exported surface. `Bridge.Send` gains an internal write deadline.

`coder/websocket` registers `context.AfterFunc(ctx, c.close)` for the duration of a write (`conn.go:171`, called from `write.go:276`). Passing the caller's context to `conn.Write` therefore means one client hanging up closes the shared connection.

- [ ] **Step 1: Write the failing test**

Append to `internal/bridge/bridge_test.go`:

```go
// Cancelling one request used to close the whole WebSocket: Send passed the
// caller's context to conn.Write, and the library closes the connection when a
// write's context is cancelled (conn.go:171, write.go:276). One client hanging
// up took every other in-flight request with it.
func TestSend_CancellingOneRequestLeavesTheSocketUsable(t *testing.T) {
	b, client := setupBridgeWithClient(t)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		b.Send(ctx, "get_document", nil, nil) //nolint:errcheck
	}()

	waitFor(t, time.Second, func() bool {
		b.mu.RLock()
		defer b.mu.RUnlock()
		return len(b.pending) == 1
	}, "the first request to be registered")

	cancel()
	<-done

	// Answer the next request from the client side. If cancelling closed the
	// socket, this never arrives and Send fails instead.
	go func() {
		var req Request
		if err := readJSON(context.Background(), client, &req); err != nil {
			return
		}
		writeJSON(context.Background(), client, Response{ //nolint:errcheck
			Type:      req.Type,
			RequestID: req.RequestID,
			Data:      map[string]any{"ok": true},
		})
	}()

	resp, err := b.Send(context.Background(), "get_document", nil, nil)
	if err != nil {
		t.Fatalf("a second request failed after a cancelled one: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("second request returned a plugin error: %s", resp.Error)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/bridge/ -run TestSend_CancellingOneRequestLeavesTheSocketUsable -v`
Expected: FAIL — the second `Send` reports a write error or "plugin not connected".

- [ ] **Step 3: Give the write its own context**

In `internal/bridge/timeout.go`, add:

```go
	// writeTimeout bounds a single frame write. It is not the tool's budget:
	// the tool's budget covers the plugin thinking, this covers getting the
	// bytes onto a loopback socket.
	writeTimeout = 10 * time.Second
```

In `internal/bridge/bridge.go`, in `Send`, replace the write block:

```go
	// Not the caller's context: the library closes the connection when a
	// write's context is cancelled (conn.go:171), so passing the caller's in
	// would let one cancelled request drop every other one. The caller's
	// context still governs the wait below.
	writeCtx, cancelWrite := context.WithTimeout(context.Background(), writeTimeout)
	b.wmu.Lock()
	writeErr := writeJSON(writeCtx, conn, req)
	b.wmu.Unlock()
	cancelWrite()
```

The rest of `Send` — including the `case <-ctx.Done():` arm that honours the caller's cancellation — is unchanged.

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/bridge/ -v && go test ./...`
Expected: PASS, including the existing `TestBridgeSend_ContextCancelled`, which asserts the caller still gets `ctx.Err()` back.

- [ ] **Step 5: Commit**

```bash
git add internal/bridge/
git commit -m "fix: a cancelled tool call no longer closes the plugin connection"
```

---

## Task 11: Bound `Close` (minor, severable)

**Files:**
- Modify: `internal/bridge/bridge.go` (`Bridge` struct, `NewBridge`, `Close`)
- Test: `internal/bridge/bridge_test.go`

**Interfaces:**
- Produces: `Bridge.closeGrace time.Duration`, unexported, overridable in tests like `pingInterval` already is.

`Conn.Close` runs the close handshake with a 5s budget (`close.go:199`) and then waits on the read goroutine with a 15s one (`close.go:231`). A plugin that vanished without a close frame therefore delays process exit, and `Close` is on the shutdown path.

- [ ] **Step 1: Write the failing test**

Append to `internal/bridge/bridge_test.go`:

```go
// setupBridgeWithClient's client never calls Read, so the library on that side
// never answers a close frame — the same trick the keepalive tests use. Close
// used to sit on the handshake for 5 seconds; it now gives up after
// closeGrace and drops the socket.
func TestClose_IsBoundedWhenThePeerNeverAnswers(t *testing.T) {
	b, _ := setupBridgeWithClient(t)
	b.closeGrace = 100 * time.Millisecond

	done := make(chan struct{})
	go func() {
		defer close(done)
		b.Close()
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close blocked on a close handshake the peer will never answer")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/bridge/ -run TestClose_IsBoundedWhenThePeerNeverAnswers -v`
Expected: FAIL — `b.closeGrace` is undefined. After Step 3's field is added but before Step 4, it fails on the 2s deadline instead.

- [ ] **Step 3: Add the field**

In the `Bridge` struct, next to the ping fields:

```go
	// closeGrace bounds the WebSocket close handshake on shutdown.
	closeGrace time.Duration
```

In `internal/bridge/timeout.go`:

```go
	// defaultCloseGrace is how long a graceful close may take before the
	// socket is simply dropped. The library allows 5s for the handshake and
	// 15s more for its goroutines, which is a long time to hold up exit for a
	// plugin that is already gone.
	defaultCloseGrace = 1 * time.Second
```

and set `closeGrace: defaultCloseGrace` in `NewBridge`.

- [ ] **Step 4: Bound `Close`**

Replace `Bridge.Close`:

```go
// Close shuts down the bridge, rejecting all pending requests. The graceful
// close runs on its own goroutine with a deadline: the close frame is still
// sent in the normal case, but a peer that has gone away no longer holds up
// shutdown. The goroutine finishes on its own.
func (b *Bridge) Close() {
	b.mu.Lock()
	for id, entry := range b.pending {
		entry.timer.Stop()
		entry.once.Do(func() { close(entry.ch) })
		delete(b.pending, id)
	}
	conn := b.conn
	b.conn = nil
	grace := b.closeGrace
	b.mu.Unlock()

	if conn == nil {
		return
	}

	closed := make(chan struct{})
	go func() {
		defer close(closed)
		if err := conn.Close(websocket.StatusNormalClosure, "bridge closed"); err != nil {
			bridgeLogger.Printf("close connection error: %v", err)
		}
	}()

	select {
	case <-closed:
	case <-time.After(grace):
		bridgeLogger.Printf("close handshake did not finish in %s — dropping the socket", grace)
	}
}
```

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/bridge/ -v && go test ./...`
Expected: PASS, including `TestBridgeClose_NoPanic` and `TestBridgeClose_DrainsPending`.

- [ ] **Step 6: Commit**

```bash
git add internal/bridge/
git commit -m "fix: bound the WebSocket close handshake so shutdown is prompt"
```

---

## Task 12: Wait out the handover (severable)

**Files:**
- Modify: `internal/bridge/bridge.go` (`Bridge` struct, `NewBridge`, `HandleUpgrade`, `Send`)
- Test: `internal/bridge/bridge_test.go`

**Interfaces:**
- Produces: `Bridge.connectGrace time.Duration`, unexported, overridable in tests.

The leader dies, a follower notices after 3–5s, binds the port, and the plugin reconnects 1.5s later (`plugin/src/ui/App.svelte:28`). `Send` currently fails instantly through that window.

- [ ] **Step 1: Write the failing test**

Append to `internal/bridge/bridge_test.go`:

```go
// After a takeover the plugin needs about 1.5s to notice and reconnect
// (App.svelte's RECONNECT_DELAY_MS). Failing instantly during that window
// reports "plugin not connected" for a plugin that is on its way back.
func TestSend_WaitsBrieflyForAReconnectingPlugin(t *testing.T) {
	b := NewBridge("0.1.1")
	b.connectGrace = time.Second

	srv := httptest.NewServer(http.HandlerFunc(b.HandleUpgrade))
	t.Cleanup(srv.Close)

	// The plugin arrives after Send has already started waiting.
	go func() {
		time.Sleep(200 * time.Millisecond)
		wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
		client, _, err := websocket.Dial(context.Background(), wsURL, nil)
		if err != nil {
			return
		}
		var req Request
		if err := readJSON(context.Background(), client, &req); err != nil {
			return
		}
		writeJSON(context.Background(), client, Response{ //nolint:errcheck
			Type:      req.Type,
			RequestID: req.RequestID,
			Data:      map[string]any{"ok": true},
		})
	}()

	resp, err := b.Send(context.Background(), "get_document", nil, nil)
	if err != nil {
		t.Fatalf("Send gave up on a plugin that was reconnecting: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("plugin error: %s", resp.Error)
	}
}

func TestSend_StillReportsAPluginThatNeverArrives(t *testing.T) {
	b := NewBridge("0.1.1")
	b.connectGrace = 50 * time.Millisecond

	_, err := b.Send(context.Background(), "get_document", nil, nil)
	if err == nil {
		t.Fatal("expected an error when no plugin connects")
	}
	if !strings.Contains(err.Error(), "plugin not connected") {
		t.Errorf("unexpected message: %v", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify the first fails**

Run: `go test ./internal/bridge/ -run 'TestSend_(WaitsBriefly|StillReports)' -v`
Expected: `TestSend_WaitsBrieflyForAReconnectingPlugin` FAILs with "plugin not connected"; the second passes already.

- [ ] **Step 3: Signal connections**

Add to the `Bridge` struct:

```go
	// connected is closed and replaced each time a plugin connects, so a Send
	// arriving during a takeover can wait for the next one instead of failing.
	connected chan struct{}

	// connectGrace is how long Send waits for a plugin that may be
	// reconnecting after a leader handover.
	connectGrace time.Duration
```

In `internal/bridge/timeout.go`:

```go
	// defaultConnectGrace covers the plugin's own reconnect delay
	// (RECONNECT_DELAY_MS = 1500 in plugin/src/ui/App.svelte) with a little
	// room, so a handover does not surface as "plugin not connected".
	defaultConnectGrace = 2 * time.Second
```

In `NewBridge`, add `connected: make(chan struct{})` and `connectGrace: defaultConnectGrace`.

In `HandleUpgrade`, inside the existing `b.mu.Lock()` block, after `b.conn = conn`:

```go
	// Wake anything waiting for a plugin, then arm the signal for the next wait.
	close(b.connected)
	b.connected = make(chan struct{})
```

- [ ] **Step 4: Wait in `Send`**

Replace the opening of `Send`:

```go
	b.mu.RLock()
	conn := b.conn
	arrived := b.connected
	grace := b.connectGrace
	b.mu.RUnlock()

	if conn == nil {
		// A leader handover leaves a gap: the new leader has the port but the
		// plugin has not noticed yet. Wait it out rather than reporting a
		// plugin that is on its way back as absent.
		select {
		case <-arrived:
			b.mu.RLock()
			conn = b.conn
			b.mu.RUnlock()
		case <-time.After(grace):
		case <-ctx.Done():
			return Response{}, ctx.Err()
		}
	}
	if conn == nil {
		return Response{}, errors.New("plugin not connected")
	}
```

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/bridge/ -v && go test ./...`
Expected: PASS. `TestBridgeSend_NotConnected` still passes — it gets the same message, 2s later.

If that test's duration becomes annoying, set `b.connectGrace = 10 * time.Millisecond` in it rather than weakening the assertion.

- [ ] **Step 6: Commit**

```bash
git add internal/bridge/
git commit -m "feat: wait out a plugin reconnect instead of failing the call"
```

---

## Task 13: `RoleUnknown` must not proxy to nobody

**Files:**
- Modify: `internal/cluster/node.go` (`Send`), `internal/cluster/election.go` (`determineRole`, `Start`)
- Test: `internal/cluster/node_test.go`, `internal/cluster/election_test.go`

**Interfaces:**
- No new exported surface. `Node.Send` returns an error when the role is `RoleUnknown`.

- [ ] **Step 1: Write the failing test**

Append to `internal/cluster/node_test.go`:

```go
// An Unknown role means the election has not settled. Falling through to the
// follower branch posts to a port nobody is listening on, and the user reads
// "connection refused", which says nothing about what is actually going on.
func TestNodeSend_UnknownRoleReportsTheRole(t *testing.T) {
	backend := &fakeBackend{}
	n := newNodeWithSender(backend)
	// newNodeWithSender leaves the node in RoleUnknown.

	_, err := n.Send(context.Background(), "get_document", nil, nil)
	if err == nil {
		t.Fatal("expected an error while the role is unknown")
	}
	if !strings.Contains(err.Error(), "no leader") {
		t.Errorf("unexpected message: %v", err)
	}
	if len(backend.calls) != 0 {
		t.Errorf("Unknown role still proxied to the leader: %v", backend.calls)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/cluster/ -run TestNodeSend_UnknownRoleReportsTheRole -v`
Expected: FAIL — the call reaches `fakeBackend`.

- [ ] **Step 3: Handle the role explicitly**

In `internal/cluster/node.go`, replace the routing block in `Send`:

```go
	switch {
	case role == RoleLeader && leader != nil:
		resp, err = leader.GetBridge().Send(ctx, tool, nodeIDs, params)
	case role == RoleFollower:
		resp, err = follower.Send(ctx, tool, nodeIDs, params)
	default:
		// The election has not settled. Say so: proxying to a port nobody
		// holds only produces "connection refused".
		return nil, errors.New("no leader yet — the server is still electing one, retry in a moment")
	}
```

- [ ] **Step 4: Write the second failing test**

Append to `internal/cluster/election_test.go`:

```go
// "Port taken but nothing answering" is a startup race, not a settled state.
// Waiting a whole monitor tick to look again leaves the node routing nowhere
// for 3-5 seconds.
func TestDetermineRole_RetriesQuicklyWhenNothingAnswers(t *testing.T) {
	port := freePort(t)

	// Hold the port with a plain listener: taken, but no /ping.
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	n := NewNode("127.0.0.1", port, "test", passthroughGuard)
	e := NewElection("127.0.0.1", port, n)
	if err := e.Start(t.Context()); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(e.Stop)

	if n.Role() != RoleUnknown {
		t.Fatalf("want RoleUnknown while the port is held, got %s", n.RoleName())
	}

	// Release it; the retry should take the port well inside a monitor tick.
	ln.Close()
	waitForRole(t, n, RoleLeader, 2*time.Second)
}

func waitForRole(t *testing.T, n *Node, want Role, limit time.Duration) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if n.Role() == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out after %s waiting for role %d, still %s", limit, want, n.RoleName())
}
```

- [ ] **Step 5: Run it to verify it fails**

Run: `go test ./internal/cluster/ -run TestDetermineRole_RetriesQuicklyWhenNothingAnswers -v`
Expected: FAIL — the role stays Unknown for a full 3–5s jitter tick, past the 2s limit.

- [ ] **Step 6: Retry quickly**

In `internal/cluster/election.go`, add a short retry loop to `Start` after `determineRole`:

```go
	monitorCtx, cancel := context.WithCancel(ctx)
	e.cancel = cancel
	if e.node.Role() == RoleUnknown {
		go e.retryUntilSettled(monitorCtx)
	}
	go e.monitor(monitorCtx)
	return nil
```

and add:

```go
// retryUntilSettled closes the startup race: the port was taken but nothing
// answered, which usually means another process is a few milliseconds from
// being ready — or is on its way out. Checking every 200ms settles the role in
// well under one monitor tick.
func (e *Election) retryUntilSettled(ctx context.Context) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if e.node.Role() != RoleUnknown {
				return
			}
			if err := e.determineRole(ctx); err != nil {
				electionLogger.Printf("retry error: %v", err)
			}
		}
	}
}
```

- [ ] **Step 7: Run the tests**

Run: `go test ./internal/cluster/ -v && go test ./...`
Expected: PASS.

- [ ] **Step 8: Update the affected tool-layer assertion**

`internal/tools/tools_handler_test.go` has tests that relied on an Unknown node producing `IsError=true` through a connection failure. `TestMakeHandler_UnknownNode` still passes — the result is still an error — but if any assertion checks for `connection refused`, change it to the new message. Find them with:

```bash
grep -rn "connection refused" internal/
```

- [ ] **Step 9: Commit**

```bash
git add internal/
git commit -m "fix: report an unsettled election instead of proxying to nobody"
```

---

## Task 14: Timeouts on the leader's HTTP server

**Files:**
- Modify: `internal/cluster/leader.go` (`Leader` struct, `NewLeader`, `Start`, `handleRPC`)
- Test: `internal/cluster/leader_test.go`

**Interfaces:**
- Produces: `Leader.readHeaderTimeout time.Duration`, unexported, overridable in tests.

**Do not set `ReadTimeout` or `WriteTimeout`.** `/ws` hijacks the connection and `coder/websocket` does not clear the deadline `http.Server` leaves on it — `hijack.go` only locates the `Hijacker` interface. A `WriteTimeout` would kill the plugin socket after exactly that many seconds. `ReadHeaderTimeout` is safe because `net/http` clears the read deadline once the headers are read when `ReadTimeout` is zero.

- [ ] **Step 1: Write the failing test**

Append to `internal/cluster/leader_test.go`:

```go
// A WebSocket has to outlive the header deadline. This is the test that stops
// someone "tidying up" by adding a WriteTimeout: with one set, the plugin's
// socket dies on a timer.
func TestLeaderStart_WebSocketOutlivesTheHeaderTimeout(t *testing.T) {
	port := freePort(t)
	leader := NewLeader("127.0.0.1", port, "test", passthroughGuard)
	leader.readHeaderTimeout = 100 * time.Millisecond
	if err := leader.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(leader.Stop)

	wsURL := fmt.Sprintf("ws://127.0.0.1:%d/ws", port)
	client, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	t.Cleanup(func() { client.Close(websocket.StatusNormalClosure, "") })

	// Well past the header deadline.
	time.Sleep(400 * time.Millisecond)

	if !leader.GetBridge().IsConnected() {
		t.Fatal("the plugin socket was closed by an HTTP server timeout")
	}
}

func TestLeaderStart_SetsAHeaderTimeoutAndNoWriteTimeout(t *testing.T) {
	port := freePort(t)
	leader := NewLeader("127.0.0.1", port, "test", passthroughGuard)
	if err := leader.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(leader.Stop)

	if leader.server.ReadHeaderTimeout == 0 {
		t.Error("ReadHeaderTimeout must be set — an idle connection can hold a slot forever")
	}
	if leader.server.WriteTimeout != 0 {
		t.Error("WriteTimeout must stay zero — /ws hijacks the connection and inherits the deadline")
	}
	if leader.server.ReadTimeout != 0 {
		t.Error("ReadTimeout must stay zero — same reason")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/cluster/ -run TestLeaderStart -v`
Expected: FAIL — `readHeaderTimeout` is undefined, and `ReadHeaderTimeout` is zero.

- [ ] **Step 3: Set the two safe timeouts**

Add to the `Leader` struct:

```go
	// readHeaderTimeout bounds how long a client may take to send its request
	// headers. Overridable so tests need not wait seconds.
	readHeaderTimeout time.Duration
```

In `NewLeader`, set `readHeaderTimeout: 5 * time.Second`.

In `Start`, replace the server construction:

```go
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: l.readHeaderTimeout,
		IdleTimeout:       60 * time.Second,
		// No ReadTimeout or WriteTimeout. /ws hijacks the connection and
		// coder/websocket does not clear the deadline http.Server leaves on it
		// (hijack.go only locates the Hijacker), so either one would kill the
		// plugin socket on a timer. ReadHeaderTimeout is safe: net/http clears
		// the read deadline after the headers when ReadTimeout is zero.
	}
```

Add `"time"` to the imports.

- [ ] **Step 4: Bound the `/rpc` body**

In `handleRPC`, replace `body, err := io.ReadAll(r.Body)` with:

```go
	// A tool call carries params, not a file. 32 MB is generous and stops an
	// unbounded read.
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 32<<20))
```

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/cluster/ -v && go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/cluster/
git commit -m "fix: bound HTTP headers and the /rpc body without touching the WebSocket"
```

---

## Task 15: Move to `log/slog`

**Files:**
- Modify: `cmd/figma-mcp-go/main.go`, `internal/bridge/bridge.go`, `internal/cluster/{node,leader,follower,election}.go`
- Test: `cmd/figma-mcp-go/main_test.go` (new)

**Interfaces:**
- Produces: `FIGMA_MCP_LOG` environment variable accepting `debug`, `info`, `warn`, `error`; default `info`.

Each package resolves its logger through a function rather than a package-level variable. A package variable is initialised before `main` runs, so it would capture the stock default handler and ignore the level `cmd` installs.

- [ ] **Step 1: Write the failing test**

Create `cmd/figma-mcp-go/main_test.go`:

```go
package main

import (
	"log/slog"
	"testing"
)

func TestLogLevelFor(t *testing.T) {
	cases := map[string]slog.Level{
		"":       slog.LevelInfo,
		"info":   slog.LevelInfo,
		"debug":  slog.LevelDebug,
		"DEBUG":  slog.LevelDebug,
		"warn":   slog.LevelWarn,
		"error":  slog.LevelError,
		"gibber": slog.LevelInfo,
	}
	for in, want := range cases {
		if got := logLevelFor(in); got != want {
			t.Errorf("logLevelFor(%q) = %v, want %v", in, got, want)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./cmd/figma-mcp-go/ -v`
Expected: FAIL — `undefined: logLevelFor`

- [ ] **Step 3: Set up logging in `cmd`**

In `cmd/figma-mcp-go/main.go`, replace `var logger = log.New(os.Stderr, "", 0)` with:

```go
// logLevelFor maps FIGMA_MCP_LOG to a level. Anything unrecognised is info —
// a typo in an environment variable should not silence the server.
func logLevelFor(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// setupLogging installs the default logger. Stderr, because stdout carries the
// MCP protocol.
func setupLogging() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevelFor(os.Getenv("FIGMA_MCP_LOG")),
	})))
}
```

Call `setupLogging()` as the first statement in `main`, and replace the `logger.Fatalf` / `logger.Printf` calls with `slog` equivalents, e.g.:

```go
	if parsedIP == nil {
		slog.Error("invalid IP address", "ip", *ip)
		os.Exit(1)
	}
	// …
	slog.Info("starting", "version", version, "role", node.RoleName())
```

Imports: add `"log/slog"` and `"strings"`, drop `"log"`.

- [ ] **Step 4: Replace the package loggers**

In each of `internal/bridge/bridge.go`, `internal/cluster/node.go`, `internal/cluster/leader.go`, `internal/cluster/follower.go` and `internal/cluster/election.go`, replace the `var xLogger = log.New(os.Stderr, "[x] ", 0)` line with:

```go
// log is resolved per call rather than held in a package variable: a package
// variable is initialised before main installs the default handler, so it
// would capture the stock one and ignore the configured level.
func log() *slog.Logger { return slog.Default().With("component", "bridge") }
```

using the matching component name in each file (`bridge`, `node`, `leader`, `follower`, `election`). Where two files share a package — `cluster` has four — name the functions distinctly (`nodeLog`, `leaderLog`, `followerLog`, `electionLog`) since they cannot all be called `log`.

Convert the `Printf` calls to structured calls, keeping the same information. For example, in `bridge.go`:

```go
	log().Info("plugin connected", "remote", r.RemoteAddr, "replaced", replaced)
	// …
	log().Warn("keepalive: no pong, dropping connection", "err", err)
```

and in `follower.go`:

```go
	followerLog().Info("proxied", "tool", tool, "ms", time.Since(start).Milliseconds())
```

Drop the now-unused `"log"` and `"os"` imports where they become unused.

- [ ] **Step 5: Run the tests**

Run: `go test ./... && make fmt-check && go vet ./...`
Expected: PASS.

- [ ] **Step 6: Check it by hand**

Run: `go build -o bin/figma-mcp-go ./cmd/figma-mcp-go && FIGMA_MCP_LOG=debug ./bin/figma-mcp-go --port 19941 2>&1 | head -5`
Expected: `level=INFO msg=starting version=… role=LEADER` style output on stderr. Stop it with Ctrl-C.

- [ ] **Step 7: Commit**

```bash
git add internal/ cmd/
git commit -m "feat: structured logging through log/slog with FIGMA_MCP_LOG"
```

---

## Task 16: Keep design content out of the default log

**Files:**
- Modify: `internal/bridge/bridge.go` (`Send`), `internal/cluster/follower.go` (`Send`)
- Test: `internal/bridge/bridge_test.go`

**Interfaces:**
- No new surface. Full params move to `debug`; info keeps tool name, node count and payload size.

`bridge.go:306` and `follower.go:35` print the whole params map on every call. That is the user's design content, in every log line, at the default level.

- [ ] **Step 1: Write the failing test**

Append to `internal/bridge/bridge_test.go`:

```go
// The params map holds whatever the user is designing — text content, colours,
// names. It is fine at debug, where someone has asked for it. It is not fine in
// the default output.
func TestSend_DoesNotLogParamsAtInfo(t *testing.T) {
	var buf bytes.Buffer
	restore := slog.Default()
	t.Cleanup(func() { slog.SetDefault(restore) })
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))

	b, _ := setupBridgeWithClient(t)
	b.Send(context.Background(), "set_text", []string{"1:1"}, //nolint:errcheck
		map[string]any{"text": "Quarterly revenue projection"})

	if strings.Contains(buf.String(), "Quarterly revenue projection") {
		t.Errorf("params reached the default log:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "set_text") {
		t.Errorf("the tool name should still be logged:\n%s", buf.String())
	}
}

func TestSend_LogsParamsAtDebug(t *testing.T) {
	var buf bytes.Buffer
	restore := slog.Default()
	t.Cleanup(func() { slog.SetDefault(restore) })
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	b, _ := setupBridgeWithClient(t)
	b.Send(context.Background(), "set_text", []string{"1:1"}, //nolint:errcheck
		map[string]any{"text": "Quarterly revenue projection"})

	if !strings.Contains(buf.String(), "Quarterly revenue projection") {
		t.Errorf("debug should carry the params:\n%s", buf.String())
	}
}
```

Add `"bytes"` and `"log/slog"` to that file's imports.

- [ ] **Step 2: Run the tests to verify the first fails**

Run: `go test ./internal/bridge/ -run 'TestSend_(DoesNotLogParams|LogsParams)' -v`
Expected: `TestSend_DoesNotLogParamsAtInfo` FAILs; the second passes.

- [ ] **Step 3: Split the two levels**

In `internal/bridge/bridge.go`, replace the outbound log line in `Send`:

```go
	log().Info("request", "id", requestID, "tool", requestType, "nodes", len(nodeIDs), "paramBytes", paramSize(params))
	log().Debug("request params", "id", requestID, "params", params)
```

and add:

```go
// paramSize is how big a params map is on the wire, for a log line that says
// something about the payload without quoting the user's design back at them.
func paramSize(params map[string]any) int {
	if params == nil {
		return 0
	}
	b, err := json.Marshal(params)
	if err != nil {
		return -1
	}
	return len(b)
}
```

In `internal/cluster/follower.go`, replace the opening log line of `Send`:

```go
	followerLog().Info("proxying", "tool", tool, "nodes", len(nodeIDs), "leader", f.leaderURL)
	followerLog().Debug("proxy params", "tool", tool, "params", params)
```

- [ ] **Step 4: Run the tests**

Run: `go test ./... && make fmt-check && go vet ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/
git commit -m "fix: keep tool parameters out of the default log level"
```

---

## Task 17: `/ping` reports state

**Files:**
- Modify: `internal/cluster/leader.go` (`Leader` struct, `NewLeader`, `handlePing`), `internal/bridge/bridge.go` (add `Pending`)
- Test: `internal/cluster/leader_test.go`

**Interfaces:**
- Produces: `bridge.Bridge.Pending() int`; `/ping` returns `status`, `version`, `role`, `connected`, `pending`, `uptimeSeconds`.
- `Follower.Ping` only reads the status code, so the extra fields are backwards compatible.

- [ ] **Step 1: Write the failing test**

Append to `internal/cluster/leader_test.go`:

```go
// /ping is the only thing a user can query when something is wrong. "ok" alone
// does not distinguish a leader with no plugin from a healthy one.
func TestLeaderPing_ReportsState(t *testing.T) {
	port := freePort(t)
	leader := NewLeader("127.0.0.1", port, "9.9.9", passthroughGuard)
	if err := leader.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(leader.Stop)

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/ping", port))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	var got map[string]any
	if err := json.UnmarshalRead(resp.Body, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	for _, key := range []string{"status", "version", "role", "connected", "pending", "uptimeSeconds"} {
		if _, ok := got[key]; !ok {
			t.Errorf("/ping is missing %q: %v", key, got)
		}
	}
	if got["connected"] != false {
		t.Errorf("want connected=false with no plugin, got %v", got["connected"])
	}
	if got["version"] != "9.9.9" {
		t.Errorf("want version 9.9.9, got %v", got["version"])
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/cluster/ -run TestLeaderPing_ReportsState -v`
Expected: FAIL — `role`, `connected`, `pending` and `uptimeSeconds` are missing.

- [ ] **Step 3: Expose the pending count**

In `internal/bridge/bridge.go`, next to `IsConnected`:

```go
// Pending is how many requests are in flight, for the health endpoint.
func (b *Bridge) Pending() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.pending)
}
```

- [ ] **Step 4: Enrich `/ping`**

Add `started time.Time` to the `Leader` struct and set `started: time.Now()` in `NewLeader`. Replace `handlePing`'s body:

```go
	w.Header().Set("Content-Type", "application/json")
	err := json.MarshalWrite(w, map[string]any{
		"status":        "ok",
		"version":       l.version,
		"role":          "LEADER",
		"connected":     l.b.IsConnected(),
		"pending":       l.b.Pending(),
		"uptimeSeconds": int(time.Since(l.started).Seconds()),
	}, json.Deterministic(true))
	if err != nil {
		leaderLog().Warn("encode ping response", "err", err)
	}
```

`role` is the literal `"LEADER"`: only a leader serves this endpoint.

- [ ] **Step 5: Run the tests**

Run: `go test ./... && make fmt-check && go vet ./...`
Expected: PASS, including the existing follower ping tests.

- [ ] **Step 6: Commit**

```bash
git add internal/
git commit -m "feat: /ping reports role, connection, pending count and uptime"
```

---

## Task 18: Document the behaviour changes

**Files:**
- Modify: `README.md` (a new subsection under `## Upgrading` at line 139, and a line under `## Development` at line 338)

**Interfaces:**
- No code. This is the record users read when something they relied on changes.

- [ ] **Step 1: Add the upgrade note**

In `README.md`, directly after the "Re-download the plugin" paragraph and before `### Breaking changes in 0.1.0`, insert:

```markdown
### Behaviour changes in 0.3.0

No tool changed its name or its arguments. Four things behave differently:

- **Log format.** Server logs are now structured (`time=… level=INFO component=bridge …`)
  rather than prefixed (`[bridge] …`). Anything grepping for `[bridge]` needs updating.
  Logs still go to stderr; stdout still carries only the MCP protocol.
- **Log level.** Set `FIGMA_MCP_LOG` to `debug`, `info`, `warn` or `error`. The default is
  `info`, and tool parameters — your text, colours and names — now only appear at `debug`.
- **Starting up.** A call made before the server has settled on a leader now reports that,
  instead of failing with `connection refused`.
- **`/ping`** returns `role`, `connected`, `pending` and `uptimeSeconds` alongside `status`
  and `version`.
```

If the release that carries this work is not 0.3.0, use its number in the heading.

- [ ] **Step 2: Document the environment variable where developers look**

Under `## Development`, after the "Testing" bullet:

```markdown
- **Logs**: stderr, structured. `FIGMA_MCP_LOG=debug` to see tool parameters and wire traffic
- **Layering**: `make deps-check` fails the build on an import that crosses the package
  boundaries the wrong way (`tools → figma`, `cluster → bridge`, and nothing backwards)
```

- [ ] **Step 3: Run the full verification**

Run: `make deps-check && make test-go && make fmt-check && go vet ./... && shasum -a 256 internal/tools/testdata/tools_schema.json`
Expected: all pass, and the sha is still `8914d70197487e6a53e8b4e4b9edc83df7f667ed553914793573cc3bfad1d874`.

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: record the logging, startup and /ping behaviour changes"
```

---

## Severable tasks

Two tasks can be dropped without touching the rest of the plan:

- **Task 11** (bounded `Close`) — a shutdown-latency fix, worth about four seconds of exit time in the case where the plugin has already vanished.
- **Task 12** (wait out the handover) — removes a "plugin not connected" that resolves itself in about 1.5 seconds.

Dropping either leaves every other task's tests and code unchanged.
