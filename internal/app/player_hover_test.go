package app

import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

func playerHoverApp() *app {
	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &qqmusic.Profile{UserID: 1, Nickname: "Amamiya"}
	tr := qqmusic.Track{ID: 7, Name: "红豆", Artists: []qqmusic.Artist{{Name: "王菲"}}, Album: qqmusic.Album{PicURL: "player-hover-cover"}, Duration: 256000}
	a.pl.Cue([]qqmusic.Track{tr})
	a.pl.SetLyrics(nil)
	a.pl.Seek(24 * time.Second)
	a.volume = 0.63
	a.pl.SetVolume(a.volume)
	art := &artwork{bmp: solidBitmap(radixCyan9), tint: radixCyan9}
	for _, size := range []int{96, 120, 240, 600} {
		a.art.art[qqmusic.Sized(tr.Album.PicURL, size)] = art
	}
	return a
}

// The volume speaker keeps its popover open after the pointer leaves, closes on explicit dismissal, and adjusts volume by wheel without opening it.
func TestPlayerVolumePopoverInteraction(t *testing.T) {
	a := playerHoverApp()
	tt := ui.NewTester(a.view, 1180, 600)
	tt.Frame()
	r, ok := tt.Find("音量")
	if !ok {
		t.Fatal("cannot target the volume speaker")
	}
	x, y := r.X+r.W/2, r.Y+r.H/2
	tt.ClickAt(x, y)
	if !a.volumeOpen {
		t.Fatal("clicking the speaker did not open its popover")
	}
	tt.Move(10, 10)
	tt.Advance(250 * time.Millisecond)
	tt.Frame()
	if !a.volumeOpen {
		t.Fatal("moving away closed the explicitly opened volume popover")
	}
	tt.ClickAt(x, y)
	if a.volumeOpen {
		t.Fatal("clicking the speaker again did not close the popover")
	}
	before := a.volume
	tt.Wheel(x, y, 0, -40)
	if a.volume <= before || a.volumeOpen {
		t.Fatal("scrolling the speaker did not raise the volume locally with the popover closed")
	}
	if a.pl.Playing() || a.pl.Position() != 24*time.Second || a.nowPlaying {
		t.Fatal("using the volume speaker changed playback or opened Now Playing")
	}
	tt.ClickAt(x, y)
	tt.Key(0, ui.KeyEscape)
	if a.volumeOpen {
		t.Fatal("Escape did not dismiss the volume popover")
	}
	tt.ClickAt(x, y)
	tt.ClickAt(10, 400)
	if a.volumeOpen {
		t.Fatal("clicking outside did not dismiss the volume popover")
	}
}

// Dragging and keyboard-adjusting the volume slider changes the volume without seeking or starting playback.
func TestPlayerVolumeSliderInput(t *testing.T) {
	a := playerHoverApp()
	tt := ui.NewTester(func(c *ui.Context) {
		a.pal = paletteFor(c.Theme())
		ui.Column(c).Fill().Center().Children(func() { a.nativeVolumeSlider(c, popoverColors(a.pal.dark)) })
	}, 100, 180)
	tt.Frame()
	r, ok := tt.Find("音量")
	if !ok {
		t.Fatal("cannot target the volume slider")
	}
	x, y := r.X+r.W/2, r.Y+r.H*0.2
	tt.Press(x, r.Y+r.H*0.75)
	tt.Move(x, y)
	tt.Release(x, y)
	tt.Frame()
	if a.volume <= 0.75 || a.volume > 1 {
		t.Fatalf("dragging the slider near its top left volume at %v", a.volume)
	}
	before := a.volume
	tt.Key(0, ui.KeyDown)
	if a.volume >= before {
		t.Fatal("the focused slider no longer accepts keyboard volume changes")
	}
	if a.pl.Playing() || a.pl.Position() != 24*time.Second {
		t.Fatal("adjusting volume changed playback or the current position")
	}
}

// Wheel input changes volume by 5% instead of the former 25% jump, while touchpad input stays continuous and volume stays within its limits.
func TestPlayerVolumeScroll(t *testing.T) {
	for _, control := range []string{"speaker", "slider"} {
		t.Run(control, func(t *testing.T) {
			a := playerHoverApp()
			a.volume = 0.5
			a.pl.SetVolume(a.volume)
			tt := ui.NewTester(func(c *ui.Context) {
				a.pal = paletteFor(c.Theme())
				ui.Column(c).Fill().Center().Children(func() {
					if control == "speaker" {
						a.volumeButton(c, a.pal.muted)
					} else {
						a.nativeVolumeSlider(c, popoverColors(a.pal.dark))
					}
				})
			}, 100, 180)
			tt.Frame()
			r, ok := tt.Find("音量")
			if !ok {
				t.Fatal("cannot target the volume control")
			}
			x, y := r.X+r.W/2, r.Y+r.H/2
			for _, step := range []struct {
				dx, dy  float32
				precise bool
				want    float64
			}{
				{dy: 99, want: 0.45}, // Windows' default wheel distance is 3 lines of 33 DIPs.
				{dy: -99, want: 0.5},
				{dy: -50, want: 0.55}, // Other platforms and system scroll settings use different distances.
				{dy: 40, want: 0.5},
				{dy: -165, want: 0.55},
				{dx: 99, want: 0.55},
				{dy: -4, precise: true, want: 0.56},
				{dy: 4, precise: true, want: 0.55},
				{dy: -400, precise: true, want: 1},
				{dy: -99, want: 1},
				{dy: 400, precise: true, want: 0},
				{dy: 99, want: 0},
			} {
				if step.precise {
					tt.Scroll(x, y, step.dx, step.dy)
				} else {
					tt.Wheel(x, y, step.dx, step.dy)
				}
				if math.Abs(a.volume-step.want) > 1e-9 {
					t.Fatalf("scroll (%v, %v), precise=%t: volume=%v, want %v", step.dx, step.dy, step.precise, a.volume, step.want)
				}
			}
			if a.volumeOpen || a.pl.Playing() || a.pl.Position() != 24*time.Second {
				t.Fatal("scrolling volume opened the popover or changed playback")
			}
		})
	}
}

// With NM_FRAMES set, saves player-bar pointer states in both appearances without loading audio or remote artwork.
func TestPlayerBarHoverFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	for _, width := range []int{880, 1180} {
		for _, dark := range []bool{false, true} {
			for _, scene := range []struct {
				name, label string
				pressed     bool
				opened      bool
			}{
				{name: "normal"},
				{name: "volume", label: "音量"},
				{name: "previous", label: "上一首"},
				{name: "play", label: "播放/暂停"},
				{name: "queue", label: "播放列表"},
				{name: "info", label: "打开正在播放"},
				{name: "like", label: "喜欢"},
				{name: "pressed-play", label: "播放/暂停", pressed: true},
				{name: "opened-volume", label: "音量", opened: true},
			} {
				if width == 880 && scene.name != "normal" && scene.name != "volume" && scene.name != "info" && !scene.opened {
					continue
				}
				a := playerHoverApp()
				tt := ui.NewTester(a.view, width, 600)
				tt.SetDark(dark)
				tt.Frame()
				tt.Move(10, 10)
				tt.Advance(time.Second)
				tt.Frame()
				if scene.label != "" {
					r, ok := tt.Find(scene.label)
					if !ok {
						t.Fatalf("cannot target %s", scene.label)
					}
					x, y := r.X+r.W/2, r.Y+r.H/2
					tt.Move(x, y)
					if scene.pressed {
						tt.Press(x, y)
					}
					if scene.opened {
						tt.ClickAt(x, y)
						tt.Move(10, 10)
					}
					tt.Advance(250 * time.Millisecond)
					tt.Frame()
				}
				img := tt.Image()
				scale := float32(img.Bounds().Dx()) / float32(width)
				height := float32(playerBarHeight + 8)
				if scene.opened {
					height = 300
				}
				crop := image.Rect(int(float32(sidebarWidth)*scale), int((600-height)*scale), img.Bounds().Dx(), img.Bounds().Dy())
				out := image.NewRGBA(image.Rect(0, 0, crop.Dx(), crop.Dy()))
				draw.Draw(out, out.Bounds(), img, crop.Min, draw.Src)
				name := fmt.Sprintf("player-bar-%d-dark-%t-%s.png", width, dark, scene.name)
				f, err := os.Create(filepath.Join(dir, name))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(f, out)
				f.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}

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

// Canceling a progress drag used to seek on capture loss; discard it and allow the next drag to commit.
func TestScrubCanceledDrag(t *testing.T) {
	for _, effect := range progressEffects {
		t.Run(effect.id, func(t *testing.T) {
			a := newTestApp()
			a.settings.setProgressEffect(effect.id)
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
			tt.CancelPress()
			tt.Frame()
			tt.Frame()
			if a.pl.Position() != 40*time.Second || a.barScrub.held || a.barScrub.active || a.barScrub.v != 40 {
				t.Fatalf("canceled drag: position %v, scrub %+v", a.pl.Position(), a.barScrub)
			}
			tt.Press(bar.X+bar.W*0.2, y)
			tt.Move(x, y+30)
			tt.Release(x, y+30)
			tt.Frame()
			if a.pl.Playing() || a.pl.Position() != 150*time.Second {
				t.Fatalf("next drag: playing %v, position %v", a.pl.Playing(), a.pl.Position())
			}
		})
	}
}

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
