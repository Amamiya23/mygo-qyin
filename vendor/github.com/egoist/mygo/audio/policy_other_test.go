//go:build !((darwin && !ios) || (((linux && !android) || windows) && (amd64 || arm64)))

package audio

import (
	"context"
	"testing"
	"time"
)

func TestOtherPlatformsShutdownDoesNotStopPlayback(t *testing.T) {
	resetOutputTest(t)
	p := testSong(44100)
	p.Play()
	if err := Shutdown(context.Background(), 100*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if !p.Playing() || shuttingDown != nil {
		t.Fatal("desktop shutdown affected a non-desktop backend")
	}
}

func TestOtherPlatformsIgnoreDesktopOptions(t *testing.T) {
	resetOutputTest(t)
	p := testSong(44100).SetPrimary(true).SetPreRoll(true)
	p.Play()
	if p.v.preRoll || outputRate != defaultOutputRate || warmupFrames != 0 {
		t.Fatal("desktop options enabled on a non-desktop backend")
	}
	// Even stale shared state cannot turn on pre-roll on a non-desktop target.
	warmupFrames = 9600
	out := make([]float32, 2)
	mixTest(out)
	if warmupFrames != 0 || p.v.at() == 0 || out[0] == 0 {
		t.Fatal("non-desktop playback consumed pre-roll")
	}
	p.Pause()
	p.Play()
	if outputRate != defaultOutputRate || warmupFrames != 0 {
		t.Fatal("resume changed non-desktop rate or armed silence")
	}
}

type fixedRateTestDevice struct{ check func([]float32) error }

func (d fixedRateTestDevice) write(out []float32) error { return d.check(out) }
func (d fixedRateTestDevice) close()                    {}

func TestOtherPlatformsColdOpenRemainsFixedRate(t *testing.T) {
	resetOutputTest(t)
	p := testSong(96000).SetPrimary(true).SetPreRoll(true)
	p.Play()
	outputRate = 96000 // simulate stale state before a reopen
	runOnce.Do(func() {})
	testMode = false
	openOutput = func(_ time.Duration, rate int) (device, int, error) {
		if rate != defaultOutputRate {
			t.Fatalf("non-desktop opened at %d", rate)
		}
		return fixedRateTestDevice{check: func(out []float32) error {
			if warmupFrames != 0 || p.v.at() == 0 || out[0] == 0 {
				t.Fatal("non-desktop cold open inserted silence")
			}
			return errRetry
		}}, 480, nil
	}
	playOutput()
}
