package internal

import "time"

// One table for how long a tool may take. The bridge times a request out, the
// follower's HTTP request has to outlast it, and a progress update resets the
// bridge's timer — three places that used to carry their own number, which is
// how batch_execute_pipeline came to have a 120s budget on the leader and a
// hardcoded 35s ceiling through a follower.

const (
	// defaultToolTimeout covers tools that finish in one plugin round-trip.
	defaultToolTimeout = 30 * time.Second

	// maxToolTimeout caps a request's total life. Progress updates extend the
	// timer, so without this a plugin reporting progress forever would hold a
	// request open forever.
	maxToolTimeout = 10 * time.Minute

	// defaultPingInterval / defaultPingTimeout keep the plugin connection
	// honest. Without a ping the bridge cannot tell a quiet plugin from a dead
	// one until a tool call times out.
	defaultPingInterval = 20 * time.Second
	defaultPingTimeout  = 10 * time.Second

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

// timeoutFor is how long the bridge waits for a tool's response.
func timeoutFor(tool string) time.Duration {
	if d, ok := toolTimeouts[tool]; ok {
		return d
	}
	return defaultToolTimeout
}

// followerTimeoutFor is how long a follower waits for the leader to answer.
func followerTimeoutFor(tool string) time.Duration {
	return timeoutFor(tool) + followerGrace
}
