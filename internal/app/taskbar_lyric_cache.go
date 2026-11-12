package app

import "github.com/egoist/mygo/ui"

type taskbarLyricLayoutKey struct {
	text, secondary, family string
	height                  float32
	dark, preview, timed    bool
}

type taskbarLyricLayoutEntry struct {
	key    taskbarLyricLayoutKey
	layout taskbarLyricLayout
}

type taskbarLyricLayoutCache struct {
	entries [lyricLayoutCacheLimit]taskbarLyricLayoutEntry
	count   int
}

// Retain the final fitted size, not just its text. A compact taskbar otherwise
// measures the large sentence and then its smaller replacement on every tick.
func retainedTaskbarLyricLayout(c *ui.Context, content desktopLyricContent, dark bool, height float32) taskbarLyricLayout {
	secondary := content.translation
	preview := secondary == "" && content.preview != ""
	if preview {
		secondary = content.preview
	}
	key := taskbarLyricLayoutKey{
		text: content.text, secondary: secondary, family: c.Theme().Font,
		height: height, dark: dark, preview: preview,
		timed: content.line.Timed && len(content.line.Words) > 0,
	}
	cache := ui.Local(c.Root(), "taskbar-fitted-lyrics", func() taskbarLyricLayoutCache { return taskbarLyricLayoutCache{} })
	for i := 0; i < cache.count; i++ {
		entry := cache.entries[i]
		if entry.key == key && (!key.timed || entry.layout.words.matches(content.line.Words)) {
			copy(cache.entries[1:i+1], cache.entries[:i])
			cache.entries[0] = entry
			return entry.layout
		}
	}
	text := taskbarLyricSingleLine(content.text)
	secondary = taskbarLyricSingleLine(secondary)
	shape := func(size float32) taskbarLyricLayout {
		font := ui.Font{Family: key.family, Size: size, Weight: 600, LineHeight: 1.15}
		layout := taskbarLyricLayout{
			outline: retainedLyricText(c, text, font, ui.TextShape{}, ui.Color{}),
			preview: preview,
		}
		if key.timed {
			layout.words = retainedLyricWords(c, content.line.Words, font, ui.TextShape{}, true)
			layout.text = layout.words.layout
		} else {
			layout.text = retainedLyricText(c, text, font, ui.TextShape{}, radixGray(dark, 12))
		}
		layout.width, layout.originalHeight = layout.text.Size()
		layout.height = layout.originalHeight
		if secondary != "" {
			layout.secondary = retainedLyricText(c, secondary, font, ui.TextShape{}, ui.Color{})
			layout.secondaryOutline = layout.secondary
			var h float32
			layout.secondaryWidth, h = layout.secondary.Size()
			layout.height += h + 1
		}
		return layout
	}
	layout := shape(16)
	if layout.height > max(1, height-2) {
		gap := float32(0)
		if layout.secondary != nil {
			gap = 1
		}
		// Preserve both lines and the existing compact-height fitting formula.
		layout = shape(max(1, 16*max(1, height-2-gap)/(layout.height-gap)))
	}
	copy(cache.entries[1:], cache.entries[:lyricLayoutCacheLimit-1])
	cache.entries[0] = taskbarLyricLayoutEntry{key: key, layout: layout}
	cache.count = min(cache.count+1, lyricLayoutCacheLimit)
	return layout
}
