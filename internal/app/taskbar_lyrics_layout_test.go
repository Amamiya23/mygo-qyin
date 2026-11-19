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

// Centered taskbars stay strictly on the left and hide when it fills; left-aligned taskbars anchor beside the right tray without bouncing as apps open.
func TestTaskbarLyricsPlacementSpace(t *testing.T) {
	tests := []struct {
		name         string
		leftTaskbar  bool
		occupied     []taskbarLyricsInterval
		start, width int
	}{
		{"centered left gap", false, []taskbarLyricsInterval{{400, 600}, {900, 1000}}, 8, 280},
		{"centered left widgets", false, []taskbarLyricsInterval{{0, 150}, {460, 600}, {900, 1000}}, 158, 280},
		{"centered left full hides instead of jumping right", false, []taskbarLyricsInterval{{0, 150}, {220, 620}, {900, 1000}}, 0, 0},
		{"centered pushed to left hides", false, []taskbarLyricsInterval{{0, 600}, {880, 1000}}, 0, 0},
		{"left taskbar anchors beside tray", true, []taskbarLyricsInterval{{0, 300}, {880, 1000}}, 592, 280},
		{"left taskbar stays fixed when more apps open", true, []taskbarLyricsInterval{{0, 500}, {880, 1000}}, 592, 280},
		{"left taskbar shrinks from left when crowded", true, []taskbarLyricsInterval{{0, 700}, {880, 1000}}, 708, 164},
		{"classic Win10 ReBar anchors to right edge", true, []taskbarLyricsInterval{{0, 400}}, 712, 280},
		{"left taskbar full hides", true, []taskbarLyricsInterval{{0, 800}, {880, 1000}}, 0, 0},
		{"left taskbar unknown hides", true, nil, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, width := taskbarLyricsPlacementSpace(1000, 280, 100, 8, tt.occupied, tt.leftTaskbar)
			if start != tt.start || width != tt.width {
				t.Fatalf("got (%d,%d), want (%d,%d)", start, width, tt.start, tt.width)
			}
		})
	}
}

// Right-aligned taskbars flush short lines to the right edge while overflowing lines still scroll left-to-right with the sung word.
func TestTaskbarLyricDrawX(t *testing.T) {
	for _, tc := range []struct {
		x, available, width, progress float32
		rightAligned                  bool
		want                          float32
	}{
		{10, 100, 80, 0, false, 10},
		{10, 100, 80, 0, true, 30},
		{10, 100, 300, 0, false, 10},
		{10, 100, 300, 0, true, 10},
		{10, 100, 300, 100, false, -35},
		{10, 100, 300, 100, true, -35},
		{10, 100, 300, 500, false, -190},
		{10, 100, 300, 500, true, -190},
	} {
		if got := taskbarLyricDrawX(tc.x, tc.available, tc.width, tc.progress, tc.rightAligned); got != tc.want {
			t.Errorf("drawX(%v,%v,%v,%v,right=%t)=%v want=%v", tc.x, tc.available, tc.width, tc.progress, tc.rightAligned, got, tc.want)
		}
	}
}
