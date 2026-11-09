package app

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestMainWindowCloseUsesConfiguredExitBehavior(t *testing.T) {
	a := newTestApp()
	a.tray = &mygo.Tray{}
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	tt := ui.NewTester(a.settingsView, 560, 2000)
	tt.Frame()
	// Verify both the default hide behavior and the real settings control.
	for _, exit := range []bool{false, true} {
		if exit {
			if err := tt.Click("退出应用"); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
		}
		hides, quits := 0, 0
		e := &mygo.CloseEvent{}
		a.mainWindowClose(e, func() { hides++ }, func() { quits++ })
		if !e.DefaultPrevented() || (exit && (quits != 1 || hides != 0)) || (!exit && (hides != 1 || quits != 0)) {
			t.Fatalf("exit=%t: hides=%d quits=%d", exit, hides, quits)
		}
	}
	// Without a usable tray/Dock, hiding must never strand the application.
	a.tray = nil
	a.settings.QuitOnClose = false
	if !a.canHideOnClose() {
		quits := 0
		a.mainWindowClose(&mygo.CloseEvent{}, func() { t.Fatal("hid without a tray") }, func() { quits++ })
		if quits != 1 {
			t.Fatal("did not quit without a way to restore the window")
		}
	}
	// Closing windows during a real quit must neither hide nor re-enter Quit.
	a.quitting = true
	e := &mygo.CloseEvent{}
	a.mainWindowClose(e, func() { t.Fatal("hid during quit") }, func() { t.Fatal("recursive quit") })
	if e.DefaultPrevented() {
		t.Fatal("window prevented final quit")
	}
}
