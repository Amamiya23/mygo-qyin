package app

import (
	"unicode/utf16"

	"github.com/egoist/mygo/ui"
	"lilt/internal/lyrics"
)

// Keep only enough retained text for the current and outgoing lyric blocks,
// including the taskbar's initial and compact font sizes.
// Position and word brightness never enter these caches.
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

type lyricWordGeometry struct {
	texts       []string
	layout      *ui.TextLayout
	left, right []float32
}

func (g *lyricWordGeometry) matches(words []lyrics.Word) bool {
	if g == nil || len(g.texts) != len(words) {
		return false
	}
	for i, word := range words {
		if g.texts[i] != word.Text {
			return false
		}
	}
	return true
}

type lyricWordLayoutKey struct {
	font       ui.Font
	shape      ui.TextShape
	singleLine bool
}

type lyricWordLayoutEntry struct {
	key      lyricWordLayoutKey
	geometry *lyricWordGeometry
}

type lyricWordLayoutCache struct {
	entries [lyricLayoutCacheLimit]lyricWordLayoutEntry
	count   int
}

func (cache *lyricWordLayoutCache) get(words []lyrics.Word, key lyricWordLayoutKey) *lyricWordGeometry {
	for i := 0; i < cache.count; i++ {
		entry := cache.entries[i]
		if entry.key == key && entry.geometry.matches(words) {
			copy(cache.entries[1:i+1], cache.entries[:i])
			cache.entries[0] = entry
			return entry.geometry
		}
	}
	g := &lyricWordGeometry{texts: make([]string, len(words))}
	spans := make([]ui.Span, len(words))
	for i, word := range words {
		g.texts[i] = word.Text
		text := word.Text
		if key.singleLine {
			text = taskbarLyricSingleLine(text)
		}
		spans[i] = ui.Span{Text: text}
	}
	// Keep the full sentence together so shaping across word boundaries,
	// including ligatures and bidirectional text, is unchanged.
	g.layout = ui.ShapeRichTextWith(spans, key.font, key.shape)
	if key.singleLine {
		g.left, g.right = make([]float32, len(words)), make([]float32, len(words))
		units := 0
		for i, span := range spans {
			g.left[i] = g.layout.Caret(units).X
			for _, r := range span.Text {
				units += utf16.RuneLen(r)
			}
			g.right[i] = g.layout.Caret(units).X
		}
	}
	copy(cache.entries[1:], cache.entries[:lyricLayoutCacheLimit-1])
	cache.entries[0] = lyricWordLayoutEntry{key: key, geometry: g}
	cache.count = min(cache.count+1, lyricLayoutCacheLimit)
	return g
}

func retainedLyricWords(c *ui.Context, words []lyrics.Word, font ui.Font, shape ui.TextShape, singleLine bool) *lyricWordGeometry {
	cache := ui.Local(c.Root(), "lyric-word-layouts", func() lyricWordLayoutCache { return lyricWordLayoutCache{} })
	return cache.get(words, lyricWordLayoutKey{font: font, shape: shape, singleLine: singleLine})
}

// Player publishes complete replacement slices. Holding the first line keeps
// that publication alive and lets each external window scan its language once.
type lyricTranslationPolicy struct {
	first *lyrics.Line
	count int
	value bool
}

func (cache *lyricTranslationPolicy) get(lines []lyrics.Line, policy func([]lyrics.Line) bool) bool {
	var first *lyrics.Line
	if len(lines) > 0 {
		first = &lines[0]
	}
	if first != cache.first || len(lines) != cache.count {
		cache.first, cache.count, cache.value = first, len(lines), policy(lines)
	} else if first == nil {
		return policy(lines)
	}
	return cache.value
}
