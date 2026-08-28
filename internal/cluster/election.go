package cluster

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"time"
)

func electionLog() *slog.Logger { return slog.Default().With("component", "election") }

// Election determines the initial role and monitors leader health.
// If the leader dies, a follower will attempt a takeover.
type Election struct {
	port     int
	node     *Node
	follower *Follower // reused across health-check ticks to avoid HTTP client pool leaks
	cancel   context.CancelFunc
}

// NewElection creates an Election for the given ip, port, and node.
func NewElection(ip string, port int, node *Node) *Election {
	return &Election{
		port:     port,
		node:     node,
		follower: NewFollower("http://" + ip + ":" + itoa(port)),
	}
}

// Start determines the initial role and launches the background monitor.
func (e *Election) Start(ctx context.Context) error {
	if err := e.determineRole(ctx); err != nil {
		return err
	}

	monitorCtx, cancel := context.WithCancel(ctx)
	e.cancel = cancel
	if e.node.Role() == RoleUnknown {
		go e.retryUntilSettled(monitorCtx)
	}
	go e.monitor(monitorCtx)
	return nil
}

// retryUntilSettled closes the startup race: the port was taken but nothing
// answered, which usually means another process is milliseconds from being
// ready — or is on its way out. Checking often settles the role well inside one
// monitor tick, instead of leaving every tool call with nowhere to go for three
// to five seconds.
func (e *Election) retryUntilSettled(ctx context.Context) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if e.node.Role() != RoleUnknown {
				return
			}
			if err := e.determineRole(ctx); err != nil {
				electionLog().Warn("retry error", "err", err)
			}
		}
	}
}

// Stop cancels the background monitor goroutine.
func (e *Election) Stop() {
	if e.cancel != nil {
		e.cancel()
	}
}

// determineRole tries to become leader; falls back to follower if a
// healthy leader already exists on the port.
func (e *Election) determineRole(ctx context.Context) error {
	if err := e.node.BecomeLeader(); err == nil {
		return nil
	}

	// Port taken — check if there is a healthy leader
	if e.follower.Ping(ctx) {
		e.node.BecomeFollower()
		return nil
	}

	// Port taken but no healthy leader — could be a race during startup.
	// Next monitor tick will retry.
	electionLog().Warn("port taken but the leader is not responding — will retry")
	return nil
}

// monitor runs a periodic check on the current role.
// Followers watch the leader; leaders do nothing.
func (e *Election) monitor(ctx context.Context) {
	for {
		// Jitter: 3–5 seconds
		jitter := time.Duration(3000+rand.Intn(2000)) * time.Millisecond
		select {
		case <-time.After(jitter):
		case <-ctx.Done():
			return
		}

		if err := e.tick(ctx); err != nil {
			electionLog().Warn("tick error", "err", err)
		}
	}
}

func (e *Election) tick(ctx context.Context) error {
	switch e.node.Role() {
	case RoleFollower:
		if !e.follower.Ping(ctx) {
			electionLog().Warn("leader not responding, attempting takeover")
			if err := e.node.BecomeLeader(); err != nil {
				electionLog().Warn("takeover failed", "err", err)
			}
		}
	case RoleUnknown:
		return e.determineRole(ctx)
	case RoleLeader:
		// Nothing — we are the leader
	}
	return nil
}

// itoa converts an int to string without importing strconv everywhere.
func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}
