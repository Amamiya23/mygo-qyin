package app

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"lilt/internal/lyrics"
)

// Playback changes used to reshape every layer; only timed foreground colors need a fresh layout.
func TestLyricLayoutsRetainStableLayers(t *testing.T) {
	for _, timed := range []bool{false, true} {
		for _, secondary := range []string{"translation", "preview"} {
			t.Run(fmt.Sprintf("timed=%t/%s", timed, secondary), func(t *testing.T) {
				line := lyrics.Line{At: time.Second, End: 8 * time.Second, Text: "Stay with me", Timed: timed,
					Words: []lyrics.Word{{Text: "Stay ", Start: time.Second, End: 3 * time.Second}, {Text: "with me", Start: 3 * time.Second, End: 8 * time.Second}}}
				content := desktopLyricContent{line: line, text: line.Text, index: 0}
				if secondary == "translation" {
					content.translation = "留在我身边"
				} else {
					content.preview = "Through the night"
				}
				position := 1500 * time.Millisecond
				var desktop desktopLyricLayouts
				dt := ui.NewTester(func(c *ui.Context) {
					desktop = desktopLyricsLayouts(c, content, position, 32, 300, radixPaper)
				}, 320, 160)
				dt.Frame()
				firstDesktop := desktop
				position = 2500 * time.Millisecond
				dt.Frame()
				if desktop.outline != firstDesktop.outline || desktop.translation != firstDesktop.translation || desktop.preview != firstDesktop.preview {
					t.Fatal("playback reshaped a stable desktop lyric layer")
				}
				if timed == (desktop.text == firstDesktop.text) {
					t.Fatal("desktop foreground either lost word colors or reshaped unchanged LRC")
				}

				for _, height := range []float32{28, 42} {
					position = 1500 * time.Millisecond
					var taskbar taskbarLyricLayout
					tt := ui.NewTester(func(c *ui.Context) {
						taskbar = makeTaskbarLyricLayout(c, content, position, true, height)
					}, 280, int(height))
					tt.Frame()
					firstTaskbar := taskbar
					position = 2500 * time.Millisecond
					tt.Frame()
					if taskbar.outline != firstTaskbar.outline || taskbar.secondary != firstTaskbar.secondary {
						t.Fatal("playback reshaped a stable taskbar lyric layer")
					}
					if timed == (taskbar.text == firstTaskbar.text) || taskbar.progressX <= firstTaskbar.progressX {
						t.Fatal("taskbar cached stale brightness or scrolling progress")
					}
				}
			})
		}
	}
}

// Retained text must change with its content and visual inputs, rather than reuse another song or size.
func TestLyricLayoutCacheInputs(t *testing.T) {
	key := lyricLayoutKey{text: "Stay with me", font: ui.Font{Size: 32, Weight: 700}, shape: ui.TextShape{Width: 300, Align: ui.Center}, color: radixPaper}
	var cache lyricLayoutCache
	initial := cache.get(key)
	for _, change := range []struct {
		name  string
		apply func(*lyricLayoutKey)
	}{
		{"text", func(k *lyricLayoutKey) { k.text = "Through the night" }},
		{"font", func(k *lyricLayoutKey) { k.font.Family = "serif" }},
		{"size", func(k *lyricLayoutKey) { k.font.Size = 40 }},
		{"width", func(k *lyricLayoutKey) { k.shape.Width = 150 }},
		{"alignment", func(k *lyricLayoutKey) { k.shape.Align = ui.Start }},
		{"theme color", func(k *lyricLayoutKey) { k.color = radixGray(true, 12) }},
	} {
		t.Run(change.name, func(t *testing.T) {
			changed := key
			change.apply(&changed)
			layout := cache.get(changed)
			if layout == initial || cache.get(changed) != layout {
				t.Fatal("changed lyric style reused stale geometry or did not retain its replacement")
			}
		})
	}
	if cache.get(key) != initial {
		t.Fatal("returning to the same lyric lost its retained layout")
	}
}

// Changing desktop content, size, width or theme must replace its corresponding retained layers.
func TestDesktopLyricLayoutsRefreshInputs(t *testing.T) {
	content := desktopLyricContent{text: "Stay with me", translation: "留在我身边"}
	size, width, color, family := float32(32), float32(300), radixPaper, "sans-serif"
	var layout desktopLyricLayouts
	tt := ui.NewTester(func(c *ui.Context) {
		theme := *c.Theme()
		theme.Font = family
		c.SetTheme(&theme)
		layout = desktopLyricsLayouts(c, content, 0, size, width, color)
	}, 320, 160)
	tt.Frame()
	for _, change := range []struct {
		name    string
		apply   func()
		changed func(desktopLyricLayouts, desktopLyricLayouts) bool
	}{
		{"song", func() { content.text = "Through the night" }, func(a, b desktopLyricLayouts) bool { return a.outline != b.outline && a.text != b.text }},
		{"translation", func() { content.translation = "陪伴漫漫长夜" }, func(a, b desktopLyricLayouts) bool { return a.translation != b.translation }},
		{"preview", func() { content.translation, content.preview = "", "Next sentence" }, func(a, b desktopLyricLayouts) bool { return b.translation == nil && b.preview != nil }},
		{"font size", func() { size = 40 }, func(a, b desktopLyricLayouts) bool { return a.outline != b.outline && a.preview != b.preview }},
		{"width", func() { width = 150 }, func(a, b desktopLyricLayouts) bool { return a.outline != b.outline && a.preview != b.preview }},
		{"font family", func() { family = "serif" }, func(a, b desktopLyricLayouts) bool { return a.outline != b.outline }},
		{"color", func() { color = radixGray(true, 12) }, func(a, b desktopLyricLayouts) bool { return a.text != b.text && a.preview != b.preview }},
	} {
		t.Run(change.name, func(t *testing.T) {
			before := layout
			change.apply()
			tt.Frame()
			if !change.changed(before, layout) {
				t.Fatal("desktop lyric retained a stale content or style layer")
			}
		})
	}
}

// Compact height and dark-theme changes must refresh taskbar text while progress stays independent.
func TestTaskbarLyricLayoutsRefreshInputs(t *testing.T) {
	content := desktopLyricContent{text: "Stay with me", translation: "留在我身边"}
	height, dark := float32(42), true
	var layout taskbarLyricLayout
	tt := ui.NewTester(func(c *ui.Context) {
		layout = makeTaskbarLyricLayout(c, content, time.Second, dark, height)
	}, 280, 42)
	tt.Frame()
	before := layout
	dark = false
	tt.Frame()
	if layout.text == before.text || layout.outline != before.outline {
		t.Fatal("theme change either kept stale foreground or reshaped its unchanged outline")
	}
	before = layout
	height = 28
	tt.Frame()
	if layout.outline == before.outline || layout.secondary == before.secondary || layout.height > height {
		t.Fatal("compact taskbar kept text measured for the previous height")
	}
	before = layout
	content.text, content.translation = "Through the night", "陪伴漫漫长夜"
	tt.Frame()
	if layout.outline == before.outline || layout.secondary == before.secondary {
		t.Fatal("track change retained an old taskbar sentence")
	}
}

// Browsing many lyric lines must evict old layouts while retaining the recently painted sentence.
func TestLyricLayoutCacheBounded(t *testing.T) {
	var cache lyricLayoutCache
	key := lyricLayoutKey{text: "first", font: ui.Font{Size: 16}}
	first := cache.get(key)
	hotKey := key
	hotKey.text = "current"
	hot := cache.get(hotKey)
	for i := 0; i < lyricLayoutCacheLimit*3; i++ {
		next := key
		next.text = fmt.Sprint(i)
		cache.get(next)
		if cache.get(hotKey) != hot {
			t.Fatal("recently used lyric was evicted")
		}
	}
	if cache.count != lyricLayoutCacheLimit || cache.get(key) == first {
		t.Fatal("old lyric layouts were retained without a bound")
	}
}
