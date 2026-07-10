// Package service contains the prune use case: the pure orchestration that
// decides whether the disk is too full, runs cleanup steps in order, stops as
// soon as the disk drops below the threshold, and raises an alert when it
// cannot. It depends only on the domain package and on injected ports, which
// keeps the decision logic fully unit-testable with fakes.
package service

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/GeorgeTyupin/prunejuice/domain"
)

// Params configures an Engine. The three ports (Prober, Runner, Notifier) are
// injected so production wiring and tests share the same code path.
type Params struct {
	// Path is the mount point to watch, e.g. "/".
	Path string
	// Threshold is the used-percent trigger (0..100).
	Threshold float64
	// Steps are the cleanup commands to try, in order.
	Steps []domain.CleanupStep
	// Host labels alerts with the machine name.
	Host string

	Prober   domain.DiskProber
	Runner   domain.Runner
	Notifier domain.Notifier // optional; nil means "never notify" (library default)
	Logger   *slog.Logger    // optional; nil discards logs

	// Now is injected for deterministic timestamps in tests. nil ⇒ time.Now.
	Now func() time.Time
}

// Engine runs the prune use case.
type Engine struct {
	path      string
	threshold float64
	steps     []domain.CleanupStep
	host      string

	prober   domain.DiskProber
	runner   domain.Runner
	notifier domain.Notifier
	log      *slog.Logger
	now      func() time.Time
}

// New builds an Engine from Params, filling in safe defaults.
func New(p Params) *Engine {
	log := p.Logger
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	now := p.Now
	if now == nil {
		now = time.Now
	}
	return &Engine{
		path:      p.Path,
		threshold: p.Threshold,
		steps:     p.Steps,
		host:      p.Host,
		prober:    p.Prober,
		runner:    p.Runner,
		notifier:  p.Notifier,
		log:       log,
		now:       now,
	}
}

// Run performs one full check-and-clean cycle and returns a Report describing
// everything that happened. The returned error mirrors Report.Err and is
// non-nil only for fatal failures (e.g. the disk could not be probed at all);
// per-step command failures are recorded on the report, not returned.
//
// Run always attempts to notify when the resulting report warrants it, so a
// caller can simply ignore the error and rely on the alert side effect.
func (e *Engine) Run(ctx context.Context) (domain.Report, error) {
	started := e.now()
	report := domain.Report{
		Host:      e.host,
		Path:      e.path,
		Threshold: e.threshold,
		StartedAt: started,
	}

	usage, err := e.prober.Usage(ctx, e.path)
	if err != nil {
		report.Err = err
		report.FinishedAt = e.now()
		e.log.Error("disk probe failed", "path", e.path, "err", err)
		e.notify(ctx, e.buildAlert(&report))
		return report, err
	}

	report.InitialUsage = usage
	report.FinalUsage = usage
	report.Triggered = usage.OverThreshold(e.threshold)

	e.log.Info("disk checked",
		"path", e.path,
		"used_percent", usage.UsedPercent,
		"threshold", e.threshold,
		"triggered", report.Triggered)

	if !report.Triggered {
		report.FinishedAt = e.now()
		return report, nil
	}

	current := usage
	for _, step := range e.steps {
		// Stop as soon as we are back under the threshold; record the
		// remaining steps as skipped so the report is complete.
		if !current.OverThreshold(e.threshold) {
			report.Steps = append(report.Steps, domain.StepResult{
				Step:        step,
				Skipped:     true,
				SkipReason:  "threshold already met",
				UsageBefore: current,
				UsageAfter:  current,
			})
			continue
		}

		result := e.runStep(ctx, step, current)
		report.Steps = append(report.Steps, result)
		if result.Ran() {
			current = result.UsageAfter
		}
	}

	report.FinalUsage = current
	report.FinishedAt = e.now()

	e.log.Info("prune finished",
		"path", e.path,
		"final_used_percent", current.UsedPercent,
		"resolved", report.Resolved(),
		"total_freed_mb", float64(report.TotalFreedBytes())/(1024*1024))

	if report.NeedsAlert() {
		e.notify(ctx, e.buildAlert(&report))
	}

	return report, nil
}

// runStep evaluates a single step against the current disk state.
func (e *Engine) runStep(ctx context.Context, step domain.CleanupStep, before domain.DiskUsage) domain.StepResult {
	res := domain.StepResult{Step: step, UsageBefore: before, UsageAfter: before}

	if skip, reason := shouldSkip(step, e.runner); skip {
		res.Skipped = true
		res.SkipReason = reason
		e.log.Info("cleanup step skipped", "step", step.Name, "reason", reason)
		return res
	}

	e.log.Info("cleanup step start",
		"step", step.Name, "command", step.Command,
		"before_used_percent", before.UsedPercent)

	start := e.now()
	runResult, runErr := e.runner.Run(ctx, step.Command, step.Args...)
	res.Duration = e.now().Sub(start)
	res.Output = combinedOutput(runResult)
	res.Err = runErr

	after, probeErr := e.prober.Usage(ctx, e.path)
	if probeErr != nil {
		// We ran the command but can no longer measure. Keep the command's
		// error if it already failed, otherwise surface the probe failure.
		if res.Err == nil {
			res.Err = probeErr
		}
		e.log.Error("post-step probe failed", "step", step.Name, "err", probeErr)
		return res
	}

	res.UsageAfter = after
	res.FreedBytes = int64(before.UsedBytes) - int64(after.UsedBytes)

	if runErr != nil {
		e.log.Error("cleanup step failed",
			"step", step.Name, "err", runErr, "freed_mb", res.FreedMB())
	} else {
		e.log.Info("cleanup step done",
			"step", step.Name,
			"freed_mb", res.FreedMB(),
			"after_used_percent", after.UsedPercent)
	}
	return res
}

// notify sends an alert if a notifier is configured, logging any delivery
// failure rather than propagating it — a broken Telegram must not crash the
// utility whose whole job is to keep the box healthy.
func (e *Engine) notify(ctx context.Context, alert domain.Alert) {
	if e.notifier == nil {
		return
	}
	if err := e.notifier.Notify(ctx, alert); err != nil {
		e.log.Error("alert delivery failed", "err", err)
	}
}

// buildAlert picks the alert level for a finished report.
func (e *Engine) buildAlert(r *domain.Report) domain.Alert {
	now := e.now()
	if r.Err != nil || len(r.FailedSteps()) > 0 {
		return domain.NewErrorAlert(e.host, r.Err, r, now)
	}
	return domain.NewDiskFullAlert(r, now)
}

// shouldSkip is the pure per-step gate: it decides whether a step runs, and
// why not if it does not. Kept free of side effects so it is trivially
// testable.
func shouldSkip(step domain.CleanupStep, runner domain.Runner) (bool, string) {
	if !step.Enabled {
		return true, "disabled in config"
	}
	if step.RequiresBinary != "" && runner != nil && !runner.Lookup(step.RequiresBinary) {
		return true, "required binary not found: " + step.RequiresBinary
	}
	return false, ""
}

// combinedOutput trims and joins stdout+stderr for the report body.
func combinedOutput(r domain.RunResult) string {
	parts := make([]string, 0, 2)
	if s := strings.TrimSpace(r.Stdout); s != "" {
		parts = append(parts, s)
	}
	if s := strings.TrimSpace(r.Stderr); s != "" {
		parts = append(parts, s)
	}
	return strings.Join(parts, "\n")
}
