package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

// Dragging the progress bar seeks once, where it is let go: not as the
// pointer moves, even off the thin bar, and not to the end, which would
// play the next song, before it is let go.
func TestScrubSeeksOnRelease(t *testing.T) {
	for _, effect := range progressEffects {
		t.Run(effect.id, func(t *testing.T) { testScrubSeeksOnRelease(t, effect.id) })
	}
}

func testScrubSeeksOnRelease(t *testing.T, effect string) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.setProgressEffect(effect)
	tr := qqmusic.Track{ID: 7, Name: "宝船", Duration: 200000}
	a.pl.Cue([]qqmusic.Track{tr, {ID: 8, Name: "下一首"}})
	a.pl.SetLyrics(nil)
	out := audio.NewPlayer(&silence{})
	out.SetVolume(0)
	out.Play()
	a.pl.Attach(out)
	defer out.Close()
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Advance(time.Second)
	tt.Frame()
	bar, ok := tt.Find("播放进度")
	if !ok {
		t.Fatal("no progress bar")
	}
	y := bar.Y + bar.H/2
	at := func(f float32) float32 { return bar.X + bar.W*f }

	tt.Press(at(0.25), y)
	tt.Frame()
	tt.Move(at(0.6), y-40) // off the bar, above it
	tt.Frame()
	tt.Move(at(1)+30, y+30) // past its end, below it
	tt.Frame()
	if pos := a.pl.Position(); pos > 10*time.Second {
		t.Fatalf("seeked to %v while dragging", pos)
	}
	if a.pl.Current().ID != 7 {
		t.Fatal("went to the next song while dragging")
	}
	tt.Move(at(0.6), y-40)
	tt.Frame()
	tt.Release(at(0.6), y-40)
	tt.Frame()
	tt.Frame()
	if pos := a.pl.Position(); pos < 110*time.Second || pos > 130*time.Second {
		t.Errorf("let go at 60%%, at %v", pos)
	}
}

// Dragging a paused squiggly bar seeks on release and leaves playback paused;
// the same slider still accepts keyboard seeking after the drag.
func TestSquigglyPausedSeek(t *testing.T) {
	a := newTestApp()
	a.settings.setProgressEffect(progressSquiggly)
	a.pl.Cue([]qqmusic.Track{{ID: 7, Name: "宝船", Duration: 200000}})
	a.pl.Seek(40 * time.Second)
	tt := ui.NewTester(func(c *ui.Context) {
		a.scrubber(c, &a.barScrub, radixSage(false, 5), radixGreen(false, 9), 16, ui.Color{}).Width(400)
	}, 400, 48)
	tt.Frame()
	bar, ok := tt.Find("播放进度")
	if !ok {
		t.Fatal("no progress bar")
	}
	y := bar.Y + bar.H/2
	x := bar.X + bar.W*0.75
	tt.Press(bar.X+bar.W*0.2, y)
	tt.Move(x, y+30)
	tt.Frame()
	if a.pl.Position() != 40*time.Second {
		t.Fatal("paused seek committed before release")
	}
	tt.Release(x, y+30)
	tt.Frame()
	if a.pl.Playing() || a.pl.Position() != 150*time.Second {
		t.Fatalf("paused release: playing %v, position %v", a.pl.Playing(), a.pl.Position())
	}
	tt.Key(0, ui.KeyLeft)
	tt.Frame()
	if a.pl.Playing() || a.pl.Position() >= 150*time.Second {
		t.Fatalf("keyboard seek: playing %v, position %v", a.pl.Playing(), a.pl.Position())
	}
}
