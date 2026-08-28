package main

import (
	"context"
	"flag"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/mark3labs/mcp-go/server"

	figmamcpgo "github.com/tunglt1810/figma-mcp-go"
	"github.com/tunglt1810/figma-mcp-go/internal/cluster"
	"github.com/tunglt1810/figma-mcp-go/internal/prompts"
	"github.com/tunglt1810/figma-mcp-go/internal/tools"
)

// logLevelFor maps FIGMA_MCP_LOG to a level. Anything unrecognised is info — a
// typo in an environment variable should not silence the server.
func logLevelFor(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// setupLogging installs the default logger. Stderr, because stdout carries the
// MCP protocol and has to stay clean.
func setupLogging() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevelFor(os.Getenv("FIGMA_MCP_LOG")),
	})))
}

func main() {
	setupLogging()

	version := figmamcpgo.GetVersion()
	ip := flag.String("ip", "127.0.0.1", "IP address to listen on (use 0.0.0.0 to accept remote connections)")
	port := flag.Int("port", 1994, "port to listen on")
	flag.Parse()

	parsedIP := net.ParseIP(*ip)
	if parsedIP == nil {
		slog.Error("invalid IP address", "ip", *ip)
		os.Exit(1)
	}
	if !parsedIP.IsLoopback() {
		slog.Warn("binding outside loopback — the server will be reachable from the network with no authentication", "ip", *ip)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	node := cluster.NewNode(*ip, *port, version, tools.Check)
	election := cluster.NewElection(*ip, *port, node)

	if err := election.Start(ctx); err != nil {
		slog.Error("election start", "err", err)
		os.Exit(1)
	}

	slog.Info("starting figma-mcp-go", "version", version, "role", node.RoleName())

	s := server.NewMCPServer("figma-mcp-go", version)
	tools.RegisterTools(s, node)
	prompts.RegisterAll(s)

	go func() {
		<-ctx.Done()
		slog.Info("shutting down")
		election.Stop()
		node.Stop()
	}()

	if err := server.ServeStdio(s); err != nil {
		slog.Error("mcp serve", "err", err)
		os.Exit(1)
	}
}
