package app

import (
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/ui"

	"lilt/internal/lyrics"
	"lilt/internal/qqmusic"
)

func notchMoveFixture(t *testing.T) (*app, *notch, *ui.Tester, *mygo.Point, *mygo.Display) {
	t.Helper()
	display := &mygo.Display{Bounds: mygo.Rectangle{X: 100, Y: 20, Width: 1600, Height: 900}}
	pt := &mygo.Point{X: 900, Y: 60}
	oldDisplay, oldCursor := notchDisplay, cursorPoint
	notchDisplay = func() mygo.Display { return *display }
	cursorPoint = func() mygo.Point { return *pt }
	t.Cleanup(func() { notchDisplay, cursorPoint = oldDisplay, oldCursor })
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.Notch, a.settings.NotchVirtual = true, true
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	a.pl.Cue([]qqmusic.Track{{ID: 7, Name: "测试歌曲", Duration: 60000}})
	n := &notch{a: a, open: true, w0: 200, h0: 32, winW: notchOpenW + 60, winH: 240}
	a.notch = n
	n.place()
	tt := ui.NewTester(n.view, int(n.winW), int(n.winH))
	tt.Advance(time.Second)
	tt.Frame()
	return a, n, tt, pt, display
}

func TestNotchHorizontalDrag(t *testing.T) {
	a, n, tt, pt, display := notchMoveFixture(t)
	grip, ok := tt.Find("左右移动刘海播放器")
	if !ok {
		t.Fatal("expanded player has no move handle")
	}
	// Neither the header nor the blank space outside the grip's padded
	// hit target should start dragging the player.
	for _, point := range []ui.Point{
		{X: n.winW/2 - 150, Y: n.h0 + 7},
		{X: n.winW/2 + 150, Y: n.h0 + 7},
		{X: n.winW / 2, Y: 8},
		{X: grip.X - 2, Y: grip.Y + grip.H/2},
		{X: grip.X + grip.W + 2, Y: grip.Y + grip.H/2},
		{X: grip.X + grip.W/2, Y: grip.Y - 2},
		{X: grip.X + grip.W/2, Y: grip.Y + grip.H + 2},
	} {
		tt.Press(point.X, point.Y)
		if n.dragging {
			t.Fatalf("blank space outside the grip started a drag: %+v", point)
		}
		tt.Release(point.X, point.Y)
	}
	x, y := grip.X+grip.W/2, grip.Y+grip.H/2
	startX, startY := n.screenX, n.screenY
	tt.Press(x, y)
	pt.X += 180
	tt.Move(x+180, y+80)
	if !n.dragging || n.screenX != startX+180 || n.screenY != startY {
		t.Fatalf("drag must move only horizontally: dragging=%v x=%v y=%v", n.dragging, n.screenX, n.screenY)
	}
	// As the native window moves, the pointer's local X returns to the
	// handle. Its unchanged screen position must not move the window again.
	tt.Move(x, y)
	if n.screenX != startX+180 {
		t.Fatal("window drifted while the screen pointer stayed still")
	}
	if _, err := os.Stat(a.settings.path); !os.IsNotExist(err) {
		t.Fatal("drag must not write settings before release")
	}
	for _, edge := range []struct {
		pointer int
		want    float32
	}{
		{-4000, float32(display.Bounds.X)},
		{4000, float32(display.Bounds.X+display.Bounds.Width) - n.winW},
	} {
		pt.X, pt.Y = edge.pointer, 700
		tt.Move(x+float32(edge.pointer), y+600)
		n.lastInside = time.Now().Add(-time.Second)
		n.poll()
		if n.screenX != edge.want || n.screenY != startY || !n.open || n.passing {
			t.Fatalf("drag lost the panel or escaped the screen: x=%v want=%v open=%v passing=%v", n.screenX, edge.want, n.open, n.passing)
		}
	}
	tt.Release(x+4000, y+600)
	if n.dragging || a.pl.Playing() || a.pl.Position() != 0 {
		t.Fatal("drag did not end cleanly or activated playback")
	}
	b, err := os.ReadFile(a.settings.path)
	if err != nil {
		t.Fatal(err)
	}
	var restored settings
	if err := json.Unmarshal(b, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.NotchPosition != 1 {
		t.Fatalf("position not persisted: %v", restored.NotchPosition)
	}
	// Recreating the panel and changing displays use the same saved choice.
	a.settings = restored
	display.Bounds = mygo.Rectangle{X: -1200, Y: -100, Width: 1200, Height: 800}
	n.place()
	if n.screenX != -n.winW || n.screenY != -100 {
		t.Fatalf("restored placement missed the new display: %v, %v", n.screenX, n.screenY)
	}
}

func TestNotchDragCancelAndCenter(t *testing.T) {
	a, n, tt, pt, _ := notchMoveFixture(t)
	n.moveTo(-0.5)
	a.settings.save()
	before, _ := os.ReadFile(a.settings.path)
	grip, _ := tt.Find("左右移动刘海播放器")
	x, y := grip.X+grip.W/2, grip.Y+grip.H/2
	tt.Press(x, y)
	pt.X += 200
	tt.Move(x+200, y)
	tt.CancelPress()
	if n.dragging || a.settings.NotchPosition != -0.5 {
		t.Fatalf("canceled drag was committed: %v", a.settings.NotchPosition)
	}
	after, _ := os.ReadFile(a.settings.path)
	if string(before) != string(after) {
		t.Fatal("canceled drag changed saved settings")
	}
	settingsUI := ui.NewTester(a.settingsView, 900, 2200)
	settingsUI.Frame()
	if err := settingsUI.Click("恢复居中"); err != nil {
		t.Fatal(err)
	}
	if a.settings.NotchPosition != 0 || n.screenX != 100+(1600-n.winW)/2 {
		t.Fatal("reset did not center the panel")
	}
	var saved settings
	b, _ := os.ReadFile(a.settings.path)
	if err := json.Unmarshal(b, &saved); err != nil || saved.NotchPosition != 0 {
		t.Fatal("centered position was not saved")
	}
	// Extra frames with the handle gone and back must not retain capture.
	n.open = false
	tt.Advance(time.Second)
	n.open, n.real = true, true
	tt.Advance(time.Second)
	if _, ok := tt.Find("左右移动刘海播放器"); !ok || n.dragging {
		t.Fatal("handle did not return cleanly over a real notch")
	}
}

// Seeking in the main window while paused refreshes the compact lyric;
// it used to stay on the old line until the notch was opened or playback resumed.
func TestNotchLyricsFollowPausedSeek(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.Notch, a.settings.NotchVirtual, a.settings.NotchActivity = true, true, true
	a.pl.Cue([]qqmusic.Track{{ID: 7, Name: "测试歌曲", Duration: 60000}})
	a.pl.SetLyrics(lyrics.Build("[00:01.00]第一句歌词\n[00:30.00]第二句歌词", "", ""))
	out := audio.NewPlayer(&silence{})
	out.SetVolume(0)
	defer out.Close()
	a.pl.Attach(out)
	a.pl.TogglePlay()
	a.pl.Seek(2 * time.Second)

	n := &notch{a: a, w0: 200, h0: 32, winW: notchOpenW + 60, winH: 32 + notchOpenH + 30}
	a.notch = n
	var mainServices, notchServices ui.Services
	main := ui.NewTester(func(c *ui.Context) {
		mainServices = c.Services()
		a.scrubber(c, &a.barScrub, white, white, 14, ui.Color{}).Width(400)
		a.syncWindows()
	}, 400, 40)
	compact := ui.NewTester(func(c *ui.Context) {
		notchServices = c.Services()
		n.view(c)
	}, int(n.winW), int(n.winH))
	main.Advance(time.Second)
	compact.Advance(time.Second)
	if !compact.HasText("第一句歌词") {
		t.Fatal("paused notch did not show the initial lyric")
	}
	if compact.FramePending() {
		t.Fatal("paused notch did not settle before seeking")
	}

	// Route the real window notifications to the two headless windows.
	a.win, n.win = &mygo.Window{}, &mygo.Window{}
	defer func(f func(*mygo.Window)) { invalidateWindow = f }(invalidateWindow)
	invalidateWindow = func(w *mygo.Window) {
		switch w {
		case a.win:
			mainServices.Invalidate()
		case n.win:
			notchServices.Invalidate()
		}
	}
	defer func(f func(time.Duration, func())) { sessionAfter = f }(sessionAfter)
	sessionAfter = func(time.Duration, func()) {}
	a.pl.Changed = a.playerChanged

	bar, ok := main.Find("播放进度")
	if !ok {
		t.Fatal("no main progress bar")
	}
	x, y := bar.X+bar.W*0.6, bar.Y+bar.H/2
	main.Press(x, y)
	main.Frame()
	main.Release(x, y)
	main.Frame()
	if a.pl.Playing() || a.pl.Position() < 30*time.Second {
		t.Fatalf("seek did not stay paused at the new position: %v", a.pl.Position())
	}
	if !compact.FramePending() {
		t.Fatal("paused seek did not request a notch frame")
	}
	// Process only requested frames: no input or forced Frame in the notch.
	compact.Advance(0)
	if !compact.HasText("第二句歌词") || compact.HasText("第一句歌词") {
		t.Fatal("paused seek did not wake the compact lyric")
	}
}

func TestNotchLyricTimer(t *testing.T) {
	a := newTestApp()
	a.pl.Cue([]qqmusic.Track{{ID: 7, Name: "测试歌曲", Duration: 60000}})
	a.pl.SetLyrics(lyrics.Build("[00:01.00]第一句歌词\n[00:04.00]第二句歌词", "", ""))
	a.pl.Seek(2 * time.Second)
	n := &notch{a: a}
	builds := 0
	tt := ui.NewTester(func(c *ui.Context) {
		builds++
		ui.Text(c, n.lyricText(c, a.pl.Current()))
	}, 360, 32)
	tt.Frame()
	before := builds
	tt.Advance(5 * time.Second)
	if builds != before || !tt.HasText("第一句歌词") {
		t.Fatal("paused lyrics should stay still without requesting frames")
	}
	a.pl.Hold(2 * time.Second)
	tt.Frame()
	// Move the playback clock, but let the lyric's timer trigger the frame.
	a.pl.Seek(4 * time.Second)
	tt.Advance(2020 * time.Millisecond)
	if !tt.HasText("第二句歌词") {
		t.Fatal("the next line did not wake the lyric view on its own")
	}
}

func TestNotchLyrics(t *testing.T) {
	for _, real := range []bool{false, true} {
		t.Run(fmt.Sprintf("real=%v", real), func(t *testing.T) {
			a := newTestApp()
			a.settings = defaultSettings()
			a.settings.Notch, a.settings.NotchVirtual = true, true
			a.pl.Cue([]qqmusic.Track{{ID: 7, Name: "测试歌曲", Artists: []qqmusic.Artist{{Name: "测试歌手"}}, Duration: 60000}})
			n := &notch{a: a, real: real, w0: 200, h0: 32, winW: notchOpenW + 60, winH: 32 + notchOpenH + 30}
			a.notch = n
			tt := ui.NewTester(n.view, int(n.winW), int(n.winH))
			settle := func() {
				tt.Frame()
				tt.Advance(time.Second)
				tt.Frame()
			}
			settle()
			if tt.HasText("歌词加载中…") == real {
				t.Fatal("loading state must only show in a virtual compact notch")
			}
			a.pl.SetLyrics(lyrics.Build("[00:01.00]第一句歌词\n[00:04.00]第二句歌词", "", ""))
			a.pl.Hold(2 * time.Second)
			settle()
			if tt.HasText("第一句歌词") == real {
				t.Fatal("compact lyrics must only show without a camera housing")
			}
			if !real {
				r, ok := tt.Find("第一句歌词")
				if !ok || r.Y < 0 || r.Y+r.H > n.h0 || r.X < (n.winW-notchLyricsW)/2+30 {
					t.Fatalf("compact lyric overlaps the cover or leaves the notch: %+v", r)
				}
			}
			n.open = true
			settle()
			line, ok := tt.Find("第一句歌词")
			title, titleOK := tt.Find("测试歌曲")
			bar, barOK := tt.Find("播放进度")
			if !ok || !titleOK || !barOK || tt.HasText("测试歌手") || line.Y < title.Y+title.H || line.Y+line.H > bar.Y {
				t.Fatalf("expanded lyric must replace the artist beneath the title: lyric %+v, title %+v, bar %+v", line, title, bar)
			}
			if dir := os.Getenv("NM_FRAMES"); dir != "" {
				f, err := os.Create(filepath.Join(dir, fmt.Sprintf("notch-lyrics-expanded-%v.png", real)))
				if err != nil {
					t.Fatal(err)
				}
				if err := png.Encode(f, tt.Image()); err != nil {
					t.Fatal(err)
				}
				f.Close()
			}
			// A seek changes the lyric in this window, without rendering the main one.
			a.pl.Seek(5 * time.Second)
			tt.Frame()
			if !tt.HasText("第二句歌词") || tt.HasText("第一句歌词") {
				t.Fatal("lyric did not follow a seek")
			}
			out := audio.NewPlayer(&silence{})
			out.SetVolume(0)
			defer out.Close()
			a.pl.Attach(out)
			a.pl.Seek(5 * time.Second)
			tt.Frame()
			if err := tt.Click("播放/暂停"); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if !tt.HasText("测试歌手") || tt.HasText("第二句歌词") {
				t.Fatal("pausing must replace the expanded lyric with the artist")
			}
			if dir := os.Getenv("NM_FRAMES"); dir != "" {
				f, err := os.Create(filepath.Join(dir, fmt.Sprintf("notch-lyrics-paused-%v.png", real)))
				if err != nil {
					t.Fatal(err)
				}
				if err := png.Encode(f, tt.Image()); err != nil {
					t.Fatal(err)
				}
				f.Close()
			}
			if err := tt.Click("播放/暂停"); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if !tt.HasText("第二句歌词") || tt.HasText("测试歌手") {
				t.Fatal("resuming must replace the artist with the current lyric")
			}
			n.open = false
			settle()
			if tt.HasText("第二句歌词") == real {
				t.Fatal("closing did not restore the correct compact layout")
			}
			if dir := os.Getenv("NM_FRAMES"); dir != "" {
				f, err := os.Create(filepath.Join(dir, fmt.Sprintf("notch-lyrics-compact-%v.png", real)))
				if err != nil {
					t.Fatal(err)
				}
				if err := png.Encode(f, tt.Image()); err != nil {
					t.Fatal(err)
				}
				f.Close()
			}
			// Missing lyrics must not leave the previous song's line on screen.
			a.pl.Cue([]qqmusic.Track{{ID: 8, Name: "无歌词歌曲", Duration: 60000}})
			a.pl.SetLyrics(nil)
			n.open = true
			settle()
			if !tt.HasText("暂无歌词") || tt.HasText("第二句歌词") {
				t.Fatal("old lyrics remained after changing tracks")
			}
		})
	}
}

// The notch, turned off, shows itself as the first song plays, and only
// then; its button keeps it on.
func TestNotchGreetsOnce(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("notch greeting is macOS-only")
	}
	a := newTestApp()
	a.settings = defaultSettings()
	n := &notch{a: a, w0: 200, h0: 32, winW: notchOpenW + 60, winH: 240}
	a.notch = n
	a.pl.Cue([]qqmusic.Track{{ID: 7, Name: "宝船", Duration: 200000}})
	a.syncWindows()
	if n.hello {
		t.Fatal("greeted before the song played")
	}
	out := audio.NewPlayer(&silence{})
	out.SetVolume(0)
	a.pl.Attach(out)
	defer out.Close()
	a.syncWindows()
	if !n.hello || !a.settings.NotchGreeted {
		t.Fatal("did not greet as the song played")
	}

	tt := ui.NewTester(n.view, int(n.winW), int(n.winH))
	tt.Advance(time.Second)
	tt.Frame()
	if err := tt.Click("开启刘海播放器"); err != nil {
		t.Fatal(err)
	}
	if !a.settings.Notch || n.hello {
		t.Fatalf("kept: notch %v, greeting %v", a.settings.Notch, n.hello)
	}

	// Turned off again, it does not greet the next song.
	a.settings.Notch = false
	a.pl.Cue([]qqmusic.Track{{ID: 8, Name: "下一首", Duration: 200000}})
	a.pl.Attach(out)
	a.syncWindows()
	if n.hello {
		t.Error("greeted twice")
	}
}

// The pointer resting on the greeting leaves it as it is, so its button can
// be clicked. It used to open the player, which hid the button, and leaving
// brought it back only to open again as the pointer came back to it.
func TestNotchGreetingKeptUnderPointer(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("notch greeting is macOS-only")
	}
	a := newTestApp()
	a.settings = defaultSettings()
	n := &notch{a: a, real: true, w0: 185, h0: 32, winW: notchOpenW + 60, winH: 240}
	a.notch = n
	a.pl.Cue([]qqmusic.Track{{ID: 7, Name: "宝船", Duration: 200000}})
	n.greet()
	if !n.hello {
		t.Fatal("did not greet")
	}

	defer func(f func() mygo.Point) { cursorPoint = f }(cursorPoint)
	cursorPoint = func() mygo.Point {
		return mygo.Point{X: int(n.screenX + n.winW/2 + 120), Y: int(n.screenY + n.h0 + 28)}
	}
	n.poll()
	n.enteredAt = n.lastInside.Add(-time.Second) // as if it had rested there
	n.poll()
	if n.open {
		t.Fatal("opened the player over the greeting")
	}

	tt := ui.NewTester(n.view, int(n.winW), int(n.winH))
	tt.Advance(time.Second)
	tt.Frame()
	if err := tt.Click("开启刘海播放器"); err != nil {
		t.Fatal(err)
	}
	if !a.settings.Notch || n.hello {
		t.Fatalf("kept: notch %v, greeting %v", a.settings.Notch, n.hello)
	}
}

// With NM_FRAMES set, saves the notch as it greets, growing and grown, and
// the player bar holding a song a login picked up.
func TestFirstUseFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	save := func(name string, tt *ui.Tester) {
		f, _ := os.Create(filepath.Join(dir, name+".png"))
		png.Encode(f, tt.Image())
		f.Close()
	}
	a := newTestApp()
	a.settings = defaultSettings()
	n := &notch{a: a, real: true, w0: 185, h0: 32, winW: notchOpenW + 60, winH: 240}
	a.notch = n
	a.pl.Cue([]qqmusic.Track{{ID: 7, Name: "NAINAINAI", Artists: []qqmusic.Artist{{Name: "ATARASHII GAKKO!"}}, Duration: 193877}})
	main := ui.NewTester(a.view, 1200, 800) // for the app's colors
	main.Frame()
	tt := ui.NewTester(n.view, int(n.winW), int(n.winH))
	tt.Frame()
	n.greet()
	for _, ms := range []int{120, 600} {
		tt.Advance(time.Duration(ms) * time.Millisecond)
		tt.Frame()
		save(fmt.Sprintf("notch-hello-%d", ms), tt)
	}

	a.picked = pickedUp{id: 7, heard: "3分钟前在手机上听过"}
	main.Advance(time.Second)
	main.Frame()
	save("pickup-bar", main)
}
