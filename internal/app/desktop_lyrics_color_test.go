package app

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

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
