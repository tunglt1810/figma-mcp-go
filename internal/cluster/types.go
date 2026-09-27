package cluster

// Data uses omitzero, not omitempty, for the same reason as the plugin wire
// types: in encoding/json/v2, omitempty would drop an any field that holds ""
// or an empty slice. omitzero drops only the Go zero value.

// RPCRequest is the wire format for follower → leader /rpc calls.
type RPCRequest struct {
	Tool    string         `json:"tool"`
	NodeIDs []string       `json:"nodeIds,omitempty"`
	Params  map[string]any `json:"params,omitempty"`
}

// RPCResponse is returned by the leader /rpc endpoint.
type RPCResponse struct {
	Data  any    `json:"data,omitzero"`
	Error string `json:"error,omitempty"`
}

// Role represents the current role of this server process.
type Role int

const (
	RoleUnknown  Role = 0
	RoleLeader   Role = 1
	RoleFollower Role = 2
)
