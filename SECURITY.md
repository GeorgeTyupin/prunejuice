# Security Policy

## Reporting a vulnerability

Please report security issues **privately**. Do not open a public GitHub issue.

- Use GitHub's [private vulnerability reporting](https://github.com/GeorgeTyupin/prunejuice/security/advisories/new)
  ("Report a vulnerability" under the Security tab), or
- email the maintainer.

Please include a description, reproduction steps, and the impact. We aim to
acknowledge reports within a few days and to ship a fix as promptly as severity
warrants.

## Scope & threat model to keep in mind

`prunejuice` runs privileged cleanup commands on a server. When reviewing or
reporting, note these design points:

- **Commands come from config, and config is trusted input.** An operator who
  can edit `config.yaml` can already run arbitrary commands as the service user;
  that is by design. The relevant risk is *unexpected* command execution, not
  configured commands.
- **Nothing destructive runs by default.** All cleanup steps default to
  `enabled: false`; `docker system prune` is off even in the shipped defaults.
- **Secrets** (Telegram token) should be provided via environment variables /
  an `EnvironmentFile`, not committed. See the systemd unit.
- **Per-command timeouts** bound execution; a hung command is killed.

Reports that increase privilege, execute commands not present in the config, or
leak the Telegram token are especially welcome.

## Supported versions

The latest tagged release on the default branch is supported.
