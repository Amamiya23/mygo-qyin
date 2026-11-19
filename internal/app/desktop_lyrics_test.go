package app

import (
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/ui"

	"lilt/internal/lyrics"
	"lilt/internal/qqmusic"
)

// Locking never removes the external tray action that restores pointer input.
func TestDesktopLyricsTrayUnlock(t *testing.T) {
	if !supportsDesktopLyrics() {
		t.Skip("no desktop lyrics entry on this platform")
	}
	a := newTestApp()
	a.setDesktopLyricsEnabled(true)
	a.desktopLyrics.setLocked(true)
	menu := a.trayMenu()
	for _, item := range menu.Items() {
		if item.Label == "解锁桌面歌词" {
			if item.Disabled {
				t.Fatal("locked lyrics cannot be unlocked from tray")
			}
			item.Click(item, nil)
			if a.settings.DesktopLyricsLocked {
				t.Fatal("tray unlock kept mouse passthrough enabled")
			}
			return
		}
	}
	t.Fatal("no external unlock action")
}

// A paused seek redraws the enabled lyrics panel along with the main player.
func TestDesktopLyricsPausedSeekRedraw(t *testing.T) {
	a := newTestApp()
	a.pl.Cue([]qqmusic.Track{{ID: 1, Duration: 60000}})
	out := audio.NewPlayer(&silence{})
	defer out.Close()
	a.pl.Attach(out)
	a.pl.TogglePlay()
	a.settings.DesktopLyrics = true
	a.win = &mygo.Window{}
	a.settingsWin = &mygo.Window{}
	a.desktopLyrics = &desktopLyrics{a: a, win: &mygo.Window{}}
	defer func(f func(*mygo.Window)) { invalidateWindow = f }(invalidateWindow)
	seen := map[*mygo.Window]int{}
	invalidateWindow = func(w *mygo.Window) { seen[w]++ }
	defer func(f func(time.Duration, func())) { sessionAfter = f }(sessionAfter)
	sessionAfter = func(time.Duration, func()) {}
	a.pl.Changed = a.playerChanged
	a.pl.Seek(12 * time.Second)
	if seen[a.desktopLyrics.win] == 0 || seen[a.win] == 0 || seen[a.settingsWin] == 0 {
		t.Fatal("paused seek did not redraw both windows")
	}
	a.settings.DesktopLyrics = false
	seen = map[*mygo.Window]int{}
	a.pl.Seek(20 * time.Second)
	if seen[a.desktopLyrics.win] != 0 {
		t.Fatal("disabled lyrics still invalidates")
	}
}

// Bounds remain reachable after a display disappears or becomes smaller.
func TestDesktopLyricsBoundsRecovery(t *testing.T) {
	work := mygo.Rectangle{X: -1280, Y: 40, Width: 1280, Height: 680}
	r := clampDesktopLyricsBounds(mygo.Rectangle{X: 2000, Y: 1600, Width: 1600, Height: 900}, work)
	if r.X != work.X || r.Y != work.Y || r.Width != work.Width || r.Height != work.Height {
		t.Fatalf("off-screen bounds were not fitted: %+v", r)
	}
}

func desktopLyricsEntryTester(a *app) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) {
		a.pal = paletteFor(c.Theme())
		a.playerBar(c)
	}, 880, int(playerBarHeight))
}

func readDesktopLyricsEntrySettings(t *testing.T, a *app) settings {
	t.Helper()
	b, err := os.ReadFile(a.settings.path)
	if err != nil {
		t.Fatal(err)
	}
	var restored settings
	if err := json.Unmarshal(b, &restored); err != nil {
		t.Fatal(err)
	}
	return restored
}

// The bottom-right switch toggles desktop lyrics and persists the choice even before a track is cued.
func TestDesktopLyricsPlayerEntryToggle(t *testing.T) {
	if !supportsDesktopLyrics() {
		t.Skip("no desktop lyrics entry on this platform")
	}
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	tt := desktopLyricsEntryTester(a)
	tt.Frame()
	for _, enabled := range []bool{true, false, true} {
		if err := tt.Click("桌面歌词"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if a.settings.DesktopLyrics != enabled || readDesktopLyricsEntrySettings(t, a).DesktopLyrics != enabled {
			t.Fatalf("desktop lyrics did not persist enabled=%t", enabled)
		}
	}
}

// A single right click directly locks or unlocks lyrics without a menu or opening Settings.
func TestDesktopLyricsPlayerEntryLockAndUnlock(t *testing.T) {
	if !supportsDesktopLyrics() {
		t.Skip("no desktop lyrics entry on this platform")
	}
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	tt := desktopLyricsEntryTester(a)
	tt.Frame()
	if err := tt.Click("桌面歌词"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for _, locked := range []bool{true, false, true, false} {
		if err := tt.RightClick("桌面歌词"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		restored := readDesktopLyricsEntrySettings(t, a)
		if !a.settings.DesktopLyrics || !restored.DesktopLyrics || a.settings.DesktopLyricsLocked != locked || restored.DesktopLyricsLocked != locked {
			t.Fatalf("right click did not persist enabled lyrics with locked=%t", locked)
		}
		if tt.Menu() != nil || a.settingsWin != nil {
			t.Fatal("right click opened a menu or Settings")
		}
	}
}

// Right-clicking a disabled lyric switch preserves its saved lock choice without enabling lyrics.
func TestDesktopLyricsPlayerEntryDisabledRightClick(t *testing.T) {
	if !supportsDesktopLyrics() {
		t.Skip("no desktop lyrics entry on this platform")
	}
	for _, locked := range []bool{false, true} {
		a := newTestApp()
		a.settings = defaultSettings()
		a.settings.path = filepath.Join(t.TempDir(), "settings.json")
		a.settings.DesktopLyricsLocked = locked
		a.settings.save()
		tt := desktopLyricsEntryTester(a)
		tt.Frame()
		if err := tt.RightClick("桌面歌词"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		restored := readDesktopLyricsEntrySettings(t, a)
		if a.settings.DesktopLyrics || restored.DesktopLyrics || a.settings.DesktopLyricsLocked != locked || restored.DesktopLyricsLocked != locked {
			t.Fatalf("disabled right click changed the saved feature or lock state: locked=%t", locked)
		}
		if tt.Menu() != nil {
			t.Fatal("disabled right click opened a context menu")
		}
	}
}

// The taskbar player switch preserves its mode across toggles and never applies desktop locking on right click.
func TestTaskbarLyricsPlayerEntryToggleAndRightClick(t *testing.T) {
	if !supportsTaskbarLyrics() {
		t.Skip("taskbar lyrics are Windows-only")
	}
	for _, locked := range []bool{false, true} {
		t.Run(map[bool]string{false: "unlocked", true: "locked"}[locked], func(t *testing.T) {
			a := newTestApp()
			a.settings = defaultSettings()
			a.settings.path = filepath.Join(t.TempDir(), "settings.json")
			a.settings.DesktopLyricsLocked = locked
			a.settings.DesktopLyricsDoubleLine = true
			a.settings.DesktopLyricsFontSize = 44
			a.setLyricsMode("taskbar")
			tt := desktopLyricsEntryTester(a)
			tt.Frame()
			for _, enabled := range []bool{true, false, true} {
				if err := tt.Click("任务栏歌词"); err != nil {
					t.Fatal(err)
				}
				tt.Frame()
				if err := tt.RightClick("任务栏歌词"); err != nil {
					t.Fatal(err)
				}
				tt.Frame()
				restored := readDesktopLyricsEntrySettings(t, a)
				if a.settings.TaskbarLyrics != enabled || restored.TaskbarLyrics != enabled || a.settings.DesktopLyrics || restored.DesktopLyrics {
					t.Fatalf("taskbar player switch did not exclusively persist enabled=%t", enabled)
				}
				if a.settings.lyricsMode() != "taskbar" || restored.lyricsMode() != "taskbar" {
					t.Fatal("turning taskbar lyrics off lost the chosen mode")
				}
				if a.settings.DesktopLyricsLocked != locked || restored.DesktopLyricsLocked != locked {
					t.Fatalf("taskbar right click changed desktop locked=%t", locked)
				}
				if tt.Menu() != nil || a.settingsWin != nil {
					t.Fatal("taskbar right click opened a menu or Settings")
				}
			}
			a.setLyricsMode("desktop")
			tt.Frame()
			if !a.settings.DesktopLyrics || a.settings.TaskbarLyrics || a.settings.DesktopLyricsLocked != locked || !a.settings.DesktopLyricsDoubleLine || a.settings.DesktopLyricsFontSize != 44 {
				t.Fatal("returning to desktop mode lost its lock, line, or font preferences")
			}
			if err := tt.RightClick("桌面歌词"); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if a.settings.DesktopLyricsLocked == locked {
				t.Fatal("returning to desktop mode did not restore right-click locking")
			}
		})
	}
}

// The tray switches the selected taskbar lyrics without exposing an enabled desktop unlock action.
func TestTaskbarLyricsTrayToggle(t *testing.T) {
	if !supportsTaskbarLyrics() {
		t.Skip("taskbar lyrics are Windows-only")
	}
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	a.settings.DesktopLyricsLocked = true
	a.setLyricsMode("taskbar")
	for _, enabled := range []bool{true, false, true} {
		clicked := false
		for _, item := range a.trayMenu().Items() {
			if item.Label == "解锁桌面歌词" && !item.Disabled {
				t.Fatal("taskbar mode exposes a desktop unlock action")
			}
			if item.Label == "任务栏歌词" {
				if item.Checked == enabled {
					t.Fatalf("tray checked state did not match the previous taskbar state before enabled=%t", enabled)
				}
				item.Click(item, nil)
				clicked = true
			}
		}
		if !clicked {
			t.Fatal("tray has no taskbar lyric switch")
		}
		restored := readDesktopLyricsEntrySettings(t, a)
		if a.settings.TaskbarLyrics != enabled || restored.TaskbarLyrics != enabled || a.settings.DesktopLyrics || restored.lyricsMode() != "taskbar" || !a.settings.DesktopLyricsLocked {
			t.Fatalf("tray did not persist taskbar enabled=%t while preserving the desktop lock preference", enabled)
		}
	}
}

// Export both lyric modes and their settings to inspect the controls at narrow widths.
func TestDesktopLyricsEntryFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip("set NM_FRAMES to inspect desktop lyrics controls")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	a := newTestApp()
	a.settings = defaultSettings()
	for _, state := range []struct {
		name, mode      string
		enabled, locked bool
	}{
		{"disabled", lyricsDesktop, false, false},
		{"enabled", lyricsDesktop, true, false},
		{"locked", lyricsDesktop, true, true},
		{"disabled", lyricsTaskbar, false, true},
		{"enabled", lyricsTaskbar, true, true},
	} {
		if state.mode == lyricsTaskbar && !supportsTaskbarLyrics() {
			continue
		}
		a.settings.LyricsMode = state.mode
		a.settings.DesktopLyrics = state.enabled && state.mode == lyricsDesktop
		a.settings.TaskbarLyrics = state.enabled && state.mode == lyricsTaskbar
		a.settings.DesktopLyricsLocked = state.locked
		tt := desktopLyricsEntryTester(a)
		tt.Frame()
		saveLyricsEntryFrame(t, dir, state.mode+"-lyrics-player-bar-"+state.name+".png", tt)
		if state.name == "enabled" {
			for _, dark := range []bool{false, true} {
				settingsView := ui.NewTester(a.settingsView, 480, 1300)
				settingsView.SetDark(dark)
				settingsView.Frame()
				appearance := "light"
				if dark {
					appearance = "dark"
				}
				saveLyricsEntryFrame(t, dir, state.mode+"-lyrics-settings-480-"+appearance+".png", settingsView)
			}
		}
	}
}

func saveLyricsEntryFrame(t *testing.T, dir, name string, tt *ui.Tester) {
	t.Helper()
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	err = png.Encode(f, tt.Image())
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("save lyrics frame: %v / %v", err, closeErr)
	}
}

// Choosing single or double desktop lyrics applies immediately and saves the choice in both directions.
func TestDesktopLyricsLineSettings(t *testing.T) {
	if !supportsDesktopLyrics() {
		t.Skip("no desktop lyrics settings on this platform")
	}
	a, d := desktopLyricsViewFixture()
	a.settings.DesktopLyricsLocked = true
	a.pl.SetLyrics(lyrics.Build("[00:01]Stay with me\n[00:12]Through the night", "[00:01]留在我身边\n[00:12]穿过黑夜", ""))
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	tt := ui.NewTester(a.settingsView, 560, 2000)
	lyricView := ui.NewTester(d.view, 760, 240)
	lyricView.SetPreferences(ui.Preferences{ReduceMotion: true})
	lyricView.Move(-10, -10)
	lyricView.Frame()
	tt.Frame()
	for _, choice := range []struct {
		label  string
		double bool
	}{
		{"双行", true},
		{"单行", false},
		{"双行", true},
	} {
		if err := tt.Click(choice.label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		restored := readDesktopLyricsEntrySettings(t, a)
		if a.settings.DesktopLyricsDoubleLine != choice.double || restored.DesktopLyricsDoubleLine != choice.double {
			t.Fatalf("after choosing %q, live/saved double-line choice = %t/%t, want %t", choice.label, a.settings.DesktopLyricsDoubleLine, restored.DesktopLyricsDoubleLine, choice.double)
		}
		lyricView.Frame()
		// Changing a Chinese-only option must not make a translated song disappear.
		frame := lyricView.Image()
		visible := false
		for y := 0; y < frame.Bounds().Dy() && !visible; y++ {
			for x := 0; x < frame.Bounds().Dx(); x++ {
				if _, _, _, alpha := frame.At(x, y).RGBA(); alpha != 0 {
					visible = true
					break
				}
			}
		}
		if !visible {
			t.Fatalf("translated lyrics disappeared after choosing %q", choice.label)
		}
	}
	if dir := os.Getenv("NM_FRAMES"); dir != "" {
		tt.Advance(time.Second)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(filepath.Join(dir, "desktop-lyrics-line-settings.png"))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, tt.Image())
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			t.Fatalf("save settings frame: %v / %v", err, closeErr)
		}
	}
}

// Presets and custom colors save across click and keyboard picker openings, Escape, and outside clicks.
func TestDesktopLyricsColorSettings(t *testing.T) {
	if !supportsDesktopLyrics() {
		t.Skip("no desktop lyrics settings on this platform")
	}
	a, _ := desktopLyricsViewFixture()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	tt := ui.NewTester(a.settingsView, 560, 2000)
	tt.SetLocale("en")
	tt.Frame()
	check := func(want ui.Color) {
		t.Helper()
		restored := readDesktopLyricsEntrySettings(t, a)
		if got := a.settings.desktopLyricsColor(); got != want {
			t.Fatalf("live lyrics color = %#v, want %#v", got, want)
		}
		if got := restored.desktopLyricsColor(); got != want {
			t.Fatalf("saved lyrics color = %#v, want %#v", got, want)
		}
	}
	if err := tt.Click("桌面歌词颜色：薄荷"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	check(ui.Hex("#b1f1cb"))
	for i, value := range []string{"#9ab8d6", "#b7a1c980", "#648fba"} {
		switch i {
		case 0:
			if err := tt.Click("自定义桌面歌词颜色"); err != nil {
				t.Fatal(err)
			}
		case 1:
			tt.Key(0, ui.KeySpace)
		case 2:
			tt.Key(0, ui.KeyEnter)
		}
		tt.Frame()
		if err := tt.Click("Hex"); err != nil {
			t.Fatal(err)
		}
		tt.Key(ui.Cmd, ui.KeyA)
		tt.Type(value)
		tt.Frame()
		check(ui.Hex(value))
		tt.Key(0, ui.KeyEscape)
		tt.Frame()
		check(ui.Hex(value))
	}
	if err := tt.Click("自定义桌面歌词颜色"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("桌面歌词颜色：月白"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	check(ui.Hex("#ffffff"))
}

// Export the settings row and open picker at narrow widths for visual inspection in both themes.
func TestDesktopLyricsColorFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip("set NM_FRAMES to inspect desktop lyric colors")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{480, 560} {
		for _, dark := range []bool{false, true} {
			a := newTestApp()
			a.settings = defaultSettings()
			a.settings.DesktopLyricsColor = radixGreenDark[11]
			tt := ui.NewTester(a.settingsView, width, 1100)
			tt.SetLocale("zh-CN")
			tt.SetDark(dark)
			tt.Frame()
			prefix := fmt.Sprintf("lyrics-color-%d-dark-%t", width, dark)
			saveLyricsEntryFrame(t, dir, prefix+".png", tt)
			if err := tt.Click("自定义桌面歌词颜色"); err != nil {
				t.Fatal(err)
			}
			tt.Move(-10, -10)
			tt.Advance(200 * time.Millisecond)
			saveLyricsEntryFrame(t, dir, prefix+"-picker.png", tt)
		}
	}
}

// Invalid persisted colors fall back to visible white while valid custom RGB and alpha values survive loading.
func TestDesktopLyricsColorParsing(t *testing.T) {
	for _, value := range []string{"", "#bad", "#gg1122", "#1122334455", "#1122334"} {
		s := settings{DesktopLyricsColor: value}
		if got := s.desktopLyricsColor(); got != radixPaper {
			t.Errorf("invalid color %q = %#v, want white", value, got)
		}
	}
	for _, tc := range []struct {
		value string
		want  ui.Color
	}{
		{"#9Ab8D6", ui.RGB(0x9a, 0xb8, 0xd6)},
		{"  9ab8d680  ", ui.Color{R: 0x9a, G: 0xb8, B: 0xd6, A: 0x80}},
	} {
		s := settings{DesktopLyricsColor: tc.value}
		if got := s.desktopLyricsColor(); got != tc.want {
			t.Errorf("color %q = %#v, want %#v", tc.value, got, tc.want)
		}
	}
}

// Zero-opacity custom colors must hide the complete lyric layer instead of inheriting white text or leaving outlines.
func TestDesktopLyricsColorTransparency(t *testing.T) {
	a, d := desktopLyricsViewFixture()
	a.settings.DesktopLyricsLocked = true
	tt := ui.NewTester(d.view, 760, 240)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	visible := func() bool {
		t.Helper()
		tt.Frame()
		frame := tt.Image()
		for y := 0; y < frame.Bounds().Dy(); y++ {
			for x := 0; x < frame.Bounds().Dx(); x++ {
				if _, _, _, alpha := frame.At(x, y).RGBA(); alpha != 0 {
					return true
				}
			}
		}
		return false
	}
	if !visible() {
		t.Fatal("fixture has no visible lyrics")
	}
	a.setDesktopLyricsColor(ui.Hex("#b1f1cb00"))
	if visible() {
		t.Fatal("fully transparent lyrics retained text or outlines")
	}
	a.setDesktopLyricsColor(ui.Hex("#b1f1cb"))
	if !visible() {
		t.Fatal("restoring opacity did not reveal the lyrics")
	}
}

// Colored words fade from white to the selected color without dimming, while the original white keeps its brightness ramp.
func TestDesktopLyricsColorProgress(t *testing.T) {
	for _, color := range []ui.Color{ui.Hex(radixGreenDark[11]), ui.Hex("#314159")} {
		for _, progress := range []float32{0, 0.5, 1} {
			got := desktopLyricWordColor(color, progress)
			if got.A != radixWhite(12).A {
				t.Fatalf("colored word opacity changed at progress %v: %v", progress, got.A)
			}
			if progress == 0 && got != radixWhite(12) {
				t.Fatalf("unsung word did not start white: %+v", got)
			}
			if progress == 1 && got != color.Alpha(desktopLyricsSungAlpha) {
				t.Fatalf("sung word lost selected color: %+v", got)
			}
			if progress == 0.5 && (got.R <= color.R || got.G <= color.G || got.B <= color.B || got.R == 255 || got.G == 255 || got.B == 255) {
				t.Fatalf("word did not gradually blend toward the selected color: %+v", got)
			}
		}
	}
	if got := desktopLyricWordColor(radixPaper, 0); got != radixPaper.Alpha(desktopLyricsRestAlpha) {
		t.Fatalf("default white lost its unsung brightness: %+v", got)
	}
	if got := desktopLyricWordColor(radixPaper, 1); got != radixWhite(12) {
		t.Fatalf("default white lost its sung brightness: %+v", got)
	}
}
