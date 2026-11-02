//go:build windows && (amd64 || arm64)

package windows

import (
	"unsafe"

	"github.com/egoist/mygo/internal/platform"
)

// stateDisplay keeps every rectangle in physical pixels. Per-display DIP
// rectangles can overlap, so they cannot select a saved window's display.
type stateDisplay struct {
	bounds, work platform.Rect
	dpi          int
}

// WindowStateBounds captures the actual normal rectangle without rounding
// its screen position through the window's current DPI.
func (w *window) WindowStateBounds() platform.Rect {
	var r rect
	procGetWindowRect.Call(w.hwnd, uintptr(unsafe.Pointer(&r)))
	return platform.Rect{X: int(r.Left), Y: int(r.Top), Width: int(r.Right - r.Left), Height: int(r.Bottom - r.Top)}
}

// RestoreWindowState resolves the monitor in physical coordinates before
// converting the saved DIP size. Legacy states have no unambiguous physical
// rectangle and fall back to the core's centered placement once.
func (b *Backend) RestoreWindowState(o *platform.WindowOptions, saved *platform.Rect) bool {
	if saved == nil {
		return false
	}
	var displays []stateDisplay
	enumMonitors(func(m uintptr) {
		info := monitorInfo(m)
		convert := func(r rect) platform.Rect {
			return platform.Rect{X: int(r.Left), Y: int(r.Top), Width: int(r.Right - r.Left), Height: int(r.Bottom - r.Top)}
		}
		displays = append(displays, stateDisplay{bounds: convert(info.Monitor), work: convert(info.Work), dpi: monitorDPI(m)})
	})
	r, ok := restoreStateBounds(*saved, platform.Size{Width: o.Width, Height: o.Height}, displays)
	if !ok {
		return false
	}
	o.StateBounds, o.Center = &r, false
	return true
}

// restoreStateBounds uses the display containing most of the saved window,
// as Windows does for a window spanning displays. Position stays in pixels;
// size follows the target display's current DPI.
func restoreStateBounds(saved platform.Rect, size platform.Size, displays []stateDisplay) (platform.Rect, bool) {
	if saved.Width <= 0 || saved.Height <= 0 || size.Width <= 0 || size.Height <= 0 {
		return platform.Rect{}, false
	}
	target, largest := -1, int64(0)
	for i, d := range displays {
		w, h := stateOverlap(saved, d.bounds)
		if area := int64(w) * int64(h); area > largest {
			target, largest = i, area
		}
	}
	if target < 0 {
		return platform.Rect{}, false
	}
	dpi := displays[target].dpi
	r := platform.Rect{X: saved.X, Y: saved.Y, Width: int(toPx(size.Width, dpi)), Height: int(toPx(size.Height, dpi))}
	for _, d := range displays {
		w, h := stateOverlap(r, d.work)
		if w >= int(toPx(min(100, size.Width), d.dpi)) && h >= int(toPx(min(50, size.Height), d.dpi)) {
			return r, true
		}
	}
	return platform.Rect{}, false
}

func stateOverlap(a, b platform.Rect) (int, int) {
	return max(0, min(a.X+a.Width, b.X+b.Width)-max(a.X, b.X)),
		max(0, min(a.Y+a.Height, b.Y+b.Height)-max(a.Y, b.Y))
}
