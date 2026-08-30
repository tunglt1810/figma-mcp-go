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
	Desc:       "Export multiple frames as a single multi-page PDF file. Each frame becomes one page in order. Ideal for pitch decks, proposals, and slide exports.",
	NodeIDs:    nodeIDsMulti,
	NodeIDsReq: true,
	NodeIDDesc: "Ordered list of frame node IDs to export as PDF pages, colon format e.g. '4029:12345'",
	Params: []paramSpec{
		{Name: "outputPath", Kind: kindString, Required: true,
			Desc: "File path to write the PDF to, must end in .pdf (relative to working directory or absolute)"},
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
	Desc: "Export nodes as images. An item with an outputPath is written to that file and answered with its metadata; " +
		"one without comes back as base64 in the response, and both kinds can be in the same call. " +
		"Omit items entirely to capture the current selection as base64. " +
		"Prefer an outputPath when you only need the file — base64 is a lot of tokens to carry an image you are going to write to disk anyway.",
	Params: []paramSpec{
		{Name: "items", Kind: kindObjectArray,
			Desc: "List of {nodeId, outputPath?, format?, scale?} objects. Omit to export the current selection.",
			ItemSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"nodeId":     map[string]any{"type": "string", "description": "Node ID in colon format e.g. '4029:12345'"},
					"outputPath": map[string]any{"type": "string", "description": "File path to write the image to. Omit to get base64 in the response instead."},
					"format":     map[string]any{"type": "string", "description": "Export format: PNG, SVG, JPG, or PDF"},
					"scale":      map[string]any{"type": "number", "description": "Export scale for raster formats"},
				},
				"required": []string{"nodeId"},
			}},
		{Name: "format", Kind: kindString, Enum: exportFormats,
			Desc: "Default export format: PNG (default), SVG, JPG, or PDF"},
		{Name: "scale", Kind: kindNumber, Positive: true,
			Desc: "Default export scale for raster formats (default 2)"},
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
					return fmt.Sprintf("items[%d].outputPath is empty — omit it to get base64 instead", i)
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
		Desc:       "Read the original bytes of the images placed on nodes, as base64. This is the asset that was imported, not a re-render — use export_screenshots when you want a picture of how a node looks now. One image used on several nodes is returned once. Nodes with no image fill are reported under `skipped` rather than failing the call.",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		NodeIDDesc: "Node IDs carrying image fills, in colon format e.g. ['4029:12345']",
	},
	{
		Name: "set_export_settings",
		Desc: "Set the export presets on nodes — the entries a designer sees under Export in the right-hand panel, and what a Figma export or a handoff pipeline uses. " +
			"This changes the document; it does not export anything. Use export_screenshots to actually produce a file.",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		NodeIDDesc: "Node IDs in colon format e.g. ['4029:12345']",
		Params: []paramSpec{
			{Name: "settings", Kind: kindObjectArray, Required: true,
				Desc: "Export presets, in the order they should appear. An empty array clears the node's presets.",
				ItemSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"format": map[string]any{"type": "string", "enum": exportFormats, "description": "PNG, JPG, SVG, or PDF"},
						"suffix": map[string]any{"type": "string", "description": "Appended to the file name, e.g. '@2x' or '-dark'"},
						"constraint": map[string]any{
							"type":        "object",
							"description": "Raster size: {type: SCALE|WIDTH|HEIGHT, value}. SCALE 2 is @2x; WIDTH 512 fixes the width. Ignored for SVG and PDF.",
							"properties": map[string]any{
								"type":  map[string]any{"type": "string", "enum": []string{"SCALE", "WIDTH", "HEIGHT"}},
								"value": map[string]any{"type": "number"},
							},
						},
						"contentsOnly":      map[string]any{"type": "boolean", "description": "Exclude overlapping content outside the node (default true)"},
						"useAbsoluteBounds": map[string]any{"type": "boolean", "description": "Export the full node bounds even when it is clipped by its parent"},
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
