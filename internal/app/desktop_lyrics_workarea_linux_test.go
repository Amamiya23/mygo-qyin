package app

import (
	"os"
	"testing"

	"github.com/egoist/mygo"
)

func TestDesktopLyricsAvoidPlasmaTaskbar(t *testing.T) {
	screen := mygo.Rectangle{Width: 1707, Height: 1067}
	panels := []desktopLyricsPanel{{Location: "bottom", Height: 46, Screen: screen}}
	work := desktopLyricsPanelWorkArea(screen, panels)
	if work.Height != 1021 {
		t.Fatalf("taskbar was not excluded: %+v", work)
	}
	// A saved position or a drag into the panel is corrected to its top edge,
	// including when a translation makes the lyrics taller.
	for _, height := range []int{117, 151, 220} {
		r := clampDesktopLyricsBounds(mygo.Rectangle{X: 400, Y: 1000, Width: 760, Height: height}, work)
		if r.Y+r.Height != 1021 {
			t.Fatalf("lyrics overlap the taskbar: %+v", r)
		}
	}
	// Applying work area insets twice must not move lyrics further up.
	if got := desktopLyricsPanelWorkArea(work, panels); got != work {
		t.Fatalf("insets applied twice: %+v", got)
	}
}

func TestDesktopLyricsPlasmaPanelsAcrossDisplays(t *testing.T) {
	left := mygo.Rectangle{X: -1707, Width: 1707, Height: 1067}
	right := mygo.Rectangle{Width: 1920, Height: 1080}
	panels := []desktopLyricsPanel{
		{Location: "left", Height: 48, Screen: left},
		{Location: "top", Height: 32, Screen: left},
		{Location: "bottom", Height: 46, Screen: right},
	}
	if got := desktopLyricsPanelWorkArea(left, panels); got != (mygo.Rectangle{X: -1659, Y: 32, Width: 1659, Height: 1035}) {
		t.Fatalf("wrong work area on the left display: %+v", got)
	}
	if got := desktopLyricsPanelWorkArea(right, panels); got != (mygo.Rectangle{Width: 1920, Height: 1034}) {
		t.Fatalf("another display's panels changed the work area: %+v", got)
	}
	if got := desktopLyricsPanelWorkArea(right, nil); got != right {
		t.Fatalf("changed output without panels: %+v", got)
	}
}

func TestDesktopLyricsPlasmaPanelsLive(t *testing.T) {
	if os.Getenv("LILT_PLASMA_CHECK") != "1" {
		t.Skip("opt-in read-only Plasma session check")
	}
	panels, err := queryDesktopLyricsPanels()
	if err != nil {
		t.Fatal(err)
	}
	if len(panels) == 0 {
		t.Fatal("no Plasma panels reported")
	}
	for _, p := range panels {
		if p.Height <= 0 || p.Screen.Width <= 0 || p.Screen.Height <= 0 {
			t.Fatalf("invalid panel: %+v", p)
		}
		t.Logf("panel: %+v, work area: %+v", p, desktopLyricsPanelWorkArea(p.Screen, panels))
	}
}
