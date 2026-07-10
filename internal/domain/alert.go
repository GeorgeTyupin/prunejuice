package domain

import (
	"fmt"
	"strings"
	"time"
)

// AlertLevel classifies why an alert is being raised.
type AlertLevel int

const (
	// AlertDiskFull means cleanup ran but the disk is still over threshold.
	AlertDiskFull AlertLevel = iota
	// AlertError means the utility itself failed (could not probe the disk,
	// a cleanup command errored, etc.).
	AlertError
)

// String renders the level as a short tag used in message titles.
func (l AlertLevel) String() string {
	switch l {
	case AlertDiskFull:
		return "DISK FULL"
	case AlertError:
		return "ERROR"
	default:
		return "ALERT"
	}
}

// Alert is a channel-agnostic notification. A Notifier turns it into whatever
// its transport needs (Telegram text, a log line, ...). Rendering lives here,
// on the entity, so every Notifier produces a consistent message.
type Alert struct {
	Level  AlertLevel
	Host   string
	Time   time.Time
	Report *Report // may be nil for a bare error alert
	Err    error
}

// NewDiskFullAlert builds an alert for a run that could not free enough space.
func NewDiskFullAlert(r *Report, now time.Time) Alert {
	host := ""
	if r != nil {
		host = r.Host
	}
	return Alert{Level: AlertDiskFull, Host: host, Time: now, Report: r}
}

// NewErrorAlert builds an alert for an operational failure of the utility.
func NewErrorAlert(host string, err error, r *Report, now time.Time) Alert {
	return Alert{Level: AlertError, Host: host, Time: now, Report: r, Err: err}
}

// Text renders the alert as a plain-text message body suitable for Telegram
// or a log line. It is deterministic and free of external dependencies so it
// can be asserted on directly in tests.
func (a Alert) Text() string {
	var b strings.Builder

	fmt.Fprintf(&b, "🧹 prunejuice [%s]\n", a.Level)
	if a.Host != "" {
		fmt.Fprintf(&b, "host: %s\n", a.Host)
	}
	fmt.Fprintf(&b, "time: %s\n", a.Time.Format(time.RFC3339))

	if a.Err != nil {
		fmt.Fprintf(&b, "\nerror: %s\n", a.Err.Error())
	}

	if a.Report != nil {
		r := a.Report
		u := r.FinalUsage
		fmt.Fprintf(&b, "\npath: %s\n", r.Path)
		fmt.Fprintf(&b, "used: %.1f%% (%.0f MB of %.0f MB), free %.0f MB\n",
			u.UsedPercent, u.UsedMB(), u.TotalMB(), u.FreeMB())
		fmt.Fprintf(&b, "threshold: %.1f%%\n", r.Threshold)

		if len(r.Steps) > 0 {
			b.WriteString("\ncleanup steps:\n")
			for _, s := range r.Steps {
				if s.Skipped {
					fmt.Fprintf(&b, "  • %s — skipped (%s)\n", s.Step.Name, s.SkipReason)
					continue
				}
				status := fmt.Sprintf("freed %.1f MB in %s", s.FreedMB(), s.Duration.Round(time.Millisecond))
				if s.Err != nil {
					status = "FAILED: " + s.Err.Error()
				}
				fmt.Fprintf(&b, "  • %s — %s\n", s.Step.Name, status)
			}
			fmt.Fprintf(&b, "total freed: %.1f MB\n", float64(r.TotalFreedBytes())/bytesPerMB)
		}
	}

	return b.String()
}
