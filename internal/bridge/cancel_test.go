package bridge

import (
	"context"
	"testing"
	"time"
)

// A caller that walks away leaves the plugin running a scan for an answer
// nobody will read, on the one connection the next request also needs. The
// bridge tells it to stop.
func TestSend_TellsThePluginWhenTheCallerCancels(t *testing.T) {
	b, clientConn := setupBridgeWithClient(t)

	frames := make(chan Response, 4)
	go func() {
		for {
			var frame Response
			if err := readJSON(context.Background(), clientConn, &frame); err != nil {
				return
			}
			frames <- frame
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		b.Send(ctx, "get_document", nil, nil) //nolint:errcheck
	}()

	// Wait for the request itself, so the cancel cannot race ahead of it.
	var requestID string
	select {
	case frame := <-frames:
		requestID = frame.RequestID
	case <-time.After(2 * time.Second):
		t.Fatal("the request never reached the plugin")
	}
	if requestID == "" {
		t.Fatal("the request carried no id")
	}

	cancel()
	<-done

	select {
	case frame := <-frames:
		if frame.Type != "cancel_request" {
			t.Fatalf("frame type = %q, want cancel_request", frame.Type)
		}
		if frame.RequestID != requestID {
			t.Errorf("cancel names %q, want %q", frame.RequestID, requestID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no cancel frame arrived")
	}
}

// The same applies when the request runs out of budget rather than being
// cancelled by its caller.
func TestSend_TellsThePluginWhenTheRequestTimesOut(t *testing.T) {
	b, clientConn := setupBridgeWithClient(t)
	b.toolTimeout = func(string) time.Duration { return 80 * time.Millisecond }

	frames := make(chan Response, 4)
	go func() {
		for {
			var frame Response
			if err := readJSON(context.Background(), clientConn, &frame); err != nil {
				return
			}
			frames <- frame
		}
	}()

	go b.Send(context.Background(), "get_document", nil, nil) //nolint:errcheck

	var requestID string
	select {
	case frame := <-frames:
		requestID = frame.RequestID
	case <-time.After(2 * time.Second):
		t.Fatal("the request never reached the plugin")
	}

	select {
	case frame := <-frames:
		if frame.Type != "cancel_request" {
			t.Fatalf("frame type = %q, want cancel_request", frame.Type)
		}
		if frame.RequestID != requestID {
			t.Errorf("cancel names %q, want %q", frame.RequestID, requestID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no cancel frame arrived after the timeout")
	}
}

// A cancel with no plugin on the other end must not panic or block.
func TestCancelRequest_NoConnection(t *testing.T) {
	b := NewBridge("0.1.1")
	b.cancelRequest("nobody")
}
