package tools

// ValidateRPC validates an incoming RPC request against the tool's expected
// input shape. It returns an error string on failure, or "" if valid.
// Every tool declares its rules in the spec table. This function looks them up.
func ValidateRPC(tool string, nodeIDs []string, params map[string]any) string {
	spec, ok := specRegistry[tool]
	if !ok {
		return ""
	}
	return validateSpec(spec, nodeIDs, params)
}
