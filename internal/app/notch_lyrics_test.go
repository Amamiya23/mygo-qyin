package app

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"lilt/internal/lyrics"
	"lilt/internal/qqmusic"
)

func notchGoldPixels(img *image.RGBA, area image.Rectangle) int {
	count := 0
	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
			p := img.RGBAAt(x, y)
			if int(p.R) > int(p.G)+15 && int(p.G) > int(p.B)+30 {
				count++
			}
		}
	}
	return count
}

// Render against deliberately uneven word times, including a real silent gap.
// Playback changes only the fill, never the retained sentence geometry.
func TestNotchWordFillAndSeek(t *testing.T) {
	line := lyrics.Build("", "", "[1000,7000]慢(1000,3000)唱(6000,2000)")[0]
	position := 1500 * time.Millisecond
	var layout notchLyricLayout
	tt := ui.NewTester(func(c *ui.Context) {
		layout = makeNotchLyricLayout(c, line, position)
		ui.Box(c).Size(100, 32).Background(ui.Hex("#000000")).Draw(func(p *ui.Painter, r ui.Rect) {
			layout.draw(p, r, ui.Start)
		})
	}, 100, 32)
	tt.Frame()
	initial, firstPixels := layout, append([]byte(nil), tt.Image().Pix...)
	initialGold := notchGoldPixels(tt.Image(), tt.Image().Bounds())
	position = 3500 * time.Millisecond
	tt.Frame()
	if notchGoldPixels(tt.Image(), tt.Image().Bounds()) <= initialGold {
		t.Fatal("the current character did not fill as its real duration elapsed")
	}
	if layout.words != initial.words || layout.fills[0].W <= initial.fills[0].W {
		t.Fatal("word progress reshaped the line or mutated the earlier frame")
	}
	position = 5 * time.Second
	tt.Frame()
	if notchGoldPixels(tt.Image(), tt.Image().Bounds()) != 0 || layout.colors[0] != notchLyricSung || layout.colors[1] != notchLyricRest {
		t.Fatal("a silent gap must leave completed words white and future words gray")
	}
	position = 7 * time.Second
	tt.Frame()
	if notchGoldPixels(tt.Image(), tt.Image().Bounds()) == 0 || layout.colors[0] != notchLyricSung {
		t.Fatal("the next word did not start its own fill")
	}
	position = 1500 * time.Millisecond
	tt.Frame()
	if !bytes.Equal(tt.Image().Pix, firstPixels) {
		t.Fatal("seeking backwards did not restore the exact earlier progress")
	}
	position = 8 * time.Second
	tt.Frame()
	if len(layout.fills) != 0 || layout.colors[0] != notchLyricSung || layout.colors[1] != notchLyricSung {
		t.Fatal("finished line retained an active word")
	}
}

func TestNotchWordRefreshOnlyWhenSinging(t *testing.T) {
	for _, test := range []struct {
		name     string
		position time.Duration
		playing  bool
		plain    bool
		animate  bool
	}{
		{"singing", 2 * time.Second, true, false, true},
		{"paused", 2 * time.Second, false, false, false},
		{"word gap", 5 * time.Second, true, false, false},
		{"finished", 9 * time.Second, true, false, false},
		{"plain LRC", 2 * time.Second, true, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := newTestApp()
			a.pl.Cue([]qqmusic.Track{{ID: 7, Name: "测试歌曲", Duration: 60000}})
			lines := lyrics.Build("", "", "[1000,7000]慢(1000,3000)唱(6000,2000)")
			if test.plain {
				lines = lyrics.Build("[00:01.00]慢唱", "", "")
			}
			a.pl.SetLyrics(lines)
			if test.playing {
				a.pl.Hold(test.position)
			} else {
				a.pl.Seek(test.position)
			}
			n, builds := &notch{a: a}, 0
			tt := ui.NewTester(func(c *ui.Context) {
				builds++
				n.lyric(c, a.pl.Current(), ui.Center).Width(200)
			}, 200, 32)
			tt.Frame()
			before, pixels := builds, append([]byte(nil), tt.Image().Pix...)
			tt.Advance(500 * time.Millisecond)
			if test.animate {
				if frames := builds - before; frames < 10 || frames > 15 {
					t.Fatalf("active fill should refresh at 25 Hz, got %d builds in 500 ms", frames)
				}
			} else if builds != before || !bytes.Equal(pixels, tt.Image().Pix) {
				t.Fatal("paused, silent or line-timed lyrics kept repainting")
			}
			if test.plain && notchGoldPixels(tt.Image(), tt.Image().Bounds()) != 0 {
				t.Fatal("LRC estimated words must not be shown as precise timing")
			}
		})
	}
}

// Long sentences follow real playback in either direction, with all paint
// clipped to the lyric's box so it cannot cover album art or the level bars.
func TestNotchLongWordLine(t *testing.T) {
	line := lyrics.Build("", "", "[1000,9000]"+strings.Repeat("前奏 ", 20)+"(1000,1000)singing(2000,7000) 结束(9000,1000)")[0]
	a := newTestApp()
	a.pl.Cue([]qqmusic.Track{{ID: 7, Name: "长句", Duration: 60000}})
	a.pl.SetLyrics([]lyrics.Line{line})
	a.pl.Seek(5500 * time.Millisecond)
	n := &notch{a: a}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Size(150, 32).Background(ui.Hex("#000000")).Children(func() {
			n.lyric(c, a.pl.Current(), ui.Start).Absolute().Left(30).Top(8).Width(90)
		})
	}, 150, 32)
	tt.Frame()
	img := tt.Image()
	if notchGoldPixels(img, image.Rect(30, 0, 120, 32)) == 0 || notchGoldPixels(img, image.Rect(0, 0, 30, 32))+notchGoldPixels(img, image.Rect(120, 0, 150, 32)) != 0 {
		t.Fatal("long-line progress is either offscreen or escaped the lyric box")
	}
	before := append([]byte(nil), img.Pix...)
	a.pl.Seek(1100 * time.Millisecond)
	tt.Frame()
	if bytes.Equal(before, tt.Image().Pix) {
		t.Fatal("paused backward seek did not scroll back")
	}
	a.pl.Seek(5500 * time.Millisecond)
	tt.Frame()
	if !bytes.Equal(before, tt.Image().Pix) {
		t.Fatal("paused forward seek did not restore the sung word")
	}
}

func TestNotchWordViews(t *testing.T) {
	line := lyrics.Build("", "", "[1000,9000]一(1000,1000)二(2000,1000)三(3000,1000)四 (4000,1000)five (5000,1000)six (6000,1000)seven (7000,1500)eight(8500,1500)")[0]
	for _, expanded := range []bool{false, true} {
		t.Run(fmt.Sprintf("expanded=%v", expanded), func(t *testing.T) {
			a := newTestApp()
			a.settings = defaultSettings()
			a.settings.Notch, a.settings.NotchVirtual, a.settings.NotchActivity = true, true, true
			a.pl.Cue([]qqmusic.Track{{ID: 7, Name: "逐字歌词", Duration: 60000}})
			a.pl.SetLyrics([]lyrics.Line{line})
			n := &notch{a: a, open: expanded, w0: 200, h0: 32, winW: notchOpenW + 60, winH: 240}
			a.notch = n
			tt := ui.NewTester(n.view, int(n.winW), int(n.winH))
			for _, pos := range []time.Duration{3500, 5600, 7600} {
				a.pl.Hold(pos * time.Millisecond)
				tt.Frame()
				tt.Advance(time.Second)
				r, ok := tt.Find(line.Text)
				if !ok || r.W <= 0 || r.H <= 0 || notchGoldPixels(tt.Image(), image.Rect(int(r.X), int(r.Y), int(r.X+r.W), int(r.Y+r.H))) == 0 {
					t.Fatalf("lyric is inaccessible or has no visible word fill: %+v", r)
				}
				if dir := os.Getenv("NM_FRAMES"); dir != "" {
					f, err := os.Create(filepath.Join(dir, fmt.Sprintf("notch-words-expanded-%v-%d.png", expanded, pos)))
					if err != nil {
						t.Fatal(err)
					}
					err = png.Encode(f, tt.Image())
					f.Close()
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			a.pl.Cue([]qqmusic.Track{{ID: 8, Name: "无歌词歌曲", Duration: 60000}})
			a.pl.SetLyrics(nil)
			tt.Frame()
			if !tt.HasText("暂无歌词") || tt.HasText(line.Text) {
				t.Fatal("changing songs left old word progress visible")
			}
		})
	}
}
