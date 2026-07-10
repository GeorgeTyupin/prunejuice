package domain

import "context"

// DiskProber measures the usage of a mount point. Implemented by the disk
// adapter (gopsutil) and by fakes in tests.
type DiskProber interface {
	Usage(ctx context.Context, path string) (DiskUsage, error)
}

// RunResult is the outcome of executing an external command.
type RunResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Runner executes a cleanup command with the given arguments. Implementations
// are responsible for enforcing a per-command timeout (honouring ctx) so a
// hung command can never wedge the daemon.
type Runner interface {
	Run(ctx context.Context, command string, args ...string) (RunResult, error)
	// Lookup reports whether an executable is available in PATH. It backs
	// CleanupStep.RequiresBinary.
	Lookup(command string) bool
}

// Notifier delivers an Alert to a human channel. The default library build
// ships a no-op notifier; the CLI wires in the Telegram notifier.
type Notifier interface {
	Notify(ctx context.Context, alert Alert) error
}
