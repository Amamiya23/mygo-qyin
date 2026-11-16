package app

import (
	"errors"
	"testing"
	"time"

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
