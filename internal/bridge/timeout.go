package bridge

import "time"

// One table for how long a tool may take. The bridge times a request out, the
// follower's HTTP request has to outlast it, and a progress update resets the
// bridge's timer — three places that used to carry their own number, which is
// how batch_execute_pipeline came to have a 120s budget on the leader and a
// hardcoded 35s ceiling through a follower.

const (
	// defaultToolTimeout covers tools that finish in one plugin round-trip.
	defaultToolTimeout = 30 * time.Second

	// MaxToolTimeout caps a request's total life. Progress updates extend the
	// timer, so without this a plugin reporting progress forever would hold a
	// request open forever.
	MaxToolTimeout = 10 * time.Minute

	// defaultPingInterval / defaultPingTimeout keep the plugin connection
	// honest. Without a ping the bridge cannot tell a quiet plugin from a dead
	// one until a tool call times out.
	defaultPingInterval = 20 * time.Second
	defaultPingTimeout  = 10 * time.Second

	// defaultConnectGrace covers the plugin's own reconnect delay
	// (RECONNECT_DELAY_MS = 1500 in plugin/src/ui/App.svelte) with a little
	// room, so a leader handover does not surface as "plugin not connected".
	defaultConnectGrace = 2 * time.Second

	// defaultCloseGrace is how long a graceful close may take before the socket
	// is simply dropped. The library allows 5s for the handshake (close.go:199)
	// and 15s more for its goroutines (close.go:231), which is a long time to
	// hold up exit for a plugin that is already gone.
	defaultCloseGrace = 1 * time.Second

	// keepaliveForgiveness is how many consecutive failed pings the keepalive
	// tolerates from a plugin that is demonstrably still sending. Three, so a
	// slow drain gets room across three ping rounds — about a minute with the
	// interval above — while a write parked on a full socket buffer, which only
	// the keepalive clears, is still bounded.
	keepaliveForgiveness = 3

	// serverInfoGrace is how long the reply to the plugin's get_server_info
	// waits for the write slot before being abandoned. It only ever waits at
	// all behind a send parked on a full socket buffer, and abandoning is the
	// safe outcome: the plugin asks again on its next connect, and the wait is
	// bounded so the goroutine cannot outlive the connection it answers.
	serverInfoGrace = 30 * time.Second

	// followerGrace keeps the follower waiting a little past the leader's
	// deadline, so the caller gets the leader's real error rather than a
	// transport timeout that says nothing about what went wrong.
	followerGrace = 5 * time.Second
)

// toolTimeouts holds the tools that need more than the default.
var toolTimeouts = map[string]time.Duration{
	"get_document":           60 * time.Second,
	"batch_execute_pipeline": 120 * time.Second,
}

// TimeoutFor is how long the bridge waits for a tool's response.
func TimeoutFor(tool string) time.Duration {
	if d, ok := toolTimeouts[tool]; ok {
		return d
	}
	return defaultToolTimeout
}

// FollowerTimeoutFor is how long a follower waits for the leader to answer.
func FollowerTimeoutFor(tool string) time.Duration {
	return TimeoutFor(tool) + followerGrace
}
