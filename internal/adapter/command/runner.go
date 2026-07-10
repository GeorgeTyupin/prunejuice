// Package command adapts os/exec to the domain.Runner port. Every command
// runs under its own timeout so a hung cleanup can never wedge the daemon.
package command

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"

	"github.com/GeorgeTyupin/prunejuice/internal/domain"
)

// DefaultTimeout is used when a non-positive timeout is supplied.
const DefaultTimeout = 60 * time.Second

// Runner executes external commands with a bounded lifetime.
type Runner struct {
	// Timeout caps the wall-clock duration of a single command. When it
	// elapses the process (and its children, best effort) is killed.
	Timeout time.Duration
}

// New returns a Runner. A non-positive timeout falls back to DefaultTimeout.
func New(timeout time.Duration) *Runner {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &Runner{Timeout: timeout}
}

// Run implements domain.Runner. It never blocks longer than Timeout because it
// derives a context.WithTimeout from ctx and hands it to exec.CommandContext,
// which kills the process when the deadline (or a parent cancellation) fires.
func (r *Runner) Run(ctx context.Context, command string, args ...string) (domain.RunResult, error) {
	cctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()

	cmd := exec.CommandContext(cctx, command, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	res := domain.RunResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if cmd.ProcessState != nil {
		res.ExitCode = cmd.ProcessState.ExitCode()
	}

	switch {
	case errors.Is(cctx.Err(), context.DeadlineExceeded):
		return res, fmt.Errorf("command %q timed out after %s", command, r.Timeout)
	case errors.Is(ctx.Err(), context.Canceled):
		return res, fmt.Errorf("command %q cancelled: %w", command, ctx.Err())
	case err != nil:
		return res, fmt.Errorf("command %q failed (exit %d): %w", command, res.ExitCode, err)
	default:
		return res, nil
	}
}

// Lookup implements domain.Runner: it reports whether command exists in PATH.
func (r *Runner) Lookup(command string) bool {
	_, err := exec.LookPath(command)
	return err == nil
}
