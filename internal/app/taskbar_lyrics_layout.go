package app

import "sort"

const (
	taskbarLyricsEdgeGap    = 8
	taskbarLyricsCoverInset = 6
	// Match the visible margin from the taskbar edge to the album cover.
	taskbarLyricsCoverGap = taskbarLyricsEdgeGap + taskbarLyricsCoverInset
)

// Native placement uses physical pixels in the Explorer parent's client space.
type taskbarLyricsPlacement struct {
	parent, taskbar     uintptr
	x, y, width, height int
	dpi                 uint32
	dark                bool
}

type taskbarLyricsInterval struct{ start, end int }

// Find a real gap without moving Explorer's buttons. No occupancy means unknown.
func taskbarLyricsFreeSpace(length, requested, minimum, gap int, occupied []taskbarLyricsInterval) (int, int) {
	if length <= 0 || requested < minimum || minimum <= 0 || gap < 0 || len(occupied) == 0 {
		return 0, 0
	}
	blocked := make([]taskbarLyricsInterval, 0, len(occupied))
	for _, item := range occupied {
		if item.end <= item.start || item.end <= 0 || item.start >= length {
			continue
		}
		blocked = append(blocked, taskbarLyricsInterval{max(0, item.start-gap), min(length, item.end+gap)})
	}
	if len(blocked) == 0 {
		return 0, 0
	}
	sort.Slice(blocked, func(i, j int) bool { return blocked[i].start < blocked[j].start })
	// Prefer the leftmost usable space, even when another gap would be wider.
	// Keep the same margin at the outer taskbar edges as around its controls.
	cursor := gap
	for _, item := range blocked {
		if width := min(requested, item.start-cursor); width >= minimum {
			return cursor, width
		}
		cursor = max(cursor, item.end)
	}
	if width := min(requested, length-gap-cursor); width >= minimum {
		return cursor, width
	}
	return 0, 0
}
