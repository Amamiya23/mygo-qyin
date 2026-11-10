//go:build linux && (amd64 || arm64)

package linux

import (
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

func TestLayerFractionalScaleBottomEdge(t *testing.T) {
	gtk := gdkRectangle{Width: 1280, Height: 800}
	logical := gdkRectangle{Width: 1707, Height: 1067}
	work := layerLogicalWorkArea(gtk, gtk, logical)
	if work != logical {
		t.Fatalf("work area still uses integer scale: %+v", work)
	}
	r := platform.Rect{X: 400, Y: 2000, Width: 760, Height: 151}
	r = clampLayerDrag(r, logical)
	if r.Y+r.Height != 1067 {
		t.Fatalf("drag stopped above the compositor's bottom edge: %+v", r)
	}
	// Display changes and content-height adjustments must agree with dragging.
	if r.Y+r.Height != int(work.Y+work.Height) {
		t.Fatal("work area and drag boundary disagree")
	}
}

func TestLayerLogicalWorkAreaInsets(t *testing.T) {
	gtk := gdkRectangle{X: -1280, Y: 20, Width: 1280, Height: 800}
	logical := gdkRectangle{X: -1707, Y: 20, Width: 1707, Height: 1067}
	work := gtk
	work.Y += 24
	work.Height -= 64
	want := logical
	want.Y += 24
	want.Height -= 64
	if got := layerLogicalWorkArea(gtk, work, logical); got != want {
		t.Fatalf("lost work area insets: got %+v, want %+v", got, want)
	}
	r := clampLayerDrag(platform.Rect{X: -3000, Y: 3000, Width: 760, Height: 151}, logical)
	if r.X != -1707 || r.Y+r.Height != 1087 {
		t.Fatalf("wrong boundary on negative-origin output: %+v", r)
	}
	if got := layerLogicalWorkArea(gtk, work, gtk); got != work {
		t.Fatalf("changed a display without fractional scaling: %+v", got)
	}
}
