package app

import (
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"lilt/internal/lyrics"
	"lilt/internal/qqmusic"
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

// Paused, hidden, inter-word-gap and completed lyrics do not keep rebuilding themselves.
func TestDesktopLyricsFrameBudget(t *testing.T) {
	for _, mode := range []string{"paused", "hidden", "lrc", "gap", "interlude", "outro", "timed"} {
		t.Run(mode, func(t *testing.T) {
			a, d := desktopLyricsViewFixture()
			qrc := "[1000,9000]慢(1000,3000)唱(8000,2000)"
			if mode == "lrc" {
				qrc = ""
			}
			if mode == "interlude" {
				qrc += "\n[20000,2000]后句(20000,2000)"
			}
			a.pl.SetLyrics(lyrics.Build("[00:01]慢唱", "", qrc))
			if mode != "paused" {
				a.pl.Hold(2 * time.Second)
			}
			if mode == "gap" {
				a.pl.Hold(5 * time.Second)
			}
			if mode == "interlude" || mode == "outro" {
				a.pl.Hold(12 * time.Second)
			}
			if mode == "hidden" {
				a.settings.DesktopLyrics = false
			}
			builds := 0
			tt := ui.NewTester(func(c *ui.Context) { builds++; d.view(c) }, 760, 180)
			tt.SetPreferences(ui.Preferences{ReduceMotion: true})
			tt.Frame()
			before := builds
			tt.Advance(500 * time.Millisecond)
			got := builds - before
			if mode == "timed" {
				if got < 10 || got > 15 {
					t.Fatalf("active words rebuilt %d times in 500ms, want about 12", got)
				}
			} else if got != 0 {
				t.Fatalf("%s lyrics rebuilt %d times while unchanged", mode, got)
			}
		})
	}
}

// A sentence change requests a bounded burst of frames and becomes idle after both layers settle.
func TestDesktopLyricsTransitionFrameBudget(t *testing.T) {
	var state desktopLyricViewState
	started, builds := false, 0
	tt := ui.NewTester(func(c *ui.Context) {
		builds++
		if !started {
			state.changed, started = c.Now(), true
		}
		state.blend(c)
	}, 100, 100)
	tt.Frame()
	before := builds
	tt.Advance(desktopLyricsFade + 40*time.Millisecond)
	if got := builds - before; got < 12 || got > 18 {
		t.Fatalf("sentence transition rebuilt %d times, want a short 60 Hz burst", got)
	}
	before = builds
	tt.Advance(time.Second)
	if builds != before {
		t.Fatalf("settled transition kept requesting frames: %d", builds-before)
	}
}

// Previewing the next Chinese lyric must not animate paused, LRC-only or completed words.
func TestDesktopLyricsDoubleLineFrameBudget(t *testing.T) {
	for _, mode := range []string{"paused", "lrc", "gap", "interlude", "outro", "timed"} {
		t.Run(mode, func(t *testing.T) {
			a, d := desktopLyricsViewFixture()
			a.settings.DesktopLyricsDoubleLine = true
			qrc := "[1000,9000]我(1000,3000)们(8000,2000)\n[20000,2000]再见(20000,2000)"
			if mode == "lrc" {
				qrc = ""
			}
			a.pl.SetLyrics(lyrics.Build("[00:01]我们\n[00:20]再见", "", qrc))
			position := 2 * time.Second
			switch mode {
			case "gap":
				position = 5 * time.Second
			case "interlude":
				position = 12 * time.Second
			case "outro":
				position = 23 * time.Second
			}
			if mode == "paused" {
				a.pl.Seek(position)
			} else {
				a.pl.Hold(position)
			}
			builds := 0
			tt := ui.NewTester(func(c *ui.Context) { builds++; d.view(c) }, 760, 240)
			tt.SetPreferences(ui.Preferences{ReduceMotion: true})
			tt.Frame()
			before := builds
			tt.Advance(500 * time.Millisecond)
			got := builds - before
			if mode == "timed" {
				if got < 10 || got > 15 {
					t.Fatalf("double-line word highlighting rebuilt %d times, want about 12", got)
				}
			} else if got != 0 {
				t.Fatalf("%s preview kept rebuilding: %d", mode, got)
			}
		})
	}
}

// Paused, hidden, missing and completed taskbar lyrics stay idle; active highlighting and long-line scrolling are bounded.
func TestTaskbarLyricsFrameBudget(t *testing.T) {
	for _, mode := range []string{"paused", "hidden", "disabled", "missing", "prelude", "lrc-short", "lrc-long", "lrc-translation", "preview-long", "preview-active-long", "preview-row2-timed", "timed", "gap", "outro"} {
		t.Run(mode, func(t *testing.T) {
			a, d := taskbarLyricsViewFixture()
			qrc := "[1000,9000]我们一起走过漫长道路(1000,3000)直到音乐带我们走向远方(8000,2000)"
			if mode == "lrc-short" || mode == "lrc-long" || mode == "lrc-translation" || mode == "preview-long" || mode == "preview-active-long" {
				qrc = ""
			}
			lrc := "[00:01]我们一起走过漫长道路，直到音乐带我们走向远方"
			if mode == "lrc-short" {
				lrc = "[00:01]我们\n[00:20]再见"
			}
			if mode == "preview-long" || mode == "preview-active-long" {
				lrc = "[00:01]我们\n[00:20]让音乐陪伴我们走过漫长道路，直到歌声带我们走向远方"
			}
			if mode == "preview-row2-timed" {
				qrc = "[1000,2000]我们(1000,2000)\n[4000,9000]让音乐陪伴(4000,3000)走向远方(7000,6000)"
			}
			translation := ""
			if mode == "lrc-translation" {
				lrc, translation = "[00:01]Stay", "[00:01]留在我身边，陪伴我们走过漫长道路直到远方"
			}
			a.pl.SetLyrics(lyrics.Build(lrc, translation, qrc))
			position := 2 * time.Second
			if mode == "preview-active-long" {
				position = 21 * time.Second
			}
			if mode == "preview-row2-timed" {
				position = 5 * time.Second
			}
			if mode == "gap" {
				position = 5 * time.Second
			}
			if mode == "outro" {
				position = 12 * time.Second
			}
			if mode == "prelude" {
				position = 0
			}
			if mode == "paused" {
				a.pl.Seek(position)
			} else {
				a.pl.Hold(position)
			}
			if mode == "hidden" {
				d.visible = false
			}
			if mode == "disabled" {
				a.settings.TaskbarLyrics = false
			}
			if mode == "missing" {
				a.pl.SetLyrics(nil)
			}
			builds := 0
			tt := ui.NewTester(func(c *ui.Context) { builds++; d.view(c) }, 160, 42)
			tt.Frame()
			before := builds
			tt.Advance(500 * time.Millisecond)
			got := builds - before
			if mode == "timed" || mode == "lrc-long" || mode == "lrc-translation" || mode == "preview-active-long" || mode == "preview-row2-timed" {
				if got < 10 || got > 15 {
					t.Fatalf("%s rebuilt %d times in 500ms, want about 12", mode, got)
				}
			} else if got != 0 {
				t.Fatalf("%s taskbar lyrics rebuilt %d times while unchanged", mode, got)
			}
		})
	}
}

// Paused or dismissed taskbar controls must not ask for frames after their interaction animations settle.
func TestTaskbarLyricsControlsFrameBudget(t *testing.T) {
	for _, mode := range []string{"paused", "closed", "hidden"} {
		t.Run(mode, func(t *testing.T) {
			_, d := taskbarControlsFixture()
			d.controlsOpen = true
			if mode == "closed" {
				d.closeControls()
			}
			if mode == "hidden" {
				d.hide()
			}
			builds := 0
			tt := ui.NewTester(func(c *ui.Context) { builds++; d.controlsView(c) }, taskbarControlsWidth, taskbarControlsHeight)
			tt.Move(-10, -10)
			tt.Frame()
			tt.Advance(time.Second)
			before := builds
			tt.Advance(time.Second)
			if builds != before {
				t.Fatalf("%s taskbar controls kept rebuilding: %d extra frames", mode, builds-before)
			}
		})
	}
}

// A ready taskbar cover must not introduce an artwork fade or keep paused lyrics rebuilding.
func TestTaskbarLyricsReadyCoverFrameBudget(t *testing.T) {
	a, d := taskbarLyricsViewFixture()
	track := *a.pl.Current()
	track.Album.PicURL = "https://example.invalid/taskbar-paused-cover.jpg"
	a.pl.Cue([]qqmusic.Track{track})
	a.art.add(qqmusic.Sized(track.Album.PicURL, 96), &artwork{bmp: taskbarCoverBitmap(color.RGBA{R: 255, A: 255})})
	builds := 0
	tt := ui.NewTester(func(c *ui.Context) { builds++; d.view(c) }, 280, 42)
	tt.Frame()
	if red, _ := taskbarCoverColors(tt.Image()); red == 0 {
		t.Fatal("fixture did not paint the ready cover")
	}
	before := builds
	tt.Advance(time.Second)
	if builds != before {
		t.Fatalf("paused cover kept rebuilding: %d extra frames", builds-before)
	}
}
