// Package notify provides simple domain.Notifier implementations that do not
// pull in a network dependency: a no-op (the library default), a stdout/logger
// notifier, and a multiplexer that fans an alert out to several notifiers.
package notify

import (
	"context"
	"log/slog"

	"github.com/GeorgeTyupin/prunejuice/internal/domain"
)

// Noop discards every alert. It is the default notifier so that a library user
// who never configures Telegram gets a working, side-effect-free engine.
type Noop struct{}

// Notify implements domain.Notifier.
func (Noop) Notify(context.Context, domain.Alert) error { return nil }

// Log writes alerts to an slog.Logger. Useful in library mode when you want
// alerts to land in your app's own logging pipeline instead of Telegram.
type Log struct {
	Logger *slog.Logger
}

// NewLog returns a Log notifier. A nil logger falls back to slog.Default().
func NewLog(l *slog.Logger) *Log {
	if l == nil {
		l = slog.Default()
	}
	return &Log{Logger: l}
}

// Notify implements domain.Notifier.
func (n *Log) Notify(_ context.Context, alert domain.Alert) error {
	n.Logger.Warn("prunejuice alert", "level", alert.Level.String(), "body", alert.Text())
	return nil
}

// Multi fans an alert out to every wrapped notifier. A failure in one does not
// stop the others; the first error encountered is returned.
type Multi struct {
	Notifiers []domain.Notifier
}

// NewMulti returns a Multi over the given notifiers.
func NewMulti(notifiers ...domain.Notifier) *Multi {
	return &Multi{Notifiers: notifiers}
}

// Notify implements domain.Notifier.
func (m *Multi) Notify(ctx context.Context, alert domain.Alert) error {
	var firstErr error
	for _, n := range m.Notifiers {
		if n == nil {
			continue
		}
		if err := n.Notify(ctx, alert); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
