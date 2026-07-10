package domain

import "time"

// CleanupStep describes one cleanup command to try, in configuration order.
//
// Steps are intentionally data, not code: the list and its order live in the
// config file so operators can enable, disable or reorder them without a
// rebuild. Nothing destructive runs unless the operator marked it Enabled.
type CleanupStep struct {
	// Name is a short human label, e.g. "journal-vacuum".
	Name string
	// Command is the executable to run, e.g. "journalctl".
	Command string
	// Args are the arguments passed to Command.
	Args []string
	// Enabled gates the step. A disabled step is never executed.
	Enabled bool
	// RequiresBinary, when non-empty, names an executable that must exist in
	// PATH for the step to run (e.g. "docker"). If it is missing the step is
	// skipped rather than failed. This is how "only if docker is installed"
	// is expressed without special-casing docker in the engine.
	RequiresBinary string
}

// StepResult captures what happened when a CleanupStep was evaluated.
type StepResult struct {
	// Step is the step this result describes.
	Step CleanupStep
	// Skipped is true when the step did not run (disabled, missing binary,
	// or the disk was already below the threshold before we reached it).
	Skipped bool
	// SkipReason explains a skip in human terms.
	SkipReason string
	// UsageBefore / UsageAfter bracket the command execution. When the step
	// is skipped they are equal.
	UsageBefore DiskUsage
	UsageAfter  DiskUsage
	// FreedBytes is UsageBefore.UsedBytes - UsageAfter.UsedBytes. It is signed
	// because a command can, in pathological cases, use more space than it
	// frees.
	FreedBytes int64
	// Output is the combined stdout/stderr, trimmed, for the alert body.
	Output string
	// Err is set when the command itself failed. A failed step does not abort
	// the whole run: the engine records it and moves on to the next step.
	Err error
	// Duration is how long the command took.
	Duration time.Duration
}

// FreedMB returns the space freed by this step in megabytes.
func (r StepResult) FreedMB() float64 { return float64(r.FreedBytes) / bytesPerMB }

// Ran reports whether the command was actually executed.
func (r StepResult) Ran() bool { return !r.Skipped }
