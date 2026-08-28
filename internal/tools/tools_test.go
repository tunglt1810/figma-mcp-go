package tools

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// ── renderResponse ────────────────────────────────────────────────────────────

// A transport failure and a plugin-reported failure arrive as the same thing —
// the sender turns the second into an error at the cluster boundary — so there
// is one branch here rather than two that did the same work.
func TestRenderResponse_Error(t *testing.T) {
	for _, msg := range []string{"connection failed", "node not found"} {
		result, err := renderResponse(nil, fmt.Errorf("%s", msg))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Errorf("expected IsError=true for %q", msg)
		}
	}
}

func TestRenderResponse_Success(t *testing.T) {
	result, err := renderResponse(map[string]any{"id": "1:1"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Error("expected IsError=false for successful response")
	}
	if len(result.Content) == 0 {
		t.Error("expected non-empty content")
	}
}

func TestRenderResponse_NilData(t *testing.T) {
	result, err := renderResponse(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Error("expected IsError=false for nil data")
	}
}

// ── toStringSlice ─────────────────────────────────────────────────────────────

func TestToStringSlice(t *testing.T) {
	raw := []any{"a", "b", 42, nil, "c"}
	got := toStringSlice(raw)
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i, s := range want {
		if got[i] != s {
			t.Errorf("[%d] = %q, want %q", i, got[i], s)
		}
	}
}

func TestToStringSlice_Empty(t *testing.T) {
	if got := toStringSlice(nil); len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

// ── coalesce ─────────────────────────────────────────────────────────────────

func TestCoalesce(t *testing.T) {
	if got := coalesce("a", "b"); got != "a" {
		t.Errorf("coalesce(a,b) = %q, want a", got)
	}
	if got := coalesce("", "b"); got != "b" {
		t.Errorf("coalesce('',b) = %q, want b", got)
	}
	if got := coalesce("", ""); got != "" {
		t.Errorf("coalesce('','') = %q, want empty", got)
	}
}

// ── inferFormat ───────────────────────────────────────────────────────────────

func TestInferFormat(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"image.png", "PNG"},
		{"image.PNG", "PNG"},
		{"icon.svg", "SVG"},
		{"photo.jpg", "JPG"},
		{"photo.jpeg", "JPG"},
		{"doc.pdf", "PDF"},
		{"noext", ""},
		{"file.txt", ""},
		{"file.gif", ""},
	}
	for _, c := range cases {
		if got := inferFormat(c.path); got != c.want {
			t.Errorf("inferFormat(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

// ── resolveOutputPath ─────────────────────────────────────────────────────────

func TestResolveOutputPath_Relative(t *testing.T) {
	dir := t.TempDir()
	got, err := resolveOutputPath("output/img.png", dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(dir, "output", "img.png")
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestResolveOutputPath_AbsoluteInsideDir(t *testing.T) {
	dir := t.TempDir()
	abs := filepath.Join(dir, "sub", "img.png")
	got, err := resolveOutputPath(abs, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != abs {
		t.Errorf("got %s, want %s", got, abs)
	}
}

func TestResolveOutputPath_Traversal_Blocked(t *testing.T) {
	dir := t.TempDir()
	_, err := resolveOutputPath("../outside.png", dir)
	if err == nil {
		t.Error("expected error for path traversal")
	}
}

func TestResolveOutputPath_AbsoluteOutsideDir_Blocked(t *testing.T) {
	dir := t.TempDir()
	_, err := resolveOutputPath("/etc/passwd", dir)
	if err == nil {
		t.Error("expected error for absolute path outside working dir")
	}
}

// ── mustBeInsideDir ───────────────────────────────────────────────────────────

func TestMustBeInsideDir_Allowed(t *testing.T) {
	dir := t.TempDir()
	inner := filepath.Join(dir, "a", "b", "c.txt")
	got, err := mustBeInsideDir(inner, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != inner {
		t.Errorf("got %s, want %s", got, inner)
	}
}

func TestMustBeInsideDir_Blocked(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Dir(dir)
	_, err := mustBeInsideDir(filepath.Join(parent, "evil.txt"), dir)
	if err == nil {
		t.Error("expected error for path outside directory")
	}
}

func TestMustBeInsideDir_SameDir(t *testing.T) {
	dir := t.TempDir()
	_, err := mustBeInsideDir(filepath.Join(dir, "file.txt"), dir)
	if err != nil {
		t.Fatalf("file directly in workDir should be allowed: %v", err)
	}
}

// ── parseSaveItem ─────────────────────────────────────────────────────────────

func TestParseSaveItem_Valid(t *testing.T) {
	raw := map[string]any{
		"nodeId":     "1:1",
		"outputPath": "out/img.png",
		"format":     "PNG",
		"scale":      float64(2),
	}
	item, err := parseSaveItem(raw)
	if err != nil {
		t.Fatalf("parseSaveItem: %v", err)
	}
	if item.NodeID != "1:1" {
		t.Errorf("NodeID = %q, want 1:1", item.NodeID)
	}
	if item.OutputPath != "out/img.png" {
		t.Errorf("OutputPath = %q, want out/img.png", item.OutputPath)
	}
	if item.Scale != 2 {
		t.Errorf("Scale = %v, want 2", item.Scale)
	}
}

func TestParseSaveItem_UnmarshalError(t *testing.T) {
	// A channel cannot be marshaled to JSON.
	_, err := parseSaveItem(make(chan int))
	if err == nil {
		t.Error("expected marshal error for non-JSON-serialisable value")
	}
}

// ── extractScreenshotExport ───────────────────────────────────────────────────

func TestExtractScreenshotExport_Valid(t *testing.T) {
	data := map[string]any{
		"exports": []any{
			map[string]any{
				"nodeId":   "1:1",
				"nodeName": "Frame",
				"base64":   "abc123",
				"width":    float64(100),
				"height":   float64(200),
			},
		},
	}
	export, err := extractScreenshotExport(data)
	if err != nil {
		t.Fatalf("extractScreenshotExport: %v", err)
	}
	if export.NodeID != "1:1" {
		t.Errorf("NodeID = %q, want 1:1", export.NodeID)
	}
	if export.Width != 100 || export.Height != 200 {
		t.Errorf("dimensions = %vx%v, want 100x200", export.Width, export.Height)
	}
}

func TestExtractScreenshotExport_EmptyExports(t *testing.T) {
	data := map[string]any{"exports": []any{}}
	_, err := extractScreenshotExport(data)
	if err == nil {
		t.Error("expected error for empty exports array")
	}
}

func TestExtractScreenshotExport_MissingExports(t *testing.T) {
	_, err := extractScreenshotExport(map[string]any{})
	if err == nil {
		t.Error("expected error when exports key is missing")
	}
}

// ── writeBase64 ───────────────────────────────────────────────────────────────

func TestWriteBase64_WritesFile(t *testing.T) {
	dir := t.TempDir()
	data := []byte("hello figma")
	b64 := base64.StdEncoding.EncodeToString(data)

	path := filepath.Join(dir, "out.png")
	n, err := writeBase64(b64, path)
	if err != nil {
		t.Fatalf("writeBase64: %v", err)
	}
	if n != len(data) {
		t.Errorf("bytes written = %d, want %d", n, len(data))
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(data) {
		t.Error("file content does not match original data")
	}
}

func TestWriteBase64_CreatesIntermediateDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c", "out.png")
	b64 := base64.StdEncoding.EncodeToString([]byte("x"))

	if _, err := writeBase64(b64, path); err != nil {
		t.Fatalf("writeBase64 should create dirs: %v", err)
	}
}

// Screenshotting the same node twice is the normal loop — capture, adjust the
// design, capture again. Refusing to overwrite meant the second capture failed
// until the user deleted the file by hand.
func TestWriteBase64_OverwritesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.png")

	if _, err := writeBase64(base64.StdEncoding.EncodeToString([]byte("first")), path); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if _, err := writeBase64(base64.StdEncoding.EncodeToString([]byte("second")), path); err != nil {
		t.Fatalf("second write: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "second" {
		t.Errorf("file contents = %q, want the second write", got)
	}
}

// Overwriting must truncate: a shorter image must not leave the tail of the
// longer one behind it.
func TestWriteBase64_TruncatesOnOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.png")

	if _, err := writeBase64(base64.StdEncoding.EncodeToString([]byte("a much longer payload")), path); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if _, err := writeBase64(base64.StdEncoding.EncodeToString([]byte("short")), path); err != nil {
		t.Fatalf("second write: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "short" {
		t.Errorf("file contents = %q, want %q", got, "short")
	}
}

func TestWriteBase64_InvalidBase64(t *testing.T) {
	dir := t.TempDir()
	_, err := writeBase64("not-valid-base64!!!", filepath.Join(dir, "out.png"))
	if err == nil {
		t.Error("expected error for invalid base64 input")
	}
}
