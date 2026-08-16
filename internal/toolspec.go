package internal

import (
	"context"
	"fmt"

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

type paramKind int

const (
	kindString paramKind = iota
	kindNumber
	kindBool
	kindStringArray
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

	// Positive drops a numeric argument that is not greater than zero rather
	// than forwarding it, letting the plugin apply its own default.
	Positive bool

	// IsNodeID marks a string argument that carries a Figma node ID, so it is
	// checked for the colon format like the dedicated nodeIDs field is.
	IsNodeID bool
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
	Validate func(nodeIDs []string, params map[string]interface{}) string
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

// specHandler builds the tool handler: pull the declared arguments out of the
// request, forward them to the plugin, render whatever comes back.
func specHandler(node *Node, spec toolSpec) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		nodeIDs, params := specArgs(spec, args)
		resp, err := node.Send(ctx, spec.Name, nodeIDs, params)
		return renderResponse(resp, err)
	}
}

// specArgs splits a request's arguments into node IDs and plugin params.
// Arguments the caller omitted are left out entirely so the plugin can apply
// its own defaults.
func specArgs(spec toolSpec, args map[string]interface{}) ([]string, map[string]interface{}) {
	var nodeIDs []string
	switch spec.NodeIDs {
	case nodeIDsSingle:
		if id, ok := args[spec.nodeIDsArg()].(string); ok && id != "" {
			nodeIDs = []string{id}
		}
	case nodeIDsMulti:
		raw, _ := args[spec.nodeIDsArg()].([]interface{})
		nodeIDs = toStringSlice(raw)
	}

	params := map[string]interface{}{}
	for _, p := range spec.Params {
		v, ok := args[p.Name]
		if !ok || v == nil {
			continue
		}
		switch p.Kind {
		case kindString:
			s, ok := v.(string)
			if !ok || s == "" {
				continue
			}
		case kindNumber:
			if p.Positive {
				n, ok := v.(float64)
				if !ok || n <= 0 {
					continue
				}
			}
		case kindStringArray:
			raw, ok := v.([]interface{})
			if !ok || len(raw) == 0 {
				continue
			}
		}
		params[p.wireName()] = v
	}

	return nodeIDs, params
}

// ── Validation ───────────────────────────────────────────────────────────────

// validateSpec applies the rules the spec states directly. Cross-field rules
// live in spec.Validate.
func validateSpec(spec toolSpec, nodeIDs []string, params map[string]interface{}) string {
	if spec.NodeIDs != nodeIDsNone {
		if spec.NodeIDsReq && len(nodeIDs) == 0 {
			return fmt.Sprintf("%s is required", spec.nodeIDsArg())
		}
		if spec.MinNodeIDs > 0 && len(nodeIDs) > 0 && len(nodeIDs) < spec.MinNodeIDs {
			return fmt.Sprintf("%s must contain at least %d nodes", spec.nodeIDsArg(), spec.MinNodeIDs)
		}
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
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
			if p.IsNodeID && s != "" && !ValidNodeID(s) {
				return fmt.Sprintf("%s must use colon format e.g. 4029:12345, got: %s", p.Name, s)
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
		case kindBool:
			if _, ok := v.(bool); !ok {
				return fmt.Sprintf("%s must be a boolean", p.Name)
			}
		}
	}

	if spec.Validate != nil {
		return spec.Validate(nodeIDs, params)
	}
	return ""
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// ── Registry ─────────────────────────────────────────────────────────────────

// specGroups lists every table-declared tool group. It is the one place a new
// group has to be added for both registration and validation to see it.
func specGroups() [][]toolSpec {
	return [][]toolSpec{
		readStyleSpecs,
	}
}

// specRegistry maps tool name to spec so validation can find a tool's rules
// from its name alone. Built once from the declarations, independent of how
// many servers get built — tests construct several.
var specRegistry = buildSpecRegistry()

func buildSpecRegistry() map[string]toolSpec {
	registry := map[string]toolSpec{}
	for _, group := range specGroups() {
		for _, spec := range group {
			if _, duplicate := registry[spec.Name]; duplicate {
				panic("duplicate tool spec: " + spec.Name)
			}
			registry[spec.Name] = spec
		}
	}
	return registry
}

// registerSpecs adds the tools to an MCP server.
func registerSpecs(s *server.MCPServer, node *Node, specs []toolSpec) {
	for _, spec := range specs {
		s.AddTool(buildTool(spec), specHandler(node, spec))
	}
}

func floatPtr(f float64) *float64 { return &f }
