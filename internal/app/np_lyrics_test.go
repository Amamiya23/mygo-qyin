package app

import (
	"encoding/json"
	"fmt"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"lilt/internal/lyrics"
	"lilt/internal/qqmusic"
)

// Paused audio keeps its clock while the minimized window skips frames.
func TestPausedLyricsAfterLongMinimize(t *testing.T) {
	for _, reduceMotion := range []bool{false, true} {
		t.Run(fmt.Sprintf("reduce-motion=%t", reduceMotion), func(t *testing.T) {
			a := newTestApp()
			a.settings = defaultSettings()
			a.win = &mygo.Window{}
			a.pl.Cue([]qqmusic.Track{{ID: 7, Duration: 400000}})
			var lrc string
			for i := 0; i < 13; i++ {
				lrc += fmt.Sprintf("[%02d:%02d.00]第%d句歌词\n", i/2, i%2*30, i)
			}
			a.pl.SetLyrics(lyrics.Build(lrc, "", ""))
			a.pl.Seek(390 * time.Second)
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Column(c).Fill().Background(radixSage(true, 1)).Children(func() { a.lyricsPane(c, a.pl.Current(), 500) })
			}, 600, 500)
			tt.SetPreferences(ui.Preferences{ReduceMotion: reduceMotion})
			tt.Advance(2 * time.Second)
			if a.lyricAt != 12 || a.lyricScroll.Y <= 0 {
				t.Fatal("fixture did not follow the paused line")
			}
			a.suspendMainWindow()
			tt.SetWindowHidden(true)
			tt.Advance(5 * time.Minute)
			// Reproduce a restored scroll layout reporting the start again,
			// while the paused audio clock and current lyric stay unchanged.
			a.lyricScroll.Y = 0
			tt.SetWindowHidden(false)
			oldInvalidate := invalidateWindow
			t.Cleanup(func() { invalidateWindow = oldInvalidate })
			invalidateWindow = func(*mygo.Window) { tt.Frame() }
			a.resumeMainWindow()
			tt.Advance(2 * time.Second)
			if a.pl.Playing() || a.pl.Position() != 390*time.Second {
				t.Fatal("window restoration changed paused playback")
			}
			if r, ok := tt.Find("第12句歌词"); !ok || r.Y < 0 || r.Y+r.H > 500 || a.lyricBrowse {
				t.Fatalf("restored lyric is not visible: box=%+v found=%v scroll=%+v line=%d browse=%v", r, ok, a.lyricScroll, a.lyricAt, a.lyricBrowse)
			}
			if dir := os.Getenv("NM_FRAMES"); dir != "" {
				saveLyricsEntryFrame(t, dir, fmt.Sprintf("lyrics-paused-restore-%t.png", reduceMotion), tt)
			}
		})
	}
}

// Playback can advance without the hidden window receiving a frame. Resuming
// rebuilds both the clock and the active lyric before the next line's timer.
func TestLyricsResumeBeforeNextLineTimer(t *testing.T) {
	frames := os.Getenv("NM_FRAMES")
	if frames != "" {
		if err := os.MkdirAll(frames, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, reduceMotion := range []bool{false, true} {
		t.Run(fmt.Sprintf("reduce-motion=%t", reduceMotion), func(t *testing.T) {
			a := newTestApp()
			a.settings = defaultSettings()
			a.win = &mygo.Window{}
			a.pl.Cue([]qqmusic.Track{{ID: 7, Duration: 400000}})
			var lrc string
			for i := 0; i < 12; i++ {
				lrc += fmt.Sprintf("[%02d:%02d.00]第%d句歌词\n", i/2, i%2*30, i)
			}
			a.pl.SetLyrics(lyrics.Build(lrc, "", ""))
			a.pl.Hold(time.Second)
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Column(c).Fill().Background(radixSage(true, 1)).Children(func() {
					ui.Text(c, clock(a.pl.Position())).TextColor(white)
					a.lyricsPane(c, a.pl.Current(), 500)
				})
			}, 600, 500)
			tt.SetPreferences(ui.Preferences{ReduceMotion: reduceMotion})
			tt.Advance(time.Second)
			if a.lyricAt != 0 {
				t.Fatalf("initial line=%d", a.lyricAt)
			}
			if frames != "" {
				saveLyricsEntryFrame(t, frames, fmt.Sprintf("lyrics-resume-%t-before.png", reduceMotion), tt)
			}
			// Move only the playback clock, as audio does while minimized.
			a.pl.Hold(190 * time.Second)
			if tt.HasText("3:10") || a.lyricAt != 0 {
				t.Fatal("fixture did not retain the old frame")
			}
			before := tt.Now()
			oldInvalidate := invalidateWindow
			defer func() { invalidateWindow = oldInvalidate }()
			requests := 0
			invalidateWindow = func(w *mygo.Window) {
				if w != a.win {
					t.Fatal("resume invalidated another window")
				}
				requests++
				tt.Frame()
			}
			a.resumeMainWindow()
			if requests != 1 || !tt.HasText("3:10") || tt.Now() != before {
				t.Fatalf("resume waited for a timer: requests=%d line=%d clock=%v", requests, a.lyricAt, tt.Texts())
			}
			tt.Advance(2 * time.Second)
			if a.lyricAt != 6 || a.lyricScroll.Y <= 0 {
				t.Fatal("resumed lyrics did not scroll to the current line")
			}
			if r, ok := tt.Find("第6句歌词"); !ok || r.Y < 0 || r.Y+r.H > 500 {
				t.Fatalf("current line is outside the resumed pane: %+v, found=%v", r, ok)
			}
			if frames != "" {
				saveLyricsEntryFrame(t, frames, fmt.Sprintf("lyrics-resume-%t-after.png", reduceMotion), tt)
			}
		})
	}
}

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

// Scrolling the lyrics stops them following the line sung; "回到当前" brings
// it back and follows again, as choosing a line to sing from does.
func TestLyricsBrowseAndFollowAgain(t *testing.T) {
	frames := os.Getenv("NM_FRAMES")
	if frames != "" {
		if err := os.MkdirAll(frames, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, reduceMotion := range []bool{false, true} {
		t.Run(fmt.Sprintf("reduce-motion=%t", reduceMotion), func(t *testing.T) {
			a := newTestApp()
			a.settings = defaultSettings()
			a.pl.Cue([]qqmusic.Track{{ID: 7, Duration: 400000}})
			var lrc string
			for i := 0; i < 24; i++ {
				lrc += fmt.Sprintf("[%02d:%02d.00]第%d句歌词\n", i/2, i%2*30, i)
			}
			a.pl.SetLyrics(lyrics.Build(lrc, "", ""))
			a.pl.Hold(time.Second)
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Column(c).Size(600, 500).Background(radixSage(true, 1)).Children(func() { a.lyricsPane(c, a.pl.Current(), 500) })
			}, 600, 500)
			tt.SetPreferences(ui.Preferences{ReduceMotion: reduceMotion})
			tt.Advance(time.Second)
			if _, ok := tt.Find("回到当前"); ok || a.lyricBrowse {
				t.Fatal("following lyrics offered to follow again")
			}

			tt.Scroll(300, 250, 0, 400)
			tt.Advance(time.Second)
			browsed := a.lyricScroll.Y
			if !a.lyricBrowse || browsed <= 0 {
				t.Fatalf("scrolling did not leave the line sung: browse=%v y=%v", a.lyricBrowse, browsed)
			}
			if _, ok := tt.Find("回到上方的当前歌词"); !ok {
				t.Fatal("the line sung, scrolled above, is not pointed up to")
			}
			// A real manual scroll remains a browsing choice after a long
			// minimize; restoration must not force the active line back.
			a.win = &mygo.Window{}
			a.suspendMainWindow()
			tt.SetWindowHidden(true)
			tt.Advance(5 * time.Minute)
			tt.SetWindowHidden(false)
			oldInvalidate := invalidateWindow
			t.Cleanup(func() { invalidateWindow = oldInvalidate })
			invalidateWindow = func(*mygo.Window) { tt.Frame() }
			a.resumeMainWindow()
			tt.Advance(2 * time.Second)
			if !a.lyricBrowse || a.lyricScroll.Y != browsed {
				t.Fatal("restoration discarded the listener's lyric browsing position")
			}
			if frames != "" {
				saveLyricsEntryFrame(t, frames, fmt.Sprintf("lyrics-browse-up-%t.png", reduceMotion), tt)
			}
			// A line further on begins: the lyrics stay where the listener
			// reads, and the button points down to it.
			a.pl.Hold(5 * time.Minute)
			tt.Frame()
			tt.Advance(2 * time.Second)
			if a.lyricScroll.Y != browsed || a.lyricAt == 10 {
				t.Fatalf("browsed lyrics followed the line sung: y=%v (was %v) line=%d", a.lyricScroll.Y, browsed, a.lyricAt)
			}
			if frames != "" {
				saveLyricsEntryFrame(t, frames, fmt.Sprintf("lyrics-browse-down-%t.png", reduceMotion), tt)
			}
			if err := tt.Click("回到下方的当前歌词"); err != nil {
				t.Fatal(err)
			}
			tt.Advance(2 * time.Second)
			if a.lyricBrowse || a.lyricAt != 10 {
				t.Fatalf("follow again: browse=%v line=%d", a.lyricBrowse, a.lyricAt)
			}
			if r, ok := tt.Find("第10句歌词"); !ok || r.Y < 0 || r.Y+r.H > 500 {
				t.Fatalf("line sung is outside the pane: %+v, found=%v", r, ok)
			}
			if _, ok := tt.Find("回到当前"); ok {
				t.Fatal("button stayed after following again")
			}

			// Scrolled away again, a line clicked is sung from and followed.
			tt.Scroll(300, 250, 0, -200)
			tt.Advance(time.Second)
			if !a.lyricBrowse {
				t.Fatal("scrolling back did not leave the line sung")
			}
			if err := tt.Click("第8句歌词"); err != nil {
				t.Fatal(err)
			}
			tt.Advance(2 * time.Second)
			if a.lyricBrowse || a.lyricAt != 8 || a.pl.Position() != 4*time.Minute {
				t.Fatalf("clicked line: browse=%v line=%d position=%v", a.lyricBrowse, a.lyricAt, a.pl.Position())
			}
		})
	}
}

func lyricSizeTestApp() *app {
	a := playerHoverApp()
	a.pl.SetLyrics(lyrics.Build("[00:01.00]还没好好地感受\n[00:20.00]雪花绽放的气候\n[00:40.00]我们一起颤抖会更明白什么是温柔", "[00:01.00]Before we could feel it all\n[00:20.00]The season when snowflakes bloom\n[00:40.00]We shiver together and begin to understand what tenderness means", ""))
	a.comments = commentState{track: a.pl.Current().ID, loaded: true}
	a.nowPlaying, a.npPane = true, paneLyrics
	return a
}

func lyricSizeTester(a *app, w, h int) *ui.Tester {
	tt := ui.NewTesterWith(a.view, w, h, ui.TesterOptions{Preferences: ui.Preferences{ReduceMotion: true}})
	tt.Move(float32(w)/2, float32(h)/2)
	tt.Advance(time.Second)
	tt.Frame()
	return tt
}

// Dragging and keyboard-adjusting the lyric size immediately resizes the lyrics, persists the choice, and leaves playback alone.
func TestNowPlayingLyricSizeInput(t *testing.T) {
	a := lyricSizeTestApp()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	tt := lyricSizeTester(a, 1200, 800)
	before, ok := tt.Find("雪花绽放的气候")
	if !ok {
		t.Fatal("cannot target the current lyric")
	}
	if err := tt.Click("歌词字号"); err != nil {
		t.Fatal(err)
	}
	slider, ok := tt.Find("调整歌词字号")
	if !ok {
		t.Fatal("cannot target the lyric size slider")
	}
	x, y := slider.X+slider.W*0.8, slider.Y+slider.H/2
	tt.Press(slider.X+slider.W/3, y)
	tt.Move(x, y)
	tt.Frame()
	size := a.settings.lyricsFontSize()
	if size <= 45 || size > 60 {
		t.Fatalf("dragging did not preview the larger size: %v", size)
	}
	after, ok := tt.Find("雪花绽放的气候")
	if !ok || after.H <= before.H {
		t.Fatalf("changing the size did not enlarge the current lyric: before=%v after=%v found=%v", before, after, ok)
	}
	tt.Release(x, y)
	tt.Frame()
	tt.Key(0, ui.KeyLeft)
	tt.Frame()
	if got := a.settings.lyricsFontSize(); got != size-1 {
		t.Fatalf("the focused slider did not step down by one: %v, was %v", got, size)
	}
	data, err := os.ReadFile(a.settings.path)
	if err != nil {
		t.Fatal(err)
	}
	var restored settings
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.lyricsFontSize() != a.settings.lyricsFontSize() {
		t.Fatalf("saved lyric size is %v, want %v", restored.lyricsFontSize(), a.settings.lyricsFontSize())
	}
	b := lyricSizeTestApp()
	b.settings = restored
	bt := lyricSizeTester(b, 1200, 800)
	if err := bt.Click("歌词字号"); err != nil {
		t.Fatal(err)
	}
	access, ok := bt.AccessByRole(ui.RoleSlider, "调整歌词字号")
	if !ok || access.Now != restored.lyricsFontSize() {
		t.Fatalf("reopening with saved settings lost the slider value: %+v found=%v", access, ok)
	}
	if a.pl.Playing() || a.pl.Position() != 24*time.Second || a.npPane != paneLyrics || !a.nowPlaying {
		t.Fatal("adjusting lyric size changed playback or left the lyric pane")
	}
	if err := tt.Click("恢复默认字号"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.settings.lyricsFontSize() != 30 {
		t.Fatalf("restoring the default left size at %v", a.settings.lyricsFontSize())
	}
	data, err = os.ReadFile(a.settings.path)
	if err != nil {
		t.Fatal(err)
	}
	restored = settings{}
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.lyricsFontSize() != 30 {
		t.Fatalf("restoring the default did not persist: %v", restored.lyricsFontSize())
	}
}

// Dismissing the size panel by Escape, its trigger, or an outside click preserves the current lyric and playback.
func TestNowPlayingLyricSizeDismissal(t *testing.T) {
	for _, dismiss := range []string{"escape", "trigger", "outside"} {
		t.Run(dismiss, func(t *testing.T) {
			a := lyricSizeTestApp()
			a.settings.LyricsFontSize = 42
			tt := lyricSizeTester(a, 1200, 800)
			if err := tt.Click("歌词字号"); err != nil {
				t.Fatal(err)
			}
			if !a.lyricsSizeOpen {
				t.Fatal("the lyric size panel did not open")
			}
			tt.Move(10, 400)
			tt.Advance(250 * time.Millisecond)
			if !a.lyricsSizeOpen {
				t.Fatal("leaving the controls closed an explicitly opened panel")
			}
			switch dismiss {
			case "escape":
				tt.Key(0, ui.KeyEscape)
			case "trigger":
				if err := tt.Click("歌词字号"); err != nil {
					t.Fatal(err)
				}
			case "outside":
				tt.ClickAt(10, 400)
			}
			tt.Frame()
			if a.lyricsSizeOpen || !a.nowPlaying || a.npPane != paneLyrics || a.settings.lyricsFontSize() != 42 || a.pl.Playing() || a.pl.Position() != 24*time.Second {
				t.Fatal("dismissing lyric size changed playback, size, or the current pane")
			}
		})
	}
}

// Leaving the lyric pane or closing Now Playing clears its open size panel so it does not return on reentry.
func TestNowPlayingLyricSizeLeavesWithPane(t *testing.T) {
	for _, leave := range []string{"pane", "now-playing"} {
		t.Run(leave, func(t *testing.T) {
			a := lyricSizeTestApp()
			tt := lyricSizeTester(a, 1200, 800)
			if err := tt.Click("歌词字号"); err != nil {
				t.Fatal(err)
			}
			if leave == "pane" {
				a.choosePane(paneQueue)
			} else {
				a.nowPlaying = false
			}
			tt.Frame()
			if a.lyricsSizeOpen {
				t.Fatal("the panel remained open after its lyric view left")
			}
			a.nowPlaying = true
			a.choosePane(paneLyrics)
			tt.Frame()
			tt.Advance(time.Second)
			if a.lyricsSizeOpen {
				t.Fatal("the size panel reopened on returning to lyrics")
			}
		})
	}
}

// Resizing paused lyrics used to follow stale line geometry; the same later line stays at its reading position and remains seekable after resizing.
func TestNowPlayingLyricSizeKeepsCurrentLine(t *testing.T) {
	a := lyricSizeTestApp()
	var lines []lyrics.Line
	for i := range 12 {
		lines = append(lines, lyrics.Line{At: time.Duration(i*10) * time.Second, Text: fmt.Sprintf("第%02d句歌词", i+1), Translation: "A translated lyric"})
	}
	a.pl.SetLyrics(lines)
	tt := lyricSizeTester(a, 1200, 800)
	// Seek after the pane has laid out so the initial anchor uses its final viewport.
	a.pl.Seek(65 * time.Second)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	before, ok := tt.Find("第07句歌词")
	if !ok {
		t.Fatal("cannot target the current later lyric")
	}
	if err := tt.Click("歌词字号"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyEnd)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	if a.settings.lyricsFontSize() != 60 {
		t.Fatalf("End did not choose the largest lyric size: %v", a.settings.lyricsFontSize())
	}
	after, ok := tt.Find("第07句歌词")
	if !ok || math.Abs(float64(after.Y-before.Y)) > 3 {
		t.Fatalf("resizing lost the current reading position: before=%v after=%v found=%v", before, after, ok)
	}
	tt.Key(0, ui.KeyEscape)
	if err := tt.Click("第07句歌词"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.pl.Position() != time.Minute || a.pl.Playing() || a.lyricAt != 6 {
		t.Fatalf("the enlarged line no longer seeks correctly: position=%v playing=%v line=%d", a.pl.Position(), a.pl.Playing(), a.lyricAt)
	}
}

// Space opens the focused size trigger and resets from its panel without invoking the player's global Space shortcut.
func TestNowPlayingLyricSizeKeyboard(t *testing.T) {
	a := lyricSizeTestApp()
	a.settings.LyricsFontSize = 42
	tt := lyricSizeTester(a, 1200, 800)
	if err := tt.Click("歌词字号"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if !tt.Focused("歌词字号") {
		t.Fatal("dismissing the panel did not return focus to the trigger")
	}
	tt.Key(0, ui.KeySpace)
	tt.Frame()
	if !a.lyricsSizeOpen || a.pl.Playing() {
		t.Fatal("Space on the focused trigger did not open the panel locally")
	}
	tt.Key(0, ui.KeyTab)
	if !tt.Focused("恢复默认字号") {
		t.Fatal("Tab from the slider did not reach the reset button")
	}
	tt.Key(0, ui.KeySpace)
	tt.Frame()
	if a.settings.lyricsFontSize() != 30 || a.pl.Playing() || a.pl.Position() != 24*time.Second {
		t.Fatalf("Space on reset changed playback or failed to reset: size=%v playing=%v position=%v", a.settings.lyricsFontSize(), a.pl.Playing(), a.pl.Position())
	}
}

// NM_FRAMES saves the lyric size panel and long translated lyrics at both size limits in regular and narrow windows.
func TestNowPlayingLyricSizeFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip("NM_FRAMES not set")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, scene := range []struct {
		w, h int
		size float64
		dark bool
	}{
		{1200, 800, 18, false},
		{1200, 800, 60, true},
		{860, 560, 18, false},
		{860, 560, 60, true},
	} {
		name := fmt.Sprintf("%dx%d-%g", scene.w, scene.h, scene.size)
		t.Run(name, func(t *testing.T) {
			a := lyricSizeTestApp()
			a.settings.LyricsFontSize = scene.size
			a.comments.total = 12345678
			a.pl.Seek(42 * time.Second)
			tt := lyricSizeTester(a, scene.w, scene.h)
			tt.SetDark(scene.dark)
			tt.Frame()
			if err := tt.Click("歌词字号"); err != nil {
				t.Fatal(err)
			}
			tt.Advance(250 * time.Millisecond)
			tt.Frame()
			f, err := os.Create(filepath.Join(dir, "lyric-size-"+name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if err := png.Encode(f, tt.Image()); err != nil {
				t.Fatal(err)
			}
		})
	}
}
