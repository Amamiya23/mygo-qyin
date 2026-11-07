package audio

import (
	"errors"
	"runtime"
	"sync"
	"time"
)

// A device is the system's audio output, opened by openDevice to hold
// about ahead of what is heard.
type device interface {
	// write plays interleaved stereo frames after those written before,
	// waiting while the device holds enough already. An error closes the
	// device, and the output opens it again.
	write(samples []float32) error
	close()
}

// A drainingDevice waits for queued audio to be played without closing it.
type drainingDevice interface{ drain() error }

// naturalDrain reports a primary natural EOF, or all remaining voices at EOF.
// It excludes user pauses/stops and never holds mu during a backend operation.
func naturalDrain() bool {
	mu.Lock()
	defer mu.Unlock()
	if !desktopAudioFeatures || len(voices) == 0 {
		return false
	}
	all := true
	for _, v := range voices {
		if v.closed || !v.playing || !v.finishing {
			all = false
			continue
		}
		// A primary track must keep its tail even while effects are active.
		if v.primary {
			return true
		}
	}
	return all
}

// idleClose is how long the output stays open with nothing playing.
const idleClose = 5 * time.Second

// Device lifecycle, guarded by mu. deviceRate describes the opened stream;
// outputRate is the requested rate and may change while a backend is opening.
type deviceState uint8

const (
	outputClosed deviceState = iota
	outputOpening
	outputReady
	outputClosing
)

var (
	outputState = outputClosed
	deviceRate  int
)

// playbackRateLocked reselects the rate from active source metadata before
// every open, including reopening after idle close, error or system wake.
// The most recently started primary music voice wins over sound effects.
func playbackRateLocked() int {
	if !desktopAudioFeatures {
		return defaultOutputRate
	}
	rate := 0
	for i := len(voices) - 1; i >= 0; i-- {
		v := voices[i]
		if !v.playing || v.closed {
			continue
		}
		if v.primary {
			return v.src.sampleRate()
		}
		// Without an explicit primary, keep the first playing voice's rate:
		// a later effect must not change a cold device's music rate.
		rate = v.src.sampleRate()
	}
	if rate > 0 {
		return rate
	}
	return outputRate
}

// preRollRequestedLocked excludes ordinary effects and platforms which do
// not support hardware pre-roll. The caller holds mu.
func preRollRequestedLocked() bool {
	if !desktopAudioFeatures {
		return false
	}
	for _, v := range voices {
		if v.playing && !v.closed && v.preRoll {
			return true
		}
	}
	return false
}

var (
	openOutput = openDevice // tests replace it

	wake     = make(chan struct{}, 1)
	runOnce  sync.Once
	testMode bool // tests mix by hand
)

// wakeOutput starts the output, or tells it a voice started. The caller
// holds mu.
func wakeOutput() {
	if testMode {
		return
	}
	runOnce.Do(func() { go run() })
	select {
	case wake <- struct{}{}:
	default:
	}
}

func active() bool {
	mu.Lock()
	defer mu.Unlock()
	return len(voices) > 0 && !testMode
}

// mixFor mixes into buf for an output open at latency lat, and reports
// whether anything plays; ok is false, and it mixes nothing, once tests
// mix by hand or the latency/rate changed.
func mixFor(buf []float32, lat Latency, rate int) (busy, ok bool) {
	mu.Lock()
	defer mu.Unlock()
	if testMode || latency != lat || outputRate != rate {
		return false, false
	}
	return mixLocked(buf), true
}

// run opens the device while anything plays, and mixes into it.
func run() {
	// Devices that need it, as COM's, stay on one thread.
	runtime.LockOSThread()
	for {
		for !active() {
			<-wake
		}
		playOutput()
	}
}

// playOutput owns one device session; only the output thread calls it.
func playOutput() {
	mu.Lock()
	outputRate = playbackRateLocked()
	lat, rate := latency, outputRate
	outputState = outputOpening
	mu.Unlock()
	dev, n, err := openOutput(lat.ahead(), rate)
	if err != nil {
		setErr(err)
		dev, n = newNullDevice(err != errUnsupported, rate), max(rate/100, 1)
	} else {
		setErr(nil)
	}
	mu.Lock()
	outputState, deviceRate = outputReady, rate
	if !testMode && outputRate == rate && latency == lat {
		held, unheard = int64(lat.ahead())*int64(rate)/int64(time.Second), 0
		if err != nil {
			// Null output holds nothing.
			held = 0
		}
		warmupFrames = 0
		if preRollRequestedLocked() {
			warmupFrames = rate / 5
		}
		for _, v := range voices {
			v.marks = v.marks[:0]
			// Recalculate any running gain ramp at the new output rate.
			if v.rampLeft > 0 {
				v.rampLeft = rampFrames()
				for i := range v.step {
					v.step[i] = (v.target[i] - v.gain[i]) / float32(v.rampLeft)
				}
			}
		}
	}
	mu.Unlock()
	buf := make([]float32, n*channels)
	idle := time.Duration(0)
	per := frames(int64(n), rate)
	for {
		busy, ok := mixFor(buf, lat, rate)
		if !ok {
			break
		}
		if err := dev.write(buf); err != nil {
			// Lost, or a null device trying the real one again.
			break
		}
		if naturalDrain() {
			if d, ok := dev.(drainingDevice); ok {
				if err := d.drain(); err != nil {
					setErr(err)
					break
				}
				mu.Lock()
				unheard = 0
				mu.Unlock()
			}
		}
		if busy {
			idle = 0
		} else if idle += per; idle >= idleClose {
			break
		}
	}
	mu.Lock()
	outputState = outputClosing
	mu.Unlock()
	dev.close()
	mu.Lock()
	outputState, deviceRate = outputClosed, 0
	held, unheard = 0, 0
	mu.Unlock()
}

var errUnsupported = errors.New("audio: no audio output on this platform")

// A nullDevice plays nothing, on time: players still end when they would
// have. One standing in for a device that failed to open fails after a
// while, so that the output tries the device again.
type nullDevice struct {
	rate  int
	next  time.Time
	retry time.Time
	// now and sleep are its clock, which tests replace.
	now   func() time.Time
	sleep func(time.Duration)
}

func newNullDevice(retry bool, rate int) *nullDevice {
	d := &nullDevice{rate: rate, next: time.Now(), now: time.Now, sleep: time.Sleep}
	if retry {
		d.retry = d.next.Add(2 * time.Second)
	}
	return d
}

var errRetry = errors.New("audio: try the device again")

func (d *nullDevice) write(samples []float32) error {
	now := d.now()
	if !d.retry.IsZero() && now.After(d.retry) {
		return errRetry
	}
	if now.Sub(d.next) > 100*time.Millisecond {
		// Behind, as after a sleep: don't hurry to catch up.
		d.next = now
	}
	d.next = d.next.Add(frames(int64(len(samples)/channels), d.rate))
	d.sleep(d.next.Sub(d.now()))
	return nil
}

func (d *nullDevice) close() {}
