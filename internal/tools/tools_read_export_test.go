package tools

import (
	"bytes"
	"encoding/base64"
	"encoding/json/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

// makeTestPDF creates a minimal valid single-page PDF in memory using pdfcpu.
func makeTestPDF(t *testing.T) []byte {
	t.Helper()
	const minimalJSON = `{"paper":"A4P","pages":{"1":{"content":{}}}}`
	var buf bytes.Buffer
	if err := api.Create(nil, bytes.NewReader([]byte(minimalJSON)), &buf, nil); err != nil {
		t.Fatalf("makeTestPDF: %v", err)
	}
	return buf.Bytes()
}

// ── extractFramePDFs ──────────────────────────────────────────────────────────

func TestExtractFramePDFs_Valid(t *testing.T) {
	pdf := makeTestPDF(t)
	b64 := base64.StdEncoding.EncodeToString(pdf)

	data := map[string]any{
		"frames": []any{
			map[string]any{"nodeId": "1:1", "nodeName": "Frame 1", "base64": b64},
			map[string]any{"nodeId": "1:2", "nodeName": "Frame 2", "base64": b64},
		},
	}

	pages, err := extractFramePDFs(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(pages))
	}
	for i, p := range pages {
		if !bytes.Equal(p, pdf) {
			t.Errorf("page %d bytes differ from input", i)
		}
	}
}

func TestExtractFramePDFs_EmptyFrames(t *testing.T) {
	data := map[string]any{"frames": []any{}}
	_, err := extractFramePDFs(data)
	if err == nil {
		t.Error("expected error for empty frames array")
	}
}

func TestExtractFramePDFs_MissingFramesKey(t *testing.T) {
	_, err := extractFramePDFs(map[string]any{})
	if err == nil {
		t.Error("expected error when frames key is absent")
	}
}

func TestExtractFramePDFs_EmptyBase64InFrame(t *testing.T) {
	data := map[string]any{
		"frames": []any{
			map[string]any{"nodeId": "1:1", "base64": ""},
		},
	}
	_, err := extractFramePDFs(data)
	if err == nil {
		t.Error("expected error for frame with empty base64")
	}
}

func TestExtractFramePDFs_InvalidBase64(t *testing.T) {
	data := map[string]any{
		"frames": []any{
			map[string]any{"nodeId": "1:1", "base64": "!!!not-valid-base64!!!"},
		},
	}
	_, err := extractFramePDFs(data)
	if err == nil {
		t.Error("expected error for invalid base64")
	}
}

func TestExtractFramePDFs_UnmarshalError(t *testing.T) {
	_, err := extractFramePDFs(make(chan int))
	if err == nil {
		t.Error("expected marshal error for non-JSON-serialisable value")
	}
}

// ── mergePDFPages ─────────────────────────────────────────────────────────────

func TestMergePDFPages_SinglePage(t *testing.T) {
	pdf := makeTestPDF(t)
	merged, err := mergePDFPages([][]byte{pdf})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(merged) == 0 {
		t.Fatal("merged PDF is empty")
	}
	if !bytes.HasPrefix(merged, []byte("%PDF")) {
		t.Error("merged output does not start with %PDF")
	}
}

func TestMergePDFPages_MultiplePages(t *testing.T) {
	pdf := makeTestPDF(t)
	merged, err := mergePDFPages([][]byte{pdf, pdf, pdf})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(merged) == 0 {
		t.Fatal("merged PDF is empty")
	}
	// Validate that pdfcpu considers the output a valid PDF.
	if err := api.Validate(bytes.NewReader(merged), nil); err != nil {
		t.Errorf("merged PDF is not valid: %v", err)
	}
}

func TestMergePDFPages_EmptyInput(t *testing.T) {
	_, err := mergePDFPages(nil)
	if err == nil {
		t.Error("expected error for nil input")
	}
	_, err = mergePDFPages([][]byte{})
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestMergePDFPages_InvalidPDFBytes(t *testing.T) {
	_, err := mergePDFPages([][]byte{[]byte("not a pdf")})
	if err == nil {
		t.Error("expected error for invalid PDF bytes")
	}
}

// ── export_screenshots ────────────────────────────────────────────────────────

// The merge's whole point: where the picture goes is an argument, so one call
// can write some items to disk and hand the rest back as base64.
func TestExportScreenshots_BothDestinationsInOneCall(t *testing.T) {
	s, fake := newTestServer(t)
	fake.data = map[string]any{
		"exports": []any{
			map[string]any{
				"nodeId": "1:1", "nodeName": "Card", "base64": "aGVsbG8=",
				"width": float64(64), "height": float64(32),
			},
		},
	}

	dir := t.TempDir()
	t.Chdir(dir)

	result := callToolResult(t, s, "export_screenshots", map[string]any{
		"items": []any{
			map[string]any{"nodeId": "1:1", "outputPath": "out/card.png"},
			map[string]any{"nodeId": "2:2"},
		},
	})
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Text)
	}

	var answer struct {
		Succeeded int `json:"succeeded"`
		Failed    int `json:"failed"`
		Results   []struct {
			OutputPath   string `json:"outputPath"`
			Base64       string `json:"base64"`
			BytesWritten int    `json:"bytesWritten"`
			ContentIndex int    `json:"contentIndex"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(result.Blocks[0].Text), &answer); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if answer.Failed != 0 || answer.Succeeded != 2 {
		t.Fatalf("succeeded = %d, failed = %d, want 2 and 0: %s", answer.Succeeded, answer.Failed, result.Text)
	}

	written, inMemory := answer.Results[0], answer.Results[1]
	if written.OutputPath == "" || written.BytesWritten != 5 {
		t.Errorf("the item with an outputPath should have been written, got %+v", written)
	}
	// Writing it and also sending the bytes back would double the cost of the
	// only argument that exists to avoid them.
	if written.Base64 != "" {
		t.Error("an item written to disk should not carry base64 as well")
	}
	// The in-memory picture travels as an image block, not as base64 text
	// inside the summary, which a model would have to read token by token.
	if inMemory.Base64 != "" {
		t.Errorf("an in-memory PNG should not be base64 in the summary, got %+v", inMemory)
	}
	if len(result.Blocks) != 2 || inMemory.ContentIndex != 1 {
		t.Fatalf("want the summary plus one image block at index 1, got %d blocks, contentIndex %d", len(result.Blocks), inMemory.ContentIndex)
	}
	if img := result.Blocks[1]; img.Type != "image" || img.Data != "aGVsbG8=" || img.MimeType != "image/png" {
		t.Errorf("want a PNG image block carrying the export, got %+v", img)
	}
	if inMemory.OutputPath != "" {
		t.Errorf("an in-memory item should have no outputPath, got %q", inMemory.OutputPath)
	}

	if _, err := os.Stat(filepath.Join(dir, "out", "card.png")); err != nil {
		t.Errorf("expected the file on disk: %v", err)
	}
}

// SVG is markup, so it comes back as a text block of that markup rather than as
// base64 of it.
func TestExportScreenshots_SVGComesBackAsMarkup(t *testing.T) {
	s, fake := newTestServer(t)
	fake.data = map[string]any{
		"exports": []any{map[string]any{"nodeId": "1:1", "base64": "PHN2Zy8+"}}, // <svg/>
	}

	result := callToolResult(t, s, "export_screenshots", map[string]any{
		"items": []any{map[string]any{"nodeId": "1:1", "format": "SVG"}},
	})
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Text)
	}
	if len(result.Blocks) != 2 || result.Blocks[1].Type != "text" || result.Blocks[1].Text != "<svg/>" {
		t.Fatalf("want a text block with the SVG markup, got %+v", result.Blocks)
	}
	if strings.Contains(result.Blocks[0].Text, "PHN2Zy8+") {
		t.Error("the summary should not repeat the SVG as base64")
	}
}

// A path outside the working directory is refused, and refusing it must not
// silently downgrade the item to a base64 answer.
func TestExportScreenshots_RefusesAPathOutsideTheWorkingDirectory(t *testing.T) {
	s, fake := newTestServer(t)
	fake.data = map[string]any{
		"exports": []any{map[string]any{"nodeId": "1:1", "base64": "aGVsbG8="}},
	}
	t.Chdir(t.TempDir())

	result := callToolResult(t, s, "export_screenshots", map[string]any{
		"items": []any{map[string]any{"nodeId": "1:1", "outputPath": "../escaped.png"}},
	})

	if !strings.Contains(result.Text, "inside the working directory") {
		t.Errorf("want a refusal naming the working directory, got: %s", result.Text)
	}
	if strings.Contains(result.Text, "aGVsbG8=") {
		t.Error("a refused path must not answer with the bytes instead")
	}
}
