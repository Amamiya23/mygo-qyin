package app

import "sort"

const (
	taskbarLyricsEdgeGap    = 8
	taskbarLyricsCoverInset = 6
	// Match the visible margin from the taskbar edge to the album cover.
	taskbarLyricsCoverGap = taskbarLyricsEdgeGap + taskbarLyricsCoverInset
	// The offset input's bounds, far past any taskbar's width.
	taskbarLyricsOffsetLimit = 5000
)

const (
	taskbarAlignLeft  = "left"
	taskbarAlignRight = "right"
)

func (s settings) taskbarLyricsAlign() string {
	if s.TaskbarLyricsAlign == taskbarAlignRight {
		return taskbarAlignRight
	}
	return taskbarAlignLeft
}

// Mirroring is offered only beside the right edge; a saved switch stays inert on the left.
func (s settings) taskbarLyricsMirrored() bool {
	return s.TaskbarLyricsMirror && s.taskbarLyricsAlign() == taskbarAlignRight
}

// Native placement uses physical pixels in the Explorer parent's client space.
type taskbarLyricsPlacement struct {
	parent, taskbar     uintptr
	x, y, width, height int
	dpi                 uint32
	dark                bool
	// noSpace marks a sample where the taskbar exists but offers no usable gap.
	noSpace bool
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

// Right alignment mirrors the taskbar so the same leftmost-gap scan picks the
// rightmost usable gap, beside the notification area, then mirrors it back.
func taskbarLyricsFreeSpaceRight(length, requested, minimum, gap int, occupied []taskbarLyricsInterval) (int, int) {
	mirrored := make([]taskbarLyricsInterval, len(occupied))
	for i, item := range occupied {
		mirrored[i] = taskbarLyricsInterval{length - item.end, length - item.start}
	}
	x, width := taskbarLyricsFreeSpace(length, requested, minimum, gap, mirrored)
	if width == 0 {
		return 0, 0
	}
	return length - x - width, width
}

// The listener's nudge, in logical pixels, never pushes the lyrics off the taskbar.
func taskbarLyricsShiftX(x, width, length, offset int, dpi uint32) int {
	if offset == 0 {
		return x
	}
	return max(0, min(length-width, x+offset*int(dpi)/96))
}
