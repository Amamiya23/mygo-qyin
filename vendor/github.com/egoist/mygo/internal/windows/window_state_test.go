//go:build windows && (amd64 || arm64)

package windows

import (
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

// TestRestoreStateBounds preserves physical placement; the regression selected a monitor by treating saved DIPs as pixels.
func TestRestoreStateBounds(t *testing.T) {
	primary := stateDisplay{
		bounds: platform.Rect{Width: 3072, Height: 1920},
		work:   platform.Rect{Width: 3072, Height: 1880},
		dpi:    192,
	}
	right := func(dpi int) []stateDisplay {
		return []stateDisplay{primary, {
			bounds: platform.Rect{X: 3072, Width: 2560, Height: 1600},
			work:   platform.Rect{X: 3072, Width: 2560, Height: 1560},
			dpi:    dpi,
		}}
	}
	size := platform.Size{Width: 900, Height: 600}
	tests := []struct {
		name     string
		saved    platform.Rect
		size     platform.Size
		displays []stateDisplay
		want     platform.Rect
		ok       bool
	}{
		{
			name:  "200 and 125 percent",
			saved: platform.Rect{X: 3200, Y: 100, Width: 1125, Height: 750},
			size:  size, displays: right(120),
			want: platform.Rect{X: 3200, Y: 100, Width: 1125, Height: 750}, ok: true,
		},
		{
			name:  "200 and 175 percent",
			saved: platform.Rect{X: 3200, Y: 100, Width: 1575, Height: 1050},
			size:  size, displays: right(168),
			want: platform.Rect{X: 3200, Y: 100, Width: 1575, Height: 1050}, ok: true,
		},
		{
			name:  "reported 4K primary and vertically offset secondary",
			saved: platform.Rect{X: 3940, Y: 747, Width: 1575, Height: 1050},
			size:  size,
			displays: []stateDisplay{
				{bounds: platform.Rect{Width: 3840, Height: 2160}, work: platform.Rect{Width: 3840, Height: 2120}, dpi: 192},
				{bounds: platform.Rect{X: 3840, Y: 647, Width: 2560, Height: 1600}, work: platform.Rect{X: 3840, Y: 647, Width: 2560, Height: 1560}, dpi: 168},
			},
			want: platform.Rect{X: 3940, Y: 747, Width: 1575, Height: 1050}, ok: true,
		},
		{
			name:  "equal DPI displays",
			saved: platform.Rect{X: 3200, Y: 100, Width: 1800, Height: 1200},
			size:  size, displays: right(192),
			want: platform.Rect{X: 3200, Y: 100, Width: 1800, Height: 1200}, ok: true,
		},
		{
			name:  "negative horizontal position",
			saved: platform.Rect{X: -2413, Y: 137, Width: 1125, Height: 750},
			size:  size,
			displays: []stateDisplay{primary, {
				bounds: platform.Rect{X: -2560, Width: 2560, Height: 1600},
				work:   platform.Rect{X: -2560, Width: 2560, Height: 1560}, dpi: 120,
			}},
			want: platform.Rect{X: -2413, Y: 137, Width: 1125, Height: 750}, ok: true,
		},
		{
			name:  "negative vertical position",
			saved: platform.Rect{X: 100, Y: -1501, Width: 1125, Height: 750},
			size:  size,
			displays: []stateDisplay{primary, {
				bounds: platform.Rect{Y: -1600, Width: 2560, Height: 1600},
				work:   platform.Rect{Y: -1600, Width: 2560, Height: 1560}, dpi: 120,
			}},
			want: platform.Rect{X: 100, Y: -1501, Width: 1125, Height: 750}, ok: true,
		},
		{
			name:  "majority overlap chooses secondary DPI",
			saved: platform.Rect{X: 2800, Y: 100, Width: 1125, Height: 750},
			size:  size, displays: right(120),
			want: platform.Rect{X: 2800, Y: 100, Width: 1125, Height: 750}, ok: true,
		},
		{
			name:  "zero origin is a saved position",
			saved: platform.Rect{Width: 1800, Height: 1200},
			size:  size, displays: right(120),
			want: platform.Rect{Width: 1800, Height: 1200}, ok: true,
		},
		{
			name:  "changed display DPI keeps logical size",
			saved: platform.Rect{X: 3200, Y: 100, Width: 1125, Height: 750},
			size:  size, displays: right(168),
			want: platform.Rect{X: 3200, Y: 100, Width: 1575, Height: 1050}, ok: true,
		},
		{
			name:  "disconnected display is offscreen",
			saved: platform.Rect{X: 3200, Y: 100, Width: 1125, Height: 750},
			size:  size, displays: []stateDisplay{primary},
		},
		{
			name:  "no connected displays",
			saved: platform.Rect{X: 3200, Y: 100, Width: 1125, Height: 750}, size: size,
		},
		{
			name:  "minimum horizontal reachability",
			saved: platform.Rect{X: 5507, Y: 100, Width: 1125, Height: 750},
			size:  size, displays: right(120),
			want: platform.Rect{X: 5507, Y: 100, Width: 1125, Height: 750}, ok: true,
		},
		{
			name:  "insufficient horizontal reachability",
			saved: platform.Rect{X: 5508, Y: 100, Width: 1125, Height: 750},
			size:  size, displays: right(120),
		},
		{
			name:  "minimum vertical reachability",
			saved: platform.Rect{X: 3200, Y: 1497, Width: 1125, Height: 750},
			size:  size, displays: right(120),
			want: platform.Rect{X: 3200, Y: 1497, Width: 1125, Height: 750}, ok: true,
		},
		{
			name:  "insufficient vertical reachability",
			saved: platform.Rect{X: 3200, Y: 1498, Width: 1125, Height: 750},
			size:  size, displays: right(120),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := restoreStateBounds(tt.saved, tt.size, tt.displays)
			if ok != tt.ok {
				t.Fatalf("restoreStateBounds() accepted = %v, want %v (bounds %+v)", ok, tt.ok, got)
			}
			if ok && got != tt.want {
				t.Fatalf("restoreStateBounds() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestRestoreStateBoundsRepeated prevents the mixed-DPI restart regression from accumulating position or size drift.
func TestRestoreStateBoundsRepeated(t *testing.T) {
	for _, tt := range []struct {
		name          string
		dpi           int
		width, height int
	}{
		{name: "125 percent secondary", dpi: 120, width: 1125, height: 750},
		{name: "175 percent secondary", dpi: 168, width: 1575, height: 1050},
	} {
		t.Run(tt.name, func(t *testing.T) {
			displays := []stateDisplay{
				{bounds: platform.Rect{Width: 3072, Height: 1920}, work: platform.Rect{Width: 3072, Height: 1880}, dpi: 192},
				{bounds: platform.Rect{X: 3072, Width: 2560, Height: 1600}, work: platform.Rect{X: 3072, Width: 2560, Height: 1560}, dpi: tt.dpi},
			}
			want := platform.Rect{X: 3203, Y: 137, Width: tt.width, Height: tt.height}
			saved, size := want, (platform.Size{Width: 900, Height: 600})
			for launch := 1; launch <= 20; launch++ {
				got, ok := restoreStateBounds(saved, size, displays)
				if !ok || got != want {
					t.Fatalf("launch %d: got %+v, accepted %v; want %+v", launch, got, ok, want)
				}
				saved = got
				size = platform.Size{Width: toDIP(int32(got.Width), tt.dpi), Height: toDIP(int32(got.Height), tt.dpi)}
			}
		})
	}
}
