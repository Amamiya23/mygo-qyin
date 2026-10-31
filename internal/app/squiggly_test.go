package app

import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

// With NM_FRAMES set, saves flowing, settling and endpoint frames in both
// themes so the played sine and straight unplayed track can be inspected.
func TestSquigglyFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	for _, dark := range []bool{false, true} {
		a := newTestApp()
		a.settings.setProgressEffect(progressSquiggly)
		a.pl.Cue([]qqmusic.Track{{ID: 7, Duration: 200000}})
		out := audio.NewPlayer(&silence{})
		out.SetVolume(0)
		defer out.Close()
		a.pl.Attach(out)
		a.pl.Seek(80 * time.Second)
		label := "播放"
		tt := ui.NewTester(func(c *ui.Context) {
			c.Root().Background(radixSage(dark, 1)).Padding(16, 24)
			ui.Column(c).Fill().Gap(10).Children(func() {
				ui.Text(c, label).FontSize(12).TextColor(radixSage(dark, 11))
				a.scrubber(c, &a.barScrub, radixSage(dark, 5), radixGreen(dark, 9), 16, ui.Color{}).Width(552)
			})
		}, 600, 76)
		tt.SetScale(2)
		tt.SetDark(dark)
		tt.Frame()
		const rows = 9
		shot := tt.Image()
		w, h := shot.Bounds().Dx(), shot.Bounds().Dy()
		strip := image.NewRGBA(image.Rect(0, 0, w, h*rows))
		capture := func(row int, title string, dt time.Duration) {
			label = title
			tt.Advance(dt)
			tt.Frame()
			draw.Draw(strip, image.Rect(0, row*h, w, (row+1)*h), tt.Image(), image.Point{}, draw.Src)
		}
		capture(0, "播放 · 0 ms", 0)
		capture(1, "播放 · 250 ms", 250*time.Millisecond)
		capture(2, "播放 · 500 ms", 250*time.Millisecond)
		a.pl.TogglePlay()
		tt.Frame()
		capture(3, "暂停 · 100 ms", 100*time.Millisecond)
		capture(4, "暂停 · 550 ms", 450*time.Millisecond)
		a.pl.Hold(0)
		a.pl.Seek(0)
		tt.Frame()
		capture(5, "播放 · 0%", time.Second)
		a.pl.Seek(2 * time.Second)
		capture(6, "播放 · 1%", 0)
		a.pl.Seek(198 * time.Second)
		capture(7, "播放 · 99%", 0)
		a.pl.Seek(200 * time.Second)
		capture(8, "播放 · 100%", 0)
		name := fmt.Sprintf("squiggly-dark-%t.png", dark)
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, strip)
		closeErr := f.Close()
		if err != nil {
			t.Fatal(err)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
	}
}
