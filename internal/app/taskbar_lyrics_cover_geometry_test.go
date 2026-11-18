package app

import "testing"

// The clickable child stays on the cover at every DPI, without covering lyrics.
func TestTaskbarLyricsCoverPlacement(t *testing.T) {
	tests := []struct {
		name               string
		dpi                uint32
		height, x, y, size int
		mirrorX            int
	}{
		{"100 percent", 96, 42, 106, 24, 34, 660},
		{"150 percent", 144, 63, 109, 26, 51, 640},
		{"200 percent", 192, 84, 112, 28, 68, 620},
		{"short taskbar", 96, 28, 106, 22, 24, 670},
		{"fractional DPI", 120, 53, 108, 25, 43, 649},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := taskbarLyricsPlacement{parent: 1, taskbar: 2, x: 100, y: 20, width: 600, height: tt.height, dpi: tt.dpi, dark: true}
			got := taskbarLyricsCoverPlacement(p, false)
			if got.x != tt.x || got.y != tt.y || got.width != tt.size || got.height != tt.size || got.parent != 1 || got.taskbar != 2 || got.dpi != tt.dpi || !got.dark {
				t.Fatalf("unexpected placement: %+v", got)
			}
			mirrored := taskbarLyricsCoverPlacement(p, true)
			if mirrored.x != tt.mirrorX || mirrored.y != tt.y || mirrored.width != tt.size || mirrored.height != tt.size {
				t.Fatalf("unexpected mirrored placement: %+v", mirrored)
			}
		})
	}
}

// Hidden or impossibly small taskbars never leave an interactive cover child.
func TestTaskbarLyricsCoverPlacementUnavailable(t *testing.T) {
	for _, p := range []taskbarLyricsPlacement{{}, {parent: 1, width: 600, height: 4, dpi: 96}, {parent: 1, width: 10, height: 42, dpi: 96}} {
		for _, mirror := range []bool{false, true} {
			if got := taskbarLyricsCoverPlacement(p, mirror); got != (taskbarLyricsPlacement{}) {
				t.Fatalf("unexpected interactive area: %+v", got)
			}
		}
	}
}
