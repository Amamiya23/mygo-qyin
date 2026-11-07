package app

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"lilt/internal/qqmusic"
)

// Changing tracks must replace the taskbar cover with its own loading state, then clear it when no cover or track exists.
func TestTaskbarLyricsCoverFollowsCurrentTrack(t *testing.T) {
	a, d := taskbarLyricsViewFixture()
	a.art.now = func() time.Time { return time.Unix(0, 0) }
	a.pl.SetLyrics(nil)
	redURL, blueURL := "https://example.invalid/taskbar-red.jpg", "https://example.invalid/taskbar-blue.jpg"
	redKey, blueKey := qqmusic.Sized(redURL, 96), qqmusic.Sized(blueURL, 96)
	a.art.add(redKey, &artwork{bmp: taskbarCoverBitmap(color.RGBA{R: 255, A: 255})})
	a.art.busy[blueKey] = &artRequest{} // Keep the pending cover offline until this test supplies it.
	track := *a.pl.Current()
	track.Album.PicURL = redURL
	a.pl.Cue([]qqmusic.Track{track})
	tt := ui.NewTester(d.view, 280, 42)
	tt.Frame()
	red, blue := taskbarCoverColors(tt.Image())
	if red == 0 || blue != 0 {
		t.Fatalf("ready current cover was not painted: red=%d blue=%d", red, blue)
	}

	track.ID++
	track.Album.PicURL = blueURL
	a.pl.Cue([]qqmusic.Track{track})
	tt.Frame()
	red, blue = taskbarCoverColors(tt.Image())
	if red != 0 || blue != 0 || !taskbarFrameHasInk(tt.Image()) {
		t.Fatalf("pending track retained a stale cover or lost its placeholder: red=%d blue=%d", red, blue)
	}
	a.art.add(blueKey, &artwork{failed: true, used: a.art.now()})
	tt.Frame()
	red, blue = taskbarCoverColors(tt.Image())
	if red != 0 || blue != 0 || !taskbarFrameHasInk(tt.Image()) {
		t.Fatalf("failed cover retained old artwork or lost its placeholder: red=%d blue=%d", red, blue)
	}

	delete(a.art.busy, blueKey)
	a.art.add(blueKey, &artwork{bmp: taskbarCoverBitmap(color.RGBA{B: 255, A: 255})})
	tt.Frame()
	red, blue = taskbarCoverColors(tt.Image())
	if red != 0 || blue == 0 {
		t.Fatalf("new cached cover did not replace the placeholder: red=%d blue=%d", red, blue)
	}

	track.Album.PicURL = ""
	a.pl.Cue([]qqmusic.Track{track})
	tt.Frame()
	if taskbarFrameHasInk(tt.Image()) {
		t.Fatal("a track without artwork kept the old cover or a reserved placeholder")
	}
	track.Album.PicURL = redURL
	a.pl.Cue([]qqmusic.Track{track})
	tt.Frame()
	if red, _ := taskbarCoverColors(tt.Image()); red == 0 {
		t.Fatal("fixture did not restore a cover before clearing the current track")
	}
	d.a = newTestApp()
	d.a.settings.TaskbarLyrics = true
	tt.Frame()
	if taskbarFrameHasInk(tt.Image()) {
		t.Fatal("clearing the current track left its cover on the taskbar")
	}
}

func taskbarCoverBitmap(c color.RGBA) *ui.Bitmap {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := range 32 {
		for x := range 32 {
			img.SetRGBA(x, y, c)
		}
	}
	return ui.NewBitmap(img)
}

func taskbarCoverColors(frame image.Image) (red, blue int) {
	for y := frame.Bounds().Min.Y; y < frame.Bounds().Max.Y; y++ {
		for x := frame.Bounds().Min.X; x < frame.Bounds().Max.X; x++ {
			r, g, b, a := frame.At(x, y).RGBA()
			if a == 0 || g > 0x1000 {
				continue
			}
			if r > 0xc000 && b < 0x1000 {
				red++
			}
			if b > 0xc000 && r < 0x1000 {
				blue++
			}
		}
	}
	return
}
