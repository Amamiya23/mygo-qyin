package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestScrollEdgesFollowContent(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	for i := range 40 {
		a.featured = append(a.featured, Playlist{ID: int64(i + 1), Name: "歌单"})
	}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if a.underTop {
		t.Error("top edge shows before scrolling")
	}
	if !a.underBottom {
		t.Error("no bottom edge over a long page")
	}
	tt.Scroll(800, 400, 0, 100000)
	tt.Advance(time.Second)
	tt.Frame()
	tt.Frame()
	if !a.underTop {
		t.Error("no top edge after scrolling")
	}
	if a.underBottom {
		t.Error("bottom edge shows at the end of the page")
	}
}

func TestShortPageHasNoEdges(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.featured = []Playlist{{ID: 1, Name: "歌单"}}
	tt := ui.NewTester(a.view, 1200, 1400)
	tt.Frame()
	if a.underTop || a.underBottom {
		t.Errorf("edges on a page that fits: top %v bottom %v", a.underTop, a.underBottom)
	}
}
