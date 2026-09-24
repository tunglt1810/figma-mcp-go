package tools

import (
	"context"
	"encoding/base64"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterTools registers every table-declared tool on the server.
func RegisterTools(s *server.MCPServer, sender Sender) {
	for _, spec := range allSpecs() {
		s.AddTool(buildTool(spec), handlerFor(sender, spec))
	}
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// makeHandler creates a simple tool handler with no parameters.
func makeHandler(sender Sender, command string, nodeIDs []string, params map[string]any) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		resp, err := sender.Send(ctx, command, nodeIDs, params)
		return renderResponse(resp, err)
	}
}

// renderResponse converts a sender's answer into an MCP tool result.
func renderResponse(data any, err error) (*mcp.CallToolResult, error) {
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	// Deterministic keeps map keys sorted. encoding/json v1 sorted them for
	// free; v2 does not, and plugin data is mostly maps, so without this the
	// same tool call would come back with its keys shuffled every time.
	text, err := json.Marshal(data, json.Deterministic(true))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshal response: %v", err)), nil
	}
	return mcp.NewToolResultText(string(text)), nil
}

// toStringSlice converts []any to []string.
func toStringSlice(raw []any) []string {
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// ── export_screenshots ───────────────────────────────────────────────────────
//
// get_screenshot and save_screenshots exported the same nodes through the same
// plugin call and differed only in where the picture went. That is one argument,
// not two tools: an item with an outputPath is written to disk, one without
// comes back as base64, and both can be in the same call.

type exportItem struct {
	NodeID string `json:"nodeId"`
	// A pointer, because absent and empty must be distinguishable: absent means
	// "answer in memory", empty is a path the caller got wrong.
	OutputPath *string `json:"outputPath,omitempty"`
	Format     string  `json:"format,omitempty"`
	Scale      float64 `json:"scale,omitzero"`
}

type exportResult struct {
	Index        int     `json:"index"`
	NodeID       string  `json:"nodeId"`
	NodeName     string  `json:"nodeName,omitempty"`
	OutputPath   string  `json:"outputPath,omitempty"`
	Base64       string  `json:"base64,omitempty"`
	Format       string  `json:"format,omitempty"`
	Width        float64 `json:"width,omitzero"`
	Height       float64 `json:"height,omitzero"`
	BytesWritten int     `json:"bytesWritten,omitzero"`
	// ContentIndex points at the content block carrying this item's picture:
	// an image block for PNG and JPG, a text block of markup for SVG. Zero
	// means none, because block zero is always this summary.
	ContentIndex int    `json:"contentIndex,omitzero"`
	Success      bool   `json:"success"`
	Error        string `json:"error,omitempty"`
}

// inMemoryScale is the default scale of a picture handed back in the response
// rather than written to disk. A model looking at a design gains little from
// retina pixels and pays for every one of them, so it gets 1x unless it asks;
// a file keeps the plugin's 2x default, since that is usually an asset.
const inMemoryScale = 1

func executeExportScreenshots(ctx context.Context, sender Sender, params map[string]any) (*mcp.CallToolResult, error) {
	rawItems, _ := params["items"].([]any)
	defaultFormat, _ := params["format"].(string)
	defaultScale, _ := params["scale"].(float64)

	workDir, err := os.Getwd()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("getwd: %v", err)), nil
	}

	var results []exportResult
	if len(rawItems) == 0 {
		// No items: the current selection, in memory. This is what
		// get_screenshot with no node ids has always done.
		results, err = exportSelection(ctx, sender, defaultFormat, defaultScale)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
	} else {
		results = make([]exportResult, 0, len(rawItems))
		for i, rawItem := range rawItems {
			item, err := parseExportItem(rawItem)
			if err != nil {
				results = append(results, exportResult{Index: i, Error: err.Error()})
				continue
			}
			results = append(results, exportScreenshotItem(ctx, sender, item, i, workDir, defaultFormat, defaultScale))
		}
	}

	succeeded, failed := 0, 0
	for _, r := range results {
		if r.Success {
			succeeded++
		} else {
			failed++
		}
	}

	blocks := inlineExports(results)

	out, err := json.Marshal(map[string]any{
		"total":     len(results),
		"succeeded": succeeded,
		"failed":    failed,
		"hasErrors": failed > 0,
		"results":   results,
	}, json.Deterministic(true))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshal results: %v", err)), nil
	}
	return &mcp.CallToolResult{Content: append([]mcp.Content{mcp.NewTextContent(string(out))}, blocks...)}, nil
}

// inlineExports moves in-memory pictures out of the JSON summary and into
// content blocks of their own. Base64 inside a JSON string is the most
// expensive way to hand a model an image: it is read as text, token by token,
// where an image block is read as a picture at a fraction of the cost. SVG is
// markup already, so it goes back as plain text rather than as base64 of it.
// PDF has no content type a client can show, so it stays base64.
func inlineExports(results []exportResult) []mcp.Content {
	var blocks []mcp.Content
	for i := range results {
		r := &results[i]
		if r.Base64 == "" {
			continue
		}
		var block mcp.Content
		switch strings.ToUpper(r.Format) {
		case "PNG":
			block = mcp.NewImageContent(r.Base64, "image/png")
		case "JPG":
			block = mcp.NewImageContent(r.Base64, "image/jpeg")
		case "SVG":
			markup, err := base64.StdEncoding.DecodeString(r.Base64)
			if err != nil {
				continue // leave it as base64 rather than lose it
			}
			block = mcp.NewTextContent(string(markup))
		default:
			continue
		}
		blocks = append(blocks, block)
		r.Base64 = ""
		r.ContentIndex = len(blocks) // block 0 is the summary
	}
	return blocks
}

// exportSelection captures whatever the user has selected, in memory. The
// plugin decides how many nodes that is, so this is the one path that cannot be
// driven per item.
func exportSelection(ctx context.Context, sender Sender, format string, scale float64) ([]exportResult, error) {
	if format == "" {
		format = "PNG"
	}
	if scale <= 0 {
		scale = inMemoryScale
	}
	params := map[string]any{"format": format, "scale": scale}
	data, err := sender.Send(ctx, "get_screenshot", nil, params)
	if err != nil {
		return nil, err
	}
	exports, err := extractScreenshotExports(data)
	if err != nil {
		return nil, err
	}
	results := make([]exportResult, 0, len(exports))
	for i, e := range exports {
		results = append(results, exportResult{
			Index: i, NodeID: e.NodeID, NodeName: e.NodeName, Base64: e.Base64,
			Format: format, Width: e.Width, Height: e.Height, Success: true,
		})
	}
	return results, nil
}

func exportScreenshotItem(ctx context.Context, sender Sender, item exportItem, index int, workDir, defaultFormat string, defaultScale float64) exportResult {
	toDisk := item.OutputPath != nil
	var resolvedPath string
	if toDisk {
		var err error
		resolvedPath, err = resolveOutputPath(*item.OutputPath, workDir)
		if err != nil {
			return exportResult{Index: index, NodeID: item.NodeID, OutputPath: *item.OutputPath, Error: err.Error()}
		}
	}

	format := coalesce(item.Format, defaultFormat)
	// Only a path can imply a format, so this stays inside the disk branch.
	inferredFormat := inferFormat(resolvedPath)
	if format == "" {
		format = inferredFormat
	}
	if format == "" {
		format = "PNG"
	}
	if inferredFormat != "" && format != inferredFormat {
		return exportResult{Index: index, NodeID: item.NodeID, OutputPath: resolvedPath,
			Error: fmt.Sprintf("format %s conflicts with file extension %s", format, inferredFormat)}
	}

	scale := item.Scale
	if scale <= 0 {
		scale = defaultScale
	}
	if scale <= 0 && !toDisk {
		scale = inMemoryScale
	}

	params := map[string]any{"format": format}
	if scale > 0 {
		params["scale"] = scale
	}

	fail := func(err error) exportResult {
		return exportResult{Index: index, NodeID: item.NodeID, OutputPath: resolvedPath, Error: err.Error()}
	}

	data, err := sender.Send(ctx, "get_screenshot", []string{item.NodeID}, params)
	if err != nil {
		return fail(err)
	}
	export, err := extractScreenshotExport(data)
	if err != nil {
		return fail(err)
	}

	result := exportResult{
		Index:    index,
		NodeID:   export.NodeID,
		NodeName: export.NodeName,
		Format:   format,
		Width:    export.Width,
		Height:   export.Height,
		Success:  true,
	}
	if !toDisk {
		result.Base64 = export.Base64
		return result
	}

	bytes, err := writeBase64(export.Base64, resolvedPath)
	if err != nil {
		return fail(err)
	}
	result.OutputPath = resolvedPath
	result.BytesWritten = bytes
	return result
}

type screenshotExport struct {
	NodeID   string  `json:"nodeId"`
	NodeName string  `json:"nodeName"`
	Base64   string  `json:"base64"`
	Width    float64 `json:"width"`
	Height   float64 `json:"height"`
}

func extractScreenshotExports(data any) ([]screenshotExport, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Exports []screenshotExport `json:"exports"`
	}
	if err := json.Unmarshal(b, &wrapper); err != nil {
		return nil, err
	}
	if len(wrapper.Exports) == 0 {
		return nil, errors.New("no screenshot export returned by plugin")
	}
	return wrapper.Exports, nil
}

func extractScreenshotExport(data any) (screenshotExport, error) {
	exports, err := extractScreenshotExports(data)
	if err != nil {
		return screenshotExport{}, err
	}
	return exports[0], nil
}

func writeBase64(b64, outputPath string) (int, error) {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return 0, fmt.Errorf("base64 decode: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return 0, fmt.Errorf("mkdir: %w", err)
	}
	// Overwrite: re-capturing a node to the same path is the normal loop, and
	// refusing meant the user had to delete the file by hand between captures.
	// resolveOutputPath has already confined the path to the working directory.
	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return 0, err
	}
	return len(data), nil
}

func resolveOutputPath(outputPath, workDir string) (string, error) {
	if filepath.IsAbs(outputPath) {
		return mustBeInsideDir(filepath.Clean(outputPath), workDir)
	}
	return mustBeInsideDir(filepath.Join(workDir, outputPath), workDir)
}

func mustBeInsideDir(resolved, workDir string) (string, error) {
	rel, err := filepath.Rel(workDir, resolved)
	if err != nil {
		return "", fmt.Errorf("outputPath must be inside the working directory: %s", workDir)
	}
	// Convert to forward slashes before prefix check so Windows paths like
	// "C:\.." don't bypass the ".." detection.
	if strings.HasPrefix(filepath.ToSlash(rel), "..") {
		return "", fmt.Errorf("outputPath must be inside the working directory: %s", workDir)
	}
	return resolved, nil
}

func inferFormat(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "PNG"
	case ".svg":
		return "SVG"
	case ".jpg", ".jpeg":
		return "JPG"
	case ".pdf":
		return "PDF"
	}
	return ""
}

func parseExportItem(raw any) (exportItem, error) {
	b, err := json.Marshal(raw)
	if err != nil {
		return exportItem{}, err
	}
	var item exportItem
	if err := json.Unmarshal(b, &item); err != nil {
		return exportItem{}, err
	}
	return item, nil
}

func coalesce(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
