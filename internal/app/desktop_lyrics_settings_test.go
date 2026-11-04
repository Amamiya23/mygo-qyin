package app

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"lilt/internal/lyrics"
)

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
