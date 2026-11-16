package player

import (
	"slices"
	"testing"
	"time"

	"lilt/internal/qqmusic"
)

func TestSessionSnapshotIsIndependent(t *testing.T) {
	p := &Player{at: -1}
	p.Cue([]qqmusic.Track{{
		ID: 1, Name: "original",
		Artists: []qqmusic.Artist{{Name: "artist"}},
		Alias:   []string{"alias"}, Privilege: &qqmusic.Privilege{Pl: 1},
		Volume: &qqmusic.Loudness{Gain: -6},
	}})
	s, ok := p.Session()
	if !ok {
		t.Fatal("no session")
	}
	p.queue[0].Name = "changed"
	p.queue[0].Artists[0].Name = "changed"
	p.queue[0].Alias[0] = "changed"
	p.queue[0].Privilege.Pl = 0
	p.queue[0].Volume.Gain = 0
	p.order[0] = 9
	tr := s.Queue[0]
	if tr.Name != "original" || tr.Artists[0].Name != "artist" || tr.Alias[0] != "alias" ||
		tr.Privilege.Pl != 1 || tr.Volume.Gain != -6 || s.Order[0] != 0 {
		t.Fatalf("queue mutation changed the session snapshot: %+v", s)
	}
}

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

func TestRemoveQueueTracks(t *testing.T) {
	t.Run("before and after current preserve playback and snapshot immutability", func(t *testing.T) {
		p := New(func(func()) {})
		p.Cue([]qqmusic.Track{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}})
		// Shuffled order: 3 -> 1 -> 4 -> 2; current is index 0 (ID 1) at pos 1.
		p.order, p.at, p.shuffle = []int{2, 0, 3, 1}, 1, true
		p.track = &p.queue[0]
		p.Hold(25 * time.Second)
		p.playing = true
		changed := 0
		p.Changed = func() { changed++ }

		oldQueue := p.Queue()
		oldOrder, _ := p.PlayOrder()
		oldQueueCopy := slices.Clone(oldQueue)
		oldOrderCopy := slices.Clone(oldOrder)
		cur := p.Current()

		// Remove queue index 2 (ID 3), which sits before current (pos 0 < 1).
		p.Remove(2)
		if !slices.EqualFunc(oldQueue, oldQueueCopy, func(a, b qqmusic.Track) bool { return a.ID == b.ID }) || !slices.Equal(oldOrder, oldOrderCopy) {
			t.Fatal("Remove mutated previously returned Queue or PlayOrder slices")
		}
		if p.Current() != cur || p.Current().ID != 1 || p.at != 0 || !p.Playing() || p.Position() != 25*time.Second {
			t.Fatalf("removing earlier track disrupted current track: cur=%v at=%d playing=%v pos=%v", p.Current(), p.at, p.Playing(), p.Position())
		}
		// Remaining queue: [ID 1 (0), ID 2 (1), ID 4 (2)]; order should be [0, 2, 1].
		order, at := p.PlayOrder()
		if at != 0 || !slices.Equal(order, []int{0, 2, 1}) {
			t.Fatalf("order=%v at=%d, want [0 2 1] at 0", order, at)
		}

		// Remove queue index 1 (ID 2), which sits after current (pos 2 > 0).
		p.Remove(1)
		if p.Current() != cur || p.at != 0 || len(p.Queue()) != 2 {
			t.Fatalf("removing later track disrupted current track: cur=%v at=%d len=%d", p.Current(), p.at, len(p.Queue()))
		}
		order, at = p.PlayOrder()
		if at != 0 || !slices.Equal(order, []int{0, 1}) || p.Queue()[1].ID != 4 {
			t.Fatalf("unexpected remaining queue=%v order=%v at=%d", p.Queue(), order, at)
		}
		if changed != 2 {
			t.Fatalf("Changed called %d times, want 2", changed)
		}
	})

	t.Run("removing current track advances to next and wraps at end", func(t *testing.T) {
		p := New(func(func()) {})
		p.Cue([]qqmusic.Track{{ID: 10}, {ID: 20}, {ID: 30}})
		p.at, p.track = 1, &p.queue[1]
		p.playing = false

		// Remove middle current track (ID 20): should load ID 30 at index 1 without auto-playing.
		p.Remove(1)
		if p.Current() == nil || p.Current().ID != 30 || p.at != 1 || p.Playing() {
			t.Fatalf("removing current middle track: cur=%v at=%d playing=%v", p.Current(), p.at, p.Playing())
		}

		// Remove last current track (ID 30): should wrap to ID 10 at index 0.
		p.playing = true
		p.Remove(1)
		if p.Current() == nil || p.Current().ID != 10 || p.at != 0 || !p.Playing() {
			t.Fatalf("removing last current track: cur=%v at=%d playing=%v", p.Current(), p.at, p.Playing())
		}

		// Remove only remaining track: clears player state and ends radio.
		p.AttachRadio("fm:DEFAULT", func(func([]qqmusic.Track)) {})
		p.Remove(0)
		order, at := p.PlayOrder()
		if p.Current() != nil || len(p.Queue()) != 0 || len(order) != 0 || at != -1 || p.Playing() || p.Loading() || p.Radio() != "" {
			t.Fatalf("removing final track left stale state: cur=%v len=%d at=%d playing=%v loading=%v radio=%q", p.Current(), len(p.Queue()), at, p.Playing(), p.Loading(), p.Radio())
		}
		if _, ok := p.Session(); ok {
			t.Fatal("empty player still reported a session")
		}
	})

	t.Run("out of bounds is a no-op", func(t *testing.T) {
		p := New(func(func()) {})
		p.Cue([]qqmusic.Track{{ID: 1}, {ID: 2}})
		p.Remove(-1)
		p.Remove(2)
		if len(p.Queue()) != 2 || p.Current().ID != 1 {
			t.Fatalf("out-of-bounds Remove mutated queue: %v", p.Queue())
		}
	})
}

