package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// Choosing a lyric font saves it, and choosing the default clears it again.
func TestLyricsFontSettings(t *testing.T) {
	families := ui.FontFamilies()
	if len(families) == 0 {
		t.Skip("no system fonts available")
	}
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	tt := ui.NewTester(a.settingsView, 900, 2000)
	tt.Frame()
	for _, choice := range []struct{ label, family string }{
		{families[0].Name, families[0].Name},
		{"跟随默认", ""},
	} {
		if err := tt.Click("选择歌词字体"); err != nil {
			t.Fatal(err)
		}
		if err := tt.Click(choice.label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		data, err := os.ReadFile(a.settings.path)
		if err != nil {
			t.Fatal(err)
		}
		var saved settings
		if err := json.Unmarshal(data, &saved); err != nil {
			t.Fatal(err)
		}
		if a.settings.LyricsFont != choice.family || saved.LyricsFont != choice.family {
			t.Fatalf("live font = %q, saved font = %q, want %q", a.settings.LyricsFont, saved.LyricsFont, choice.family)
		}
	}
}

// Changing the lyric font asks the playing line to follow its new layout.
func TestNowPlayingLyricFontRelayout(t *testing.T) {
	a := lyricSizeTestApp()
	tt := lyricSizeTester(a, 1200, 800)
	a.settings.LyricsFont = "serif"
	tt.Frame()
	if a.lyricFont != "serif" || a.lyricAt != -2 {
		t.Fatalf("font = %q, follow line = %d after font change", a.lyricFont, a.lyricAt)
	}
	tt.Advance(time.Millisecond)
	tt.Frame()
	if a.lyricAt < 0 {
		t.Fatalf("following did not resume after the new layout: %d", a.lyricAt)
	}
}
