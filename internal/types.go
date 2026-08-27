package internal

// Numeric and any-typed fields below use omitzero rather than omitempty.
// Under encoding/json/v2 omitempty drops a value that encodes to an empty JSON
// string, object or array — it no longer drops a zero number, and it does drop
// an any field holding "" or an empty slice. omitzero drops exactly the Go zero
// value, which is what the plugin wire format has always meant here.

// BridgeRequest is sent from the Go server to the Figma plugin over WebSocket.
type BridgeRequest struct {
	Type      string         `json:"type"`
	RequestID string         `json:"requestId"`
	NodeIDs   []string       `json:"nodeIds,omitempty"`
	Params    map[string]any `json:"params,omitempty"`
}

// BridgeResponse is received from the Figma plugin over WebSocket.
type BridgeResponse struct {
	Type      string `json:"type"`
	RequestID string `json:"requestId"`
	Text      string `json:"text,omitempty"`
	Data      any    `json:"data,omitzero"`
	Error     string `json:"error,omitempty"`
	// Progress fields — sent mid-operation for long-running commands
	Progress int    `json:"progress,omitzero"`
	Message  string `json:"message,omitempty"`
}

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
