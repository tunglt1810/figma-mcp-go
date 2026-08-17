package internal

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/pdfcpu/pdfcpu/pkg/api"
)

var exportFormats = []string{"PNG", "SVG", "JPG", "PDF"}

var getScreenshotSpec = toolSpec{
	Name:       "get_screenshot",
	Desc:       "Export a screenshot of one or more nodes as base64-encoded image data (held in memory). Use save_screenshots instead when you want to write images directly to disk without base64 in the response.",
	NodeIDs:    nodeIDsMulti,
	NodeIDDesc: "Optional node IDs to export, colon format. If empty, exports current selection.",
	Params: []paramSpec{
		{Name: "format", Kind: kindString, Enum: exportFormats,
			Desc: "Export format: PNG (default), SVG, JPG, or PDF"},
		{Name: "scale", Kind: kindNumber, Positive: true,
			Desc: "Export scale for raster formats (default 2)"},
	},
}

var exportFramesToPDFSpec = toolSpec{
	Name:       "export_frames_to_pdf",
	Desc:       "Export multiple frames as a single multi-page PDF file. Each frame becomes one page in order. Ideal for pitch decks, proposals, and slide exports.",
	NodeIDs:    nodeIDsMulti,
	NodeIDsReq: true,
	NodeIDDesc: "Ordered list of frame node IDs to export as PDF pages, colon format e.g. '4029:12345'",
	Params: []paramSpec{
		{Name: "outputPath", Kind: kindString, Required: true,
			Desc: "File path to write the PDF to, must end in .pdf (relative to working directory or absolute)"},
	},
}

var saveScreenshotsSpec = toolSpec{
	Name: "save_screenshots",
	Desc: "Export screenshots for multiple nodes and write them to the local filesystem. Returns file metadata (path, size, dimensions) — no base64 in the response. Use get_screenshot instead when you need the image data in memory.",
	Params: []paramSpec{
		{Name: "items", Kind: kindObjectArray, Required: true,
			Desc: "List of {nodeId, outputPath, format?, scale?} objects",
			ItemSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"nodeId":     map[string]any{"type": "string", "description": "Node ID in colon format e.g. '4029:12345'"},
					"outputPath": map[string]any{"type": "string", "description": "File path to write the image to"},
					"format":     map[string]any{"type": "string", "description": "Export format: PNG, SVG, JPG, or PDF"},
					"scale":      map[string]any{"type": "number", "description": "Export scale for raster formats"},
				},
				"required": []string{"nodeId", "outputPath"},
			}},
		{Name: "format", Kind: kindString, Enum: exportFormats,
			Desc: "Default export format: PNG (default), SVG, JPG, or PDF"},
		{Name: "scale", Kind: kindNumber, Positive: true,
			Desc: "Default export scale for raster formats (default 2)"},
	},
	Validate: func(_ []string, params map[string]interface{}) string {
		items, _ := params["items"].([]interface{})
		if len(items) == 0 {
			return "items must be a non-empty array"
		}
		for i, item := range items {
			m, _ := item.(map[string]interface{})
			if nodeID, _ := m["nodeId"].(string); !ValidNodeID(nodeID) {
				return fmt.Sprintf("items[%d].nodeId must use colon format e.g. 4029:12345", i)
			}
			if outputPath, _ := m["outputPath"].(string); outputPath == "" {
				return fmt.Sprintf("items[%d].outputPath is required", i)
			}
		}
		return ""
	},
}

// exportSpecs are validated from the table like every other tool, but their
// handlers write files rather than simply forwarding to the plugin.
var exportSpecs = []toolSpec{getScreenshotSpec, exportFramesToPDFSpec, saveScreenshotsSpec}

func registerReadExportTools(s *server.MCPServer, node *Node) {
	registerSpecs(s, node, []toolSpec{getScreenshotSpec})

	registerCustom(s, exportFramesToPDFSpec, func(ctx context.Context, nodeIDs []string, params map[string]interface{}) (*mcp.CallToolResult, error) {
		outputPath, _ := params["outputPath"].(string)
		return executeExportFramesToPDF(ctx, node, nodeIDs, outputPath)
	})

	registerCustom(s, saveScreenshotsSpec, func(ctx context.Context, _ []string, params map[string]interface{}) (*mcp.CallToolResult, error) {
		return executeSaveScreenshots(ctx, node, params)
	})
}

func executeExportFramesToPDF(ctx context.Context, node *Node, nodeIDs []string, outputPath string) (*mcp.CallToolResult, error) {
	workDir, err := os.Getwd()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("getwd: %v", err)), nil
	}
	resolvedPath, err := resolveOutputPath(outputPath, workDir)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if strings.ToLower(filepath.Ext(resolvedPath)) != ".pdf" {
		return mcp.NewToolResultError("outputPath must have a .pdf extension"), nil
	}

	resp, err := node.Send(ctx, "export_frames_to_pdf", nodeIDs, nil)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if resp.Error != "" {
		return mcp.NewToolResultError(resp.Error), nil
	}

	pages, err := extractFramePDFs(resp.Data)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	merged, err := mergePDFPages(pages)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("merge PDFs: %v", err)), nil
	}

	if err := os.MkdirAll(filepath.Dir(resolvedPath), 0o755); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("mkdir: %v", err)), nil
	}
	if _, statErr := os.Stat(resolvedPath); statErr == nil {
		return mcp.NewToolResultError(fmt.Sprintf("file already exists: %s", resolvedPath)), nil
	}
	if err := os.WriteFile(resolvedPath, merged, 0o644); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("write file: %v", err)), nil
	}

	out, _ := json.Marshal(map[string]interface{}{
		"outputPath":   resolvedPath,
		"bytesWritten": len(merged),
		"pageCount":    len(pages),
		"success":      true,
	})
	return mcp.NewToolResultText(string(out)), nil
}

// extractFramePDFs parses the plugin response `{frames:[{base64:...},...]}` and
// returns raw PDF bytes for each frame.
func extractFramePDFs(data interface{}) ([][]byte, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Frames []struct {
			Base64 string `json:"base64"`
		} `json:"frames"`
	}
	if err := json.Unmarshal(b, &wrapper); err != nil {
		return nil, err
	}
	if len(wrapper.Frames) == 0 {
		return nil, errors.New("no PDF frames returned by plugin")
	}
	pages := make([][]byte, 0, len(wrapper.Frames))
	for i, f := range wrapper.Frames {
		if f.Base64 == "" {
			return nil, fmt.Errorf("frame %d has empty base64", i)
		}
		raw, err := base64.StdEncoding.DecodeString(f.Base64)
		if err != nil {
			return nil, fmt.Errorf("frame %d: base64 decode: %w", i, err)
		}
		pages = append(pages, raw)
	}
	return pages, nil
}

// mergePDFPages merges one or more single-page PDFs into one multi-page PDF
// using pdfcpu. Each element of pages must be a valid PDF byte slice.
func mergePDFPages(pages [][]byte) ([]byte, error) {
	if len(pages) == 0 {
		return nil, errors.New("no pages to merge")
	}
	readers := make([]io.ReadSeeker, len(pages))
	for i, p := range pages {
		readers[i] = bytes.NewReader(p)
	}
	var buf bytes.Buffer
	if err := api.MergeRaw(readers, &buf, false, nil); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
