package player

import (
	"context"
	"errors"
	"math"
	"os"
	"testing"
	"testing/synctest"
	"time"

	"github.com/egoist/mygo/audio"

	"lilt/internal/qqmusic"
)

func TestCachedPlaybackDoesNotWaitForSlowLoudness(t *testing.T) {
	data, err := os.ReadFile("testdata/seek.mp3")
	if err != nil {
		t.Fatal(err)
	}
	c := NewCache(t.TempDir(), 1<<30)
	st := &qqmusic.Stream{Size: int64(len(data)), Level: "exhigh", Type: "mp3", Gain: -2, Peak: 0.9}
	f := c.part(st)
	if f == nil {
		t.Fatal("no cache file")
	}
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if _, ok := c.keep(1, st, f.Name(), st.Size); !ok {
		t.Fatal("not cached")
	}
	old := songLoudness
	defer func() { songLoudness = old }()
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		defer close(release)
		songLoudness = func(qqmusic.Track) (qqmusic.Loudness, error) {
			<-release
			return qqmusic.Loudness{Gain: -6, Peak: 1}, nil
		}
		start := time.Now()
		info, s, late, err := openTrack(context.Background(), c, qqmusic.Track{ID: 1, Duration: 5000}, "exhigh", true, songURL)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		if late == nil || time.Since(start) > loudnessWait+time.Millisecond {
			t.Fatal("cached audio waited for the metadata request")
		}
		if info.Gain != -2 || info.Peak != 0.9 {
			t.Fatal("pending measurements replaced the cached gain")
		}
		if n, err := s.Read(make([]float32, 4096)); n == 0 || err != nil {
			t.Fatalf("cached audio did not decode: %d, %v", n, err)
		}
	})
}

func TestLateLoudnessRespectsPlaybackChanges(t *testing.T) {
	for _, state := range []string{"current", "disabled", "next song"} {
		t.Run(state, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				mode := NormalizeStandard
				original := &qqmusic.Stream{Gain: -2, Peak: 0.9}
				p := &Player{volume: 0.5, info: original, Normalize: func() string { return mode }}
				p.applyGain()
				switch state {
				case "disabled":
					mode = NormalizeOff
					p.applyGain()
				case "next song":
					p.gen++
				}
				before := p.gain()
				p.applyLateLoudness(0, qqmusic.Loudness{Gain: -6.6, Peak: 1})
				p.fade.mu.Lock()
				initial := p.fade.currentGain(time.Now())
				p.fade.mu.Unlock()
				if math.Abs(initial-before) > 1e-9 {
					t.Fatal("late gain changed abruptly")
				}
				time.Sleep(250 * time.Millisecond)
				synctest.Wait()
				want := before
				if state == "current" {
					want = 0.25 * math.Pow(10, -2.6/20)
				}
				p.fade.mu.Lock()
				got := p.fade.currentGain(time.Now())
				p.fade.mu.Unlock()
				if math.Abs(got-want) > 1e-9 || original.Gain != -2 {
					t.Fatalf("late metadata in %s: gain %v, want %v", state, got, want)
				}
			})
		})
	}
}

func TestGainRampPreservesPauseFade(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var f fader
		f.setGain(1)
		f.attach(nil, 1)
		paused := make(chan struct{})
		f.fade(0, 100*time.Millisecond, func() { close(paused) })
		f.rampGain(0.5, 200*time.Millisecond)
		time.Sleep(120 * time.Millisecond)
		synctest.Wait()
		select {
		case <-paused:
		default:
			t.Fatal("gain ramp canceled the pause completion")
		}
		f.setGain(0.8)
		time.Sleep(200 * time.Millisecond)
		synctest.Wait()
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.currentGain(time.Now()) != 0.8 || f.level(time.Now()) != 0 {
			t.Fatal("old gain ramp overrode a volume change or resumed playback")
		}
	})
}

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
			info, s, late, err := openTrack(context.Background(), c, qqmusic.Track{ID: 1, MID: "test-mid", Duration: 5000}, "exhigh", tc.normalize, songURL)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if late != nil {
				t.Fatal("immediate metadata was left pending")
			}
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
			mode := NormalizeStandard
			st := &qqmusic.Stream{}
			p := &Player{post: func(fn func()) { ch <- fn }, volume: 0.5, info: st, track: &qqmusic.Track{MID: "playing"}, Normalize: func() string { return mode }}
			out := audio.NewPlayer(silence{})
			defer out.Close()
			p.fade.attach(out, 1)
			p.RefreshGain()
			want := 0.25 * math.Pow(10, -2.6/20)
			switch state {
			case "disabled":
				mode = NormalizeOff
				p.RefreshGain()
				want = 0.25
			case "next song":
				p.gen++
				p.info = &qqmusic.Stream{Gain: -2, Peak: 0.9}
				p.applyGain()
				want = 0.25 / 0.9
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
