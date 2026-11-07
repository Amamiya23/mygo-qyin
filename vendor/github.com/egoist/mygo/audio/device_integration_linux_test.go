//go:build linux && !android && (amd64 || arm64)

package audio

import (
	"fmt"
	"os"
	"testing"
)

// Opt-in native API check; writes silence only, never changes system routing.
// It cannot establish that a physical DAC changed its clock or kept a music tail.
func TestLinuxPulseNativeLifecycle(t *testing.T) {
	if os.Getenv("MYGO_AUDIO_DEVICE_TEST") != "1" {
		t.Skip("set MYGO_AUDIO_DEVICE_TEST=1 to use the host PulseAudio/PipeWire output")
	}
	for _, rate := range []int{44100, 48000, 88200, 96000} {
		t.Run(fmt.Sprint(rate), func(t *testing.T) {
			d, err := openPulse(LatencyPlayback.ahead(), rate)
			if err != nil {
				t.Fatal(err)
			}
			defer d.close()
			silence := make([]float32, rate/100*channels)
			for i := 0; i < 30; i++ {
				if err := d.write(silence); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := d.(drainingDevice).drain(); err != nil {
				t.Fatal(err)
			}
			if err := d.write(silence); err != nil {
				t.Fatal(err)
			}
			if _, err := d.(drainingDevice).drain(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
