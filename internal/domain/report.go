package domain

import "time"

// Report is the full outcome of a single prune run. It is what the engine
// returns to callers (library users get it directly) and what an Alert is
// built from.
type Report struct {
	// Host is the machine name, included so a Telegram alert says where the
	// disk is filling up.
	Host string
	// Path is the monitored mount point.
	Path string
	// Threshold is the used-percent trigger that was configured.
	Threshold float64
	// InitialUsage is the disk state before any cleanup.
	InitialUsage DiskUsage
	// FinalUsage is the disk state after the run (equal to InitialUsage when
	// nothing was triggered).
	FinalUsage DiskUsage
	// Steps holds one StepResult per configured step that was considered.
	Steps []StepResult
	// Triggered is true when InitialUsage was at/above Threshold, i.e. cleanup
	// was attempted.
	Triggered bool
	// StartedAt / FinishedAt bracket the whole run.
	StartedAt  time.Time
	FinishedAt time.Time
	// Err is a fatal error that stopped the run (e.g. the disk could not be
	// probed at all). Per-step command failures live on the individual
	// StepResult, not here.
	Err error
}

// Resolved reports whether the disk ended up below the threshold. A run that
// was never triggered is considered resolved.
func (r Report) Resolved() bool {
	return !r.FinalUsage.OverThreshold(r.Threshold)
}

// NeedsAlert reports whether this run warrants notifying a human: the run
// failed outright, a cleanup command failed, or cleanup ran but could not get
// the disk below the threshold.
func (r Report) NeedsAlert() bool {
	if r.Err != nil {
		return true
	}
	if len(r.FailedSteps()) > 0 {
		return true
	}
	return r.Triggered && !r.Resolved()
}

// FailedSteps returns the steps whose cleanup command errored.
func (r Report) FailedSteps() []StepResult {
	out := make([]StepResult, 0)
	for _, s := range r.Steps {
		if s.Err != nil {
			out = append(out, s)
		}
	}
	return out
}

// TotalFreedBytes sums the space freed across every executed step.
func (r Report) TotalFreedBytes() int64 {
	var total int64
	for _, s := range r.Steps {
		total += s.FreedBytes
	}
	return total
}

// ExecutedSteps returns only the steps that actually ran a command.
func (r Report) ExecutedSteps() []StepResult {
	out := make([]StepResult, 0, len(r.Steps))
	for _, s := range r.Steps {
		if s.Ran() {
			out = append(out, s)
		}
	}
	return out
}

// Duration returns how long the run took.
func (r Report) Duration() time.Duration {
	return r.FinishedAt.Sub(r.StartedAt)
}
