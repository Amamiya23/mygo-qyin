//go:build (darwin && !ios) || (((linux && !android) || windows) && (amd64 || arm64))

package audio

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

type reviewDrainingDevice struct {
	rateTestDevice
	drains int
}

func (d *reviewDrainingDevice) drain() (bool, error) { d.drains++; return false, nil }

func TestNaturalRateSwitchKeepsTail(t *testing.T) {
	for _, lat := range []Latency{LatencyInteractive, LatencyPlayback} {
		for _, nativeDrain := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/drain=%t", lat, nativeDrain), func(t *testing.T) {
				resetOutputTest(t)
				latency = lat
				tracks := []*Player{testSong(44100).SetPrimary(true), testSong(96000).SetPrimary(true), testSong(48000).SetPrimary(true), testSong(88200).SetPrimary(true), testSong(192000).SetPrimary(true)}
				tracks[0].Play()
				runOnce.Do(func() {})
				testMode = false
				for i, old := range tracks {
					ended := make(chan struct{})
					old.OnEnd(func() { close(ended) })
					d := &reviewDrainingDevice{}
					completed := false
					openOutput = func(_ time.Duration, rate int) (device, int, error) {
						if rate != int(old.v.rate) {
							t.Fatalf("open rate %d", rate)
						}
						if nativeDrain {
							return d, rate / 100, nil
						}
						return &d.rateTestDevice, rate / 100, nil
					}
					d.writeFn = func(_ []float32) error {
						if !old.Playing() && !completed {
							<-ended
							completed = true
							if old.Position() < old.Duration() {
								t.Fatalf("tail cut: audible %s of %s", old.Position(), old.Duration())
							}
							if i+1 < len(tracks) {
								tracks[i+1].Play()
							} else {
								mu.Lock()
								testMode = true
								mu.Unlock()
							}
						}
						return nil
					}
					playOutput()
					if !completed || !d.closed {
						t.Fatal("track did not finish and close")
					}
					if nativeDrain && d.drains != 1 {
						t.Fatalf("native drains %d", d.drains)
					}
				}
			})
		}
	}
}

func TestManualRateSwitchDuringPrerollDoesNotDrain(t *testing.T) {
	resetOutputTest(t)
	old := testSong(44100).SetPrimary(true)
	next := testSong(96000).SetPrimary(true)
	old.Play()
	runOnce.Do(func() {})
	testMode = false
	d := &reviewDrainingDevice{}
	openOutput = func(_ time.Duration, rate int) (device, int, error) { return d, rate / 100, nil }
	d.writeFn = func(_ []float32) error { old.Close(); next.Play(); return nil }
	playOutput()
	if !d.closed || d.drains != 0 || next.v.at() != 0 {
		t.Fatal("manual skip waited for natural drain or consumed next song")
	}
}

func TestSameRateNaturalTransitionDoesNotWarmAgain(t *testing.T) {
	resetOutputTest(t)
	old := testSong(44100).SetPrimary(true)
	next := testSong(44100).SetPrimary(true)
	old.Play()
	runOnce.Do(func() {})
	testMode = false
	ended := make(chan struct{})
	old.OnEnd(func() { close(ended) })
	d := &reviewDrainingDevice{}
	openOutput = func(_ time.Duration, rate int) (device, int, error) { return d, rate / 100, nil }
	switched := false
	d.writeFn = func(out []float32) error {
		if !old.Playing() && !switched {
			<-ended
			switched = true
			next.Play()
			if warmupFrames != 0 {
				t.Fatal("same-rate natural transition added pre-roll")
			}
		} else if switched {
			if out[0] == 0 || next.v.at() == 0 {
				t.Fatal("same-rate next song did not start")
			}
			mu.Lock()
			testMode = true
			mu.Unlock()
		}
		return nil
	}
	playOutput()
	if !switched || d.drains != 1 {
		t.Fatal("same-rate transition did not drain old tail")
	}
}

func TestDurationConcurrentWithPlay(t *testing.T) {
	resetOutputTest(t)
	p := testSong(96000)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			p.Pause()
			p.Play()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			if p.Duration() != time.Second {
				t.Error("duration changed")
				return
			}
		}
	}()
	wg.Wait()
}

func TestPrimaryNaturalEndDrainsWithLoopingEffect(t *testing.T) {
	resetOutputTest(t)
	old := testSong(44100).SetPrimary(true)
	effect := testSong(44100).SetPreRoll(false).SetLoop(true)
	next := testSong(96000).SetPrimary(true)
	old.Play()
	effect.Play()
	runOnce.Do(func() {})
	testMode = false
	ended := make(chan struct{})
	old.OnEnd(func() { close(ended) })
	d := &reviewDrainingDevice{}
	openOutput = func(_ time.Duration, rate int) (device, int, error) { return d, rate / 100, nil }
	d.writeFn = func(_ []float32) error {
		if !old.Playing() {
			<-ended
			if old.Position() != old.Duration() || d.drains != 1 || !effect.Playing() {
				t.Fatal("primary tail was not drained alongside effect")
			}
			next.Play()
		}
		return nil
	}
	playOutput()
	if !d.closed || d.drains != 1 {
		t.Fatal("primary transition failed")
	}
}

func TestManualSkipReplacesPendingNaturalEndWithOutputFade(t *testing.T) {
	resetOutputTest(t)
	old := testSong(44100).SetPrimary(true)
	next := testSong(96000).SetPrimary(true)
	old.Play()
	runOnce.Do(func() {})
	testMode = false
	d := &reviewDrainingDevice{}
	openOutput = func(_ time.Duration, rate int) (device, int, error) { return d, rate / 100, nil }
	skipped := false
	d.writeFn = func(_ []float32) error {
		if old.v.finishing {
			skipped = true
			old.Close()
			next.Play()
		}
		return nil
	}
	playOutput()
	if !skipped || d.drains != 1 || !d.closed || old.Playing() || next.v.at() != 0 {
		t.Fatal("manual skip did not replace the pending natural finish with a short output fade")
	}
}
