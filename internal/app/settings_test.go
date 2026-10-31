package app

import (
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// Choosing how the main panel closes updates and saves the choice in both directions.
func TestQuitOnCloseSettings(t *testing.T) {
	a := newTestApp()
	a.tray = &mygo.Tray{} // A successfully created tray makes hiding available.
	a.settings = defaultSettings()
	a.settings.QuitOnClose = false
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	tt := ui.NewTester(a.settingsView, 560, 2000)
	tt.Frame()
	keepRunning := "最小化到托盘"
	switch runtime.GOOS {
	case "darwin":
		keepRunning = "保持后台运行"
	case "linux":
		keepRunning = "隐藏到托盘"
	}
	for _, choice := range []struct {
		label       string
		quitOnClose bool
	}{
		{"退出应用", true},
		{keepRunning, false},
		{"退出应用", true},
	} {
		if err := tt.Click(choice.label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if a.settings.QuitOnClose != choice.quitOnClose {
			t.Fatalf("after choosing %q, QuitOnClose = %t, want %t", choice.label, a.settings.QuitOnClose, choice.quitOnClose)
		}
		b, err := os.ReadFile(a.settings.path)
		if err != nil {
			t.Fatal(err)
		}
		restored := defaultSettings()
		restored.QuitOnClose = !choice.quitOnClose
		if err := json.Unmarshal(b, &restored); err != nil {
			t.Fatal(err)
		}
		if restored.QuitOnClose != choice.quitOnClose {
			t.Fatalf("after choosing %q, restored QuitOnClose = %t, want %t", choice.label, restored.QuitOnClose, choice.quitOnClose)
		}
	}
}

// With NM_FRAMES set, saves settings and startup migration progress in both themes.
func TestSettingsFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	checks, downloads, checked := true, false, 0
	defer fakeUpdater(&checks, &downloads, &checked)()
	for _, dark := range []bool{false, true} {
		for _, width := range []int{480, 560, 880} {
			a := newTestApp()
			a.tray = &mygo.Tray{}
			a.settings = defaultSettings()
			a.settings.QuitOnClose = dark
			a.settings.CacheDir = filepath.Join(t.TempDir(), "音乐缓存")
			a.settings.CacheMB = 5120
			a.pl.Cache = newSongCache(a.settings)
			a.cacheUsed = 5 << 30
			if dark {
				pending := filepath.Join(t.TempDir(), "另一个磁盘")
				a.settings.PendingCacheDir = &pending
			}
			a.updates = true
			view, height := a.settingsView, 1400
			if width == 880 {
				a.cacheMoving = true
				a.cacheMoveDone, a.cacheMoveTotal = 2<<30, 5<<30
				view, height = a.view, 580
			}
			tt := ui.NewTester(view, width, height)
			tt.SetScale(2)
			tt.SetDark(dark)
			tt.Frame()
			appearance := "light"
			if dark {
				appearance = "dark"
			}
			name := fmt.Sprintf("settings-%d-%s.png", width, appearance)
			if a.cacheMoving {
				name = "cache-migration-" + appearance + ".png"
			}
			f, err := os.Create(filepath.Join(dir, name))
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
}

// Choosing a Windows sidebar material saves it for the next launch, in both directions.
func TestSidebarMaterialSettings(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows system backdrops")
	}
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	tt := ui.NewTester(a.settingsView, 560, 2000)
	tt.Frame()
	for _, choice := range []struct {
		label    string
		material string
		vibrancy mygo.Vibrancy
	}{
		{"Mica", "mica", mygo.VibrancyMica},
		{"亚克力", "acrylic", mygo.VibrancyAcrylic},
		{"Mica", "mica", mygo.VibrancyMica},
	} {
		if err := tt.Click(choice.label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if a.settings.SidebarMaterial != choice.material {
			t.Fatalf("selected %q, want %q", a.settings.SidebarMaterial, choice.material)
		}
		b, err := os.ReadFile(a.settings.path)
		if err != nil {
			t.Fatal(err)
		}
		restored := defaultSettings()
		if err := json.Unmarshal(b, &restored); err != nil {
			t.Fatal(err)
		}
		if restored.SidebarMaterial != choice.material || restored.sidebarVibrancy() != choice.vibrancy {
			t.Fatalf("restored material %q (%q), want %q (%q)", restored.SidebarMaterial, restored.sidebarVibrancy(), choice.material, choice.vibrancy)
		}
	}
}

// Choosing a progress effect from the dropdown saves it across launches and replaces the old smoke-only choice.
func TestProgressEffectSettings(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	if err := json.Unmarshal([]byte("{\"smoke\":true}"), &a.settings); err != nil {
		t.Fatal(err)
	}
	if got := a.settings.progressEffect(); got != progressSmoke {
		t.Fatalf("old smoke setting selects %q, want %q", got, progressSmoke)
	}
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	tt := ui.NewTester(a.settingsView, 560, 2000)
	tt.Frame()
	for _, choice := range []struct{ label, effect string }{
		{"波浪", progressSquiggly},
		{"标准", progressStandard},
		{"烟雾", progressSmoke},
		{"波浪", progressSquiggly},
	} {
		if err := tt.Click("选择进度条特效"); err != nil {
			t.Fatal(err)
		}
		if err := tt.Click(choice.label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if got := a.settings.progressEffect(); got != choice.effect {
			t.Fatalf("after choosing %q, progress effect = %q, want %q", choice.label, got, choice.effect)
		}
		b, err := os.ReadFile(a.settings.path)
		if err != nil {
			t.Fatal(err)
		}
		restored := defaultSettings()
		if err := json.Unmarshal(b, &restored); err != nil {
			t.Fatal(err)
		}
		if got := restored.progressEffect(); got != choice.effect {
			t.Fatalf("after choosing %q, restored effect = %q, want %q", choice.label, got, choice.effect)
		}
		if restored.Smoke != (choice.effect == progressSmoke) {
			t.Fatalf("after choosing %q, saved smoke compatibility = %t", choice.label, restored.Smoke)
		}
	}
}
