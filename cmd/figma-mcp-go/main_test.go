package main

import (
	"log/slog"
	"testing"
)

func TestLogLevelFor(t *testing.T) {
	cases := map[string]slog.Level{
		"":       slog.LevelInfo,
		"info":   slog.LevelInfo,
		"debug":  slog.LevelDebug,
		"DEBUG":  slog.LevelDebug,
		"warn":   slog.LevelWarn,
		"error":  slog.LevelError,
		"gibber": slog.LevelInfo,
	}
	for in, want := range cases {
		if got := logLevelFor(in); got != want {
			t.Errorf("logLevelFor(%q) = %v, want %v", in, got, want)
		}
	}
}
