package bridge

import "time"

// One table sets how long each tool may take. Three places use it: the bridge
// times a request out, the follower's HTTP request must last longer, and a
// progress update resets the bridge's timer. Each place used to keep its own
// number. That is how batch_execute_pipeline ended up with 120s on the leader
// but a hardcoded 35s limit through a follower.

const (
	// defaultToolTimeout covers tools that finish in one plugin round-trip.
	defaultToolTimeout = 30 * time.Second

	// MaxToolTimeout caps a request's total life. Progress updates extend the
	// timer, so without this cap a plugin that keeps reporting progress would
	// hold a request open forever.
	MaxToolTimeout = 10 * time.Minute

	// defaultPingInterval and defaultPingTimeout check that the plugin is
	// still there. Without pings the bridge cannot tell a quiet plugin from a
	// dead one until a tool call times out.
	defaultPingInterval = 20 * time.Second
	defaultPingTimeout  = 10 * time.Second

	// defaultConnectGrace is a little longer than the plugin's reconnect delay
	// (RECONNECT_DELAY_MS = 1500 in plugin/src/ui/App.svelte), so a leader
	// handover does not show up as "plugin not connected".
	defaultConnectGrace = 2 * time.Second

	// defaultCloseGrace is how long a graceful close may take before the socket
	// is dropped. The library allows 5s for the handshake (close.go:199) and
	// 15s more for its goroutines (close.go:231). That is too long to delay
	// exit for a plugin that is already gone.
	defaultCloseGrace = 1 * time.Second

	// keepaliveForgiveness is how many failed pings in a row the keepalive
	// accepts from a plugin that is clearly still sending. Three gives a slow
	// reader three ping rounds, about a minute with the interval above. A write
	// stuck on a full socket buffer, which only the keepalive clears, still has
	// a limit.
	keepaliveForgiveness = 3

	// serverInfoGrace is how long the reply to the plugin's get_server_info
	// waits for the write slot before it is dropped. It only waits behind a
	// send stuck on a full socket buffer. Dropping it is safe: the plugin asks
	// again on its next connect. The limit also stops the goroutine from
	// outliving the connection it answers.
	serverInfoGrace = 30 * time.Second

	// followerGrace makes the follower wait a little past the leader's
	// deadline. The caller then gets the leader's real error, not a transport
	// timeout that says nothing about what went wrong.

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
