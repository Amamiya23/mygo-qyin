//go:build (darwin && !ios) || (((linux && !android) || windows) && (amd64 || arm64))

package audio

import (
	"fmt"
	"testing"
	"time"
)

// Model the application's Pause followed by delayed Close without relying on
// a timer: the old player stays alive until the new session has played music.
// Distinct channels identify stale old-track frames even during a gain ramp.
func TestManualRateSwitchDoesNotReplayPausedTail(t *testing.T) {
	for _, rates := range [][2]int{{48000, 44100}, {44100, 48000}, {88200, 96000}, {96000, 44100}} {
		for _, lat := range []Latency{LatencyInteractive, LatencyPlayback} {
			for _, preRoll := range []bool{false, true} {
				t.Run(fmt.Sprintf("%d-%d/%d/preroll=%t", rates[0], rates[1], lat, preRoll), func(t *testing.T) {
					resetOutputTest(t)
					latency = lat
					left, right := make([]float32, rates[0]*channels), make([]float32, rates[1]*channels)
					for i := 0; i < len(left); i += channels {
						left[i] = .5
					}
					for i := 1; i < len(right); i += channels {
						right[i] = .25
					}
					old := NewSound(rates[0], channels, left).NewPlayer().SetPrimary(true).SetPreRoll(preRoll)
					next := NewSound(rates[1], channels, right).NewPlayer().SetPrimary(true).SetPreRoll(preRoll)
					old.Play()
					runOnce.Do(func() {})
					testMode = false
					first, second := &reviewDrainingDevice{}, &reviewDrainingDevice{}
					opens := 0
					openOutput = func(_ time.Duration, rate int) (device, int, error) {
						opens++
						if rate != rates[opens-1] {
							t.Fatalf("session %d opened at %d", opens, rate)
						}
						n := 7
						if preRoll {
							n += rate / 5
						}
						if opens == 1 {
							return first, n, nil
						}
						if !first.closed {
							t.Fatal("new session opened before the old one closed")
						}
						return second, n, nil
					}
					oldWrites, tailFrames := 0, 0
					first.writeFn = func(out []float32) error {
						oldWrites++
						if oldWrites == 1 {
							old.Pause()
							next.Play()
							return nil
						}
						if old.v.at() != 7 || next.v.at() != 0 {
							t.Fatal("output fade advanced a song's cursor")
						}
						for i := 0; i < len(out); i += channels {
							want := float32(.5) * float32(rates[0]/200-1-tailFrames) / float32(rates[0]/200)
							if out[i] != want || out[i+1] != 0 {
								t.Fatalf("old output fade frame %d = %v, want %v", tailFrames, out[i:i+channels], want)
							}
							tailFrames++
						}
						return nil
					}
					playOutput()
					oldAt := old.v.at()
					if first.drains != 1 || tailFrames != rates[0]/200 || next.v.at() != 0 {
						t.Fatal("manual skip did not finish its output envelope or consumed the next song on the old output")
					}
					second.writeFn = func(out []float32) error {
						silent := 0
						if preRoll {
							silent = rates[1] / 5 * channels
						}
						for i, sample := range out {
							want := float32(0)
							if i >= silent && i%channels == 1 {
								want = .25
							}
							if sample != want {
								t.Fatalf("new output sample %d = %v, want %v (old track leaked across rate change)", i, sample, want)
							}
						}
						if old.v.at() != oldAt || next.v.at() != 7 {
							t.Fatal("paused source advanced or next song's first frames were skipped")
						}
						mu.Lock()
						testMode = true
						mu.Unlock()
						return nil
					}
					playOutput()
					// A retired player is still reusable until its owner closes it.
					old.Play()
					if !old.v.active || !old.Playing() || old.v.at() != oldAt {
						t.Fatal("retired paused player could not resume")
					}
					resumed := make([]float32, (warmupFrames+1)*channels)
					mixTest(resumed)
					if got, want := resumed[len(resumed)-channels], float32(.5)/float32(rampFrames()); got != want {
						t.Fatalf("resume first sample %v, want ramped sample %v", got, want)
					}
					old.Close()
				})
			}
		}
	}
}

func TestRateTransitionEnvelopeFitsBackendBuffers(t *testing.T) {
	for _, rate := range []int{44100, 48000, 88200, 96000, 192000, 384000} {
		t.Run(fmt.Sprint(rate), func(t *testing.T) {
			resetOutputTest(t)
			testMode = false
			outputRate = rate + 1
			const block = 512 // AudioQueue's buffer capacity, even at 384 kHz.
			last := [channels]float32{.75, -.5}
			d := &reviewDrainingDevice{}
			frames := 0
			previous := last
			d.writeFn = func(out []float32) error {
				if len(out) == 0 || len(out) > block*channels {
					t.Fatal("transition overflows backend buffer")
				}
				for i := 0; i < len(out); i += channels {
					if out[i] > previous[0] || out[i] < 0 || out[i+1] < previous[1] || out[i+1] > 0 {
						t.Fatal("transition envelope is not monotonic towards silence")
					}
					copy(previous[:], out[i:i+channels])
					frames++
				}
				return nil
			}
			if err := finishRateTransition(d, latency, rate, block, last); err != nil {
				t.Fatal(err)
			}
			if frames != rate/200 || previous != [channels]float32{} || d.drains != 1 {
				t.Fatal("transition did not end at zero after exactly 5ms of old-rate frames")
			}
		})
	}
}

func TestManualTransitionAfterSameRateNaturalEndStillFades(t *testing.T) {
	resetOutputTest(t)
	old := testSong(48000).SetPrimary(true)
	next := testSong(48000).SetPrimary(true)
	third := testSong(44100).SetPrimary(true)
	ended := make(chan struct{})
	old.OnEnd(func() { close(ended) })
	old.Play()
	runOnce.Do(func() {})
	testMode = false
	d := &reviewDrainingDevice{}
	openOutput = func(_ time.Duration, rate int) (device, int, error) { return d, rate / 100, nil }
	stage := 0
	d.writeFn = func([]float32) error {
		if stage == 0 && !old.Playing() {
			<-ended
			stage = 1
			next.Play()
		} else if stage == 1 {
			stage = 2
			next.Pause()
			third.Play()
		}
		return nil
	}
	playOutput()
	if stage != 2 || !d.closed || d.drains != 2 || third.v.at() != 0 {
		t.Fatal("natural completion suppressed the following manual output fade")
	}
}

func TestManualSameRateSwitchKeepsPauseRamp(t *testing.T) {
	resetOutputTest(t)
	const rate = 48000
	samples := make([]float32, rate*channels)
	for i := 0; i < len(samples); i += channels {
		samples[i] = .5
	}
	old := NewSound(rate, channels, samples).NewPlayer().SetPrimary(true)
	next := NewSound(rate, channels, make([]float32, rate*channels)).NewPlayer().SetPrimary(true).SetPreRoll(true)
	old.Play()
	mixTest(make([]float32, 7*channels))
	old.Pause()
	next.Play()
	if warmupFrames != 0 {
		t.Fatal("same-rate switch added pre-roll")
	}
	// A pause on the same running output must keep its short anti-click ramp.
	out := make([]float32, (rampFrames()+1)*channels)
	mixTest(out)
	if out[0] <= 0 || out[0] >= .5 || out[len(out)-channels] != 0 || old.v.active {
		t.Fatal("same-session fade did not run to silence")
	}
	for i := channels; i < len(out); i += channels {
		if out[i] > out[i-channels] || out[i] < 0 {
			t.Fatal("pause ramp was not monotonic")
		}
	}
}
