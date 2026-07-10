// Command prunejuice watches a mount point and frees space when it fills up,
// alerting to Telegram when it cannot. It is designed to be run one-shot from a
// systemd timer (recommended) or as a long-lived daemon (-daemon).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"gopkg.in/yaml.v3"

	"github.com/GeorgeTyupin/prunejuice"
	"github.com/GeorgeTyupin/prunejuice/config"
	"github.com/GeorgeTyupin/prunejuice/logging"
)

// Version is overridden at build time via -ldflags "-X main.Version=...".
var Version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	var (
		cfgPath      = flag.String("config", "config.yaml", "path to the YAML config file")
		daemon       = flag.Bool("daemon", false, "run continuously on check_interval instead of one-shot")
		printConfig  = flag.Bool("print-config", false, "print a commented default config to stdout and exit")
		printVersion = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *printVersion {
		fmt.Println("prunejuice", Version)
		return 0
	}
	if *printConfig {
		if err := printDefaultConfig(); err != nil {
			fmt.Fprintln(os.Stderr, "prunejuice:", err)
			return 1
		}
		return 0
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "prunejuice:", err)
		return 1
	}

	logger, closer, err := logging.Setup(logging.Options{
		Level:      cfg.Log.Level,
		JSON:       cfg.Log.JSON,
		FilePath:   cfg.Log.File,
		MaxSizeMB:  cfg.Log.MaxSizeMB,
		MaxBackups: cfg.Log.MaxBackups,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "prunejuice:", err)
		return 1
	}
	defer func() { _ = closer.Close() }()

	// Assemble options. Telegram is opt-in via config; alerts are always also
	// written to the log so there is a durable local record.
	opts := []prunejuice.Option{
		prunejuice.WithLogger(logger),
		prunejuice.WithLogNotifier(logger),
	}
	if cfg.Telegram.Enabled {
		opts = append(opts, prunejuice.WithTelegram(cfg.Telegram.BotToken, cfg.Telegram.ChatID))
		logger.Info("telegram alerts enabled", "chat_id", cfg.Telegram.ChatID)
	} else {
		logger.Info("telegram alerts disabled (running in log-only mode)")
	}

	pruner, err := prunejuice.New(prunejuice.Config{
		MountPath:        cfg.MountPath,
		ThresholdPercent: cfg.ThresholdPercent,
		CommandTimeout:   cfg.CommandTimeout.Duration,
		Host:             cfg.Host,
		Steps:            cfg.DomainSteps(),
	}, opts...)
	if err != nil {
		logger.Error("failed to initialise pruner", "err", err)
		return 1
	}

	// Cancel the context on SIGINT/SIGTERM. The command runner honours the
	// context, so an in-flight cleanup command is killed promptly and no
	// process is left hanging.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if *daemon {
		logger.Info("starting daemon", "interval", cfg.CheckInterval.Duration, "version", Version)
		if err := pruner.Run(ctx, cfg.CheckInterval.Duration); err != nil && !isShutdown(err) {
			logger.Error("daemon stopped with error", "err", err)
			return 1
		}
		logger.Info("shutdown complete")
		return 0
	}

	report, err := pruner.RunOnce(ctx)
	if err != nil {
		// A fatal error (e.g. disk unreadable) was already alerted by the engine.
		logger.Error("prune run failed", "err", err)
		return 1
	}
	if report.NeedsAlert() {
		logger.Warn("run completed but disk could not be fully cleared or a step failed",
			"final_used_percent", report.FinalUsage.UsedPercent)
	}
	return 0
}

func isShutdown(err error) bool {
	return err == context.Canceled || err == context.DeadlineExceeded
}

func printDefaultConfig() error {
	out, err := yaml.Marshal(config.Default())
	if err != nil {
		return fmt.Errorf("marshal default config: %w", err)
	}
	fmt.Print(configHeader)
	fmt.Print(string(out))
	return nil
}

const configHeader = `# prunejuice configuration.
# Secrets can be supplied via environment variables instead of this file:
#   PRUNEJUICE_TELEGRAM_BOT_TOKEN, PRUNEJUICE_TELEGRAM_CHAT_ID, PRUNEJUICE_HOST
#
`
