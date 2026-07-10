package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotatingWriter_RotatesAndBoundsBackups(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")

	// 1 MB cap, keep 2 backups. Force rotation with sub-MB-but-repeated writes
	// by using a tiny cap instead: reconstruct with a small size via internal
	// field so the test stays fast.
	w, err := NewRotatingWriter(path, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	w.maxSizeBytes = 1024 // shrink cap to 1 KiB for the test
	defer w.Close()

	line := strings.Repeat("x", 200) + "\n"
	for i := 0; i < 100; i++ { // ~20 KiB total, far past the 1 KiB cap
		if _, err := w.Write([]byte(line)); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}

	// The active file plus at most maxBackups files may exist, and nothing else.
	if _, err := os.Stat(path); err != nil {
		t.Errorf("active log missing: %v", err)
	}
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Errorf("path.3 should not exist (maxBackups=2), stat err=%v", err)
	}

	// Every retained file must be within the cap (allowing one record of slack).
	for _, suffix := range []string{"", ".1", ".2"} {
		info, err := os.Stat(path + suffix)
		if err != nil {
			continue // backup may legitimately not exist yet
		}
		if info.Size() > 1024+int64(len(line)) {
			t.Errorf("file %q size %d exceeds cap", path+suffix, info.Size())
		}
	}
}

func TestSetup_StdoutOnly_NoFile(t *testing.T) {
	logger, closer, err := Setup(Options{Level: "debug"})
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()
	if logger == nil {
		t.Fatal("nil logger")
	}
	// Closing the no-op closer must be safe.
	if err := closer.Close(); err != nil {
		t.Errorf("close: %v", err)
	}
}
