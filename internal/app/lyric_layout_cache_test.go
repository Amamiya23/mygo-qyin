package app

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"lilt/internal/lyrics"
)

// Playback used to reshape timed text; geometry now stays fixed while each frame owns its word colors.
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
				if desktop.text != firstDesktop.text {
					t.Fatal("playback reshaped desktop foreground")
				}
				if timed && (desktop.wordColors[0] == firstDesktop.wordColors[0] || firstDesktop.wordColors[0] != desktopLyricWordColor(radixPaper, line.Words[0].Progress(1500*time.Millisecond))) {
					t.Fatal("desktop word colors are stale or mutated an earlier frame")
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
					if taskbar.text != firstTaskbar.text || taskbar.progressX <= firstTaskbar.progressX {
						t.Fatal("taskbar cached stale brightness or scrolling progress")
					}
					if timed && (taskbar.wordColors[0] == firstTaskbar.wordColors[0] || firstTaskbar.wordColors[0] != radixGray(true, 12).Alpha(desktopLyricsRestAlpha+(desktopLyricsSungAlpha-desktopLyricsRestAlpha)*line.Words[0].Progress(1500*time.Millisecond))) {
						t.Fatal("taskbar word colors are stale or mutated an earlier frame")
					}
				}
			})
		}
	}
}

// Identical joined text can have different word boundaries, so a new timing partition must repaint the right characters.
func TestTimedLyricWordPartition(t *testing.T) {
	content := desktopLyricContent{index: 0, text: "office"}
	content.line = lyrics.Line{Text: content.text, Timed: true, Words: []lyrics.Word{
		{Text: "of", Start: 0, End: time.Second},
		{Text: "fice", Start: time.Second, End: 2 * time.Second},
	}}
	var desktop desktopLyricLayouts
	var taskbar taskbarLyricLayout
	tt := ui.NewTester(func(c *ui.Context) {
		desktop = desktopLyricsLayouts(c, content, 500*time.Millisecond, 32, 300, radixPaper)
		taskbar = makeTaskbarLyricLayout(c, content, 500*time.Millisecond, true, 28)
	}, 320, 160)
	tt.Frame()
	firstDesktop, firstTaskbar := desktop, taskbar
	content.line.Words = []lyrics.Word{
		{Text: "off", Start: 0, End: time.Second},
		{Text: "ice", Start: time.Second, End: 2 * time.Second},
	}
	tt.Frame()
	if desktop.text == firstDesktop.text || taskbar.text == firstTaskbar.text {
		t.Fatal("new word partition reused the previous color boundaries")
	}
	firstDesktop, firstTaskbar = desktop, taskbar
	// The same chorus geometry remains reusable with different timestamps.
	content.line.Words = []lyrics.Word{
		{Text: "off", Start: time.Second, End: 2 * time.Second},
		{Text: "ice", Start: 2 * time.Second, End: 3 * time.Second},
	}
	tt.Frame()
	if desktop.text != firstDesktop.text || taskbar.text != firstTaskbar.text ||
		desktop.wordColors[0] == firstDesktop.wordColors[0] || taskbar.progressX == firstTaskbar.progressX {
		t.Fatal("new chorus timings either reshaped text or retained old progress")
	}
}

// Taskbar scrolling must use normalized UTF-16 boundaries, including astral and combining characters.
func TestTaskbarLyricRetainedWordCarets(t *testing.T) {
	words := []lyrics.Word{
		{Text: "风😀\r\n", Start: 0, End: time.Second},
		{Text: "e\u0301\t終", Start: time.Second, End: 2 * time.Second},
	}
	content := desktopLyricContent{index: 0, text: "风😀\r\ne\u0301\t終", translation: "translation",
		line: lyrics.Line{Timed: true, Words: words}}
	for _, height := range []float32{28, 42} {
		var layout taskbarLyricLayout
		tt := ui.NewTester(func(c *ui.Context) {
			layout = makeTaskbarLyricLayout(c, content, 1500*time.Millisecond, true, height)
		}, 300, int(height))
		tt.Frame()
		left, right := layout.text.Caret(4).X, layout.text.Caret(8).X
		if layout.progressX != left+(right-left)*0.5 {
			t.Fatalf("height %v: word scrolling lost UTF-16/newline normalization: %v", height, layout.progressX)
		}
	}
}

// A newly published lyric slice must replace the cached language decision even when its length stays the same.
func TestLyricTranslationPolicyPublication(t *testing.T) {
	for _, policy := range []func([]lyrics.Line) bool{desktopLyricsTranslation, taskbarLyricsTranslation} {
		var cache lyricTranslationPolicy
		chinese := []lyrics.Line{{Text: "我们"}}
		japanese := []lyrics.Line{{Text: "かな", Translation: "我们"}}
		if cache.get(chinese, policy) || !cache.get(japanese, policy) || cache.get(chinese, policy) {
			t.Fatal("language decision remained attached to an earlier lyric publication")
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
