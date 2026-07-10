# prunejuice 🧹

[English](README.md) | [Русский](README.ru.md)

[![CI](https://github.com/GeorgeTyupin/prunejuice/actions/workflows/ci.yml/badge.svg)](https://github.com/GeorgeTyupin/prunejuice/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/GeorgeTyupin/prunejuice.svg)](https://pkg.go.dev/github.com/GeorgeTyupin/prunejuice)
[![Go Report Card](https://goreportcard.com/badge/github.com/GeorgeTyupin/prunejuice)](https://goreportcard.com/report/github.com/GeorgeTyupin/prunejuice)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**Keep your server's disk from filling up — automatically.**

`prunejuice` watches a mount point and, when used space crosses a threshold,
runs an ordered list of cleanup commands (journal vacuum, `apt clean`,
`docker system prune`, ...), stopping the moment the disk drops back below the
line. If it still can't free enough — or if anything goes wrong — it fires an
alert to Telegram.

> Prune juice is what you drink to clean things out. This is that, for your disk.

It ships as **two things at once**:

- 🛠️ a **standalone binary** you drop on a server (systemd timer or daemon),
  with Telegram alerts built in; and
- 📦 a **Go library** you embed in your own service, with zero Telegram (or any
  network) dependency at runtime unless you opt in.

---

## Why this exists

This was born from a real incident on a box called `sof-tunnel`: journald and
application logs quietly ate the root filesystem until it hit 100%, at which
point Docker refused to start containers and SSH sessions began failing. The
box needed a human at 3 a.m. to run three commands that a script could have run
by itself — and, crucially, to *know it was happening* before users did.

See [`docs/server-disk-cleanup-runbook.md`](docs/server-disk-cleanup-runbook.md)
for the full post-mortem and the manual runbook this tool automates.

---

## Features

- ✅ Threshold-based disk monitoring via [`gopsutil`](https://github.com/shirou/gopsutil).
- ✅ Ordered, **configurable** cleanup steps — enable/disable/reorder without a rebuild.
- ✅ Re-checks free space after every step and **stops as soon as it's enough**.
- ✅ Telegram alerts on *"couldn't free enough space"* **and** on *"the utility itself errored"*.
- ✅ Per-command timeouts; hung commands are killed, never left hanging.
- ✅ Graceful shutdown on `SIGINT`/`SIGTERM`.
- ✅ Self-limiting logs with built-in rotation — it will **never** become the
  thing that fills your disk.
- ✅ Nothing destructive runs unless you explicitly enable it (`docker prune` is **off by default**).
- ✅ Clean architecture, dependency-light, unit-tested decision logic.

---

## Install

```bash
# as a CLI
go install github.com/GeorgeTyupin/prunejuice/cmd/prunejuice@latest

# as a library
go get github.com/GeorgeTyupin/prunejuice
```

Or build from source:

```bash
git clone https://github.com/GeorgeTyupin/prunejuice
cd prunejuice
make build      # produces ./bin/prunejuice
```

---

## Quick start (CLI)

1. Generate a config:

   ```bash
   prunejuice -print-config > config.yaml
   ```

2. Edit it (see [Configuration](#configuration)). At minimum, set your threshold
   and — if you want alerts — enable Telegram.

3. Run once (the recommended mode, driven by a systemd timer):

   ```bash
   prunejuice -config /etc/prunejuice/config.yaml
   ```

   Or run as a long-lived daemon with an internal ticker:

   ```bash
   prunejuice -config /etc/prunejuice/config.yaml -daemon
   ```

### Recommended deployment: systemd timer

Running one-shot on a timer is simpler and more robust than a daemon: no
long-lived process to leak, restart, or supervise, and the schedule lives in
one obvious place. Copy the units from [`deploy/systemd`](deploy/systemd):

```bash
sudo cp bin/prunejuice /usr/local/bin/
sudo mkdir -p /etc/prunejuice && sudo cp config.yaml /etc/prunejuice/
sudo cp deploy/systemd/prunejuice.* /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now prunejuice.timer
```

Secrets can be supplied via `systemctl edit prunejuice.service` (an environment
override) instead of the config file — see the runbook. **Daemon vs. timer is
discussed in detail in [`deploy/systemd/README.md`](deploy/systemd/README.md).**

---

## Run in Docker

prunejuice can run as a long-lived container that maintains its **host**. The
trick: the host root is bind-mounted read-only for the disk check, and cleanup
commands run in the host's namespaces via `nsenter -t 1`, so `journalctl`,
`apt`, and `docker` act on the host rather than the container.

```bash
# edit configs/config.docker.yaml if needed (threshold, enable docker-prune, ...)
cat > prunejuice.env <<'EOF'
PRUNEJUICE_TELEGRAM_BOT_TOKEN=123456789:AA...
PRUNEJUICE_TELEGRAM_CHAT_ID=123456789
PRUNEJUICE_HOST=sof-tunnel
EOF
chmod 600 prunejuice.env

docker compose up -d --build
docker compose logs -f
```

The provided [`docker-compose.yml`](docker-compose.yml) runs the container with
`pid: host` and `privileged: true` — required for `nsenter` to enter the host
namespaces.

> ⚠️ **Security:** a privileged, `pid: host` container is effectively root on
> the host. That's the cost of doing host maintenance from a container (a
> comparable trust level to running the systemd unit as root, but a different
> attack surface). If you don't specifically need Docker, the
> [systemd timer](deploy/systemd) deployment is simpler and less privileged.
> A lighter, Docker-socket-only variant (prune Docker + monitor disk, no
> `privileged`) is documented in the compose file.

See [`configs/config.docker.yaml`](configs/config.docker.yaml) for the container
config, which wraps each cleanup step in `nsenter`.

---

## Configuration

`prunejuice` reads YAML. Secrets may be supplied via environment variables
instead of the file:

| Env var | Overrides |
| --- | --- |
| `PRUNEJUICE_TELEGRAM_BOT_TOKEN` | `telegram.bot_token` |
| `PRUNEJUICE_TELEGRAM_CHAT_ID` | `telegram.chat_id` |
| `PRUNEJUICE_HOST` | `host` |

See [`configs/config.example.yaml`](configs/config.example.yaml) for a fully commented example.
The essentials:

```yaml
mount_path: /            # filesystem to watch
threshold_percent: 85    # trigger cleanup at/above this used %
check_interval: 5m       # daemon mode tick (ignored for one-shot)
command_timeout: 60s     # per-command hard cap

log:
  level: info
  file: /var/log/prunejuice/prunejuice.log
  max_size_mb: 10        # rotate at 10 MB, keep 3 files → bounded at ~40 MB
  max_backups: 3

telegram:
  enabled: true          # off ⇒ log-only mode
  # token & chat id come from env vars above

steps:
  - name: journal-vacuum
    command: journalctl
    args: ["--vacuum-time=7d"]
    enabled: true
  - name: apt-clean
    command: apt
    args: ["clean"]
    enabled: true
  - name: docker-prune
    command: docker
    args: ["system", "prune", "-f"]
    enabled: false        # ⚠️ destructive — opt in deliberately
    requires_binary: docker  # skipped automatically if docker isn't installed
```

---

## Use as a library

Embed the exact same engine in your own Go service and run it on your own
schedule (e.g. from an HTTP handler, a cron goroutine, or a k8s sidecar).
**No Telegram dependency is pulled into your runtime unless you call
`WithTelegram`.** By default alerts go nowhere; route them wherever you like.

```go
package main

import (
	"context"
	"log"

	"github.com/GeorgeTyupin/prunejuice"
)

func main() {
	p, err := prunejuice.New(prunejuice.Config{
		MountPath:        "/",
		ThresholdPercent: 85,
		Steps:            prunejuice.DefaultSteps(),
	})
	if err != nil {
		log.Fatal(err)
	}

	report, err := p.RunOnce(context.Background())
	if err != nil {
		log.Printf("prune failed: %v", err)
	}
	if !report.Resolved() {
		log.Printf("disk still at %.1f%% after cleanup", report.FinalUsage.UsedPercent)
		// ... wire your own alerting / metrics here
	}
}
```

Opt into Telegram, plug in your own alert sink, or inject fakes for testing:

```go
p, _ := prunejuice.New(cfg,
	prunejuice.WithTelegram(token, chatID),   // enable Telegram
	prunejuice.WithLogNotifier(myLogger),     // also mirror alerts into slog
	prunejuice.WithNotifier(myPagerDuty),     // ...or anything implementing Notifier
	prunejuice.WithLogger(myLogger),          // step-by-step operational logs
)
```

`Notifier` is a one-method interface, so integrating Slack, PagerDuty, email, or
a Prometheus pushgateway is trivial:

```go
type Notifier interface {
	Notify(ctx context.Context, alert prunejuice.Alert) error
}
```

A runnable example lives in [`examples/library`](examples/library).

---

## Architecture

`prunejuice` follows clean architecture — dependencies point **inward**, and the
core has no idea Telegram or gopsutil exist:

The module exposes exactly **one public package** (`prunejuice`, the facade);
everything else lives under `internal/` so the implementation can evolve without
breaking library users.

```
cmd/prunejuice              ← wiring: flags, signals, config → adapters → engine
      │
      ├── internal/config   ← YAML + env loading via cleanenv (CLI only)
      ├── internal/logging  ← slog + size-capped rotation (CLI only)
      │
prunejuice (facade)         ← the one public package: New(), RunOnce(), Run(),
      │                        functional options, re-exported domain types
      │
   internal/service         ← the use case: threshold decision, step orchestration
      │                        (pure; depends only on domain — this is unit-tested)
      │
   internal/domain          ← entities (DiskUsage, CleanupStep, Report, Alert) +
                              port interfaces (DiskProber, Runner, Notifier). Zero deps.
      ▲
   internal/adapter/*       ← implement the ports: disk (gopsutil), command (os/exec),
                              telegram, notify (noop/log/multi)

configs/                    ← YAML config files (config.example.yaml, config.docker.yaml)
```

Because the decision logic lives behind interfaces, the entire "should we clean,
what should we run, do we alert" flow is tested with in-memory fakes — no real
disk, no shelling out, no network. See [`internal/service/engine_test.go`](internal/service/engine_test.go).

---

## Safety

- **Nothing destructive by default.** Every cleanup step is `enabled: false`
  until you say otherwise, and `docker system prune -f` in particular ships
  disabled because it can delete resources owned by other stacks on the host.
- **Timeouts everywhere.** Each command runs under `command_timeout`; if it
  hangs, it's killed.
- **Bounded logs.** The tool that guards against full disks refuses to fill one
  itself: file logs rotate at `max_size_mb` and keep only `max_backups`.
- **Least privilege.** It only needs to read disk stats and run the cleanup
  commands you enabled. See the runbook for a hardened systemd unit.

---

## Development

```bash
make test        # go test ./... with the race detector
make lint        # gofmt + go vet (+ golangci-lint if installed)
make build       # build the binary into ./bin
```

Contributions welcome — see [CONTRIBUTING.md](CONTRIBUTING.md) and the
[Code of Conduct](CODE_OF_CONDUCT.md). Security issues: [SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE) © George Tyupin
