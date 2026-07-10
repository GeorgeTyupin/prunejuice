package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GeorgeTyupin/prunejuice/internal/domain"
)

// --- fakes -----------------------------------------------------------------

// fakeProber returns scripted usages: one per call, clamping to the last entry
// once exhausted. It can also be told to fail on a specific call.
type fakeProber struct {
	usages []domain.DiskUsage
	calls  int
	failAt int // 1-based call index to fail on; 0 = never
	err    error
}

func (f *fakeProber) Usage(context.Context, string) (domain.DiskUsage, error) {
	f.calls++
	if f.failAt != 0 && f.calls == f.failAt {
		return domain.DiskUsage{}, f.err
	}
	idx := f.calls - 1
	if idx >= len(f.usages) {
		idx = len(f.usages) - 1
	}
	return f.usages[idx], nil
}

type fakeRunner struct {
	ran     []string
	missing map[string]bool  // binaries reported as absent
	failCmd map[string]error // commands that return an error
}

func (f *fakeRunner) Run(_ context.Context, cmd string, _ ...string) (domain.RunResult, error) {
	f.ran = append(f.ran, cmd)
	if err := f.failCmd[cmd]; err != nil {
		return domain.RunResult{Stderr: "boom"}, err
	}
	return domain.RunResult{Stdout: "ok"}, nil
}

func (f *fakeRunner) Lookup(cmd string) bool { return !f.missing[cmd] }

type fakeNotifier struct{ alerts []domain.Alert }

func (f *fakeNotifier) Notify(_ context.Context, a domain.Alert) error {
	f.alerts = append(f.alerts, a)
	return nil
}

// --- helpers ---------------------------------------------------------------

const testTotal = uint64(100 * 1024 * 1024) // 100 MiB

func usageAt(percent float64) domain.DiskUsage {
	used := uint64(percent / 100 * float64(testTotal))
	return domain.DiskUsage{
		Path:        "/",
		TotalBytes:  testTotal,
		UsedBytes:   used,
		FreeBytes:   testTotal - used,
		UsedPercent: percent,
	}
}

func step(name, cmd string, enabled bool) domain.CleanupStep {
	return domain.CleanupStep{Name: name, Command: cmd, Enabled: enabled}
}

func newEngine(t *testing.T, p *fakeProber, r *fakeRunner, n *fakeNotifier, threshold float64, steps ...domain.CleanupStep) *Engine {
	t.Helper()
	if r == nil {
		r = &fakeRunner{}
	}
	return New(Params{
		Path:      "/",
		Threshold: threshold,
		Steps:     steps,
		Host:      "test-host",
		Prober:    p,
		Runner:    r,
		Notifier:  n,
		Now:       func() time.Time { return time.Unix(0, 0) },
	})
}

// --- tests -----------------------------------------------------------------

func TestRun_BelowThreshold_NoCleanup(t *testing.T) {
	prober := &fakeProber{usages: []domain.DiskUsage{usageAt(50)}}
	runner := &fakeRunner{}
	notifier := &fakeNotifier{}
	e := newEngine(t, prober, runner, notifier, 85,
		step("journal", "journalctl", true))

	report, err := e.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Triggered {
		t.Errorf("Triggered = true, want false at 50%% used")
	}
	if len(runner.ran) != 0 {
		t.Errorf("ran %v commands, want none", runner.ran)
	}
	if len(notifier.alerts) != 0 {
		t.Errorf("got %d alerts, want 0", len(notifier.alerts))
	}
}

func TestRun_StopsAfterFirstStepFreesEnough(t *testing.T) {
	// 90% initially, first step drops it to 80% (< 85 threshold).
	prober := &fakeProber{usages: []domain.DiskUsage{usageAt(90), usageAt(80)}}
	runner := &fakeRunner{}
	notifier := &fakeNotifier{}
	e := newEngine(t, prober, runner, notifier, 85,
		step("journal", "journalctl", true),
		step("apt", "apt", true),
		step("docker", "docker", true))

	report, err := e.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := runner.ran, []string{"journalctl"}; len(got) != len(want) || got[0] != want[0] {
		t.Errorf("ran %v, want %v (should stop after first step)", got, want)
	}
	if !report.Resolved() {
		t.Errorf("Resolved = false, want true")
	}
	if len(notifier.alerts) != 0 {
		t.Errorf("got %d alerts, want 0 once resolved", len(notifier.alerts))
	}
	// Remaining two steps should be recorded as skipped with a threshold reason.
	skipped := 0
	for _, s := range report.Steps {
		if s.Skipped {
			skipped++
		}
	}
	if skipped != 2 {
		t.Errorf("skipped %d steps, want 2", skipped)
	}
}

func TestRun_NeverResolves_SendsDiskFullAlert(t *testing.T) {
	prober := &fakeProber{usages: []domain.DiskUsage{usageAt(95)}} // stays at 95
	runner := &fakeRunner{}
	notifier := &fakeNotifier{}
	e := newEngine(t, prober, runner, notifier, 85,
		step("journal", "journalctl", true),
		step("apt", "apt", true))

	report, err := e.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runner.ran) != 2 {
		t.Errorf("ran %v, want both steps", runner.ran)
	}
	if report.Resolved() {
		t.Errorf("Resolved = true, want false")
	}
	if len(notifier.alerts) != 1 {
		t.Fatalf("got %d alerts, want 1", len(notifier.alerts))
	}
	if lvl := notifier.alerts[0].Level; lvl != domain.AlertDiskFull {
		t.Errorf("alert level = %v, want AlertDiskFull", lvl)
	}
}

func TestRun_SkipsDisabledAndMissingBinarySteps(t *testing.T) {
	prober := &fakeProber{usages: []domain.DiskUsage{usageAt(95)}}
	runner := &fakeRunner{missing: map[string]bool{"docker": true}}
	notifier := &fakeNotifier{}
	e := newEngine(t, prober, runner, notifier, 85,
		step("disabled", "journalctl", false),
		domain.CleanupStep{Name: "docker", Command: "docker", Enabled: true, RequiresBinary: "docker"},
		step("apt", "apt", true))

	report, _ := e.Run(context.Background())

	if len(runner.ran) != 1 || runner.ran[0] != "apt" {
		t.Errorf("ran %v, want only [apt]", runner.ran)
	}
	reasons := map[string]string{}
	for _, s := range report.Steps {
		if s.Skipped {
			reasons[s.Step.Name] = s.SkipReason
		}
	}
	if reasons["disabled"] == "" {
		t.Errorf("disabled step was not skipped")
	}
	if reasons["docker"] == "" {
		t.Errorf("docker step with missing binary was not skipped")
	}
}

func TestRun_CommandFailure_AlertsAsError(t *testing.T) {
	// Command fails, yet disk ends up resolved. A failed cleanup command must
	// still raise an alert, at Error level.
	prober := &fakeProber{usages: []domain.DiskUsage{usageAt(90), usageAt(80)}}
	runner := &fakeRunner{failCmd: map[string]error{"journalctl": errors.New("permission denied")}}
	notifier := &fakeNotifier{}
	e := newEngine(t, prober, runner, notifier, 85,
		step("journal", "journalctl", true),
		step("apt", "apt", true))

	report, _ := e.Run(context.Background())

	if len(report.FailedSteps()) != 1 {
		t.Fatalf("FailedSteps = %d, want 1", len(report.FailedSteps()))
	}
	if !report.Resolved() {
		t.Errorf("Resolved = false, want true (disk dropped below threshold)")
	}
	if len(notifier.alerts) != 1 {
		t.Fatalf("got %d alerts, want 1", len(notifier.alerts))
	}
	if lvl := notifier.alerts[0].Level; lvl != domain.AlertError {
		t.Errorf("alert level = %v, want AlertError", lvl)
	}
}

func TestRun_ProbeFailure_ReturnsErrorAndAlerts(t *testing.T) {
	prober := &fakeProber{usages: []domain.DiskUsage{usageAt(90)}, failAt: 1, err: errors.New("no such device")}
	notifier := &fakeNotifier{}
	e := newEngine(t, prober, &fakeRunner{}, notifier, 85,
		step("journal", "journalctl", true))

	report, err := e.Run(context.Background())
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if report.Err == nil {
		t.Errorf("report.Err = nil, want the probe error")
	}
	if len(notifier.alerts) != 1 || notifier.alerts[0].Level != domain.AlertError {
		t.Errorf("expected one Error alert, got %+v", notifier.alerts)
	}
}

func TestShouldSkip(t *testing.T) {
	runner := &fakeRunner{missing: map[string]bool{"docker": true}}
	cases := []struct {
		name     string
		step     domain.CleanupStep
		wantSkip bool
	}{
		{"enabled runnable", domain.CleanupStep{Name: "a", Command: "apt", Enabled: true}, false},
		{"disabled", domain.CleanupStep{Name: "b", Command: "apt", Enabled: false}, true},
		{"missing binary", domain.CleanupStep{Name: "c", Command: "docker", Enabled: true, RequiresBinary: "docker"}, true},
		{"present binary", domain.CleanupStep{Name: "d", Command: "apt", Enabled: true, RequiresBinary: "apt"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			skip, reason := shouldSkip(tc.step, runner)
			if skip != tc.wantSkip {
				t.Errorf("shouldSkip = %v (%q), want %v", skip, reason, tc.wantSkip)
			}
		})
	}
}
