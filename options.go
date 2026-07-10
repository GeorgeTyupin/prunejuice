package prunejuice

import (
	"log/slog"
	"time"

	"github.com/GeorgeTyupin/prunejuice/internal/adapter/notify"
	"github.com/GeorgeTyupin/prunejuice/internal/adapter/telegram"
	"github.com/GeorgeTyupin/prunejuice/internal/domain"
)

// Option customises a Pruner built with New.
type Option func(*settings)

// settings accumulates option state before New wires the engine.
type settings struct {
	prober    domain.DiskProber
	runner    domain.Runner
	notifiers []domain.Notifier
	telegram  *telegramCreds
	logger    *slog.Logger
	now       func() time.Time
}

// telegramCreds defers Telegram client construction (a network call) until New.
type telegramCreds struct {
	token  string
	chatID int64
}

func (c telegramCreds) build() (domain.Notifier, error) {
	return telegram.New(c.token, c.chatID)
}

// WithNotifier routes alerts to a custom Notifier. Call it more than once to
// register several; alerts fan out to all of them.
func WithNotifier(n domain.Notifier) Option {
	return func(s *settings) {
		if n != nil {
			s.notifiers = append(s.notifiers, n)
		}
	}
}

// WithTelegram enables Telegram alerts. This is the only path that imports a
// network client, so a library that never calls it carries no Telegram
// runtime dependency at execution time. The client is created (and the token
// validated) when New runs.
func WithTelegram(botToken string, chatID int64) Option {
	return func(s *settings) {
		s.telegram = &telegramCreds{token: botToken, chatID: chatID}
	}
}

// WithLogNotifier routes alerts into an slog.Logger instead of an external
// channel — handy when embedding prunejuice in a service that already ships
// its logs somewhere.
func WithLogNotifier(l *slog.Logger) Option {
	return func(s *settings) {
		s.notifiers = append(s.notifiers, notify.NewLog(l))
	}
}

// WithLogger sets the engine's operational logger (the step-by-step log). nil
// is ignored. Without it, engine logs are discarded.
func WithLogger(l *slog.Logger) Option {
	return func(s *settings) {
		if l != nil {
			s.logger = l
		}
	}
}

// WithProber overrides the disk prober; mainly for tests.
func WithProber(p domain.DiskProber) Option {
	return func(s *settings) {
		if p != nil {
			s.prober = p
		}
	}
}

// WithRunner overrides the command runner; mainly for tests or sandboxes.
func WithRunner(r domain.Runner) Option {
	return func(s *settings) {
		if r != nil {
			s.runner = r
		}
	}
}

// WithClock injects a time source for deterministic timestamps in tests.
func WithClock(now func() time.Time) Option {
	return func(s *settings) {
		if now != nil {
			s.now = now
		}
	}
}
