package player

import (
	"slices"
	"testing"
	"time"

	"lilt/internal/qqmusic"
)

// Playing a song next inserts one occurrence after the current song without reordering the rest.
func TestPlayNextOrder(t *testing.T) {
	for _, tc := range []struct {
		name    string
		order   []int
		at      int
		shuffle bool
		track   qqmusic.Track
		want    []int64
	}{
		{"middle", []int{0, 1, 2, 3}, 1, false, qqmusic.Track{ID: 5}, []int64{1, 2, 5, 3, 4}},
		{"last", []int{0, 1, 2, 3}, 3, false, qqmusic.Track{ID: 5}, []int64{1, 2, 3, 4, 5}},
		{"shuffled", []int{2, 0, 3, 1}, 1, true, qqmusic.Track{ID: 5}, []int64{3, 1, 5, 4, 2}},
		{"already queued", []int{0, 1, 2, 3}, 1, false, qqmusic.Track{ID: 4}, []int64{1, 2, 4, 3, 4}},
		{"current song", []int{0, 1, 2, 3}, 1, false, qqmusic.Track{ID: 2}, []int64{1, 2, 2, 3, 4}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := New(func(func()) {})
			p.Cue([]qqmusic.Track{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}})
			p.order, p.at, p.shuffle = slices.Clone(tc.order), tc.at, tc.shuffle
			p.track = &p.queue[p.order[p.at]]
			current := p.Current()
			order, _ := p.PlayOrder()
			p.PlayNext(tc.track)
			if p.Current() != current || p.at != tc.at || p.Shuffle() != tc.shuffle {
				t.Fatalf("insertion changed current song, position or shuffle: current %v, at %d, shuffle %v", p.Current(), p.at, p.Shuffle())
			}
			if !slices.Equal(order, tc.order) {
				t.Fatalf("insertion changed a previously exposed play order: %v", order)
			}
			var ids []int64
			seen := make(map[int]bool)
			for _, i := range p.order {
				if i < 0 || i >= len(p.queue) || seen[i] {
					t.Fatalf("play order is not a permutation of the queue: %v", p.order)
				}
				seen[i] = true
				ids = append(ids, p.queue[i].ID)
			}
			if len(p.order) != len(p.queue) || !slices.Equal(ids, tc.want) {
				t.Fatalf("play order %v, want %v; queue %v", ids, tc.want, p.queue)
			}
		})
	}
}

// Inserting a next song preserves playback and repeat mode and notifies session persistence while paused too.
func TestPlayNextKeepsPlayback(t *testing.T) {
	for _, playing := range []bool{false, true} {
		for _, repeat := range []RepeatMode{RepeatOff, RepeatAll, RepeatOne} {
			p := New(func(func()) {})
			p.Cue([]qqmusic.Track{{ID: 1}, {ID: 2}})
			p.Hold(47 * time.Second)
			p.playing, p.loading, p.repeat, p.gen = playing, true, repeat, 9
			p.info = &qqmusic.Stream{Level: "exhigh"}
			current, stream := p.Current(), p.Stream()
			changed := 0
			p.Changed = func() { changed++ }
			p.PlayNext(qqmusic.Track{ID: 3})
			if p.Current() != current || p.Playing() != playing || !p.Loading() || p.Position() != 47*time.Second || p.gen != 9 || p.Stream() != stream || p.Repeat() != repeat {
				t.Fatalf("insertion changed playback: current %v, playing %v, loading %v, position %v, generation %d, repeat %v", p.Current(), p.Playing(), p.Loading(), p.Position(), p.gen, p.Repeat())
			}
			if changed != 1 {
				t.Fatalf("Changed called %d times, want once", changed)
			}
			s, ok := p.Session()
			if !ok || s.At != 0 || s.Position != 47000 || len(s.Order) != 3 || len(s.Queue) != 3 || s.Queue[s.Order[1]].ID != 3 {
				t.Fatalf("session lost the inserted song or playback position: %+v, available %v", s, ok)
			}
		}
	}
}

// With no current song, playing next queues the song without starting playback.
func TestPlayNextEmptyQueue(t *testing.T) {
	p := New(func(func()) {})
	p.PlayNext(qqmusic.Track{ID: 5})
	order, at := p.PlayOrder()
	if len(p.Queue()) != 1 || p.Queue()[0].ID != 5 || !slices.Equal(order, []int{0}) || at != -1 || p.Current() != nil || p.Playing() || p.Loading() || p.gen != 0 {
		t.Fatalf("empty queue unexpectedly started playback: queue %v, order %v at %d, current %v, playing %v, loading %v", p.Queue(), order, at, p.Current(), p.Playing(), p.Loading())
	}
}

// A manually queued next song stays ahead of radio suggestions without leaving the radio.
func TestPlayNextKeepsRadio(t *testing.T) {
	p := New(func(func()) {})
	p.Cue([]qqmusic.Track{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}})
	asked := 0
	p.AttachRadio("fm:DEFAULT", func(got func([]qqmusic.Track)) { asked++ })
	gen := p.radioGen
	p.PlayNext(qqmusic.Track{ID: 5})
	if p.Radio() != "fm:DEFAULT" || p.feed == nil || p.radioGen != gen || asked != 0 {
		t.Fatalf("insertion changed radio: radio %q, generation %d, feed calls %d", p.Radio(), p.radioGen, asked)
	}
	var ids []int64
	for _, i := range p.order {
		ids = append(ids, p.queue[i].ID)
	}
	if !slices.Equal(ids, []int64{1, 5, 2, 3, 4}) {
		t.Fatalf("radio songs displaced the selected next song: %v", ids)
	}
}

// A song ending naturally advances to the inserted song, including at the old end of the queue.
func TestPlayNextAfterSongEnds(t *testing.T) {
	for _, repeat := range []RepeatMode{RepeatOff, RepeatAll} {
		for _, at := range []int{0, 1} {
			p := New(func(func()) {})
			p.Cue([]qqmusic.Track{{ID: 1}, {ID: 2}})
			p.at, p.track, p.repeat = at, &p.queue[at], repeat
			p.PlayNext(qqmusic.Track{ID: 3})
			p.ended()
			if p.Current() == nil || p.Current().ID != 3 || !p.Playing() {
				t.Fatalf("song end did not play the requested song: repeat %v, original position %d, current %v, playing %v", repeat, at, p.Current(), p.Playing())
			}
		}
	}
}

// Choosing another song to play next puts the latest choice first and keeps earlier choices queued.
func TestPlayNextTwice(t *testing.T) {
	p := New(func(func()) {})
	p.Cue([]qqmusic.Track{{ID: 1}, {ID: 2}})
	p.PlayNext(qqmusic.Track{ID: 3})
	p.PlayNext(qqmusic.Track{ID: 4})
	var ids []int64
	for _, i := range p.order {
		ids = append(ids, p.queue[i].ID)
	}
	if !slices.Equal(ids, []int64{1, 4, 3, 2}) {
		t.Fatalf("latest choice did not play next: %v", ids)
	}
}
