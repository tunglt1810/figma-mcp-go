package bridge

import (
	"testing"
	"time"
)

func TestTimeoutFor(t *testing.T) {
	if got := TimeoutFor("get_nodes_info"); got != defaultToolTimeout {
		t.Errorf("TimeoutFor(get_nodes_info) = %s, want the default %s", got, defaultToolTimeout)
	}
	for tool, want := range toolTimeouts {
		if got := TimeoutFor(tool); got != want {
			t.Errorf("TimeoutFor(%s) = %s, want %s", tool, got, want)
		}
	}
}

// The follower proxies to the leader over HTTP. If its deadline is shorter than
// the leader's, a slow tool fails at the follower while the leader is still
// working — batch_execute_pipeline gets 120s on the leader and used to get a
// hardcoded 35s through the follower.
func TestFollowerDeadline_OutlastsTheLeader(t *testing.T) {
	for tool := range toolTimeouts {
		leader := TimeoutFor(tool)
		if got := FollowerTimeoutFor(tool); got <= leader {
			t.Errorf("follower deadline for %s = %s, must exceed the leader's %s", tool, got, leader)
		}
	}
	if got := FollowerTimeoutFor("get_nodes_info"); got <= defaultToolTimeout {
		t.Errorf("follower default deadline = %s, must exceed the leader's %s", got, defaultToolTimeout)
	}
}

// A progress update should extend a request's life, never shorten it. Resetting
// to a hardcoded 60s cut a 120s pipeline down to 70s if the plugin reported
// progress at the 10s mark.
func TestProgressExtension_NeverShortensTheTimeout(t *testing.T) {
	for tool := range toolTimeouts {
		if TimeoutFor(tool) < defaultToolTimeout {
			continue
		}
		entry := &pendingEntry{timeout: TimeoutFor(tool), hardDeadline: time.Now().Add(MaxToolTimeout)}
		if got := entry.nextTimeout(); got != TimeoutFor(tool) {
			t.Errorf("%s: progress reset to %s, want %s", tool, got, TimeoutFor(tool))
		}
	}
}

// Progress updates must not keep a request alive forever.
func TestProgressExtension_StopsAtTheHardDeadline(t *testing.T) {
	entry := &pendingEntry{timeout: 30 * time.Second, hardDeadline: time.Now().Add(5 * time.Second)}
	got := entry.nextTimeout()
	if got <= 0 || got > 5*time.Second {
		t.Errorf("next timeout = %s, want it clamped to the remaining 5s", got)
	}

	expired := &pendingEntry{timeout: 30 * time.Second, hardDeadline: time.Now().Add(-time.Second)}
	if got := expired.nextTimeout(); got > 0 {
		t.Errorf("next timeout past the hard deadline = %s, want no extension", got)
	}
}
