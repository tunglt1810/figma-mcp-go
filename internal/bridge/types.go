package bridge

// Number and any fields below use omitzero, not omitempty. In encoding/json/v2,
// omitempty drops a value that encodes to an empty JSON string, object or
// array. It no longer drops a zero number, and it does drop an any field that
// holds "" or an empty slice. omitzero drops only the Go zero value, which is
// what the plugin wire format has always meant here.

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
	// Progress fields, sent during long-running commands.
	Progress int    `json:"progress,omitzero"`
	Message  string `json:"message,omitempty"`
	// Version and Handlers say what the plugin is and what it can do. They come
	// on the plugin-info frame the plugin sends when it connects, and are unset
	// on every other frame.

	Version  string   `json:"version,omitempty"`
	Handlers []string `json:"handlers,omitempty"`
}
