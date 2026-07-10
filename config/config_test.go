package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad_MergesOntoDefaults(t *testing.T) {
	path := writeConfig(t, `
mount_path: /data
threshold_percent: 90
check_interval: 10m
steps:
  - name: journal
    command: journalctl
    args: ["--vacuum-time=3d"]
    enabled: true
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.MountPath != "/data" {
		t.Errorf("MountPath = %q, want /data", cfg.MountPath)
	}
	if cfg.ThresholdPercent != 90 {
		t.Errorf("ThresholdPercent = %v, want 90", cfg.ThresholdPercent)
	}
	if cfg.CheckInterval.Duration != 10*time.Minute {
		t.Errorf("CheckInterval = %v, want 10m", cfg.CheckInterval.Duration)
	}
	// CommandTimeout was not set in the file, so the default must survive.
	if cfg.CommandTimeout.Duration != 60*time.Second {
		t.Errorf("CommandTimeout = %v, want default 60s", cfg.CommandTimeout.Duration)
	}
	if len(cfg.Steps) != 1 || cfg.Steps[0].Command != "journalctl" {
		t.Errorf("Steps = %+v, want single journalctl step", cfg.Steps)
	}
}

func TestLoad_EnvOverridesSecrets(t *testing.T) {
	path := writeConfig(t, `
threshold_percent: 80
telegram:
  enabled: true
`)
	t.Setenv(EnvBotToken, "123:abc")
	t.Setenv(EnvChatID, "-1001234567890")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Telegram.BotToken != "123:abc" {
		t.Errorf("BotToken not taken from env: %q", cfg.Telegram.BotToken)
	}
	if cfg.Telegram.ChatID != -1001234567890 {
		t.Errorf("ChatID = %d, want -1001234567890", cfg.Telegram.ChatID)
	}
}

func TestValidate_Rejects(t *testing.T) {
	cases := map[string]Config{
		"empty mount":       {ThresholdPercent: 85, CommandTimeout: Duration{time.Second}},
		"threshold zero":    {MountPath: "/", ThresholdPercent: 0, CommandTimeout: Duration{time.Second}},
		"threshold over":    {MountPath: "/", ThresholdPercent: 150, CommandTimeout: Duration{time.Second}},
		"no timeout":        {MountPath: "/", ThresholdPercent: 85},
		"telegram no token": {MountPath: "/", ThresholdPercent: 85, CommandTimeout: Duration{time.Second}, Telegram: TelegramConfig{Enabled: true, ChatID: 1}},
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			if err := cfg.Validate(); err == nil {
				t.Errorf("Validate() = nil, want error")
			}
		})
	}
}

func TestValidate_AcceptsDefault(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Errorf("Default().Validate() = %v, want nil", err)
	}
}

func TestDomainSteps_PreservesOrder(t *testing.T) {
	cfg := Default()
	steps := cfg.DomainSteps()
	if len(steps) != len(cfg.Steps) {
		t.Fatalf("got %d steps, want %d", len(steps), len(cfg.Steps))
	}
	for i := range steps {
		if steps[i].Name != cfg.Steps[i].Name {
			t.Errorf("step %d = %q, want %q (order not preserved)", i, steps[i].Name, cfg.Steps[i].Name)
		}
	}
}
