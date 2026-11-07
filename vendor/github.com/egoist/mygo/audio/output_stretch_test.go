//go:build (darwin && !ios) || (((linux && !android) || windows) && (amd64 || arm64))

package audio

import (
	"fmt"
	"testing"
	"time"
)

// Pitch-preserving playback follows its speed at the native output rate after pre-roll.
func TestNativeRatePreservesPitchPosition(t *testing.T) {
	for _, rate := range []int{44100, 96000} {
		for _, speed := range []float64{0.5, 1.5} {
			t.Run(fmt.Sprintf("%d/%g", rate, speed), func(t *testing.T) {
				resetOutputTest(t)
				p := testSong(rate).SetSpeed(speed).SetPreservesPitch(true)
				p.Play()
				mixTest(make([]float32, rate/5*channels))
				if p.Position() != 0 {
					t.Fatal("pre-roll advanced the stretched source")
				}
				mixTest(make([]float32, rate/10*channels))
				want := time.Duration(speed * float64(100*time.Millisecond))
				if delta := p.Position() - want; delta < -time.Microsecond || delta > time.Microsecond {
					t.Fatalf("position = %v, want %v after 100ms output", p.Position(), want)
				}
			})
		}
	}
}
