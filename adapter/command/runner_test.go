package command

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRunner_Success(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX echo")
	}
	r := New(5 * time.Second)
	res, err := r.Run(context.Background(), "echo", "hello")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(res.Stdout, "hello") {
		t.Errorf("stdout = %q, want it to contain hello", res.Stdout)
	}
	if res.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", res.ExitCode)
	}
}

func TestRunner_Timeout_KillsHungCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX sleep")
	}
	r := New(50 * time.Millisecond)
	start := time.Now()
	_, err := r.Run(context.Background(), "sleep", "5")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("error = %v, want a timeout", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("command was not killed promptly: took %s", elapsed)
	}
}

func TestRunner_Lookup(t *testing.T) {
	r := New(time.Second)
	if !r.Lookup("sh") {
		t.Error("Lookup(sh) = false, want true")
	}
	if r.Lookup("prunejuice-nonexistent-binary-zzz") {
		t.Error("Lookup(nonexistent) = true, want false")
	}
}
