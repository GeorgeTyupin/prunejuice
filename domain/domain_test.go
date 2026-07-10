package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestDiskUsage_OverThreshold(t *testing.T) {
	cases := []struct {
		used      float64
		threshold float64
		want      bool
	}{
		{84.9, 85, false},
		{85, 85, true}, // boundary is inclusive
		{85.1, 85, true},
		{0, 85, false},
		{100, 85, true},
	}
	for _, c := range cases {
		got := DiskUsage{UsedPercent: c.used}.OverThreshold(c.threshold)
		if got != c.want {
			t.Errorf("OverThreshold(used=%v, thr=%v) = %v, want %v", c.used, c.threshold, got, c.want)
		}
	}
}

func TestReport_NeedsAlert(t *testing.T) {
	overStep := StepResult{Step: CleanupStep{Name: "x"}}
	failStep := StepResult{Step: CleanupStep{Name: "y"}, Err: errors.New("boom")}

	cases := []struct {
		name   string
		report Report
		want   bool
	}{
		{
			name:   "not triggered",
			report: Report{Threshold: 85, FinalUsage: DiskUsage{UsedPercent: 50}},
			want:   false,
		},
		{
			name:   "triggered and resolved",
			report: Report{Threshold: 85, Triggered: true, FinalUsage: DiskUsage{UsedPercent: 70}, Steps: []StepResult{overStep}},
			want:   false,
		},
		{
			name:   "triggered and unresolved",
			report: Report{Threshold: 85, Triggered: true, FinalUsage: DiskUsage{UsedPercent: 92}},
			want:   true,
		},
		{
			name:   "fatal error",
			report: Report{Threshold: 85, Err: errors.New("no disk")},
			want:   true,
		},
		{
			name:   "step failed but resolved",
			report: Report{Threshold: 85, Triggered: true, FinalUsage: DiskUsage{UsedPercent: 70}, Steps: []StepResult{failStep}},
			want:   true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.report.NeedsAlert(); got != c.want {
				t.Errorf("NeedsAlert() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestReport_TotalFreedBytes(t *testing.T) {
	r := Report{Steps: []StepResult{
		{FreedBytes: 100},
		{FreedBytes: 250},
		{Skipped: true, FreedBytes: 0},
	}}
	if got := r.TotalFreedBytes(); got != 350 {
		t.Errorf("TotalFreedBytes() = %d, want 350", got)
	}
}

func TestAlert_Text_ContainsKeyFacts(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	r := &Report{
		Host:       "sof-tunnel",
		Path:       "/",
		Threshold:  85,
		Triggered:  true,
		FinalUsage: DiskUsage{UsedPercent: 91.5, UsedBytes: 9600 * 1024 * 1024, TotalBytes: 10000 * 1024 * 1024, FreeBytes: 400 * 1024 * 1024},
		Steps: []StepResult{
			{Step: CleanupStep{Name: "journal-vacuum"}, FreedBytes: 120 * 1024 * 1024, Duration: 2 * time.Second},
			{Step: CleanupStep{Name: "docker-prune"}, Skipped: true, SkipReason: "disabled in config"},
		},
	}
	alert := NewDiskFullAlert(r, now)
	text := alert.Text()

	for _, want := range []string{"DISK FULL", "sof-tunnel", "91.5%", "threshold: 85", "journal-vacuum", "docker-prune", "disabled in config", "2026-07-10"} {
		if !strings.Contains(text, want) {
			t.Errorf("alert text missing %q\n---\n%s", want, text)
		}
	}
}

func TestAlert_Text_ErrorOnly(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	alert := NewErrorAlert("box-1", errors.New("exec journalctl: not found"), nil, now)
	text := alert.Text()
	if !strings.Contains(text, "ERROR") || !strings.Contains(text, "not found") {
		t.Errorf("error alert text unexpected:\n%s", text)
	}
}
