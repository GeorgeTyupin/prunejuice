# Runbook: server disk fills up (the `sof-tunnel` incident)

> This is the incident write-up and manual runbook that `prunejuice` automates.
> It is kept in the repo both as motivation and as the fallback procedure for
> when you need to do this by hand.

## What happened

On the `sof-tunnel` server the **root filesystem (`/`) filled to 100%**. The
sequence was roughly:

1. `journald` was configured with no effective size cap, and a chatty service
   plus Docker container logs steadily grew.
2. Free space on `/` crossed 85%, then 95%, with nobody watching.
3. At 100%, the failure cascaded:
   - Docker could not write to `/var/lib/docker` and refused to start / restart
     containers.
   - New SSH sessions became flaky (can't write PTY/records, shell history).
   - Anything that needed a temp file (`apt`, package tooling) errored out.
4. Recovery required a human to log in and free space manually.

The painful part was not the fix — it was three commands — but that **nobody
knew until users reported the outage.** There was no alert.

## Root causes

- **No monitoring of free disk space** with an actionable alert.
- **Unbounded logs**: journald retention and container log files were not
  capped, so logs were the primary consumer of the disk.
- **Manual, tribal-knowledge recovery**: the fix lived in someone's head.

## Immediate manual fix (the runbook)

Run these in order, checking free space (`df -h /`) after each. Stop once you're
comfortably below your threshold.

```bash
df -h /                              # see how bad it is

# 1. Vacuum old systemd journals (keep the last 7 days)
sudo journalctl --vacuum-time=7d
df -h /

# 2. Drop the local apt package cache
sudo apt clean
df -h /

# 3. ONLY IF docker is installed and you understand the blast radius:
#    remove stopped containers, unused networks, dangling images & build cache.
#    This can delete resources other stacks rely on — be sure.
sudo docker system prune -f
df -h /
```

If that still isn't enough, investigate the biggest consumers:

```bash
sudo du -xh / | sort -rh | head -40
sudo du -xh /var/log | sort -rh | head -20
```

## Preventive fixes that were applied

1. **Cap Docker container logs** in `/etc/docker/daemon.json` (already done on
   the server — do **not** let `prunejuice` touch this):

   ```json
   {
     "log-driver": "json-file",
     "log-opts": { "max-size": "10m", "max-file": "3" }
   }
   ```

   > `prunejuice` deliberately does **not** manage per-container log files,
   > because this daemon-level limit already bounds them.

2. **Cap journald** in `/etc/systemd/journald.conf`:

   ```ini
   [Journal]
   SystemMaxUse=500M
   ```

3. **Automate detection + first-response**: this repository. `prunejuice` runs
   the three commands above automatically when `/` crosses the threshold, and —
   the whole point — **alerts a human via Telegram** when it can't cope or when
   it hits an error of its own.

## How `prunejuice` maps onto this runbook

| Runbook step | prunejuice config step | Default |
| --- | --- | --- |
| `journalctl --vacuum-time=7d` | `journal-vacuum` | enabled |
| `apt clean` | `apt-clean` | enabled |
| `docker system prune -f` | `docker-prune` | **disabled** (opt-in; auto-skipped if docker absent) |
| "check `df -h` after each" | re-probes after every step, stops early | always |
| "alert a human" | Telegram alert on unresolved / error | when `telegram.enabled` |

## Deploying the fix

See [`../deploy/systemd`](../deploy/systemd) for the timer + service units, and
the main [README](../README.md) for configuration. Suggested threshold: **85%**,
checked every **5 minutes**.
