package audio

import "testing"

// All tests drive frames explicitly; no device, decoder worker or clock sleeps.
func resetOutputTest(t *testing.T) {
	t.Helper()
	mu.Lock()
	testMode = true
	voices = nil
	shuttingDown = nil
	outputRate, warmupFrames = 48000, 0
	outputState, deviceRate = outputClosed, 0
	mixed, held, unheard = 0, 0, 0
	volume, latency = 1, LatencyInteractive
	mu.Unlock()
	original := openOutput
	t.Cleanup(func() {
		mu.Lock()
		for _, v := range voices {
			v.close()
		}
		voices = nil
		shuttingDown = nil
		testMode = true
		openOutput = original
		mu.Unlock()
	})
}

func mixTest(buf []float32) bool {
	mu.Lock()
	outputState, deviceRate = outputReady, outputRate
	mu.Unlock()
	return mix(buf)
}

func testSong(rate int) *Player {
	samples := make([]float32, rate*2)
	for i := range samples {
		samples[i] = float32(i%97-48) / 64
	}
	return NewSound(rate, 2, samples).NewPlayer().SetPreRoll(true)
}
