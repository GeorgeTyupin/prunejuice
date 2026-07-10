# Contributing to prunejuice

Thanks for taking the time to contribute! 🧹 This project aims to stay small,
dependency-light, and boring-in-a-good-way — the kind of tool you can read in an
afternoon and trust on a production box.

## Ways to help

- **Report bugs** and **request features** via [issues](https://github.com/GeorgeTyupin/prunejuice/issues).
- **Add cleanup adapters or notifiers** (Slack, PagerDuty, email, a new cleanup
  step). Notifiers are a one-method interface — see below.
- **Improve docs**, the runbook, or the systemd hardening.

## Development setup

```bash
git clone https://github.com/GeorgeTyupin/prunejuice
cd prunejuice
make test    # go test -race ./...
make lint    # gofmt + go vet (+ golangci-lint if installed)
make build
```

You need Go 1.23+.

## Architecture in one paragraph

Dependencies point inward (clean architecture). `domain` holds entities and the
port interfaces and has **zero** external dependencies. `service` is the pure
use case (the decision logic) and depends only on `domain`. Adapters under
`adapter/` implement the ports (gopsutil, os/exec, Telegram). The root
`prunejuice` package is the library facade; `cmd/prunejuice` is the CLI wiring.
**Please keep new external dependencies out of `domain` and `service`.**

### Adding a notifier

Implement the one-method port and register it via an option:

```go
type Notifier interface {
	Notify(ctx context.Context, alert domain.Alert) error
}
```

Put it under `adapter/<name>` and, if it's broadly useful, add a `With<Name>`
option in `options.go`.

### Adding a cleanup step

Cleanup steps are **data, not code** — they're configured in YAML, so most
"new step" requests need no code change at all, just documentation. Only touch
`config.Default()` / `DefaultSteps()` if a step should ship on by default (and
non-destructive steps only).

## Pull request checklist

- [ ] `make lint test` passes locally.
- [ ] New behaviour has unit tests (use the in-memory fakes in `service` as a
      model — no real disk or network in tests).
- [ ] Public API changes are documented in the README and doc comments.
- [ ] Nothing destructive is enabled by default.
- [ ] Commit messages are clear; conventional-commit prefixes appreciated.

## Reporting security issues

Please **do not** open a public issue for security problems. See
[SECURITY.md](SECURITY.md).

## Code of Conduct

By participating you agree to the [Code of Conduct](CODE_OF_CONDUCT.md).
