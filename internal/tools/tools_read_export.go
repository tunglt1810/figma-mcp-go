package tools

import (
	"github.com/tunglt1810/figma-mcp-go/internal/figma"

	"bytes"
	"context"
	"encoding/base64"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/pdfcpu/pdfcpu/pkg/api"
)

var exportFormats = []string{"PNG", "SVG", "JPG", "PDF"}

var exportFramesToPDFSpec = toolSpec{
	Name:       "export_frames_to_pdf",
	Desc:       "Export frames as one PDF file, one page per frame, in order.",
	NodeIDs:    nodeIDsMulti,
	NodeIDsReq: true,
	NodeIDDesc: "Frame IDs, in page order",
	Params: []paramSpec{
		{Name: "outputPath", Kind: kindString, Required: true,
			Desc: "Path of the .pdf file, inside the working directory"},
	},
	Custom: func(sender Sender) customHandler {
		return func(ctx context.Context, nodeIDs []string, params map[string]any) (*mcp.CallToolResult, error) {
			outputPath, _ := params["outputPath"].(string)
			return executeExportFramesToPDF(ctx, sender, nodeIDs, outputPath)
		}
	},
}

var exportScreenshotsSpec = toolSpec{
	Name: "export_screenshots",
	Desc: "Export nodes as images. Items with outputPath are saved to file; others are returned " +
		"(PNG/JPG as image, SVG as text, PDF as base64). Omit items to export the selection. Use outputPath if you only need the file.",
	Params: []paramSpec{
		{Name: "items", Kind: kindObjectArray,
			Desc: "{nodeId, outputPath?, format?, scale?}. Omit to export the selection.",
			ItemSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"nodeId":     map[string]any{"type": "string", "description": "Node ID"},
					"outputPath": map[string]any{"type": "string", "description": "File to save to. Omit to return the image."},
					"format":     map[string]any{"type": "string", "description": "PNG, SVG, JPG, or PDF"},
					"scale":      map[string]any{"type": "number", "description": "Scale for PNG/JPG"},
				},
				"required": []string{"nodeId"},
			}},
		{Name: "format", Kind: kindString, Enum: exportFormats,
			Desc: "Default format: PNG (default), SVG, JPG, PDF"},
		{Name: "scale", Kind: kindNumber, Positive: true,
			Desc: "Default scale for PNG/JPG (2 for files, 1 when returned)"},
	},
	Validate: func(_ []string, params map[string]any) string {
		items, hasItems := params["items"]
		if !hasItems {
			return ""
		}
		list, _ := items.([]any)
		if len(list) == 0 {
			return "items must be a non-empty array — omit it entirely to export the current selection"
		}
		for i, item := range list {
			m, _ := item.(map[string]any)
			if nodeID, _ := m["nodeId"].(string); !figma.ValidNodeID(nodeID) {
				return fmt.Sprintf("items[%d].nodeId must use colon format e.g. 4029:12345", i)
			}
			// Absent means "answer in memory"; present and empty is a path the
			// caller got wrong, and silently returning base64 would hide it.
			if raw, present := m["outputPath"]; present {
				if path, _ := raw.(string); path == "" {
					return fmt.Sprintf("items[%d].outputPath is empty — omit it to get the image in the response", i)
				}
			}
			// The per-item format is nested a level below anything a paramSpec
			// enum can reach, and the plugin is the only thing that would have
			// rejected it — after the round trip.
			if format, present := m["format"].(string); present && !containsString(exportFormats, format) {
				return fmt.Sprintf("items[%d].format must be one of %v, got: %s", i, exportFormats, format)
			}
			if scale, present := m["scale"].(float64); present && scale <= 0 {
				return fmt.Sprintf("items[%d].scale must be positive, got: %g", i, scale)
			}
		}
		return ""
	},
	Custom: func(sender Sender) customHandler {
		return func(ctx context.Context, _ []string, params map[string]any) (*mcp.CallToolResult, error) {
			return executeExportScreenshots(ctx, sender, params)
		}
	},
}

// exportSpecs are validated from the table like every other tool, but their
// handlers write files rather than simply forwarding to the plugin.
var exportSpecs = []toolSpec{
	{
		Name:       "get_image_bytes",
		Desc:       "Get the original image files used in nodes' image fills, as base64. For a picture of how a node looks, use export_screenshots. Each image is returned once; nodes without images are listed in `skipped`.",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		NodeIDDesc: "Node IDs carrying image fills",
	},
	{
		Name: "set_export_settings",
		Desc: "Set the Export presets shown in a node's right panel. Does not export a file; use export_screenshots for that.",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		NodeIDDesc: "Node IDs",
		Params: []paramSpec{
			{Name: "settings", Kind: kindObjectArray, Required: true,
				Desc: "Presets in order. [] clears them.",
				ItemSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"format": map[string]any{"type": "string", "enum": exportFormats, "description": "PNG, JPG, SVG, or PDF"},
						"suffix": map[string]any{"type": "string", "description": "File name suffix e.g. '@2x'"},
						"constraint": map[string]any{
							"type":        "object",
							"description": "Size for PNG/JPG: {type: SCALE|WIDTH|HEIGHT, value}. SCALE 2 = @2x.",
							"properties": map[string]any{
								"type":  map[string]any{"type": "string", "enum": []string{"SCALE", "WIDTH", "HEIGHT"}},
								"value": map[string]any{"type": "number"},
							},
						},
						"contentsOnly":      map[string]any{"type": "boolean", "description": "Skip content that overlaps from outside (default true)"},
						"useAbsoluteBounds": map[string]any{"type": "boolean", "description": "Use full bounds even if the parent clips the node"},
					},
					"required": []string{"format"},
				}},
		},
		Validate: func(_ []string, params map[string]any) string {
			settings, ok := params["settings"].([]any)
			if !ok {
				return "settings must be an array of export presets"
			}
			return figma.ValidateExportSettings(settings, exportFormats)
		},
	},
	exportFramesToPDFSpec, exportScreenshotsSpec}

func executeExportFramesToPDF(ctx context.Context, sender Sender, nodeIDs []string, outputPath string) (*mcp.CallToolResult, error) {
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

	data, err := sender.Send(ctx, "export_frames_to_pdf", nodeIDs, nil)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	pages, err := extractFramePDFs(data)
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
	// Overwrite, as export_screenshots does: re-exporting after a design change is
	// the normal loop. The path is already confined to the working directory.
	_, statErr := os.Stat(resolvedPath)
	replaced := statErr == nil
	if err := os.WriteFile(resolvedPath, merged, 0o644); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("write file: %v", err)), nil
	}

	out, _ := json.Marshal(map[string]any{
		"outputPath":   resolvedPath,
		"bytesWritten": len(merged),
		"pageCount":    len(pages),
		"replaced":     replaced,
		"success":      true,
	}, json.Deterministic(true))
	return mcp.NewToolResultText(string(out)), nil
}

// extractFramePDFs parses the plugin response `{frames:[{base64:...},...]}` and
// returns raw PDF bytes for each frame.
func extractFramePDFs(data any) ([][]byte, error) {
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
