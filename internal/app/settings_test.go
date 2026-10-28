package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

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
		{"亚克力", "acrylic", mygo.VibrancyAcrylic},
		{"Mica", "mica", mygo.VibrancyMica},
		{"亚克力", "acrylic", mygo.VibrancyAcrylic},
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
