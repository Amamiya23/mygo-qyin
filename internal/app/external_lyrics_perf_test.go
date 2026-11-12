package app

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"lilt/internal/lyrics"
)

func externalLyricsBenchmarkContent() desktopLyricContent {
	texts := []string{"让音乐", "陪伴我们", "走过", "漫长道路，", "直到", "歌声", "带我们", "走向远方"}
	words := make([]lyrics.Word, len(texts))
	for i, text := range texts {
		words[i] = lyrics.Word{Text: text, Start: time.Duration(i+1) * time.Second, End: time.Duration(i+2) * time.Second}
	}
	text := strings.Join(texts, "")
	return desktopLyricContent{
		index: 0, text: text, translation: "Let the music stay with us all the way",
		line: lyrics.Line{At: time.Second, End: 9 * time.Second, Text: text, Timed: true, Words: words},
	}
}

// Advancing timed desktop lyrics measures view construction and text layout without raster drawing.
func BenchmarkExternalLyricsDesktopTimedLayout(b *testing.B) {
	content := externalLyricsBenchmarkContent()
	position := time.Second
	var layout desktopLyricLayouts
	tt := ui.NewTester(func(c *ui.Context) {
		layout = desktopLyricsLayouts(c, content, position, 36, 700, radixPaper)
	}, 760, 200)
	tt.Frame()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		position = time.Second + time.Duration(i%200)*40*time.Millisecond
		tt.Frame()
	}
	if layout.text == nil {
		b.Fatal("missing desktop lyric layout")
	}
}

// Advancing timed compact taskbar lyrics measures fitting and scroll geometry without raster drawing.
func BenchmarkExternalLyricsTaskbarTimedCompactLayout(b *testing.B) {
	content := externalLyricsBenchmarkContent()
	position := time.Second
	var layout taskbarLyricLayout
	tt := ui.NewTester(func(c *ui.Context) {
		layout = makeTaskbarLyricLayout(c, content, position, true, 28)
	}, 360, 28)
	tt.Frame()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		position = time.Second + time.Duration(i%200)*40*time.Millisecond
		tt.Frame()
	}
	if layout.text == nil {
		b.Fatal("missing taskbar lyric layout")
	}
}
