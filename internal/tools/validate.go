package tools

// ValidateRPC validates an incoming RPC request against the tool's expected
// input shape. Returns an error string on failure, empty string if valid.
// Every tool declares its rules in the spec table; this is the lookup.
func ValidateRPC(tool string, nodeIDs []string, params map[string]any) string {
	spec, ok := specRegistry[tool]
	if !ok {
		return ""
	}
	return validateSpec(spec, nodeIDs, params)
}
