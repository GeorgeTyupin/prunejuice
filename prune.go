// Package prunejuice keeps a server's disk from filling up.
//
// It watches a mount point and, when used space crosses a threshold, runs a
// configurable, ordered list of cleanup commands (journal vacuum, apt clean,
// docker prune, ...), stopping as soon as the disk drops back below the
// threshold. If it still cannot free enough — or if the utility itself errors —
// it raises an alert.
//
// # Two ways to use it
//
// As a standalone daemon/one-shot, via the cmd/prunejuice binary, which reads a
// YAML config and (by default, when you enable it) alerts to Telegram.
//
// As a library embedded in your own Go service, where you construct a Pruner
// and call RunOnce on your own schedule. In library mode there is no Telegram
// dependency unless you opt in with WithTelegram; by default alerts go nowhere
// (WithNotifier / WithLogNotifier let you route them into your own systems).
//
//	p, err := prunejuice.New(prunejuice.Config{
//	    MountPath:        "/",
//	    ThresholdPercent: 85,
//	    Steps:            prunejuice.DefaultSteps(),
//	})
//	if err != nil { ... }
//	report, _ := p.RunOnce(ctx)
//	if !report.Resolved() { /* wire your own alerting */ }
package prunejuice

import (
	"context"
	"fmt"
	"time"

	"github.com/GeorgeTyupin/prunejuice/adapter/command"
	"github.com/GeorgeTyupin/prunejuice/adapter/disk"
	"github.com/GeorgeTyupin/prunejuice/adapter/notify"
	"github.com/GeorgeTyupin/prunejuice/domain"
	"github.com/GeorgeTyupin/prunejuice/service"
)

// Re-exported domain types so library users can depend on a single package.
type (
	// Report is the outcome of a prune run.
	Report = domain.Report
	// Alert is a channel-agnostic notification built from a Report.
	Alert = domain.Alert
	// CleanupStep is one ordered cleanup command.
	CleanupStep = domain.CleanupStep
	// Notifier delivers alerts; implement it to route alerts anywhere.
	Notifier = domain.Notifier
	// DiskProber measures disk usage; override it in tests via WithProber.
	DiskProber = domain.DiskProber
	// Runner executes cleanup commands; override it via WithRunner.
	Runner = domain.Runner
)

// Config is the minimal configuration a Pruner needs. The CLI's richer
// config.Config maps onto this.
type Config struct {
	// MountPath is the filesystem to monitor. Empty ⇒ "/".
	MountPath string
	// ThresholdPercent triggers cleanup at/above this used percentage. Empty ⇒ 85.
	ThresholdPercent float64
	// CommandTimeout caps each cleanup command. Empty ⇒ 60s.
	CommandTimeout time.Duration
	// Host labels alerts. Empty is fine (alerts simply omit the host line).
	Host string
	// Steps are the ordered cleanup commands. Empty ⇒ DefaultSteps().
	Steps []CleanupStep
}

// Pruner is a configured, ready-to-run prune engine.
type Pruner struct {
	engine *service.Engine
}

// New builds a Pruner. Adapters default to the real disk prober, an
// exec-based command runner, and a no-op notifier; options override any of
// them. New returns an error only when an option fails to construct (e.g.
// WithTelegram with a bad token).
func New(cfg Config, opts ...Option) (*Pruner, error) {
	cfg = withDefaults(cfg)

	s := &settings{}
	for _, opt := range opts {
		opt(s)
	}

	if s.prober == nil {
		s.prober = disk.New()
	}
	if s.runner == nil {
		s.runner = command.New(cfg.CommandTimeout)
	}

	notifier, err := s.buildNotifier()
	if err != nil {
		return nil, err
	}

	engine := service.New(service.Params{
		Path:      cfg.MountPath,
		Threshold: cfg.ThresholdPercent,
		Steps:     cfg.Steps,
		Host:      cfg.Host,
		Prober:    s.prober,
		Runner:    s.runner,
		Notifier:  notifier,
		Logger:    s.logger,
		Now:       s.now,
	})
	return &Pruner{engine: engine}, nil
}

// RunOnce performs a single check-and-clean cycle and returns its Report.
// This is the entry point for library users who schedule runs themselves.
func (p *Pruner) RunOnce(ctx context.Context) (Report, error) {
	return p.engine.Run(ctx)
}

// Run performs a check every interval until ctx is cancelled, then returns
// ctx.Err(). It runs once immediately on entry. This is a convenience for a
// long-lived daemon; the recommended production deployment uses a systemd
// timer invoking RunOnce instead (see deploy/systemd).
func (p *Pruner) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		return fmt.Errorf("prunejuice: interval must be positive, got %s", interval)
	}

	// Run immediately so we do not wait a full interval before the first check.
	if _, err := p.engine.Run(ctx); err != nil && ctx.Err() != nil {
		return ctx.Err()
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Errors are already reported via alerts and the report; a failed
			// tick must not stop the loop.
			_, _ = p.engine.Run(ctx)
		}
	}
}

// DefaultSteps returns the runbook's cleanup sequence: journal vacuum, apt
// clean, then docker prune. docker-prune is disabled by default because it can
// remove resources owned by other stacks; enable it deliberately.
func DefaultSteps() []CleanupStep {
	return []CleanupStep{
		{Name: "journal-vacuum", Command: "journalctl", Args: []string{"--vacuum-time=7d"}, Enabled: true},
		{Name: "apt-clean", Command: "apt", Args: []string{"clean"}, Enabled: true},
		{Name: "docker-prune", Command: "docker", Args: []string{"system", "prune", "-f"}, Enabled: false, RequiresBinary: "docker"},
	}
}

func withDefaults(cfg Config) Config {
	if cfg.MountPath == "" {
		cfg.MountPath = "/"
	}
	if cfg.ThresholdPercent == 0 {
		cfg.ThresholdPercent = 85
	}
	if cfg.CommandTimeout <= 0 {
		cfg.CommandTimeout = command.DefaultTimeout
	}
	if len(cfg.Steps) == 0 {
		cfg.Steps = DefaultSteps()
	}
	return cfg
}

// buildNotifier collapses configured notifiers into one, defaulting to no-op.
func (s *settings) buildNotifier() (Notifier, error) {
	notifiers := append([]Notifier(nil), s.notifiers...)

	if s.telegram != nil {
		tg, err := s.telegram.build()
		if err != nil {
			return nil, err
		}
		notifiers = append(notifiers, tg)
	}

	switch len(notifiers) {
	case 0:
		return notify.Noop{}, nil
	case 1:
		return notifiers[0], nil
	default:
		return notify.NewMulti(notifiers...), nil
	}
}
