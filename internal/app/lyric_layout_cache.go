package app

import "github.com/egoist/mygo/ui"

// Keep only enough retained text for the current and outgoing lyric blocks,
// including the taskbar's initial and compact font sizes.
// Position and word brightness never enter this cache: changing word colors
// still uses its own layout, while outlines and secondary sentences stay put.
const lyricLayoutCacheLimit = 16

type lyricLayoutKey struct {
	text  string
	font  ui.Font
	shape ui.TextShape
	color ui.Color
}

type lyricLayoutEntry struct {
	key    lyricLayoutKey
	layout *ui.TextLayout
}

type lyricLayoutCache struct {
	entries [lyricLayoutCacheLimit]lyricLayoutEntry
	count   int
}

func (cache *lyricLayoutCache) get(key lyricLayoutKey) *ui.TextLayout {
	for i := 0; i < cache.count; i++ {
		if cache.entries[i].key == key {
			entry := cache.entries[i]
			copy(cache.entries[1:i+1], cache.entries[:i])
			cache.entries[0] = entry
			return entry.layout
		}
	}
	layout := ui.ShapeRichTextWith([]ui.Span{{Text: key.text, Color: key.color}}, key.font, key.shape)
	copy(cache.entries[1:], cache.entries[:lyricLayoutCacheLimit-1])
	cache.entries[0] = lyricLayoutEntry{key: key, layout: layout}
	cache.count = min(cache.count+1, lyricLayoutCacheLimit)
	return layout
}

func retainedLyricText(c *ui.Context, text string, font ui.Font, shape ui.TextShape, color ui.Color) *ui.TextLayout {
	cache := ui.Local(c.Root(), "lyric-text-layouts", func() lyricLayoutCache { return lyricLayoutCache{} })
	return cache.get(lyricLayoutKey{text: text, font: font, shape: shape, color: color})
}
