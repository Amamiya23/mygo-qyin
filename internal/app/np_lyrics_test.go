package app

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"lilt/internal/lyrics"
	"lilt/internal/qqmusic"
)

// Clicking a QRC line seeks to its absolute start and keeps a paused player
// paused while the lyric selection follows both forward and backwards seeks.
func TestQRCLyricsPausedSeek(t *testing.T) {
	a := newTestApp()
	tr := qqmusic.Track{ID: 7, Name: "逐字测试", Duration: 20000}
	a.pl.Cue([]qqmusic.Track{tr})
	lines := lyrics.Build("", "", "[1000,3000]第一句(1000,3000)\n[5000,3000]第二句(5000,3000)\n[9000,4000]第三句(9000,4000)")
	a.pl.SetLyrics(lines)
	a.pl.Seek(2 * time.Second)
	builds := 0
	tt := ui.NewTester(func(c *ui.Context) {
		builds++
		ui.Column(c).Size(600, 500).Children(func() { a.lyricsPane(c, a.pl.Current(), 500) })
	}, 600, 500)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tt.Frame()
	for _, step := range []struct {
		text string
		at   time.Duration
		line int
	}{{"第三句", 9 * time.Second, 2}, {"第一句", time.Second, 0}} {
		if err := tt.Click(step.text); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if a.pl.Position() != step.at || a.pl.Playing() || a.lyricAt != step.line {
			t.Fatalf("click %q: position=%v playing=%v line=%d", step.text, a.pl.Position(), a.pl.Playing(), a.lyricAt)
		}
		tt.Advance(time.Second)
		before := builds
		tt.Advance(time.Second)
		if a.pl.Position() != step.at || builds != before {
			t.Fatal("paused word highlighting kept advancing or requesting frames")
		}
	}
}

// QRC snapshots show irregular word durations and a pause without asserting
// on text colours or font pixels, which belong to visual inspection.
func TestQRCLyricsFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip("set NM_FRAMES to save QRC lyric frames")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	a := newTestApp()
	tr := qqmusic.Track{ID: 7, Name: "逐字测试", Duration: 20000}
	a.pl.Cue([]qqmusic.Track{tr})
	a.pl.SetLyrics(lyrics.Build("", "[00:01.00]A long note, then a pause", "[1000,7000]慢(1000,2000)慢(3000,500) (3500,1500)唱(5000,3000)\n[9000,4000]下一句歌词(9000,4000)"))
	a.pl.Seek(2 * time.Second)
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Size(800, 500).Padding(24).Background(radixSage(true, 1)).Children(func() { a.lyricsPane(c, a.pl.Current(), 500) })
	}, 800, 500)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	for _, frame := range []struct {
		name string
		pos  time.Duration
	}{{"qrc-held-note", 2 * time.Second}, {"qrc-pause", 4 * time.Second}, {"qrc-next-word", 6500 * time.Millisecond}} {
		a.pl.Seek(frame.pos)
		tt.Frame()
		tt.Advance(time.Second)
		f, err := os.Create(filepath.Join(dir, frame.name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, tt.Image())
		closeErr := f.Close()
		if err != nil {
			t.Fatal(err)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
	}
}
