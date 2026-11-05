package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// Paused or dismissed taskbar controls must not ask for frames after their interaction animations settle.
func TestTaskbarLyricsControlsFrameBudget(t *testing.T) {
	for _, mode := range []string{"paused", "closed", "hidden"} {
		t.Run(mode, func(t *testing.T) {
			_, d := taskbarControlsFixture()
			d.controlsOpen = true
			if mode == "closed" {
				d.closeControls()
			}
			if mode == "hidden" {
				d.hide()
			}
			builds := 0
			tt := ui.NewTester(func(c *ui.Context) { builds++; d.controlsView(c) }, taskbarControlsWidth, taskbarControlsHeight)
			tt.Move(-10, -10)
			tt.Frame()
			tt.Advance(time.Second)
			before := builds
			tt.Advance(time.Second)
			if builds != before {
				t.Fatalf("%s taskbar controls kept rebuilding: %d extra frames", mode, builds-before)
			}
		})
	}
}
