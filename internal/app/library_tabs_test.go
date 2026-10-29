package app

import (
	"image/color"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// Switching liked albums or purchases moves the selected capsule rather than replacing it at the new position.
func TestLibraryTabsAnimateSelection(t *testing.T) {
	tests := []struct {
		name    string
		app     func() *app
		prepare func(*app)
		open    route
		next    string
	}{
		{"liked", likedAlbumsApp, nil, route{page: pagePlaylist, id: 7}, "专辑"},
		{"liked loading", likedAlbumsApp, func(a *app) {
			delete(a.playlists, 7)
			a.loadingPl[7] = true
		}, route{page: pagePlaylist, id: 7, tab: 1}, "单曲"},
		{"purchased", purchasedApp, nil, route{page: pagePurchased, tab: purchasedAlbums}, "单曲"},
		{"purchased loading", purchasedApp, func(a *app) {
			a.purchased[purchasedSongs] = &purchasedData{page: paged{loading: true}}
		}, route{page: pagePurchased, tab: purchasedAlbums}, "单曲"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := tt.app()
			if tt.prepare != nil {
				tt.prepare(a)
			}
			a.open(tt.open)
			tester := ui.NewTester(a.view, 1200, 800)
			tester.Advance(time.Second)
			tester.Frame()
			tester.Advance(time.Second)
			first, _ := tester.Find("单曲")
			second, _ := tester.Find("专辑")
			pixel := func(r ui.Rect) color.RGBA {
				return tester.Image().RGBAAt(int(r.X-8), int(r.Y+r.H/2))
			}
			firstBefore, secondBefore := pixel(first), pixel(second)
			if err := tester.Click(tt.next); err != nil {
				t.Fatal(err)
			}
			if pixel(first) != firstBefore || pixel(second) != secondBefore {
				t.Fatal("the selected capsule jumped to the new segment before the animation advanced")
			}
			tester.Advance(400 * time.Millisecond)
			tester.Frame()
			if pixel(first) == firstBefore || pixel(second) == secondBefore {
				t.Fatal("the selected capsule did not move to the new segment")
			}
		})
	}
}
