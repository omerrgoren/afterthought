// Package notify delivers local notifications; no network, no outside infrastructure.
package notify

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/omerrgoren/afterthought/internal/config"
)

// Desktop shows a desktop notification, or errors where no GUI route exists.
func Desktop(title, msg string) error {
	switch runtime.GOOS {
	case "darwin":
		esc := func(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) }
		return exec.Command("osascript", "-e", fmt.Sprintf(`display notification "%s" with title "%s"`, esc(msg), esc(title))).Run()
	case "linux":
		if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
			return fmt.Errorf("no display")
		}
		return exec.Command("notify-send", title, msg).Run()
	}
	return fmt.Errorf("desktop notifications unsupported on %s", runtime.GOOS)
}

func noticePath() string { return filepath.Join(config.Dir(), "notice.txt") }

// Queue stores a terminal notice to show at the next session start (no-GUI fallback).
func Queue(msg string) error {
	if err := os.MkdirAll(config.Dir(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(noticePath(), []byte(msg), 0o644)
}

// Pending returns and clears the queued notice.
func Pending() string {
	b, err := os.ReadFile(noticePath())
	if err != nil {
		return ""
	}
	_ = os.Remove(noticePath())
	return strings.TrimSpace(string(b))
}

// Send tries a desktop notification and falls back to the queued terminal notice.
func Send(title, msg string) {
	if Desktop(title, msg) != nil {
		_ = Queue(title + ": " + msg)
	}
}
