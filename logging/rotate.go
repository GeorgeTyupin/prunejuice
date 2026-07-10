package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const bytesPerMB = 1024 * 1024

// RotatingWriter is a minimal size-based log rotator. It exists so prunejuice —
// a tool born from a disk filled by runaway logs — never becomes the thing it
// guards against, and it does so without pulling in an external logging
// dependency.
//
// When the active file would exceed MaxSizeBytes, it is rotated to "<path>.1",
// existing backups shift up ("<path>.1" → "<path>.2", ...), and anything beyond
// MaxBackups is deleted. It is safe for concurrent use.
type RotatingWriter struct {
	path         string
	maxSizeBytes int64
	maxBackups   int

	mu   sync.Mutex
	file *os.File
	size int64
}

// NewRotatingWriter opens (creating parent dirs as needed) the log file at path.
// maxSizeMB ≤ 0 defaults to 10 MB; maxBackups < 0 defaults to 3.
func NewRotatingWriter(path string, maxSizeMB, maxBackups int) (*RotatingWriter, error) {
	if maxSizeMB <= 0 {
		maxSizeMB = 10
	}
	if maxBackups < 0 {
		maxBackups = 3
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("logging: create dir %q: %w", dir, err)
		}
	}
	w := &RotatingWriter{
		path:         path,
		maxSizeBytes: int64(maxSizeMB) * bytesPerMB,
		maxBackups:   maxBackups,
	}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

// open attaches to the current log file, seeding size from its length so a
// restart does not forget how full the file already is.
func (w *RotatingWriter) open() error {
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("logging: open %q: %w", w.path, err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("logging: stat %q: %w", w.path, err)
	}
	w.file = f
	w.size = info.Size()
	return nil
}

// Write implements io.Writer, rotating first if the incoming record would push
// the file past its size cap.
func (w *RotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.size+int64(len(p)) > w.maxSizeBytes && w.size > 0 {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

// rotate closes the active file, shifts the backup chain, and opens a fresh one.
func (w *RotatingWriter) rotate() error {
	if err := w.file.Close(); err != nil {
		return fmt.Errorf("logging: close during rotate: %w", err)
	}

	if w.maxBackups == 0 {
		// No history kept: just start over.
		if err := os.Remove(w.path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("logging: remove during rotate: %w", err)
		}
		return w.open()
	}

	// Drop the oldest, then shift each backup up by one.
	oldest := w.backupName(w.maxBackups)
	if err := os.Remove(oldest); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("logging: drop oldest backup: %w", err)
	}
	for i := w.maxBackups - 1; i >= 1; i-- {
		src, dst := w.backupName(i), w.backupName(i+1)
		if err := os.Rename(src, dst); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("logging: shift backup %d: %w", i, err)
		}
	}
	if err := os.Rename(w.path, w.backupName(1)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("logging: rotate active file: %w", err)
	}
	return w.open()
}

func (w *RotatingWriter) backupName(i int) string {
	return fmt.Sprintf("%s.%d", w.path, i)
}

// Close closes the underlying file.
func (w *RotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	return w.file.Close()
}
