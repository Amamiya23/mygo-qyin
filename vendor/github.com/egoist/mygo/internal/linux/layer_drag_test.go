//go:build linux && (amd64 || arm64)

package linux

import (
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

// Layer panels move with the held pointer and stop on release instead of using xdg-toplevel dragging.
func TestLayerDrag(t *testing.T) {
	d := layerDrag{x: 20, y: 30, pending: true}
	r := platform.Rect{X: 100, Y: 200, Width: 400, Height: 100}
	if next, moved := d.move(r, 25, 33, true); moved || next != r {
		t.Fatal("moved before the drag threshold")
	}
	next, moved := d.move(r, 30, 36, true)
	if !moved || next.X != 110 || next.Y != 206 {
		t.Fatalf("first move: %+v, moved=%v", next, moved)
	}
	// Once the surface moves, its held point returns to the original local position.
	r = next
	next, moved = d.move(r, 23, 28, true)
	if !moved || next.X != 113 || next.Y != 204 {
		t.Fatalf("continued move: %+v, moved=%v", next, moved)
	}
	if _, moved = d.move(next, 40, 40, false); moved || d.active || d.pending {
		t.Fatal("release did not stop the drag")
	}
	if _, moved = d.move(next, 50, 50, true); moved {
		t.Fatal("motion after release restarted the drag")
	}
}
