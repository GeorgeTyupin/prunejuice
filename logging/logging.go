// Package logging builds the slog.Logger used by the CLI: stdout plus, when
// configured, a size-capped rotating file. The whole point is bounded output —
// prunejuice must never fill the disk it is supposed to protect.
package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// Options controls logger construction.
type Options struct {
	// Level is one of debug, info, warn, error (case-insensitive). Empty ⇒ info.
	Level string
	// JSON selects the JSON handler instead of the text handler.
	JSON bool
	// FilePath, when non-empty, additionally writes logs to a rotating file.
	FilePath string
	// MaxSizeMB is the rotation threshold per file (default 10).
	MaxSizeMB int
	// MaxBackups is how many rotated files to keep (default 3).
	MaxBackups int
}

// Setup returns a logger and an io.Closer. The closer flushes/closes the log
// file (a no-op when logging only to stdout) and should be deferred by the
// caller.
func Setup(o Options) (*slog.Logger, io.Closer, error) {
	writers := []io.Writer{os.Stdout}
	var closer io.Closer = noopCloser{}

	if o.FilePath != "" {
		rw, err := NewRotatingWriter(o.FilePath, o.MaxSizeMB, o.MaxBackups)
		if err != nil {
			return nil, nil, err
		}
		writers = append(writers, rw)
		closer = rw
	}

	handlerOpts := &slog.HandlerOptions{Level: parseLevel(o.Level)}
	out := io.MultiWriter(writers...)

	var handler slog.Handler
	if o.JSON {
		handler = slog.NewJSONHandler(out, handlerOpts)
	} else {
		handler = slog.NewTextHandler(out, handlerOpts)
	}
	return slog.New(handler), closer, nil
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

type noopCloser struct{}

func (noopCloser) Close() error { return nil }
