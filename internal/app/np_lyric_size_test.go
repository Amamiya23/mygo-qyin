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

	"github.com/egoist/mygo/ui"

	"lilt/internal/lyrics"
)

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
