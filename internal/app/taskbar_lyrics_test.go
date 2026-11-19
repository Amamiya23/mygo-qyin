package app

import (
	"image"
	"image/color"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

// Changing the taskbar panel material persists both choices without closing controls or committing a pending seek.
func TestTaskbarLyricsMaterialSettings(t *testing.T) {
	if !supportsTaskbarLyrics() {
		t.Skip("taskbar lyrics are Windows-only")
	}
	a, d := taskbarControlsFixture()
	a.settings.LyricsMode = lyricsTaskbar
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	d.controlsOpen = true
	d.scrub = scrubState{v: 120, held: true, active: true}
	tt := ui.NewTester(a.settingsView, 560, 2400)
	tt.Frame()
	for _, choice := range []struct {
		label, material string
		vibrancy        mygo.Vibrancy
	}{
		{"白色", taskbarMaterialWhite, mygo.VibrancyNone},
		{"亚克力", taskbarMaterialAcrylic, mygo.VibrancyAcrylic},
		{"白色", taskbarMaterialWhite, mygo.VibrancyNone},
	} {
		if choice.material == taskbarMaterialAcrylic {
			// Move within the focused taskbar selector; the sidebar also has an Acrylic segment.
			tt.Key(0, ui.KeyLeft)
		} else if err := tt.Click(choice.label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		restored := readDesktopLyricsEntrySettings(t, a)
		if a.settings.TaskbarLyricsMaterial != choice.material || restored.TaskbarLyricsMaterial != choice.material || restored.taskbarControlsVibrancy() != choice.vibrancy {
			t.Fatalf("material not applied/saved: live=%q saved=%q backdrop=%q", a.settings.TaskbarLyricsMaterial, restored.TaskbarLyricsMaterial, restored.taskbarControlsVibrancy())
		}
		if !d.controlsOpen || !d.scrub.held || !d.scrub.active || !a.settings.TaskbarLyrics || a.pl.Position() != 40*time.Second {
			t.Fatal("material change closed controls, changed lyrics mode, or disturbed the pending seek")
		}
	}
}

// Choosing a lyric position swaps the active owner, saves the mode, and stops the previous taskbar monitor.
func TestTaskbarLyricsSettingsModeSwitch(t *testing.T) {
	if !supportsTaskbarLyrics() {
		t.Skip("taskbar lyrics are Windows-only")
	}
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	a.setDesktopLyricsEnabled(true)
	tt := ui.NewTester(a.settingsView, 560, 2200)
	tt.Frame()
	stopped := 0
	for _, choice := range []struct {
		label, mode string
		taskbar     bool
	}{
		{"任务栏歌词", "taskbar", true},
		{"桌面歌词", "desktop", false},
		{"任务栏歌词", "taskbar", true},
		{"桌面歌词", "desktop", false},
	} {
		if err := tt.Click(choice.label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		restored := readDesktopLyricsEntrySettings(t, a)
		if a.settings.lyricsMode() != choice.mode || restored.lyricsMode() != choice.mode {
			t.Fatalf("position %q was not applied and saved: live=%q saved=%q", choice.label, a.settings.lyricsMode(), restored.lyricsMode())
		}
		if a.settings.TaskbarLyrics != choice.taskbar || restored.TaskbarLyrics != choice.taskbar || a.settings.DesktopLyrics == choice.taskbar || restored.DesktopLyrics == choice.taskbar {
			t.Fatalf("position %q did not exclusively enable its lyrics: live=%+v saved=%+v", choice.label, a.settings, restored)
		}
		if (a.taskbarLyrics != nil) != choice.taskbar {
			t.Fatalf("taskbar owner lifecycle did not follow position %q", choice.label)
		}
		if choice.taskbar {
			a.taskbarLyrics.stop = func() { stopped++ }
		}
	}
	if stopped != 2 {
		t.Fatalf("disabling did not stop each monitor: stopped=%d", stopped)
	}
	a.setLyricsMode("taskbar")
	b := newTestApp()
	b.settings = readDesktopLyricsEntrySettings(t, a)
	b.settings.normalizeLyrics()
	b.syncTaskbarLyrics()
	if b.taskbarLyrics == nil {
		t.Fatal("restored preference did not recreate taskbar lyrics")
	}
}

// Choosing a taskbar position while lyrics are off saves the preference without opening either lyric window.
func TestTaskbarLyricsSettingsModeWhileDisabled(t *testing.T) {
	if !supportsTaskbarLyrics() {
		t.Skip("taskbar lyrics are Windows-only")
	}
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	tt := ui.NewTester(a.settingsView, 560, 2200)
	tt.Frame()
	for _, label := range []string{"任务栏歌词", "桌面歌词", "任务栏歌词"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if a.settings.lyricsEnabled() || a.desktopLyrics != nil || a.taskbarLyrics != nil {
			t.Fatalf("choosing %q opened disabled lyrics", label)
		}
	}
	restored := readDesktopLyricsEntrySettings(t, a)
	if restored.lyricsMode() != "taskbar" || restored.lyricsEnabled() {
		t.Fatal("disabled taskbar preference did not survive saving")
	}
	player := desktopLyricsEntryTester(a)
	player.Frame()
	if err := player.Click("任务栏歌词"); err != nil {
		t.Fatal(err)
	}
	player.Frame()
	if !a.settings.TaskbarLyrics || a.settings.DesktopLyrics || a.taskbarLyrics == nil {
		t.Fatal("player switch did not open the saved taskbar mode")
	}
}

// Paused seeks invalidate visible taskbar lyrics, while a hidden taskbar requests no redraw.
func TestTaskbarLyricsPausedSeekRedraw(t *testing.T) {
	a := newTestApp()
	a.pl.Cue([]qqmusic.Track{{ID: 1, Duration: 60000}})
	out := audio.NewPlayer(&silence{})
	defer out.Close()
	a.pl.Attach(out)
	a.pl.TogglePlay()
	a.settings.TaskbarLyrics = true
	a.taskbarLyrics = &taskbarLyrics{a: a, win: &mygo.Window{}, visible: true}
	defer func(f func(*mygo.Window)) { invalidateWindow = f }(invalidateWindow)
	seen := 0
	invalidateWindow = func(w *mygo.Window) {
		if w == a.taskbarLyrics.win {
			seen++
		}
	}
	defer func(f func(time.Duration, func())) { sessionAfter = f }(sessionAfter)
	sessionAfter = func(time.Duration, func()) {}
	a.pl.Changed = a.playerChanged
	a.pl.Seek(12 * time.Second)
	if seen == 0 || a.taskbarLyrics.revision == 0 {
		t.Fatal("paused seek did not refresh taskbar lyrics")
	}
	a.taskbarLyrics.visible = false
	seen = 0
	a.pl.Seek(20 * time.Second)
	if seen != 0 {
		t.Fatal("hidden taskbar lyrics still invalidate")
	}
}

// Occupied buttons and their safety margins never overlap the selected lyrics gap.
func TestTaskbarLyricsFreeSpace(t *testing.T) {
	tests := []struct {
		name         string
		occupied     []taskbarLyricsInterval
		start, width int
	}{
		{"centered buttons", []taskbarLyricsInterval{{400, 600}, {900, 1000}}, 8, 280},
		{"left before wider right gap", []taskbarLyricsInterval{{180, 500}}, 8, 164},
		{"left widgets", []taskbarLyricsInterval{{0, 150}, {500, 800}}, 158, 280},
		{"overlapping intervals", []taskbarLyricsInterval{{400, 800}, {0, 450}, {900, 1000}}, 0, 0},
		{"narrow but usable", []taskbarLyricsInterval{{0, 600}, {800, 1000}}, 608, 184},
		{"full", []taskbarLyricsInterval{{0, 1000}}, 0, 0},
		{"unknown", nil, 0, 0},
		{"invalid", []taskbarLyricsInterval{{20, 10}}, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, width := taskbarLyricsFreeSpace(1000, 280, 100, 8, tt.occupied)
			if start != tt.start || width != tt.width {
				t.Fatalf("got (%d,%d), want (%d,%d)", start, width, tt.start, tt.width)
			}
		})
	}
}

// The clickable child stays on the cover at every DPI, without covering lyrics.
func TestTaskbarLyricsCoverPlacement(t *testing.T) {
	tests := []struct {
		name               string
		dpi                uint32
		height, x, y, size int
	}{
		{"100 percent", 96, 42, 106, 24, 34},
		{"150 percent", 144, 63, 109, 26, 51},
		{"200 percent", 192, 84, 112, 28, 68},
		{"short taskbar", 96, 28, 106, 22, 24},
		{"fractional DPI", 120, 53, 108, 25, 43},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := taskbarLyricsCoverPlacement(taskbarLyricsPlacement{parent: 1, taskbar: 2, x: 100, y: 20, width: 600, height: tt.height, dpi: tt.dpi, dark: true})
			if p.x != tt.x || p.y != tt.y || p.width != tt.size || p.height != tt.size || p.parent != 1 || p.taskbar != 2 || p.dpi != tt.dpi || !p.dark {
				t.Fatalf("unexpected placement: %+v", p)
			}
		})
	}
}

// Hidden or impossibly small taskbars never leave an interactive cover child.
func TestTaskbarLyricsCoverPlacementUnavailable(t *testing.T) {
	for _, p := range []taskbarLyricsPlacement{{}, {parent: 1, width: 600, height: 4, dpi: 96}, {parent: 1, width: 10, height: 42, dpi: 96}} {
		if got := taskbarLyricsCoverPlacement(p); got != (taskbarLyricsPlacement{}) {
			t.Fatalf("unexpected interactive area: %+v", got)
		}
	}
}

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
