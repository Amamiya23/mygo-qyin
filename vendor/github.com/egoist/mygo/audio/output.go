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

// A drainingDevice waits for queued audio to be played. reopen reports that
// draining stopped the backend and a fresh session is needed before more audio.
type drainingDevice interface {
	drain() (reopen bool, err error)
}

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
// mix by hand or the latency/rate changed. primary reports a primary voice
// rendering audio rather than waiting for a previously rendered natural tail.
func mixFor(buf []float32, lat Latency, rate int) (busy, ok, primary bool) {
	mu.Lock()
	defer mu.Unlock()
	if testMode || latency != lat || outputRate != rate {
		return false, false, false
	}
	for _, v := range voices {
		primary = primary || v.primary && v.playing && !v.closed && !v.finishing
	}
	return mixLocked(buf), true, primary
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
		retirePausedFadesLocked()
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
	var last [channels]float32
	naturalTailPlayed := false
	hasAudio := false
	idle := time.Duration(0)
	per := frames(int64(n), rate)
	for {
		busy, ok, primary := mixFor(buf, lat, rate)
		if !ok {
			if hasAudio && !naturalTailPlayed {
				if err := finishRateTransition(dev, lat, rate, n, last); err != nil {
					setErr(err)
				}
			}
			break
		}
		if primary {
			naturalTailPlayed = false
		}
		if err := dev.write(buf); err != nil {
			// Lost, or a null device trying the real one again.
			break
		}
		copy(last[:], buf[len(buf)-channels:])
		if !hasAudio {
			for _, s := range buf {
				if s != 0 {
					hasAudio = true
					break
				}
			}
		}
		if naturalDrain() {
			if d, ok := dev.(drainingDevice); ok {
				reopen, err := d.drain()
				if err != nil {
					setErr(err)
					break
				}
				mu.Lock()
				unheard = 0
				// The natural tail was heard already; do not append a manual fade.
				naturalTailPlayed = true
				hasAudio = false
				if reopen {
					// Publish the heard tail before retiring the stopped backend.
					// An empty mix completes EOF without consuming the next song.
					mixLocked(nil)
				}
				mu.Unlock()
				if reopen {
					break
				}
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

// finishRateTransition gently brings the old output to zero before changing
// its rate. The source cursors are deliberately untouched: the old player may
// already be closed, and the new player must only run on its own output.
// This waits for the short output queue, not for the old song's natural EOF.
func finishRateTransition(dev device, lat Latency, rate, block int, last [channels]float32) error {
	mu.Lock()
	changing := desktopAudioFeatures && !testMode && latency == lat && outputRate != rate
	mu.Unlock()
	d, canDrain := dev.(drainingDevice)
	if !changing || !canDrain {
		return nil
	}
	frames := max(rate/200, 1)
	tail := make([]float32, frames*channels)
	for i := range frames {
		gain := float32(frames-1-i) / float32(frames)
		for ch := range channels {
			tail[i*channels+ch] = last[ch] * gain
		}
	}
	// AudioQueue buffers have a fixed capacity, including at high rates.
	for len(tail) > 0 {
		n := min(len(tail), block*channels)
		if err := dev.write(tail[:n]); err != nil {
			return err
		}
		tail = tail[n:]
	}
	_, err := d.drain()
	return err
}

// A paused voice may still be fading out on the old output. That fade belongs
// to that session: replaying it after a new session's silence brings back a
// fragment of the previous song. Retire it without closing or seeking its
// source, so a later Play can still resume with the usual fade-in. Voices
// which are playing (including effects and natural EOF tails) stay intact.
// The caller holds mu; same-session pauses never pass through here.
func retirePausedFadesLocked() {
	if !desktopAudioFeatures {
		return
	}
	n := 0
	for _, v := range voices {
		if !v.playing && !v.closed {
			v.active, v.finishing = false, false
			v.gain, v.target, v.step = [4]float32{}, [4]float32{}, [4]float32{}
			v.rampLeft = 0
			if v.orphan {
				v.close()
			}
			continue
		}
		voices[n] = v
		n++
	}
	clear(voices[n:])
	voices = voices[:n]
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
