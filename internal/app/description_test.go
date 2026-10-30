package app

import (
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

func descriptionApp(kind pageKind, description string) *app {
	a := catalogApp()
	a.albums[8].album.Description = description
	a.playlists[8] = &qqmusic.Playlist{ID: 8, Name: "夜间漫游", Description: description, Tracks: a.albums[8].album.Songs}
	a.open(route{page: kind, id: 8})
	return a
}

func longDescription() string {
	return "每一首歌，都记录着一段旅程。\n\n" + strings.Repeat("从城市的清晨到深夜的灯火，在旋律中慢慢寻找属于自己的片刻。", 24) + "\n\n最后，愿这些音乐能陪伴你走过每一个平凡而珍贵的日子。"
}

func descriptionBox(t *testing.T, tt *ui.Tester, label string) ui.Rect {
	t.Helper()
	r, ok := tt.Find(label)
	if !ok {
		t.Fatalf("missing %s", label)
	}
	return r
}

func descriptionCardShown(tt *ui.Tester) bool {
	_, shown := tt.Find("完整简介")
	return shown
}

// Hover reveals previously unreachable prose without moving songs, and the card stays readable while scrolling or crossing its gap.
func TestDescriptionHover(t *testing.T) {
	for _, kind := range []pageKind{pageAlbum, pagePlaylist} {
		t.Run(map[pageKind]string{pageAlbum: "album", pagePlaylist: "playlist"}[kind], func(t *testing.T) {
			description := longDescription()
			a := descriptionApp(kind, description)
			tt := ui.NewTester(a.view, 1200, 800)
			tt.Frame()
			tt.Advance(time.Second)
			anchor := descriptionBox(t, tt, "简介")
			song := descriptionBox(t, tt, "春日影")
			x, y := anchor.X+20, anchor.Y+anchor.H/2
			tt.Move(x, y)
			tt.Advance(300 * time.Millisecond)
			if descriptionCardShown(tt) {
				t.Fatal("card opened while the pointer was only passing over the preview")
			}
			tt.Advance(300 * time.Millisecond)
			card := descriptionBox(t, tt, "完整简介")
			if got := descriptionBox(t, tt, "春日影"); got != song {
				t.Fatalf("hover moved a song from %+v to %+v", song, got)
			}
			if card.X < 0 || card.Y < 0 || card.X+card.W > 1200 || card.Y+card.H > 800 {
				t.Fatalf("card escaped window: %+v", card)
			}
			scroll := descriptionBox(t, tt, "简介内容")
			// Cross the gap in two pointer events, as a real pointer does.
			tt.Move(x, anchor.Y+anchor.H+4)
			tt.Advance(60 * time.Millisecond)
			tt.Move(scroll.X+scroll.W/2, scroll.Y+30)
			tt.Advance(time.Second)
			if !descriptionCardShown(tt) {
				t.Fatal("card closed as the pointer entered it")
			}
			tt.Scroll(scroll.X+scroll.W/2, scroll.Y+30, 0, 180)
			tt.Advance(time.Second)
			if !descriptionCardShown(tt) {
				t.Fatal("scrolling dismissed the reading card")
			}
			if got := descriptionBox(t, tt, "春日影"); got != song {
				t.Fatalf("scrolling the prose moved the underlying songs: %+v", got)
			}
			tt.Move(20, 500)
			tt.Advance(300 * time.Millisecond)
			if descriptionCardShown(tt) {
				t.Fatal("card remained after leaving both preview and card")
			}
			tt.Move(x, y)
			tt.Advance(600 * time.Millisecond)
			if !descriptionCardShown(tt) {
				t.Fatal("card did not reopen on a new hover")
			}
			tt.Key(0, ui.KeyEscape)
			tt.Advance(time.Second)
			if descriptionCardShown(tt) {
				t.Fatal("Escape did not dismiss the card until the next hover")
			}
		})
	}
}

// A preview that already fits stays quiet instead of opening a redundant hover card.
func TestDescriptionFits(t *testing.T) {
	for _, description := range []string{"MyGO!!!!! 的第一张专辑。", "第一段\n第二段\n第三段"} {
		a := descriptionApp(pageAlbum, description)
		tt := ui.NewTester(a.view, 1200, 800)
		tt.Frame()
		r := descriptionBox(t, tt, "简介")
		tt.Move(r.X+10, r.Y+r.H/2)
		tt.Advance(time.Second)
		if descriptionCardShown(tt) {
			t.Fatalf("a fitting preview opened a card: %q", description)
		}
	}
}

// Keyboard focus reveals long prose and Escape leaves it dismissed while the anchor retains focus.
func TestDescriptionKeyboard(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(func(c *ui.Context) {
		a.pal = paletteFor(c.Theme())
		ui.Column(c).Width(400).Padding(24).Children(func() {
			a.collectionDescription(c, longDescription())
			ui.Button(c, "下一项")
		})
	}, 800, 600)
	tt.Frame()
	tt.Key(0, ui.KeyTab)
	if !descriptionCardShown(tt) {
		t.Fatal("keyboard focus did not reveal the description")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Advance(time.Second)
	if descriptionCardShown(tt) {
		t.Fatal("Escape reopened the card while focus stayed on the preview")
	}
	tt.Key(0, ui.KeyTab)
	tt.Key(ui.Shift, ui.KeyTab)
	if !descriptionCardShown(tt) {
		t.Fatal("returning keyboard focus did not reveal the description again")
	}
}

// With NM_FRAMES set, saves the full description card at regular and narrow sizes in both appearances.
func TestDescriptionFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	for _, width := range []int{1200, 900} {
		for _, dark := range []bool{false, true} {
			a := descriptionApp(pageAlbum, longDescription())
			tt := ui.NewTester(a.view, width, 800)
			tt.SetDark(dark)
			tt.Frame()
			r := descriptionBox(t, tt, "简介")
			tt.Move(r.X+20, r.Y+r.H/2)
			tt.Advance(time.Second)
			if !descriptionCardShown(tt) {
				t.Fatal("no card to capture")
			}
			name := "description"
			if width == 900 {
				name += "-narrow"
			}
			if dark {
				name += "-dark"
			}
			f, err := os.Create(filepath.Join(dir, name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(f, tt.Image())
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
			scroll := descriptionBox(t, tt, "简介内容")
			tt.Move(scroll.X+30, scroll.Y+30)
			tt.Scroll(scroll.X+30, scroll.Y+30, 0, 10000)
			tt.Advance(time.Second)
			f, err = os.Create(filepath.Join(dir, name+"-end.png"))
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
}
