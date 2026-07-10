# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Initial release.
- Threshold-based disk monitoring via gopsutil.
- Ordered, configurable cleanup steps (`journalctl --vacuum`, `apt clean`,
  `docker system prune`), re-checking free space after each and stopping early.
- Telegram alerts on unresolved disk pressure and on operational errors.
- Library API (`prunejuice.New`, `RunOnce`, `Run`) with functional options and
  opt-in Telegram; no network dependency in library mode by default.
- CLI with YAML config, env-var secret overrides, one-shot and `-daemon` modes,
  `-print-config`, and graceful `SIGINT`/`SIGTERM` shutdown.
- Self-limiting, rotating file logging.
- systemd service + timer units and a hardened deployment guide.
- Unit tests for the decision logic, config loading, log rotation, and the
  command runner's timeout.

[Unreleased]: https://github.com/GeorgeTyupin/prunejuice/commits/main
