package app

import "testing"

// Occupied buttons and their safety margins never overlap the selected lyrics gap.
func TestTaskbarLyricsFreeSpace(t *testing.T) {
	tests := []struct {
		name         string
		occupied     []taskbarLyricsInterval
		start, width int
	}{
		{"centered buttons", []taskbarLyricsInterval{{400, 600}, {900, 1000}}, 8, 280},
		{"left before wider right gap", []taskbarLyricsInterval{{180, 500}}, 8, 164},
		{"left widgets", []taskbarLyricsInterval{{0, 150}, {500, 800}}, 158, 280},
		{"overlapping intervals", []taskbarLyricsInterval{{400, 800}, {0, 450}, {900, 1000}}, 0, 0},
		{"narrow but usable", []taskbarLyricsInterval{{0, 600}, {800, 1000}}, 608, 184},
		{"full", []taskbarLyricsInterval{{0, 1000}}, 0, 0},
		{"unknown", nil, 0, 0},
		{"invalid", []taskbarLyricsInterval{{20, 10}}, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, width := taskbarLyricsFreeSpace(1000, 280, 100, 8, tt.occupied)
			if start != tt.start || width != tt.width {
				t.Fatalf("got (%d,%d), want (%d,%d)", start, width, tt.start, tt.width)
			}
		})
	}
}

// Right alignment keeps the same margins beside the rightmost gap's controls.
func TestTaskbarLyricsFreeSpaceRight(t *testing.T) {
	tests := []struct {
		name         string
		occupied     []taskbarLyricsInterval
		start, width int
	}{
		{"beside notification area", []taskbarLyricsInterval{{400, 600}, {900, 1000}}, 612, 280},
		{"right after widgets", []taskbarLyricsInterval{{0, 150}, {500, 800}}, 808, 184},
		{"right of center buttons", []taskbarLyricsInterval{{180, 500}}, 712, 280},
		{"single middle gap", []taskbarLyricsInterval{{0, 600}, {800, 1000}}, 608, 184},
		{"overlapping intervals", []taskbarLyricsInterval{{400, 800}, {0, 450}, {900, 1000}}, 0, 0},
		{"full", []taskbarLyricsInterval{{0, 1000}}, 0, 0},
		{"unknown", nil, 0, 0},
		{"invalid", []taskbarLyricsInterval{{20, 10}}, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, width := taskbarLyricsFreeSpaceRight(1000, 280, 100, 8, tt.occupied)
			if start != tt.start || width != tt.width {
				t.Fatalf("got (%d,%d), want (%d,%d)", start, width, tt.start, tt.width)
			}
		})
	}
}

// The horizontal nudge scales with DPI and never leaves the taskbar.
func TestTaskbarLyricsShiftX(t *testing.T) {
	tests := []struct {
		name              string
		x, width, offset  int
		dpi               uint32
		want              int
	}{
		{"zero keeps placement", 100, 280, 0, 96, 100},
		{"positive moves right", 100, 280, 50, 96, 150},
		{"negative moves left", 100, 280, -50, 96, 50},
		{"scales with dpi", 100, 280, 50, 192, 200},
		{"clamped at left edge", 100, 280, -500, 96, 0},
		{"clamped at right edge", 700, 280, 500, 96, 720},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := taskbarLyricsShiftX(tt.x, tt.width, 1000, tt.offset, tt.dpi); got != tt.want {
				t.Fatalf("got %d, want %d", got, tt.want)
			}
		})
	}
}
