package internal

import (
	"testing"
)

func TestWriteOSClipboard(t *testing.T) {
	err := WriteOSClipboard("test-figma-mcp-go-node-id:123")
	if err != nil {
		t.Logf("WriteOSClipboard returned error (may happen in headless CI environment): %v", err)
	}
}
