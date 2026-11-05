package app

import "math"

// The interactive native child covers only the album art. The lyric window
// remains entirely click-through, including the text and surrounding space.
func taskbarLyricsCoverPlacement(p taskbarLyricsPlacement) taskbarLyricsPlacement {
	if p.parent == 0 || p.width <= 0 || p.height <= 0 {
		return taskbarLyricsPlacement{}
	}
	scale := float64(max(p.dpi, 96)) / 96
	size := int(math.Round(min(34*scale, float64(p.height)-4*scale)))
	left := int(math.Round(taskbarLyricsCoverInset * scale))
	if size <= 0 || left+size > p.width {
		return taskbarLyricsPlacement{}
	}
	p.x += left
	p.y += int(math.Round(float64(p.height-size) / 2))
	p.width, p.height = size, size
	return p
}
