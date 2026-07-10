# Deploying prunejuice with systemd

Two supported shapes. **The timer is recommended.**

## Recommended: one-shot binary + systemd timer

`prunejuice.service` is `Type=oneshot`: it runs a single check-and-clean cycle
and exits. `prunejuice.timer` fires it every 5 minutes.

Why this over a daemon:

- **Nothing to leak or supervise.** There's no long-lived process to accumulate
  goroutines, file descriptors, or memory; each run starts clean and exits.
- **The schedule lives in one obvious place** (`prunejuice.timer`), editable
  with `systemctl edit` and independent of the binary.
- **Robust to missed runs.** `Persistent=true` means a machine that was asleep
  runs immediately on resume.
- **Trivial to reason about in an incident.** `systemctl list-timers` and
  `journalctl -u prunejuice.service` tell the whole story.

### Install

```bash
sudo install -m 0755 bin/prunejuice /usr/local/bin/prunejuice
sudo mkdir -p /etc/prunejuice /var/log/prunejuice
sudo cp config.example.yaml /etc/prunejuice/config.yaml   # then edit it

# Secrets (recommended over putting the token in config.yaml):
sudo tee /etc/prunejuice/prunejuice.env >/dev/null <<'EOF'
PRUNEJUICE_TELEGRAM_BOT_TOKEN=123456:ABC-your-token
PRUNEJUICE_TELEGRAM_CHAT_ID=-1001234567890
EOF
sudo chmod 600 /etc/prunejuice/prunejuice.env

sudo cp deploy/systemd/prunejuice.service deploy/systemd/prunejuice.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now prunejuice.timer
```

### Operate

```bash
systemctl list-timers prunejuice.timer     # when it next runs
sudo systemctl start prunejuice.service     # run it right now
journalctl -u prunejuice.service -f         # watch its output
```

## Alternative: long-lived daemon

If you'd rather have a single resident process with an internal ticker
(`check_interval` in the config), run the binary with `-daemon` and use a plain
`Type=simple` service instead of the timer:

```ini
[Service]
Type=simple
ExecStart=/usr/local/bin/prunejuice -config /etc/prunejuice/config.yaml -daemon
Restart=on-failure
EnvironmentFile=-/etc/prunejuice/prunejuice.env
```

`prunejuice` handles `SIGINT`/`SIGTERM` cleanly, so `systemctl stop` shuts it
down without leaving cleanup commands running. This is a fine choice too — just
one more moving part than the timer.
