package tools

import (
	"github.com/tunglt1810/figma-mcp-go/internal/figma"

	"context"
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Most tools are the same shape: read a few arguments, copy them into a params
// map, forward them to the plugin. Writing that out per tool meant the argument
// list appeared twice — once as an MCP schema, once as validation — with nothing
// keeping the two in step. A toolSpec states it once; the schema, the handler
// and the validation are all derived from it.
//
// Tools that do real work in Go (save_screenshots, export_frames_to_pdf) keep
// hand-written handlers.

// Sender carries a tool call to the Figma plugin. The tool layer does not know
// how — a leader writes to its WebSocket, a follower proxies over HTTP.
type Sender interface {
	Send(ctx context.Context, tool string, nodeIDs []string, params map[string]any) (any, error)
}

type paramKind int

const (
	kindString paramKind = iota
	kindNumber
	kindBool
	kindStringArray
	kindNumberArray
	kindObjectArray
	kindArray // an array whose element shape the schema does not state
	kindObject
	kindAny
)

// paramSpec describes one tool argument.
type paramSpec struct {
	Name string
	// Wire overrides the parameter name sent to the plugin when it differs
	// from the argument name exposed to the client.
	Wire     string
	Kind     paramKind
	Required bool
	Desc     string

	// Enum restricts a string argument to a fixed set. Used for validation;
	// spell the options out in Desc for the client to read.
	Enum []string

	// Min and Max bound a numeric argument, inclusive.
	Min, Max *float64

	// Positive requires a numeric argument to be greater than zero. Distinct
	// from Min: sizes and radii reject zero, which an inclusive bound cannot say.
	Positive bool

	// IsNodeID marks a string argument that carries a Figma node ID, so it is
	// checked for the colon format like the dedicated nodeIDs field is.
	IsNodeID bool

	// IsHexColor marks a string argument that carries a hex color.
	IsHexColor bool

	// AllowEmpty forwards an empty string instead of treating it as absent.
	// Replacement strings and text bodies use "" to mean "clear this".
	AllowEmpty bool

	// ItemSchema spells out an array's element schema where "an object" is too
	// vague to be useful to the client.
	ItemSchema map[string]any
}

func (p paramSpec) wireName() string {
	if p.Wire != "" {
		return p.Wire
	}
	return p.Name
}

// nodeIDMode says how a tool receives the nodes it acts on. Node IDs travel in
// their own field on the wire rather than inside params.
type nodeIDMode int

const (
	nodeIDsNone   nodeIDMode = iota
	nodeIDsSingle            // one "nodeId" string argument
	nodeIDsMulti             // a "nodeIds" array argument
)

// toolSpec is the single declaration of a tool.
type toolSpec struct {
	Name string
	Desc string

	NodeIDs     nodeIDMode
	NodeIDDesc  string
	NodeIDsReq  bool // node IDs are a required argument
	MinNodeIDs  int  // minimum count once supplied (0 means no minimum)
	NodeIDsName string

	Params []paramSpec

	// Validate expresses rules a paramSpec cannot: "at least one of x or y",
	// mutually exclusive arguments, nested shapes.
	Validate func(nodeIDs []string, params map[string]any) string

	// Custom, when set, replaces the default forwarder. It takes the Sender
	// because a spec is a package-level variable and cannot capture one at
	// declaration time. The schema and the checking still come from the table,
	// so a tool that does work in Go cannot drift from it either.
	Custom func(Sender) customHandler
}

func (s toolSpec) nodeIDsArg() string {
	if s.NodeIDsName != "" {
		return s.NodeIDsName
	}
	if s.NodeIDs == nodeIDsMulti {
		return "nodeIds"
	}
	return "nodeId"
}

// ── Schema ───────────────────────────────────────────────────────────────────

func (p paramSpec) toolOption() mcp.ToolOption {
	opts := []mcp.PropertyOption{}
	if p.Required {
		opts = append(opts, mcp.Required())
	}
	opts = append(opts, mcp.Description(p.Desc))

	switch p.Kind {
	case kindNumber:
		return mcp.WithNumber(p.Name, opts...)
	case kindBool:
		return mcp.WithBoolean(p.Name, opts...)
	case kindStringArray:
		return mcp.WithArray(p.Name, append(opts, mcp.WithStringItems())...)
	case kindNumberArray:
		return mcp.WithArray(p.Name, append(opts, mcp.Items(map[string]any{"type": "number"}))...)
	case kindObjectArray:
		items := p.ItemSchema
		if items == nil {
			items = map[string]any{"type": "object"}
		}
		return mcp.WithArray(p.Name, append(opts, mcp.Items(items))...)
	case kindArray:
		return mcp.WithArray(p.Name, opts...)
	case kindObject:
		return mcp.WithObject(p.Name, opts...)
	case kindAny:
		return mcp.WithAny(p.Name, opts...)
	default:
		return mcp.WithString(p.Name, opts...)
	}
}

// buildTool turns a spec into the MCP tool definition. Node IDs come first so
// the generated schema matches how these tools have always been declared.
func buildTool(spec toolSpec) mcp.Tool {
	opts := []mcp.ToolOption{mcp.WithDescription(spec.Desc)}

	if spec.NodeIDs != nodeIDsNone {
		nodeOpts := []mcp.PropertyOption{}
		if spec.NodeIDsReq {
			nodeOpts = append(nodeOpts, mcp.Required())
		}
		nodeOpts = append(nodeOpts, mcp.Description(spec.NodeIDDesc))

		if spec.NodeIDs == nodeIDsMulti {
			opts = append(opts, mcp.WithArray(spec.nodeIDsArg(), append(nodeOpts, mcp.WithStringItems())...))
		} else {
			opts = append(opts, mcp.WithString(spec.nodeIDsArg(), nodeOpts...))
		}
	}

	for _, p := range spec.Params {
		opts = append(opts, p.toolOption())
	}

	return mcp.NewTool(spec.Name, opts...)
}

// ── Handler ──────────────────────────────────────────────────────────────────

// specArgs splits a request's arguments into node IDs and plugin params.
// Arguments the caller omitted are left out entirely so the plugin can apply
// its own defaults.
func specArgs(spec toolSpec, args map[string]any) ([]string, map[string]any) {
	var nodeIDs []string
	switch spec.NodeIDs {
	case nodeIDsSingle:
		if id, ok := args[spec.nodeIDsArg()].(string); ok && id != "" {
			nodeIDs = []string{id}
		}
	case nodeIDsMulti:
		raw, _ := args[spec.nodeIDsArg()].([]any)
		nodeIDs = toStringSlice(raw)
	}

	params := map[string]any{}
	for _, p := range spec.Params {
		v, ok := args[p.Name]
		if !ok || v == nil {
			continue
		}
		switch p.Kind {
		case kindString:
			s, ok := v.(string)
			if !ok || (s == "" && !p.AllowEmpty) {
				continue
			}
		case kindStringArray:
			raw, ok := v.([]any)
			if !ok || len(raw) == 0 {
				continue
			}
		}
		params[p.wireName()] = v
	}

	return nodeIDs, params
}

// ── Normalization ────────────────────────────────────────────────────────────

// nodeIDParams are the parameter names that carry a Figma node ID and so need
// the same hyphen→colon normalization as the nodeIDs slice.
// nodeIDParams name the arguments that carry a single node ID. They are
// matched at any depth: a pipeline step nests a whole parameter set of its own
// under steps[].params, and a reaction nests a destination below that again.
var nodeIDParams = map[string]bool{
	"nodeId": true, "parentId": true, "pageId": true, "componentId": true,
	"startNodeId": true, "endNodeId": true, "destinationId": true,
}

// normalizeArgs returns copies of the arguments with node IDs normalized.
// Copies, not in-place edits: the caller's slice and map belong to the caller.
func normalizeArgs(nodeIDs []string, params map[string]any) ([]string, map[string]any) {
	var ids []string
	if nodeIDs != nil {
		ids = make([]string, len(nodeIDs))
		for i, id := range nodeIDs {
			ids[i] = figma.NormalizeNodeID(id)
		}
	}

	var p map[string]any
	if params != nil {
		p, _ = normalizeValue(params).(map[string]any)
	}

	return ids, p
}

// normalizeValue rebuilds v with every node ID it can find normalized. Maps and
// slices are rebuilt rather than edited, so nothing the caller passed in
// changes underfoot.
func normalizeValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			switch {
			case nodeIDParams[k]:
				if s, ok := val.(string); ok {
					out[k] = figma.NormalizeNodeID(s)
					continue
				}
				out[k] = normalizeValue(val)
			case k == "nodeIds":
				out[k] = normalizeIDList(val)
			default:
				out[k] = normalizeValue(val)
			}
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = normalizeValue(item)
		}
		return out
	default:
		return v
	}
}

// normalizeIDList handles a "nodeIds" value, which is a list of ids rather than
// a nested structure.
func normalizeIDList(v any) any {
	list, ok := v.([]any)
	if !ok {
		return normalizeValue(v)
	}
	out := make([]any, len(list))
	for i, item := range list {
		if s, ok := item.(string); ok {
			out[i] = figma.NormalizeNodeID(s)
			continue
		}
		out[i] = normalizeValue(item)
	}
	return out
}

// ── Validation ───────────────────────────────────────────────────────────────

// validateSpec applies the rules the spec states directly. Cross-field rules
// live in spec.Validate.
func validateSpec(spec toolSpec, nodeIDs []string, params map[string]any) string {
	if spec.NodeIDs != nodeIDsNone {
		if spec.NodeIDsReq && len(nodeIDs) == 0 {
			return fmt.Sprintf("%s is required", spec.nodeIDsArg())
		}
		if spec.MinNodeIDs > 0 && len(nodeIDs) > 0 && len(nodeIDs) < spec.MinNodeIDs {
			return fmt.Sprintf("%s must contain at least %d nodes", spec.nodeIDsArg(), spec.MinNodeIDs)
		}
		for _, id := range nodeIDs {
			if !figma.ValidNodeID(id) {
				return fmt.Sprintf("invalid nodeId: %s — must use colon format e.g. 4029:12345", id)
			}
		}
	}

	for _, p := range spec.Params {
		v, present := params[p.wireName()]
		if !present {
			if p.Required {
				return fmt.Sprintf("%s is required", p.Name)
			}
			continue
		}

		switch p.Kind {
		case kindString:
			s, ok := v.(string)
			if !ok {
				return fmt.Sprintf("%s must be a string", p.Name)
			}
			if len(p.Enum) > 0 && !containsString(p.Enum, s) {
				return fmt.Sprintf("%s must be one of %v, got: %s", p.Name, p.Enum, s)
			}
			if p.IsNodeID && s != "" && !figma.ValidNodeID(s) {
				return fmt.Sprintf("%s must use colon format e.g. 4029:12345, got: %s", p.Name, s)
			}
			if p.IsHexColor && !figma.ValidHexColor(s) {
				return fmt.Sprintf("%s must be a hex color e.g. #FF5733, got: %s", p.Name, s)
			}
		case kindNumber:
			n, ok := v.(float64)
			if !ok {
				return fmt.Sprintf("%s must be a number", p.Name)
			}
			if p.Min != nil && n < *p.Min {
				return fmt.Sprintf("%s must be at least %g, got: %g", p.Name, *p.Min, n)
			}
			if p.Max != nil && n > *p.Max {
				return fmt.Sprintf("%s must be at most %g, got: %g", p.Name, *p.Max, n)
			}
			if p.Positive && n <= 0 {
				return fmt.Sprintf("%s must be positive", p.Name)
			}
		case kindBool:
			if _, ok := v.(bool); !ok {
				return fmt.Sprintf("%s must be a boolean", p.Name)
			}
		case kindObject:
			if _, ok := v.(map[string]any); !ok {
				return fmt.Sprintf("%s must be an object", p.Name)
			}
		case kindStringArray, kindNumberArray, kindObjectArray, kindArray:
			if msg := validateArrayParam(p, v); msg != "" {
				return msg
			}
		}
	}

	if spec.Validate != nil {
		return spec.Validate(nodeIDs, params)
	}
	return ""
}

// validateArrayParam checks an array argument and, where the spec states an
// element type, each of its elements.
func validateArrayParam(p paramSpec, v any) string {
	items, ok := v.([]any)
	if !ok {
		return fmt.Sprintf("%s must be an array", p.Name)
	}
	for i, item := range items {
		var wrong bool
		var want string
		switch p.Kind {
		case kindStringArray:
			_, good := item.(string)
			wrong, want = !good, "a string"
		case kindNumberArray:
			_, good := item.(float64)
			wrong, want = !good, "a number"
		case kindObjectArray:
			_, good := item.(map[string]any)
			wrong, want = !good, "an object"
		}
		if wrong {
			return fmt.Sprintf("%s[%d] must be %s", p.Name, i, want)
		}
	}
	return ""
}

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

// variantSpec describes one variant of a merged tool: the arguments it accepts
// and the ones it cannot do without.
type variantSpec struct {
	Allowed  []string
	Required []string
}

// requireVariant builds a Validate for a tool that merged several tools behind
// a discriminator argument.
//
// Merging tools trades one failure for another: the model stops picking the
// wrong tool and starts picking the wrong argument. That trade is only worth
// making if the wrong argument is reported rather than quietly ignored, so an
// argument belonging to a different variant is an error here, named and
// attributed to the variant it belongs to.
func requireVariant(discriminator string, variants map[string]variantSpec, common ...string) func([]string, map[string]any) string {
	return func(_ []string, params map[string]any) string {
		kind, _ := params[discriminator].(string)
		variant, known := variants[kind]
		if !known {
			// The discriminator's Enum has already reported an unknown value,
			// and a missing one is reported by Required.
			return ""
		}

		allowed := map[string]bool{discriminator: true}
		for _, k := range common {
			allowed[k] = true
		}
		for _, k := range variant.Allowed {
			allowed[k] = true
		}

		var stray []string
		for k := range params {
			if !allowed[k] {
				stray = append(stray, k)
			}
		}
		if len(stray) > 0 {
			sort.Strings(stray) // a map's order must not reach the message
			return fmt.Sprintf("%s does not apply when %s is %s", stray[0], discriminator, kind)
		}

		for _, k := range variant.Required {
			if _, ok := params[k]; !ok {
				return fmt.Sprintf("%s is required when %s is %s", k, discriminator, kind)
			}
		}
		return ""
	}
}

// variantKinds lists a variant map's keys in sorted order, for the
// discriminator's Enum.
func variantKinds(variants map[string]variantSpec) []string {
	kinds := make([]string, 0, len(variants))
	for k := range variants {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	return kinds
}

// requireAnyOf builds a Validate rejecting a request that supplies none of the
// listed arguments. Several tools accept a choice of fields but need one.
func requireAnyOf(msg string, keys ...string) func([]string, map[string]any) string {
	return func(_ []string, params map[string]any) string {
		for _, k := range keys {
			if _, ok := params[k]; ok {
				return ""
			}
		}
		return msg
	}
}

func containsString(haystack []string, needle string) bool {
	return slices.Contains(haystack, needle)
}

// ── Registry ─────────────────────────────────────────────────────────────────

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

// specRegistry maps tool name to spec so validation can find a tool's rules
// from its name alone. Built once from the declarations, independent of how
// many servers get built — tests construct several.
var specRegistry = buildSpecRegistry()

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

// handlerFor builds a tool's MCP handler: split the arguments, check them, then
// run either the tool's own Go code or the plain forwarder. Both paths check,
// which is what makes "every call is validated exactly once" true by
// construction rather than by remembering to do it.
func handlerFor(sender Sender, spec toolSpec) server.ToolHandlerFunc {
	var handle customHandler
	if spec.Custom != nil {
		// A Custom handler is free to call a different tool than the one it was
		// invoked as — save_screenshots calls get_screenshot once per item, with
		// params it builds itself. Checking the arguments this handler received
		// says nothing about those, so give it a Sender that checks each call
		// against the spec of whatever tool the call actually names.
		handle = spec.Custom(checkedSender{sender})
	} else {
		handle = forwarder(spec.Name)(sender)
	}

	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nodeIDs, params := specArgs(spec, req.GetArguments())
		nodeIDs, params, err := Check(spec.Name, nodeIDs, params)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return handle(ctx, nodeIDs, params)
	}
}

// checkedSender applies Check to every call passing through it, by the name of
// the tool actually being called. The invariant is about Sender calls, not about
// entry points: a handler that reaches the plugin under a second tool's name has
// to satisfy that tool's spec too.
type checkedSender struct{ inner Sender }

func (c checkedSender) Send(ctx context.Context, tool string, nodeIDs []string, params map[string]any) (any, error) {
	nodeIDs, params, err := Check(tool, nodeIDs, params)
	if err != nil {
		return nil, err
	}
	return c.inner.Send(ctx, tool, nodeIDs, params)
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

// customHandler is the handler of a tool that does work in Go — writing files,
// merging PDFs — around its call to the plugin. It is handed arguments already
// split, normalized and checked against the spec.
type customHandler func(ctx context.Context, nodeIDs []string, params map[string]any) (*mcp.CallToolResult, error)

func floatPtr(f float64) *float64 { return new(f) }
