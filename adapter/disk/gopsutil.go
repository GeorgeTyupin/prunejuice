// Package disk adapts gopsutil to the domain.DiskProber port.
package disk

import (
	"context"
	"fmt"

	gopsutil "github.com/shirou/gopsutil/v3/disk"

	"github.com/GeorgeTyupin/prunejuice/domain"
)

// Prober measures real disk usage via gopsutil.
type Prober struct{}

// New returns a Prober.
func New() *Prober { return &Prober{} }

// Usage implements domain.DiskProber.
func (Prober) Usage(ctx context.Context, path string) (domain.DiskUsage, error) {
	st, err := gopsutil.UsageWithContext(ctx, path)
	if err != nil {
		return domain.DiskUsage{}, fmt.Errorf("read disk usage for %q: %w", path, err)
	}
	return domain.DiskUsage{
		Path:        path,
		TotalBytes:  st.Total,
		UsedBytes:   st.Used,
		FreeBytes:   st.Free,
		UsedPercent: st.UsedPercent,
	}, nil
}
