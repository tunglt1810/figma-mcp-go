package tools

import (
	"fmt"
	"maps"
	"regexp"
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
					"action":      map[string]any{"type": "string", "description": "Write tool to run e.g. create_node"},
					"params":      map[string]any{"type": "object", "description": "The tool's arguments"},
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
// here. It is the same pattern the plugin uses.
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

// validatePipelineSteps checks each step as the call it becomes. A step that
// skipped this was the one way to reach the plugin with arguments nothing had
// checked.
func validatePipelineSteps(params map[string]any) string {
	steps, _ := params["steps"].([]any)
	for i, raw := range steps {
		step, _ := raw.(map[string]any)
		action, _ := step["action"].(string)
		spec, ok := specRegistry[action]
		// The plugin runs steps through its write handlers only. A tool that
		// reads, or that does its work in Go, cannot be a step.
		if !ok || spec.ReadOnly || spec.Custom != nil || action == pipelineTool {
			return fmt.Sprintf("steps[%d].action %q is not a write tool — use a tool name such as create_node or set_paint", i, action)
		}
		stepParams, _ := step["params"].(map[string]any)
		if msg := validateStep(spec, stepParams); msg != "" {
			return fmt.Sprintf("steps[%d] (%s): %s", i, action, msg)
		}
	}
	return ""
}

// validateStep checks one step's arguments against the spec of its tool. An
// argument that holds a $variable is left out of the check. The rules that
// span several arguments are skipped when any argument is left out, because
// they cannot be decided without it.
func validateStep(spec toolSpec, stepParams map[string]any) string {
	params := maps.Clone(stepParams)
	if params == nil {
		params = map[string]any{}
	}
	deferred := false

	var nodeIDs []string
	if spec.NodeIDs != nodeIDsNone {
		// A step carries its node ids inside params, as nodeIds or nodeId.
		// The plugin moves them to the request's nodeIds field.
		ids, hasIDs := params["nodeIds"]
		if !hasIDs {
			if id, ok := params["nodeId"]; ok {
				ids, hasIDs = []any{id}, true
			}
		}
		delete(params, "nodeIds")
		delete(params, "nodeId")
		if hasPipelineVariable(ids) {
			deferred = true
			spec.NodeIDs = nodeIDsNone
		} else if hasIDs {
			list, ok := ids.([]any)
			if !ok {
				return "nodeIds must be an array"
			}
			nodeIDs = toStringSlice(list)
		}
	}

	checked := make([]paramSpec, 0, len(spec.Params))
	for _, p := range spec.Params {
		if hasPipelineVariable(params[p.wireName()]) {
			deferred = true
			continue
		}
		checked = append(checked, p)
	}
	spec.Params = checked
	if deferred {
		spec.Validate = nil
	}
	return validateSpec(spec, nodeIDs, params)
}
