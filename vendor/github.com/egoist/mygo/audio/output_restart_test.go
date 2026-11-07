//go:build (darwin && !ios) || (((linux && !android) || windows) && (amd64 || arm64))

package audio

import (
	"slices"
	"testing"
	"time"
)

type stoppedAfterDrainDevice struct {
	rateTestDevice
	drains int
}

func (d *stoppedAfterDrainDevice) drain() (bool, error) {
	d.drains++
	return true, nil
}

// A backend stopped by natural drain must reopen before consuming the next song.
func TestStoppedDrainReopensWithConfiguredPreroll(t *testing.T) {
	for _, tc := range []struct {
		name    string
		preRoll bool
	}{{"enabled", true}, {"disabled", false}} {
		t.Run(tc.name, func(t *testing.T) {
			resetOutputTest(t)
			const rate = 48000
			old := testSong(rate).SetPrimary(true).SetPreRoll(tc.preRoll)
			next := testSong(rate).SetPrimary(true).SetPreRoll(tc.preRoll)
			started := make(chan struct{})
			old.OnEnd(func() {
				next.Play()
				close(started)
			})
			old.Play()
			runOnce.Do(func() {})
			testMode = false
			first := &stoppedAfterDrainDevice{}
			second := &rateTestDevice{}
			opens := 0
			openOutput = func(_ time.Duration, gotRate int) (device, int, error) {
				opens++
				if gotRate != rate {
					t.Fatalf("rate = %d", gotRate)
				}
				if opens == 1 {
					return first, rate / 100, nil
				}
				if !first.closed || opens != 2 {
					t.Fatal("reopened without closing the stopped output")
				}
				n := 7
				if tc.preRoll {
					n += rate / 5
				}
				return second, n, nil
			}
			first.writeFn = func([]float32) error {
				if !old.Playing() {
					<-started
				}
				mu.Lock()
				if next.v.at() > 0 {
					testMode = true // terminate a broken session without real-time waits
				}
				mu.Unlock()
				return nil
			}
			playOutput()
			<-started
			if first.drains != 1 || !first.closed || next.v.at() != 0 {
				t.Fatal("stopped output consumed the next song instead of reopening")
			}
			if old.Position() != old.Duration() {
				t.Fatal("reopening discarded the previous song's tail")
			}
			second.writeFn = func(out []float32) error {
				silent := 0
				if tc.preRoll {
					silent = rate / 5 * channels
				}
				if slices.ContainsFunc(out[:silent], func(x float32) bool { return x != 0 }) {
					t.Fatal("reopened output did not begin with configured silence")
				}
				src := next.v.src.(*memSource)
				if next.v.at() != 7 || !slices.Equal(out[silent:], src.samples[:7*channels]) {
					t.Fatal("restart consumed or changed the next song's first frames")
				}
				mu.Lock()
				testMode = true
				mu.Unlock()
				return nil
			}
			playOutput()
			if opens != 2 || !second.closed {
				t.Fatal("replacement session did not finish")
			}
		})
	}
}
