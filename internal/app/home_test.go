package app

import (
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

func TestHomePublishesSectionsIndependently(t *testing.T) {
	a := newTestApp()
	a.homeGen, a.homeLoading = 1, true
	h := &homeLoad{gen: 1, at: time.Now(), pending: 4, cataloguePending: 2}
	h.answered(a, func() { a.featured = []qqmusic.Playlist{{ID: 1}} }, nil, true)
	if len(a.featured) != 1 || !a.homeLoading {
		t.Fatal("first section waited for the remaining requests")
	}
	h.answered(a, func() { t.Fatal("failed section was published") }, errors.New("offline"), true)
	if a.homeAt != h.at || a.homeErr != "" {
		t.Fatal("one failed catalogue discarded a successful section")
	}
	h.answered(a, func() { a.daily = []qqmusic.Track{{ID: 2}} }, nil, false)
	h.answered(a, func() { a.recPlaylists = []qqmusic.Playlist{{ID: 3}} }, nil, false)
	if a.homeLoading || len(a.daily) != 1 || len(a.recPlaylists) != 1 {
		t.Fatal("home did not finish after all sections answered")
	}
	a.homeGen++
	h.answered(a, func() { t.Fatal("stale home result was applied") }, nil, false)
}

func TestHomeFailuresPreserveLoadedSections(t *testing.T) {
	a := newTestApp()
	a.daily = []qqmusic.Track{{ID: 7}}
	a.homeLoading = true
	h := &homeLoad{pending: 3, cataloguePending: 2}
	fail := func() { t.Fatal("failed section was applied") }
	h.answered(a, fail, errors.New("offline"), true)
	h.answered(a, fail, errors.New("offline"), true)
	h.answered(a, fail, errors.New("offline"), false)
	if a.homeLoading || a.homeErr == "" || len(a.daily) != 1 || !a.homeAt.IsZero() {
		t.Fatal("failed refresh lost loaded data or was marked fresh")
	}
}

func TestHomeRequestsRunConcurrently(t *testing.T) {
	featured, charts, daily, recommended := homeFeatured, homeCharts, homeDaily, homeRecommended
	defer func() {
		homeFeatured, homeCharts, homeDaily, homeRecommended = featured, charts, daily, recommended
	}()
	started, finished := make(chan struct{}, 4), make(chan struct{}, 4)
	release := make(chan struct{})
	wait := func() {
		started <- struct{}{}
		<-release
		finished <- struct{}{}
	}
	homeFeatured = func(int) ([]qqmusic.Playlist, error) { wait(); return nil, nil }
	homeCharts = func() ([]qqmusic.Playlist, error) { wait(); return nil, nil }
	homeDaily = func() ([]qqmusic.Track, error) { wait(); return nil, nil }
	homeRecommended = func() ([]qqmusic.Playlist, error) { wait(); return nil, nil }
	defer func() {
		close(release)
		for range 4 {
			select {
			case <-finished:
			case <-time.After(2 * time.Second):
				t.Error("home request did not finish")
			}
		}
	}()
	a := newTestApp()
	a.profile = &qqmusic.Profile{UserID: 1}
	a.songShelves = &songShelfData{loading: true}
	a.loadHome()
	for range 4 {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("a home request waited for another request to finish")
		}
	}
}

// The daily songs count as new from 6:00 in Beijing, wherever the listener
// is, so a home page loaded before then loads again.
func TestDailyRenewed(t *testing.T) {
	at := func(day, hour, min int) time.Time { return time.Date(2026, 10, day, hour, min, 0, 0, beijing) }
	for _, c := range []struct {
		loaded, now time.Time
		want        bool
	}{
		{at(26, 23, 0), at(27, 5, 59), false},
		{at(26, 23, 0), at(27, 6, 0), true},
		{at(27, 5, 59), at(27, 6, 1), true},
		{at(27, 6, 30), at(27, 23, 59), false},
		{at(27, 6, 30), at(28, 5, 0), false},
		{at(27, 6, 30), at(29, 1, 0), true},
	} {
		if got := dailyRenewed(c.loaded, c.now); got != c.want {
			t.Errorf("loaded %v, now %v: renewed %v", c.loaded, c.now, got)
		}
	}

	// Midnight where the listener is isn't NetEase's new day: 0:30 in
	// Los Angeles is 15:30 in Beijing, long after its 6:00.
	la, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Skip(err)
	}
	loaded := time.Date(2026, 10, 26, 23, 0, 0, 0, la)
	if dailyRenewed(loaded, time.Date(2026, 10, 27, 0, 30, 0, 0, la)) {
		t.Error("renewed at midnight in Los Angeles")
	}
	if !dailyRenewed(loaded, time.Date(2026, 10, 27, 15, 0, 0, 0, la)) {
		t.Error("not renewed past 6:00 in Beijing")
	}
}

// The record follows song changes using the player bar's cached cover, with a daily fallback.
func TestDailyRecordCurrentCover(t *testing.T) {
	a := newTestApp()
	tracks := []qqmusic.Track{
		{ID: 1, Album: qqmusic.Album{PicURL: "https://y.gtimg.cn/music/photo_new/T002R500x500M000first.jpg"}},
		{ID: 2, Album: qqmusic.Album{PicURL: "https://y.gtimg.cn/music/photo_new/T002R500x500M000second.jpg"}},
		{ID: 3},
	}
	now := time.Unix(1000, 0)
	a.art.now = func() time.Time { return now }
	for _, tr := range tracks[:2] {
		a.art.add(qqmusic.Sized(tr.Album.PicURL, 120), &artwork{})
	}
	// Keep the sleeve and the old label size local too, so a regression cannot fetch.
	for _, px := range []int{240, 400} {
		a.art.add(qqmusic.Sized(tracks[0].Album.PicURL, px), &artwork{})
	}
	tt := ui.NewTester(func(c *ui.Context) {
		a.dailyStack(c, tracks[:1], false, 1)
	}, stackWidth, sleeveSize)
	for _, tc := range []struct {
		name string
		at   int
		want int
	}{
		{"no current song", -1, 0},
		{"first song", 0, 0},
		{"changed song while paused", 1, 1},
		{"missing current cover", 2, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.at >= 0 {
				a.pl.Cue(tracks[tc.at : tc.at+1])
			}
			now = now.Add(time.Second)
			tt.Frame()
			for i, tr := range tracks[:2] {
				art := a.art.art[qqmusic.Sized(tr.Album.PicURL, 120)]
				if got := art.used.Equal(now); got != (i == tc.want) {
					t.Errorf("song %d cover used=%t, want %t", i, got, i == tc.want)
				}
			}
			if len(a.art.busy) != 0 {
				t.Fatal("record requested an uncached image despite the player cover being cached")
			}
		})
	}
}
