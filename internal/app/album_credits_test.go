package app

import (
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"lilt/internal/qqmusic"
)

const sampleAlbumCredits = "(P) 2020(5,7),2021(2-4,11),2022(1,6,8,10,12,14) Sony Music Labels Inc. / 2019(9,13) Sony Music Entertainment (Japan) Inc."

// Width changes must reflow the entire notice, including explicit newlines.
func TestAlbumCreditsReflow(t *testing.T) {
	a := newTestApp()
	notice := sampleAlbumCredits + "\n" + strings.Repeat("Long licensing statement without truncation. ", 6)
	var font ui.Font
	tt := ui.NewTester(func(c *ui.Context) {
		font = ui.Font{Family: c.Theme().Font, Size: 12.5, LineHeight: 1.5}
		ui.Column(c).Fill().Padding(28).Children(func() { a.albumCredits(c, notice) })
	}, 1100, 700)
	tt.Frame()
	wide, ok := tt.Find(notice)
	if !ok || wide.W > 760.5 {
		t.Fatalf("missing or oversized wide notice: %+v", wide)
	}
	tt.SetSize(450, 700)
	tt.Frame()
	narrow, ok := tt.Find(notice)
	_, height := ui.ShapeText(notice, font, narrow.W).Size()
	if !ok || narrow.X+narrow.W > 450 || narrow.H <= wide.H || narrow.H < height-1 {
		t.Fatalf("notice did not reflow fully: wide=%+v narrow=%+v expected height=%v", wide, narrow, height)
	}
}

func TestAlbumCreditsVisibility(t *testing.T) {
	a := catalogApp()
	al := a.albums[8].album
	al.Songs = al.Songs[:1]
	al.Company = sampleAlbumCredits
	a.open(route{page: pageAlbum, id: 8})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if _, shown := tt.Find(sampleAlbumCredits); shown != albumCreditsEnabled {
		t.Fatal("album credits do not follow the platform policy")
	}
	al.Company = " \n "
	tt.Frame()
	if _, shown := tt.Find(sampleAlbumCredits); shown {
		t.Fatal("empty company left the previous notice visible")
	}
	al.Company = sampleAlbumCredits
	tt.Frame()
	if _, shown := tt.Find(sampleAlbumCredits); shown != albumCreditsEnabled {
		t.Fatal("notice did not return after empty metadata")
	}
}

func TestAlbumCreditsScrollAbovePlayer(t *testing.T) {
	if !albumCreditsEnabled {
		t.Skip("desktop footer")
	}
	a := catalogApp()
	al := a.albums[8].album
	for i := range 30 {
		al.Songs = append(al.Songs, qqmusic.Track{ID: int64(100 + i), Name: "曲目"})
	}
	al.Company = sampleAlbumCredits
	a.open(route{page: pageAlbum, id: 8})
	tt := ui.NewTester(a.view, 900, 700)
	tt.Frame()
	tt.Scroll(700, 400, 0, 100000)
	tt.Advance(time.Second)
	tt.Frame()
	tt.Frame()
	r, ok := tt.Find(sampleAlbumCredits)
	if !ok || r.Y < 0 || r.Y+r.H > 700-playerBarHeight {
		t.Fatalf("notice hidden behind player: %+v (found %t)", r, ok)
	}
}

// Optional previews use a locally saved QQ album response and cover; no network in tests.
func TestAlbumCreditsFrames(t *testing.T) {
	dir, source := os.Getenv("NM_FRAMES"), os.Getenv("NM_ALBUM_SOURCE")
	if dir == "" || source == "" || !albumCreditsEnabled {
		t.Skip("set NM_FRAMES and NM_ALBUM_SOURCE for album previews")
	}
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Data struct {
			Name    string `json:"name"`
			Company string `json:"company"`
			Date    string `json:"aDate"`
			Singer  string `json:"singername"`
			Songs   []struct {
				Name     string `json:"songname"`
				Interval int64  `json:"interval"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{1440, 900} {
		a := catalogApp()
		al := a.albums[8].album
		al.Name, al.Description, al.Company = response.Data.Name, "", response.Data.Company
		date, _ := time.Parse("2006-01-02", response.Data.Date)
		al.PublishTime = date.UnixMilli()
		al.Artists = []qqmusic.Artist{{Name: response.Data.Singer}}
		al.PicURL = "preview-cover"
		al.Songs = nil
		for i, song := range response.Data.Songs {
			al.Songs = append(al.Songs, qqmusic.Track{ID: int64(i + 1), Name: song.Name, Duration: song.Interval * 1000, Artists: al.Artists, Album: al.Album})
		}
		if path := os.Getenv("NM_ALBUM_COVER"); path != "" {
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			art, err := decodeArt(b, artCover)
			if err != nil {
				t.Fatal(err)
			}
			a.art.add(qqmusic.Sized(al.PicURL, 600), art)
		}
		a.open(route{page: pageAlbum, id: 8})
		tt := ui.NewTester(a.view, width, 1300)
		tt.SetDark(true)
		tt.Advance(time.Second)
		tt.Frame()
		name := "album-credits-wide.png"
		if width == 900 {
			name = "album-credits-narrow.png"
		}
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, tt.Image())
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
}
