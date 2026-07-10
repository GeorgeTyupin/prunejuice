// Package domain holds the core entities and port interfaces of prunejuice.
//
// It has no dependencies outside the standard library. Every other layer
// (service, adapters, config, cmd) depends on this package and never the
// other way around — that is the dependency rule of clean architecture.
package domain

// bytesPerMB is used to render byte counts as megabytes for humans.
const bytesPerMB = 1024 * 1024

// DiskUsage is a point-in-time snapshot of a single mount point.
type DiskUsage struct {
	// Path is the mount point that was measured, e.g. "/".
	Path string
	// TotalBytes is the size of the filesystem.
	TotalBytes uint64
	// UsedBytes is how much is currently occupied.
	UsedBytes uint64
	// FreeBytes is how much is still available.
	FreeBytes uint64
	// UsedPercent is UsedBytes/TotalBytes expressed as 0..100.
	UsedPercent float64
}

// UsedMB returns the used space in megabytes.
func (u DiskUsage) UsedMB() float64 { return float64(u.UsedBytes) / bytesPerMB }

// FreeMB returns the free space in megabytes.
func (u DiskUsage) FreeMB() float64 { return float64(u.FreeBytes) / bytesPerMB }

// TotalMB returns the total space in megabytes.
func (u DiskUsage) TotalMB() float64 { return float64(u.TotalBytes) / bytesPerMB }

// OverThreshold reports whether the used percentage is at or above threshold.
//
// This is the single source of truth for "is the disk too full?" and is kept
// deliberately trivial so it can be unit-tested in isolation.
func (u DiskUsage) OverThreshold(threshold float64) bool {
	return u.UsedPercent >= threshold
}
