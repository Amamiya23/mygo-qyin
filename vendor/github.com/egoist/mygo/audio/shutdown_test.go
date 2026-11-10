//go:build (darwin && !ios) || (((linux && !android) || windows) && (amd64 || arm64))

package audio

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"
)

// Shutdown must render the requested pause-length fade before closing, without reopening.
func TestShutdownFadesClosesAndNeverReopens(t *testing.T) {
	for _, rate := range []int{44100, 48000, 88200, 96000, 192000, 384000} {
		for _, lat := range []Latency{LatencyInteractive, LatencyPlayback} {
			t.Run(fmt.Sprintf("%d/%d", rate, lat), func(t *testing.T) {
				resetOutputTest(t)
				latency = lat
				samples := make([]float32, rate*channels)
				for i := 0; i < len(samples); i += channels {
					samples[i] = .25 + float32((i/channels)%31)/128
					samples[i+1] = -.5 + float32((i/channels)%29)/128
				}
				p := NewSound(rate, channels, samples).NewPlayer().SetPrimary(true).SetPreRoll(true)
				p.Play()
				runOnce.Do(func() {})
				testMode = false
				d := &reviewDrainingDevice{}
				opens := 0
				openOutput = func(_ time.Duration, got int) (device, int, error) {
					opens++
					if got != rate {
						t.Fatal("wrong shutdown device rate")
					}
					return d, min(rate/100, 512), nil
				}
				var request *outputShutdown
				tail := 0
				at := float64(0)
				d.writeFn = func(out []float32) error {
					if request == nil {
						if out[0] == 0 { // initial hardware pre-roll
							return nil
						}
						at = p.v.at()
						request = beginShutdown(750 * time.Millisecond)
						p.Play() // terminal shutdown must suppress restarts
						if p.Playing() || p.v.closed || !p.v.active {
							t.Fatal("shutdown must report stopped but keep the source alive for its fade")
						}
						return nil
					}
					if p.v.at() != at+float64(tail+len(out)/channels) || p.v.closed {
						t.Fatal("shutdown source did not advance exactly through its music fade")
					}
					for i := 0; i < len(out); i += channels {
						gain := float32(0.5 + 0.5*math.Cos(math.Pi*float64(tail+1)/float64(rate*3/4)))
						j := (int(at) + tail) * channels
						if out[i] != samples[j]*gain || out[i+1] != samples[j+1]*gain {
							t.Fatalf("shutdown did not fade continuous stereo music at frame %d", tail)
						}
						tail++
					}
					return nil
				}
				playOutput()
				if request == nil || tail != rate*3/4 || !p.v.closed || p.v.active || d.drains != 1 || !d.closed || outputState != outputClosed || active() {
					t.Fatal("shutdown did not complete fade/drain/close")
				}
				if err := Shutdown(context.Background(), 100*time.Millisecond); err != nil {
					t.Fatal(err)
				}
				playOutput() // even a queued wake must not reopen
				if opens != 1 {
					t.Fatal("shutdown reopened the device")
				}
			})
		}
	}
}

// Disabling the exit fade must not consume more music, but still drain and close.
func TestShutdownWithoutMusicFade(t *testing.T) {
	resetOutputTest(t)
	p := testSong(48000).SetPreRoll(false)
	p.Play()
	runOnce.Do(func() {})
	testMode = false
	d := &reviewDrainingDevice{}
	openOutput = func(time.Duration, int) (device, int, error) { return d, 480, nil }
	writes := 0
	var position float64
	d.writeFn = func([]float32) error {
		writes++
		if writes == 1 {
			position = p.v.at()
			beginShutdown(0)
		} else if p.v.at() != position {
			t.Fatal("disabled exit fade consumed music")
		}
		return nil
	}
	playOutput()
	if writes != 2 || p.v.at() != position || !p.v.closed || !d.closed || d.drains != 1 {
		t.Fatal("disabled exit fade did not limit shutdown to the short anti-click tail")
	}
}

func TestShutdownDuringPrerollDoesNotStartMusic(t *testing.T) {
	resetOutputTest(t)
	p := testSong(48000)
	p.Play()
	runOnce.Do(func() {})
	testMode = false
	d := &reviewDrainingDevice{}
	openOutput = func(time.Duration, int) (device, int, error) { return d, 480, nil }
	writes := 0
	d.writeFn = func([]float32) error { writes++; beginShutdown(100 * time.Millisecond); return nil }
	playOutput()
	if writes != 1 || d.drains != 0 || p.v.at() != 0 || !p.v.closed || !d.closed {
		t.Fatal("quitting during pre-roll read music or added a fade")
	}
}

func TestShutdownDuringRateChangeOnlyFadesOriginalMusic(t *testing.T) {
	resetOutputTest(t)
	old := testSong(48000).SetPrimary(true).SetPreRoll(false)
	next := testSong(96000).SetPrimary(true)
	old.Play()
	runOnce.Do(func() {})
	testMode = false
	d := &reviewDrainingDevice{}
	openOutput = func(time.Duration, int) (device, int, error) { return d, 480, nil }
	writes := 0
	d.writeFn = func([]float32) error {
		writes++
		if writes == 1 {
			next.Play()
			beginShutdown(100 * time.Millisecond)
		}
		return nil
	}
	playOutput()
	if writes != 11 || old.v.at() != 480+4800 || next.v.at() != 0 || !next.v.closed || !d.closed {
		t.Fatal("shutdown used the new rate or read a new song on the old output")
	}
}

func TestShutdownEOFDoesNotStartNextSong(t *testing.T) {
	resetOutputTest(t)
	p := NewSound(48000, channels, make([]float32, 600*channels)).NewPlayer().SetPrimary(true)
	for i := range p.v.src.(*memSource).samples {
		p.v.src.(*memSource).samples[i] = .25
	}
	ended := make(chan struct{}, 1)
	p.OnEnd(func() { ended <- struct{}{} })
	p.Play()
	runOnce.Do(func() {})
	testMode = false
	d := &reviewDrainingDevice{}
	openOutput = func(time.Duration, int) (device, int, error) { return d, 480, nil }
	d.writeFn = func([]float32) error { beginShutdown(100 * time.Millisecond); return nil }
	playOutput()
	if !p.v.closed || !d.closed || p.v.position(p.v.at()) != 600 {
		t.Fatal("short source did not end during shutdown")
	}
	select {
	case <-ended:
		t.Fatal("shutdown dispatched natural OnEnd")
	default:
	}
}

func TestShutdownClosedOutputDoesNotOpen(t *testing.T) {
	resetOutputTest(t)
	openOutput = func(time.Duration, int) (device, int, error) { t.Fatal("opened on quit"); return nil, 0, nil }
	if err := Shutdown(context.Background(), 100*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	testSong(96000).Play()
	if len(voices) != 0 || active() {
		t.Fatal("new playback started after shutdown")
	}
}

func TestShutdownDuringOpenClosesWithoutWriting(t *testing.T) {
	resetOutputTest(t)
	testSong(96000).Play()
	runOnce.Do(func() {})
	testMode = false
	d := &rateTestDevice{writeFn: func([]float32) error { t.Fatal("wrote after shutdown during open"); return nil }}
	openOutput = func(time.Duration, int) (device, int, error) {
		beginShutdown(100 * time.Millisecond)
		return d, 960, nil
	}
	playOutput()
	if !d.closed || outputState != outputClosed {
		t.Fatal("in-flight open was not retired on shutdown")
	}
	if err := Shutdown(context.Background(), 100*time.Millisecond); err != nil {
		t.Fatal(err)
	}
}

func TestShutdownCallerCancellationDoesNotCancelCleanup(t *testing.T) {
	resetOutputTest(t)
	outputState = outputReady
	request := beginShutdown(100 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	waiting := make(chan struct{})
	results := make(chan error)
	go func() { results <- Shutdown(shutdownWaitContext{Context: ctx, waiting: waiting}, 100*time.Millisecond) }()
	<-waiting
	cancel()
	if err := <-results; err != context.Canceled {
		t.Fatalf("cancellation error = %v", err)
	}
	select {
	case <-request.done:
		t.Fatal("caller cancellation incorrectly completed native cleanup")
	default:
	}
	mu.Lock()
	outputState = outputClosed
	completeShutdownLocked(nil)
	mu.Unlock()
	if err := Shutdown(context.Background(), 100*time.Millisecond); err != nil {
		t.Fatal(err)
	}
}

// Synchronize cancellation with the actual wait rather than a timer/sleep.
type shutdownWaitContext struct {
	context.Context
	waiting chan struct{}
}

func (c shutdownWaitContext) Done() <-chan struct{} {
	close(c.waiting)
	return c.Context.Done()
}

type shutdownCloseDevice struct {
	reviewDrainingDevice
	closing, release chan struct{}
}

func (d *shutdownCloseDevice) close() {
	close(d.closing)
	<-d.release
	d.rateTestDevice.close()
}

func TestShutdownWaitsForOwningThreadToClose(t *testing.T) {
	resetOutputTest(t)
	testSong(48000).SetPreRoll(false).Play()
	runOnce.Do(func() {})
	testMode = false
	d := &shutdownCloseDevice{closing: make(chan struct{}), release: make(chan struct{})}
	openOutput = func(time.Duration, int) (device, int, error) { return d, 480, nil }
	d.writeFn = func([]float32) error { beginShutdown(100 * time.Millisecond); return nil }
	workerDone := make(chan struct{})
	go func() { playOutput(); close(workerDone) }()
	<-d.closing
	select {
	case <-shuttingDown.done:
		t.Fatal("reported shutdown complete before native close returned")
	default:
	}
	// A canceled caller can leave while another caller still awaits native close.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Shutdown(ctx, 100*time.Millisecond); err != context.Canceled {
		t.Fatalf("cancel error = %v", err)
	}
	result := make(chan error, 1)
	go func() { result <- Shutdown(context.Background(), 100*time.Millisecond) }()
	close(d.release)
	<-workerDone
	if err := <-result; err != nil || !d.closed {
		t.Fatal("shared shutdown did not finish after native close")
	}
}

func TestShutdownPausedSilentOutputNeedsNoExtraDrain(t *testing.T) {
	resetOutputTest(t)
	p := testSong(48000).SetPreRoll(false)
	p.Play()
	runOnce.Do(func() {})
	testMode = false
	d := &reviewDrainingDevice{}
	openOutput = func(time.Duration, int) (device, int, error) { return d, 480, nil }
	writes := 0
	d.writeFn = func([]float32) error {
		writes++
		if writes == 1 {
			p.Pause()
		} else if writes == 20 { // well past fade-out plus the 40ms look-ahead
			beginShutdown(100 * time.Millisecond)
		}
		return nil
	}
	playOutput()
	if writes != 20 || d.drains != 0 || !d.closed {
		t.Fatal("already silent output added a fade/drain delay on quit")
	}
}
