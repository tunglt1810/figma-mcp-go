package tools

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

const pipelineTool = "batch_execute_pipeline"

var batchPipelineSpec = toolSpec{
	Name: pipelineTool,
	Desc: "Run write steps in order. A step can use earlier results through $variables. " +
		"With stop_on_error, a failure undoes created nodes and changed properties, " +
		"but not deletes, group/ungroup, detach, reparent, or steps that target nodes by name.",

	Params: []paramSpec{
		{Name: "stop_on_error", Kind: kindBool,
			Desc: "Stop and undo on error (default true)"},
		{Name: "steps", Kind: kindObjectArray, Required: true,
			Desc: "Steps in order: {id, action, params, export_vars?}",
			ItemSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":          map[string]any{"type": "string", "description": "Step ID, used by $variables in later steps"},
					"action":      map[string]any{"type": "string", "description": "Name of the write tool to run, for example create_node"},
					"params":      map[string]any{"type": "object", "description": "Arguments of the tool"},
					"export_vars": map[string]any{"type": "object", "description": "Variable name to result field"},
				},
				"required": []string{"action"},
			}},
	},
	Validate: func(_ []string, params map[string]any) string {
		steps, _ := params["steps"].([]any)
		if len(steps) == 0 {
			return "steps must not be empty"
		}
		for i, raw := range steps {
			step, _ := raw.(map[string]any)
			action, _ := step["action"].(string)
			if strings.TrimSpace(action) == "" {
				return fmt.Sprintf("steps[%d].action is required", i)
			}
		}
		return ""
	},
}

// pipelineVariable matches a step argument that names an earlier step's
// result. The plugin puts the value in at run time, so it cannot be checked
// here. It is VARIABLE_REFERENCE in plugin/src/batch-pipeline.ts, and
// TestPipelineVariable_MatchesThePlugin keeps the two equal. The plugin reads
// a string that starts with $$ as an escaped literal, and this pattern cannot
// match such a string.
var pipelineVariable = regexp.MustCompile(`^\$[A-Za-z_][A-Za-z0-9_]*$`)

func hasPipelineVariable(v any) bool {
	switch t := v.(type) {
	case string:
		return pipelineVariable.MatchString(t)
	case []any:
		for _, item := range t {
			if hasPipelineVariable(item) {
				return true
			}
		}
	case map[string]any:
		for _, item := range t {
			if hasPipelineVariable(item) {
				return true
			}
		}
	}
	return false
}

// validatePipelineSteps checks each step as the call it becomes. Without it, a
// step reaches the plugin with arguments that nothing has checked. It also
// replaces params["steps"] with the steps the plugin gets: each step keeps only
// the arguments the same direct call would send. params must be the caller's
// own copy, as normalizeArgs makes it.
func validatePipelineSteps(params map[string]any) string {
	steps, _ := params["steps"].([]any)
	sent := make([]any, len(steps))
	for i, raw := range steps {
		step, _ := raw.(map[string]any)
		action, _ := step["action"].(string)
		spec, ok := specRegistry[action]
		// The plugin runs steps through its write handlers only. A tool that
		// reads, or that does its work in Go, cannot be a step.
		if !ok || spec.ReadOnly || spec.Custom != nil || action == pipelineTool ||
			slices.Contains(readOnlyModeExports, action) {
			return fmt.Sprintf("steps[%d].action %q is not a write tool. Use a tool that changes the file, for example create_node or set_paint", i, action)
		}
		stepParams, _ := step["params"].(map[string]any)
		kept, msg := validateStep(spec, stepParams)
		if msg != "" {
			return fmt.Sprintf("steps[%d] (%s): %s", i, action, msg)
		}
		out := maps.Clone(step)
		out["params"] = kept
		sent[i] = out
	}
	params["steps"] = sent
	return ""
}

// validateStep checks one step's arguments against the spec of its tool. It
// reads them as specArgs reads a direct call: only declared arguments count,
// and a null or empty one counts as absent. So a step is refused only where
// the same direct call is refused. An argument that holds a $variable is left
// out of the check. The rules that span several arguments are skipped when any
// argument is left out, because they cannot be decided without it.
//
// It returns the arguments to send: the checked ones, the $variables as
// written, and the node ids.
func validateStep(spec toolSpec, stepParams map[string]any) (map[string]any, string) {
	deferred := false
	kept := map[string]any{}

	var nodeIDs []string
	if spec.NodeIDs != nodeIDsNone {
		// A step carries its node ids inside params, as nodeIds or nodeId.
		// The plugin moves them to the request's nodeIds field.
		for _, k := range []string{"nodeIds", "nodeId"} {
			if v, ok := stepParams[k]; ok {
				kept[k] = v
			}
		}
		ids, hasIDs := stepParams["nodeIds"]
		if !hasIDs {
			if id, ok := stepParams["nodeId"]; ok {
				ids, hasIDs = []any{id}, true
			}
		}
		if hasPipelineVariable(ids) {
			deferred = true
			spec.NodeIDs = nodeIDsNone
		} else if hasIDs {
			list, ok := ids.([]any)
			if !ok {
				return nil, "nodeIds must be an array"
			}
			nodeIDs = toStringSlice(list)
		}
	}

	params := map[string]any{}
	checked := make([]paramSpec, 0, len(spec.Params))
	for _, p := range spec.Params {
		v, given := stepParams[p.wireName()]
		if hasPipelineVariable(v) {
			deferred = true
			kept[p.wireName()] = v
			continue
		}
		checked = append(checked, p)
		if !given {
			continue
		}
		if v == nil {
			if p.Nullable {
				params[p.wireName()] = nil
			}
			continue
		}
		if keepArg(p, v) {
			params[p.wireName()] = v
		}
	}
	spec.Params = checked
	if deferred {
		spec.Validate = nil
	}
	if msg := validateSpec(spec, nodeIDs, params); msg != "" {
		return nil, msg
	}
	maps.Copy(kept, params)
	return kept, ""
}
