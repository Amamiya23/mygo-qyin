package app

import (
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// Hovering or clicking the metadata info icon reveals long or multi-line notices without shifting tracks, short single-line notices render inline, and changes reset any open popover.
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
	card = descriptionBox(t, tt, "发行信息详情")
	tt.ClickAt(card.X+20, card.Y+24)
	if !open() {
		t.Fatal("click inside the panel dismissed it")
	}
	// Short single-line release info renders directly inline and dismisses any prior popover.
	album.Company = "独立发行"
	tt.Frame()
	if open() {
		t.Fatal("changing album release info left the previous popover open")
	}
	if _, ok := tt.Find("发行信息"); ok {
		t.Fatal("short single-line release notice showed a redundant info icon")
	}
	if !tt.HasText("独立发行") {
		t.Fatal("short single-line release notice did not render inline")
	}
	if tt.FramePending() {
		t.Fatal("inline release notice kept requesting idle frames")
	}
	// Short multi-line notices still use a compact popover on demand.
	album.Company = "独立发行\n版权所有"
	tt.Frame()
	if err := tt.Click("发行信息"); err != nil {
		t.Fatal(err)
	}
	shortCard := descriptionBox(t, tt, "发行信息详情")
	if shortCard.W >= 200 {
		t.Fatalf("short release notice stretched popover to %.1fpx", shortCard.W)
	}
	tt.Move(20, 500)
	tt.Advance(time.Second)
	if open() {
		t.Fatal("mouse clicks kept the panel open after the pointer left")
	}
	if err := tt.Click("发行信息"); err != nil {
		t.Fatal(err)
	}
	descriptionBox(t, tt, "发行信息详情")
	tt.ClickAt(650, 520)
	if open() {
		t.Fatal("outside click did not dismiss the panel")
	}
}

// Single-line release info displays inline when it fits in the metadata row and collapses to the info icon when space is tight.
func TestAlbumReleaseInfoAdaptive(t *testing.T) {
	switch runtime.GOOS {
	case "windows", "darwin", "linux":
	default:
		t.Skip("desktop album page")
	}
	const company = "Sony Music Entertainment (Japan) Inc."
	a := descriptionApp(pageAlbum, "")
	a.albums[8].album.PublishTime = time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	a.albums[8].album.Company = company

	wide := ui.NewTester(a.view, 1200, 650)
	wide.Frame()
	if !wide.HasText(company) {
		t.Fatal("fitting release notice was not displayed inline on a wide window")
	}
	if _, ok := wide.Find("发行信息"); ok {
		t.Fatal("fitting release notice showed a redundant info icon on a wide window")
	}
	wide.Advance(time.Second)
	if wide.FramePending() {
		t.Fatal("wide album page kept requesting idle frames")
	}

	narrow := ui.NewTester(a.view, 760, 650)
	narrow.Frame()
	if narrow.HasText(company) {
		t.Fatal("overflowing release notice stayed inline on a narrow window")
	}
	if err := narrow.Click("发行信息"); err != nil {
		t.Fatal(err)
	}
	if _, ok := narrow.Find(company); !ok {
		t.Fatal("clicking the narrow-window info icon did not reveal the release notice")
	}
	narrow.Advance(time.Second)
	if narrow.FramePending() {
		t.Fatal("narrow album page kept requesting idle frames")
	}
}

// Pressing Enter opens the release notice for keyboard reading, Tab moves into its scroll area, and leaving or pressing Escape closes it.
func TestReleaseInfoKeyboard(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("desktop release information")
	}
	a := newTestApp()
	tt := ui.NewTester(func(c *ui.Context) {
		a.pal = paletteFor(c.Theme())
		ui.Row(c).Padding(24).Children(func() {
			a.releaseInfoButton(c, "独立发行")
			ui.Button(c.Key("next"), "下一项")
		})
	}, 640, 480)
	tt.Frame()
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyEnter)
	tt.Advance(time.Second)
	if _, ok := tt.Find("独立发行"); !ok {
		t.Fatal("Enter did not keep the panel open for keyboard reading")
	}
	tt.Key(0, ui.KeyEscape)
	if _, ok := tt.Find("独立发行"); ok || !tt.Focused("发行信息") {
		t.Fatal("Escape did not dismiss the panel and keep focus on its button")
	}
	tt.Key(0, ui.KeyEnter)
	tt.Key(0, ui.KeyTab)
	tt.Advance(time.Second)
	if _, ok := tt.Find("独立发行"); !ok || !tt.Focused("发行信息内容") {
		t.Fatal("moving keyboard focus into the panel did not keep it open")
	}
	tt.Key(0, ui.KeyTab)
	tt.Advance(time.Second)
	if _, ok := tt.Find("独立发行"); ok || !tt.Focused("下一项") {
		t.Fatal("moving keyboard focus outside did not dismiss the panel")
	}
}

// With NM_FRAMES set, saves album release info frames for short inline, short popover, and long notices in both light and dark appearances.
func TestAlbumReleaseInfoFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("desktop album page")
	}
	const longNotice = "(P) 2020(5,7),2021(2-4,11),2022(1,6,8,10,12,14) Sony Music Labels Inc. / 2019(9,13) Sony Music Entertainment (Japan) Inc."
	for _, scene := range []struct {
		name, company string
		open          bool
	}{
		{"short-inline", "环球唱片", false},
		{"closed", longNotice, false},
		{"short-open", "独立发行\n版权所有", true},
		{"long-open", longNotice, true},
	} {
		for _, dark := range []bool{false, true} {
			a := descriptionApp(pageAlbum, "每一首歌，都记录着一段旅程。")
			a.albums[8].album.PublishTime = time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
			a.albums[8].album.Company = scene.company
			tt := ui.NewTester(a.view, 960, 620)
			tt.SetDark(dark)
			tt.Frame()
			if scene.open {
				if err := tt.Click("发行信息"); err != nil {
					t.Fatal(err)
				}
				tt.Advance(200 * time.Millisecond)
			}
			theme := "light"
			if dark {
				theme = "dark"
			}
			f, err := os.Create(filepath.Join(dir, "release-info-"+scene.name+"-"+theme+".png"))
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(f, tt.Image()); err != nil {
				f.Close()
				t.Fatal(err)
			}
			f.Close()
		}
	}
}
