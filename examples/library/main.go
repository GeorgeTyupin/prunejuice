// Command example demonstrates embedding prunejuice as a library.
//
// It builds a Pruner, wires prunejuice's alerts into the app's own slog
// logger (no Telegram, no network dependency), and runs a single cycle. Run it
// from the repo root with:
//
//	go run ./examples/library
//
// On a machine below the threshold it simply reports "disk healthy".
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/GeorgeTyupin/prunejuice"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// A custom Notifier: route alerts wherever your app already sends signals
	// (here, just the logger). This is all it takes to integrate Slack,
	// PagerDuty, email, etc.
	p, err := prunejuice.New(prunejuice.Config{
		MountPath:        "/",
		ThresholdPercent: 85,
		Steps:            prunejuice.DefaultSteps(),
	},
		prunejuice.WithLogger(logger),
		prunejuice.WithNotifier(alertSink{logger}),
	)
	if err != nil {
		logger.Error("init prunejuice", "err", err)
		os.Exit(1)
	}

	report, err := p.RunOnce(context.Background())
	if err != nil {
		logger.Error("prune run failed", "err", err)
		os.Exit(1)
	}

	switch {
	case !report.Triggered:
		logger.Info("disk healthy",
			"used_percent", report.InitialUsage.UsedPercent,
			"threshold", report.Threshold)
	case report.Resolved():
		logger.Info("disk cleaned up",
			"freed_mb", float64(report.TotalFreedBytes())/(1024*1024),
			"used_percent", report.FinalUsage.UsedPercent)
	default:
		logger.Warn("disk still full after cleanup",
			"used_percent", report.FinalUsage.UsedPercent)
	}
}

// alertSink implements prunejuice.Notifier.
type alertSink struct{ log *slog.Logger }

func (s alertSink) Notify(_ context.Context, alert prunejuice.Alert) error {
	s.log.Warn("ALERT", "level", alert.Level.String(), "body", alert.Text())
	return nil
}
