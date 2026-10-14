package bridge

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const clipboardTimeout = 2 * time.Second

var (
	clipboardMu  sync.Mutex
	clipboardSeq atomic.Uint64
)

// writeLatestClipboard writes text unless a newer copy request arrived first.
// Each copy runs in its own goroutine, so without this an older copy could
// finish last and leave stale text on the clipboard.
func writeLatestClipboard(text string) (written bool, err error) {
	seq := clipboardSeq.Add(1)
	clipboardMu.Lock()
	defer clipboardMu.Unlock()
	if clipboardSeq.Load() != seq {
		return false, nil
	}
	return true, WriteOSClipboard(text)
}

// WriteOSClipboard writes text to the operating system's native clipboard.
// It runs on the host OS, so browser iframe security rules do not apply.

func WriteOSClipboard(text string) error {
	ctx, cancel := context.WithTimeout(context.Background(), clipboardTimeout)
	defer cancel()

	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "pbcopy")
	case "windows":
		cmd = exec.CommandContext(ctx, "cmd", "/c", "clip")
	case "linux":
		if _, err := exec.LookPath("wl-copy"); err == nil {
			cmd = exec.CommandContext(ctx, "wl-copy")
		} else if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.CommandContext(ctx, "xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.CommandContext(ctx, "xsel", "--clipboard", "--input")
		} else {
			return fmt.Errorf("no clipboard utility found (wl-copy, xclip, xsel)")
		}
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	cmd.Stdin = strings.NewReader(text)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("clipboard write error: %w", err)
	}

	return nil
}
