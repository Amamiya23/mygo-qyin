package app

import (
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

// Clicking the current entry pauses and resumes without reloading its audio or
// replacing the queue: song and episode rows used to start it over from zero.
func TestCurrentEntryTogglesPlayback(t *testing.T) {
	for _, page := range []string{"playlist", "daily picks", "queue", "podcast"} {
		t.Run(page, func(t *testing.T) {
			a := newTestApp()
			a.settings = defaultSettings()
			tr := qqmusic.Track{ID: 7, Name: "宝船", Duration: 200000}
			if page == "podcast" {
				tr.ProgramID, tr.PodcastID = 70, 5
			}
			// The same song can be clicked in a list other than its queue.
			listed := []qqmusic.Track{tr, {ID: 8, Name: "列表中的另一首", Duration: 200000}}
			queued := []qqmusic.Track{tr, {ID: 9, Name: "原队列的下一首", Duration: 200000}}
			a.pl.Cue(queued)
			a.pl.SetLyrics(nil)
			a.pl.ToggleShuffle()
			order, at := a.pl.PlayOrder()
			order = slices.Clone(order)
			out := audio.NewPlayer(&silence{})
			out.SetVolume(0)
			const position = 73 * time.Second
			out.Seek(position)
			out.Play()
			a.pl.Attach(out)
			defer out.Close()

			pod := &qqmusic.Podcast{ID: 5, Name: "测试播客"}
			pr := qqmusic.Program{ID: 70, Name: tr.Name, Duration: tr.Duration}
			pr.MainSong.ID = tr.ID
			d := &podcastData{podcast: pod, programs: []qqmusic.Program{pr}}
			view := func(c *ui.Context) {
				a.pal = paletteFor(c.Theme())
				// Keep other copies of the title in the sidebar and player bar
				// out of the lookup, so the click targets the entry itself.
				ui.Column(c).Fill().Children(func() {
					switch page {
					case "playlist":
						a.trackRows(c, listed, true, func(c *ui.Context) {})
					case "daily picks":
						a.dailyPicks(c, listed)
					case "queue":
						a.queuePane(c)
					case "podcast":
						a.programRow(c, d, 0, a.pl.Current())
					}
				})
			}
			tt := ui.NewTester(view, 1200, 800)
			tt.Advance(time.Second)
			tt.Frame()
			for _, wantPlaying := range []bool{false, true} {
				if err := tt.Click(tr.Name); err != nil {
					t.Fatal(err)
				}
				tt.Frame()
				if got := a.pl.Playing(); got != wantPlaying {
					t.Fatalf("playing = %v, want %v", got, wantPlaying)
				}
				if !a.pl.Ready() || a.pl.Position() < position {
					t.Fatalf("audio reloaded or progress lost: ready %v, position %v", a.pl.Ready(), a.pl.Position())
				}
				if !reflect.DeepEqual(a.pl.Queue(), queued) {
					t.Fatal("click replaced the original queue")
				}
				gotOrder, gotAt := a.pl.PlayOrder()
				if !slices.Equal(gotOrder, order) || gotAt != at {
					t.Fatalf("play order changed: %v at %d, want %v at %d", gotOrder, gotAt, order, at)
				}
				if dir := os.Getenv("NM_FRAMES"); dir != "" && page == "playlist" {
					name := "track-row-paused.png"
					if wantPlaying {
						name = "track-row-playing-hover.png"
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
		})
	}
}
