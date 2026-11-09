// Package llm calls the user's own AI CLI; afterthought never holds credentials.
package llm

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/omerrgoren/afterthought/internal/adapter"
)

type Runner interface {
	Run(ctx context.Context, prompt string, web bool) (string, error)
}

type CLI struct{ Cmd []string }

func (c CLI) Run(ctx context.Context, prompt string, web bool) (string, error) {
	if len(c.Cmd) == 0 {
		return "", fmt.Errorf("no AI CLI configured")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	args := append([]string{}, c.Cmd[1:]...)
	if web && filepath.Base(c.Cmd[0]) == "claude" {
		args = append(args, "--allowedTools", "WebSearch,WebFetch")
	}
	cmd := exec.CommandContext(ctx, c.Cmd[0], args...)
	cmd.Stdin = strings.NewReader(adapter.InternalMarker + "\n" + prompt)
	cmd.Env = append(os.Environ(), "AFTERTHOUGHT_RUNNING=1") // stops our own hook re-firing
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %w: %s", c.Cmd[0], err, strings.TrimSpace(errb.String()))
	}
	return strings.TrimSpace(out.String()), nil
}
