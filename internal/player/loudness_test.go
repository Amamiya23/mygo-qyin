package player

import (
	"errors"
	"math"
	"os"
	"testing"

	"github.com/egoist/mygo/audio"

	"lilt/internal/qqmusic"
)

// Cached songs receive the same loudness metadata as streams and still open when metadata is unavailable.
func TestCachedSongLoudness(t *testing.T) {
	b, err := os.ReadFile("testdata/seek.mp3")
	if err != nil {
		t.Fatal(err)
	}
	original := songLoudness
	defer func() { songLoudness = original }()
	for _, tc := range []struct {
		name               string
		normalize          bool
		v                  qqmusic.Loudness
		err                error
		wantGain, wantPeak float64
	}{
		{"measured", true, qqmusic.Loudness{Gain: -7.603, Peak: 0.946}, nil, -7.603, 0.946},
		{"unmeasured", true, qqmusic.Loudness{}, nil, 0, 0},
		{"offline", true, qqmusic.Loudness{}, errors.New("offline"), -2, 0.9},
		{"disabled", false, qqmusic.Loudness{Gain: -7.603, Peak: 0.946}, nil, -2, 0.9},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := NewCache(t.TempDir(), 1<<30)
			st := &qqmusic.Stream{Size: int64(len(b)), Level: "exhigh", Type: "mp3", Gain: -2, Peak: 0.9}
			f := c.part(st)
			if f == nil {
				t.Fatal("no cache file")
			}
			if _, err := f.Write(b); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			if _, ok := c.keep(1, st, f.Name(), int64(len(b))); !ok {
				t.Fatal("not cached")
			}
			songLoudness = func(track qqmusic.Track) (qqmusic.Loudness, error) {
				if track.ID != 1 || track.MID != "test-mid" {
					return qqmusic.Loudness{}, errors.New("wrong track")
				}
				return tc.v, tc.err
			}
			p := &Player{Cache: c}
			info, s, err := p.open(qqmusic.Track{ID: 1, MID: "test-mid", Duration: 5000}, "exhigh", tc.normalize)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if info.Gain != tc.wantGain || info.Peak != tc.wantPeak {
				t.Fatalf("gain/peak = %v/%v, want %v/%v", info.Gain, info.Peak, tc.wantGain, tc.wantPeak)
			}
			if n, err := s.Read(make([]float32, 4096)); n == 0 || err != nil {
				t.Fatalf("cached audio did not decode: %d, %v", n, err)
			}
		})
	}
}

// Enabling during playback fetches missing measurements; late replies respect a disabled switch or a changed song.
func TestEnableNormalizationDuringPlayback(t *testing.T) {
	original := songLoudness
	defer func() { songLoudness = original }()
	songLoudness = func(qqmusic.Track) (qqmusic.Loudness, error) {
		return qqmusic.Loudness{Gain: -6.6, Peak: 1}, nil
	}
	for _, state := range []string{"current", "disabled", "next song"} {
		t.Run(state, func(t *testing.T) {
			ch := make(chan func(), 4)
			enabled := true
			st := &qqmusic.Stream{}
			p := &Player{post: func(fn func()) { ch <- fn }, volume: 0.5, info: st, track: &qqmusic.Track{MID: "playing"}, Normalize: func() bool { return enabled }}
			out := audio.NewPlayer(silence{})
			defer out.Close()
			p.fade.attach(out, 1)
			p.RefreshGain()
			want := 0.25 * math.Pow(10, -6.6/20)
			switch state {
			case "disabled":
				enabled = false
				p.RefreshGain()
				want = 0.25
			case "next song":
				p.gen++
				p.info = &qqmusic.Stream{Gain: -2, Peak: 0.9}
				p.applyGain()
				want = 0.25 * math.Pow(10, -2.0/20)
			}
			next(t, ch)
			if got := out.Volume(); math.Abs(got-want) > 1e-9 {
				t.Fatalf("output gain %v, want %v", got, want)
			}
			if st.Gain != 0 || st.Peak != 0 {
				t.Fatal("download metadata was mutated")
			}
		})
	}
}
