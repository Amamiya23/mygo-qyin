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
