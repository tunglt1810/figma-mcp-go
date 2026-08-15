package internal

import (
	"context"
	"encoding/json"
	"strings"
)

// unknownRequestTypePrefix is what the plugin replies with for a request type it
// does not recognise — see the `Unknown request type` throw in
// plugin/src/main.ts. Keep the two in step: this string is the only signal the
// server has that it is talking to an older plugin.
//
// It is worth matching on because the plugin is installed by hand from a release
// zip while the server updates itself through npx, so a server that knows a
// merged command routinely meets a plugin that does not.
const unknownRequestTypePrefix = "Unknown request type"

// legacyCall is one of the single-purpose commands an older plugin understands.
// appliedKey is both the field that command reports back and the key the merged
// response records it under, which is what lets the two paths share a shape.
type legacyCall struct {
	tool       string
	appliedKey string
	params     map[string]interface{}
}

// sendWithFanout issues the merged command and, only when the plugin reports it
// as unknown, replays the equivalent legacy commands.
//
// Transport errors and genuine plugin errors are returned untouched: retrying
// those would repeat work already applied and hide the real message.
func sendWithFanout(
	ctx context.Context,
	s sender,
	modernTool string,
	nodeIDs []string,
	params map[string]interface{},
	legacy []legacyCall,
) (BridgeResponse, error) {
	resp, err := s.Send(ctx, modernTool, nodeIDs, params)
	if err != nil {
		return resp, err
	}
	if !strings.HasPrefix(resp.Error, unknownRequestTypePrefix) {
		return resp, nil
	}
	return fanout(ctx, s, modernTool, nodeIDs, legacy)
}

// fanout runs the legacy commands in order and folds their per-node results
// into the merged command's {results:[{nodeId, applied, errors}]} shape.
func fanout(
	ctx context.Context,
	s sender,
	modernTool string,
	nodeIDs []string,
	calls []legacyCall,
) (BridgeResponse, error) {
	applied := map[string]map[string]interface{}{}
	perProperty := map[string]map[string]string{}
	nodeError := map[string]string{}
	order := make([]string, 0, len(nodeIDs))

	for _, call := range calls {
		resp, err := s.Send(ctx, call.tool, nodeIDs, call.params)
		if err != nil {
			return BridgeResponse{}, err
		}
		if resp.Error != "" {
			return resp, nil
		}

		for _, entry := range legacyResults(resp.Data) {
			id, _ := entry["nodeId"].(string)
			if id == "" {
				continue
			}
			if _, seen := applied[id]; !seen {
				applied[id] = map[string]interface{}{}
				order = append(order, id)
			}

			if msg, _ := entry["error"].(string); msg != "" {
				// "Node not found" is about the node, not the property, so it is
				// reported once rather than repeated for every legacy call.
				if msg == "Node not found" {
					nodeError[id] = msg
					continue
				}
				if perProperty[id] == nil {
					perProperty[id] = map[string]string{}
				}
				perProperty[id][call.appliedKey] = msg
				continue
			}

			if v, ok := entry[call.appliedKey]; ok {
				applied[id][call.appliedKey] = v
			}
		}
	}

	results := make([]map[string]interface{}, 0, len(order))
	for _, id := range order {
		entry := map[string]interface{}{"nodeId": id}
		if msg := nodeError[id]; msg != "" {
			entry["error"] = msg
		} else {
			entry["applied"] = applied[id]
			if len(perProperty[id]) > 0 {
				entry["errors"] = perProperty[id]
			}
		}
		results = append(results, entry)
	}

	return BridgeResponse{
		Type: modernTool,
		Data: map[string]interface{}{"results": results},
	}, nil
}

// legacyResults pulls the results array out of a legacy command's response.
func legacyResults(data interface{}) []map[string]interface{} {
	b, err := json.Marshal(data)
	if err != nil {
		return nil
	}
	var wrapper struct {
		Results []map[string]interface{} `json:"results"`
	}
	if err := json.Unmarshal(b, &wrapper); err != nil {
		return nil
	}
	return wrapper.Results
}
