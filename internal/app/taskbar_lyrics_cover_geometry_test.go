package app

import "testing"

// The clickable child stays on the cover at every DPI on either edge, without covering lyrics.
func TestTaskbarLyricsCoverPlacement(t *testing.T) {
	tests := []struct {
		name               string
		dpi                uint32
		height, x, y, size int
		rightX             int
	}{
		{"100 percent", 96, 42, 106, 24, 34, 660},
		{"150 percent", 144, 63, 109, 26, 51, 640},
		{"200 percent", 192, 84, 112, 28, 68, 620},
		{"short taskbar", 96, 28, 106, 22, 24, 670},
		{"fractional DPI", 120, 53, 108, 25, 43, 649},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := taskbarLyricsPlacement{parent: 1, taskbar: 2, x: 100, y: 20, width: 600, height: tt.height, dpi: tt.dpi, dark: true}
			p := taskbarLyricsCoverPlacement(base)
			if p.x != tt.x || p.y != tt.y || p.width != tt.size || p.height != tt.size || p.parent != 1 || p.taskbar != 2 || p.dpi != tt.dpi || !p.dark {
				t.Fatalf("unexpected left placement: %+v", p)
			}
			base.rightAligned = true
			rp := taskbarLyricsCoverPlacement(base)
			if rp.x != tt.rightX || rp.y != tt.y || rp.width != tt.size || rp.height != tt.size || !rp.rightAligned {
				t.Fatalf("unexpected right placement: %+v", rp)
			}
		})
	}
}

// Hidden or impossibly small taskbars never leave an interactive cover child.
func TestTaskbarLyricsCoverPlacementUnavailable(t *testing.T) {
	for _, p := range []taskbarLyricsPlacement{{}, {parent: 1, width: 600, height: 4, dpi: 96}, {parent: 1, width: 10, height: 42, dpi: 96}} {
		for _, right := range []bool{false, true} {
			p.rightAligned = right
			if got := taskbarLyricsCoverPlacement(p); got != (taskbarLyricsPlacement{}) {
				t.Fatalf("unexpected interactive area (right=%t): %+v", right, got)
			}
		}
	}
}
