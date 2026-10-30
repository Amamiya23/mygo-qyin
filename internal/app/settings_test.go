package app

import (
	"encoding/json"
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

// With NM_FRAMES set, saves the close choices at the settings window's minimum width in both themes.
func TestCloseSettingsFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	for _, dark := range []bool{false, true} {
		a := newTestApp()
		a.tray = &mygo.Tray{}
		a.settings = defaultSettings()
		a.settings.QuitOnClose = dark
		tt := ui.NewTester(a.settingsView, 480, 720)
		tt.SetScale(2)
		tt.SetDark(dark)
		tt.Frame()
		name := "settings-close-light.png"
		if dark {
			name = "settings-close-dark.png"
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
