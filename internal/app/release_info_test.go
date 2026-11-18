package app

import (
	"runtime"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestAlbumReleaseInfoHover(t *testing.T) {
	switch runtime.GOOS {
	case "windows", "darwin", "linux":
	default:
		t.Skip("desktop album page")
	}
	const notice = "(P) 2020(5,7),2021(2-4,11),2022(1,6,8,10,12,14) Sony Music Labels Inc. / 2019(9,13) Sony Music Entertainment (Japan) Inc."
	a := descriptionApp(pageAlbum, "")
	album := a.albums[8].album
	album.Songs = album.Songs[:1]
	tt := ui.NewTester(a.view, 900, 650)
	tt.Frame()
	if _, ok := tt.Find("发行信息"); ok {
		t.Fatal("button visible without release information")
	}
	album.Company = notice
	tt.Frame()
	button := descriptionBox(t, tt, "发行信息")
	song := descriptionBox(t, tt, "春日影")
	open := func() bool { _, shown := tt.Find("发行信息详情"); return shown }
	x, y := button.X+button.W/2, button.Y+button.H/2
	tt.Move(x, y)
	tt.Advance(300 * time.Millisecond)
	if open() {
		t.Fatal("brief hover opened the panel")
	}
	tt.Advance(250 * time.Millisecond)
	card := descriptionBox(t, tt, "发行信息详情")
	if _, ok := tt.Find(notice); !ok || card.X < 0 || card.Y < 0 || card.X+card.W > 900 || card.Y+card.H > 650 {
		t.Fatalf("notice is missing or extends outside the window: %+v", card)
	}
	if got := descriptionBox(t, tt, "春日影"); got != song {
		t.Fatalf("opening the panel moved the track from %+v to %+v", song, got)
	}
	tt.Move(x, button.Y+button.H+4)
	tt.Advance(60 * time.Millisecond)
	tt.Move(card.X+20, card.Y+20)
	tt.Advance(time.Second)
	if !open() {
		t.Fatal("panel closed while the pointer moved into it")
	}
	tt.Move(20, 500)
	tt.Advance(300 * time.Millisecond)
	if open() {
		t.Fatal("hover preview stayed open after pointer left")
	}
	tt.Move(x, y)
	tt.Advance(time.Second)
	tt.Key(0, ui.KeyEscape)
	tt.Advance(time.Second)
	if open() {
		t.Fatal("Escape did not keep the hover preview closed")
	}
	if err := tt.Click("发行信息"); err != nil {
		t.Fatal(err)
	}
	tt.Move(20, 500)
	tt.Advance(time.Second)
	if !open() {
		t.Fatal("clicked panel did not stay open")
	}
	tt.ClickAt(650, 520)
	if open() {
		t.Fatal("outside click did not dismiss the panel")
	}
}

func TestReleaseInfoKeyboard(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("desktop release information")
	}
	a := newTestApp()
	tt := ui.NewTester(func(c *ui.Context) {
		a.pal = paletteFor(c.Theme())
		ui.Row(c).Padding(24).Children(func() { a.releaseInfoButton(c, "独立发行") })
	}, 640, 480)
	tt.Frame()
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyEnter)
	if _, ok := tt.Find("独立发行"); !ok {
		t.Fatal("Enter did not open the panel")
	}
	tt.Key(0, ui.KeyEscape)
	if _, ok := tt.Find("独立发行"); ok || !tt.Focused("发行信息") {
		t.Fatal("Escape did not dismiss the panel and keep focus on its button")
	}
}
