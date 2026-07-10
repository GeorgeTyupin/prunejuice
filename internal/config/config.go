// Package config loads prunejuice's YAML configuration and environment
// overrides via cleanenv, validates it, and converts cleanup steps into domain
// types. It is used only by the CLI; library callers construct prunejuice.Config
// directly.
package config

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
	"gopkg.in/yaml.v3"

	"github.com/GeorgeTyupin/prunejuice/internal/domain"
)

// Environment variables that override secret / deployment-specific fields so
// tokens never have to live in a checked-in file. They are declared as `env`
// struct tags below; these constants exist for docs and error messages.
const (
	EnvBotToken = "PRUNEJUICE_TELEGRAM_BOT_TOKEN"
	EnvChatID   = "PRUNEJUICE_TELEGRAM_CHAT_ID"
	EnvHost     = "PRUNEJUICE_HOST"
)

// Config is the full on-disk configuration. YAML fields are read from the file;
// the `env` tags let cleanenv override secrets/host from the environment.
type Config struct {
	// MountPath is the filesystem to monitor.
	MountPath string `yaml:"mount_path"`
	// ThresholdPercent triggers cleanup when used space reaches it.
	ThresholdPercent float64 `yaml:"threshold_percent"`
	// CheckInterval is the daemon tick. Ignored in one-shot mode.
	CheckInterval Duration `yaml:"check_interval"`
	// CommandTimeout caps each cleanup command.
	CommandTimeout Duration `yaml:"command_timeout"`
	// Host labels alerts; empty ⇒ os.Hostname().
	Host string `yaml:"host" env:"PRUNEJUICE_HOST"`

	Log      LogConfig      `yaml:"log"`
	Telegram TelegramConfig `yaml:"telegram"`
	Steps    []StepConfig   `yaml:"steps"`
}

// LogConfig controls logging output and rotation.
type LogConfig struct {
	Level      string `yaml:"level"`
	JSON       bool   `yaml:"json"`
	File       string `yaml:"file"`
	MaxSizeMB  int    `yaml:"max_size_mb"`
	MaxBackups int    `yaml:"max_backups"`
}

// TelegramConfig holds alert-channel settings.
type TelegramConfig struct {
	// Enabled turns Telegram alerting on. When false the CLI runs with a no-op
	// notifier — this is the "library mode" behaviour, usable from the CLI too.
	Enabled  bool   `yaml:"enabled"`
	BotToken string `yaml:"bot_token" env:"PRUNEJUICE_TELEGRAM_BOT_TOKEN"`
	ChatID   int64  `yaml:"chat_id" env:"PRUNEJUICE_TELEGRAM_CHAT_ID"`
}

// StepConfig is a cleanup step as written in YAML.
type StepConfig struct {
	Name           string   `yaml:"name"`
	Command        string   `yaml:"command"`
	Args           []string `yaml:"args"`
	Enabled        bool     `yaml:"enabled"`
	RequiresBinary string   `yaml:"requires_binary"`
}

// Duration is a time.Duration that unmarshals from a Go duration string
// ("5m", "30s", "1h30m") or a bare number of seconds. yaml.v3 (used by
// cleanenv for the file) has no native duration support, so this wrapper adds
// it.
type Duration struct {
	time.Duration
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err == nil {
		parsed, perr := time.ParseDuration(s)
		if perr != nil {
			return fmt.Errorf("config: invalid duration %q: %w", s, perr)
		}
		d.Duration = parsed
		return nil
	}
	var secs int64
	if err := value.Decode(&secs); err == nil {
		d.Duration = time.Duration(secs) * time.Second
		return nil
	}
	return fmt.Errorf("config: duration must be a string like \"5m\" or a number of seconds, got %q", value.Value)
}

// MarshalYAML implements yaml.Marshaler, emitting the canonical string form.
func (d Duration) MarshalYAML() (any, error) { return d.Duration.String(), nil }

// Default returns a Config matching the documented defaults and the cleanup
// sequence from the runbook. docker-prune is present but disabled because it
// can touch data owned by other stacks on the host.
func Default() Config {
	return Config{
		MountPath:        "/",
		ThresholdPercent: 85,
		CheckInterval:    Duration{5 * time.Minute},
		CommandTimeout:   Duration{60 * time.Second},
		Log: LogConfig{
			Level:      "info",
			File:       "",
			MaxSizeMB:  10,
			MaxBackups: 3,
		},
		Telegram: TelegramConfig{Enabled: false},
		Steps: []StepConfig{
			{Name: "journal-vacuum", Command: "journalctl", Args: []string{"--vacuum-time=7d"}, Enabled: true},
			{Name: "apt-clean", Command: "apt", Args: []string{"clean"}, Enabled: true},
			{Name: "docker-prune", Command: "docker", Args: []string{"system", "prune", "-f"}, Enabled: false, RequiresBinary: "docker"},
		},
	}
}

// Load reads the YAML file at path onto the defaults, applies environment
// overrides (via cleanenv `env` tags), resolves the host name, and validates
// the result.
func Load(path string) (Config, error) {
	cfg := Default()
	if err := cleanenv.ReadConfig(path, &cfg); err != nil {
		return Config{}, fmt.Errorf("config: load %q: %w", path, err)
	}
	if cfg.Host == "" {
		if h, err := os.Hostname(); err == nil {
			cfg.Host = h
		}
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate checks the invariants the engine relies on.
func (c Config) Validate() error {
	if c.MountPath == "" {
		return fmt.Errorf("config: mount_path must not be empty")
	}
	if c.ThresholdPercent <= 0 || c.ThresholdPercent > 100 {
		return fmt.Errorf("config: threshold_percent must be in (0,100], got %v", c.ThresholdPercent)
	}
	if c.CommandTimeout.Duration <= 0 {
		return fmt.Errorf("config: command_timeout must be positive")
	}
	if c.Telegram.Enabled {
		if c.Telegram.BotToken == "" {
			return fmt.Errorf("config: telegram.enabled but bot token is missing (set %s)", EnvBotToken)
		}
		if c.Telegram.ChatID == 0 {
			return fmt.Errorf("config: telegram.enabled but chat_id is missing (set %s)", EnvChatID)
		}
	}
	for i, s := range c.Steps {
		if s.Name == "" {
			return fmt.Errorf("config: steps[%d] has empty name", i)
		}
		if s.Command == "" {
			return fmt.Errorf("config: step %q has empty command", s.Name)
		}
	}
	return nil
}

// DomainSteps converts the configured steps into domain cleanup steps in order.
func (c Config) DomainSteps() []domain.CleanupStep {
	out := make([]domain.CleanupStep, 0, len(c.Steps))
	for _, s := range c.Steps {
		out = append(out, domain.CleanupStep{
			Name:           s.Name,
			Command:        s.Command,
			Args:           s.Args,
			Enabled:        s.Enabled,
			RequiresBinary: s.RequiresBinary,
		})
	}
	return out
}

// WriteDefault writes a commented default configuration as YAML to w. It backs
// the CLI's -print-config flag.
func WriteDefault(w io.Writer) error {
	out, err := yaml.Marshal(Default())
	if err != nil {
		return fmt.Errorf("config: marshal default: %w", err)
	}
	if _, err := io.WriteString(w, defaultHeader); err != nil {
		return err
	}
	_, err = w.Write(out)
	return err
}

const defaultHeader = `# prunejuice configuration.
# Secrets can be supplied via environment variables instead of this file:
#   PRUNEJUICE_TELEGRAM_BOT_TOKEN, PRUNEJUICE_TELEGRAM_CHAT_ID, PRUNEJUICE_HOST
#
`
