package bridge

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		name   string
		plugin string
		server string
		want   VersionSkew
	}{
		{"identical", "0.3.0", "0.3.0", SkewNone},
		{"patch drift is expected, not skew", "0.3.0", "0.3.9", SkewNone},
		{"patch drift the other way", "0.3.9", "0.3.0", SkewNone},
		{"plugin a minor behind", "0.3.0", "0.4.0", SkewPluginOld},
		{"server a minor behind", "0.4.0", "0.3.0", SkewServerOld},
		{"plugin a major behind", "0.9.0", "1.0.0", SkewPluginOld},
		{"server a major behind", "2.0.0", "1.9.0", SkewServerOld},
		{"the major outranks the minor", "1.9.0", "2.0.0", SkewPluginOld},
		{"a v prefix parses", "v0.3.0", "0.3.0", SkewNone},
		{"a prerelease suffix parses", "0.4.0-beta.1", "0.3.0", SkewServerOld},
		{"a dev plugin stays silent", "dev", "0.3.0", SkewUnknown},
		{"a dev server stays silent", "0.3.0", "dev", SkewUnknown},
		{"an absent version stays silent", "", "0.3.0", SkewUnknown},
		{"a major-only version is not enough", "1", "1.0.0", SkewUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CompareVersions(tc.plugin, tc.server); got != tc.want {
				t.Errorf("CompareVersions(%q, %q) = %v, want %v", tc.plugin, tc.server, got, tc.want)
			}
		})
	}
}

func TestVersionSkewMessage(t *testing.T) {
	t.Run("silent when the versions line up", func(t *testing.T) {
		if msg := VersionSkewMessage("0.3.0", "0.3.7"); msg != "" {
			t.Errorf("want no message, got %q", msg)
		}
	})

	t.Run("silent when a version cannot be read", func(t *testing.T) {
		if msg := VersionSkewMessage("dev", "0.3.0"); msg != "" {
			t.Errorf("want no message, got %q", msg)
		}
	})

	t.Run("points at the plugin when it is behind", func(t *testing.T) {
		msg := VersionSkewMessage("0.3.0", "0.4.0")
		if !strings.Contains(msg, "re-import the plugin") {
			t.Errorf("want the plugin remedy, got %q", msg)
		}
	})

	t.Run("points at the server when it is behind", func(t *testing.T) {
		msg := VersionSkewMessage("0.4.0", "0.3.0")
		if !strings.Contains(msg, "@latest") {
			t.Errorf("want the server remedy, got %q", msg)
		}
	})
}

// The plugin announces itself on connect with a frame that carries no request
// id. Without its own branch in readLoop it would fall through to the
// "empty requestId" path and be dropped, so pin that the bridge records it.
func TestReadLoop_RecordsThePluginVersion(t *testing.T) {
	b, clientConn := setupBridgeWithClient(t)

	if got := b.PluginVersion(); got != "" {
		t.Fatalf("PluginVersion before any announcement = %q, want empty", got)
	}

	err := writeJSON(context.Background(), clientConn, Response{
		Type:    "plugin-info",
		Version: "0.4.2",
	})
	if err != nil {
		t.Fatalf("write plugin-info: %v", err)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if b.PluginVersion() == "0.4.2" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("PluginVersion = %q, want %q", b.PluginVersion(), "0.4.2")
}

// A plugin-info frame must not resolve or disturb an in-flight request.
func TestReadLoop_PluginInfoDoesNotDisturbAPendingRequest(t *testing.T) {
	b, clientConn := setupBridgeWithClient(t)
	ctx := context.Background()

	go func() {
		var req Request
		if err := readJSON(ctx, clientConn, &req); err != nil {
			return
		}
		// Announce first, then answer. The announcement must be skipped over.
		writeJSON(ctx, clientConn, Response{Type: "plugin-info", Version: "0.3.0"}) //nolint:errcheck
		writeJSON(ctx, clientConn, Response{                                        //nolint:errcheck
			RequestID: req.RequestID,
			Type:      req.Type,
			Data:      map[string]any{"id": "1:1"},
		})
	}()

	got, err := b.Send(ctx, "get_node", []string{"1:1"}, nil)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got.Data == nil {
		t.Error("expected the real response to still arrive")
	}
}

func TestCheckPluginSupports(t *testing.T) {
	t.Run("a plugin that announced nothing is not second-guessed", func(t *testing.T) {
		// An older plugin sends no handler list. Refusing its every call would
		// break a setup that works today.
		b := NewBridge("0.3.0")
		b.setPluginInfo("0.2.0", nil)
		if msg := b.checkPluginSupports("boolean_operation"); msg != "" {
			t.Errorf("want no refusal, got %q", msg)
		}
	})

	t.Run("nothing is refused before any plugin connects", func(t *testing.T) {
		b := NewBridge("0.3.0")
		if msg := b.checkPluginSupports("get_node"); msg != "" {
			t.Errorf("want no refusal, got %q", msg)
		}
	})

	t.Run("a tool the plugin announced is allowed", func(t *testing.T) {
		b := NewBridge("0.3.0")
		b.setPluginInfo("0.3.0", []string{"get_node", "set_text"})
		if msg := b.checkPluginSupports("get_node"); msg != "" {
			t.Errorf("want no refusal, got %q", msg)
		}
	})

	t.Run("a tool it did not announce is refused with a remedy", func(t *testing.T) {
		b := NewBridge("0.4.0")
		b.setPluginInfo("0.3.0", []string{"get_node"})
		msg := b.checkPluginSupports("boolean_operation")
		if msg == "" {
			t.Fatal("want a refusal")
		}
		if !strings.Contains(msg, "boolean_operation") {
			t.Errorf("the refusal should name the tool, got %q", msg)
		}
		if !strings.Contains(msg, "v0.3.0") {
			t.Errorf("the refusal should name the plugin version, got %q", msg)
		}
		if !strings.Contains(msg, "re-import") {
			t.Errorf("the refusal should say what to do, got %q", msg)
		}
	})

	t.Run("a reconnecting plugin replaces the old capabilities", func(t *testing.T) {
		b := NewBridge("0.4.0")
		b.setPluginInfo("0.3.0", []string{"get_node"})
		b.setPluginInfo("0.4.0", []string{"get_node", "boolean_operation"})
		if msg := b.checkPluginSupports("boolean_operation"); msg != "" {
			t.Errorf("want no refusal after the upgrade, got %q", msg)
		}
	})

	t.Run("a plugin that stops announcing stops being second-guessed", func(t *testing.T) {
		b := NewBridge("0.4.0")
		b.setPluginInfo("0.4.0", []string{"get_node"})
		b.setPluginInfo("0.2.0", nil)
		if msg := b.checkPluginSupports("boolean_operation"); msg != "" {
			t.Errorf("want no refusal, got %q", msg)
		}
	})
}

// The refusal must happen before a request id is spent or anything is written.
func TestSend_RefusesAToolThePluginDoesNotHave(t *testing.T) {
	b, clientConn := setupBridgeWithClient(t)
	b.setPluginInfo("0.3.0", []string{"get_node"})

	frames := make(chan Response, 2)
	go func() {
		var frame Response
		if err := readJSON(context.Background(), clientConn, &frame); err == nil {
			frames <- frame
		}
	}()

	_, err := b.Send(context.Background(), "boolean_operation", nil, nil)
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "boolean_operation") {
		t.Errorf("error should name the tool, got %v", err)
	}

	select {
	case frame := <-frames:
		t.Fatalf("nothing should have been written, got %+v", frame)
	case <-time.After(200 * time.Millisecond):
	}
}
