package audio

import (
	"context"
	"math"
	"time"
)

type outputShutdown struct {
	done   chan struct{}
	err    error
	voices []*voice
	fade   time.Duration
}

// Guarded by mu. Shutdown is terminal for the process's audio output.
var shuttingDown *outputShutdown

// Shutdown stops all players and closes the desktop output on its owning
// thread, fading continuous music for fade when playing, then draining. It never opens
// a device. Subsequent Play calls do nothing. Concurrent calls share completion.
// ctx limits waiting, not an in-flight native call: after a timeout cleanup
// continues on the output thread. Intended for final application shutdown.
// On Web, mobile and unsupported backends this is a no-op.
// A nonpositive fade skips the music fade, retaining only a short anti-click tail.
func Shutdown(ctx context.Context, fade time.Duration) error {
	if !desktopAudioFeatures {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s := beginShutdown(fade)
	select {
	case <-s.done:
		return s.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func beginShutdown(fade time.Duration) *outputShutdown {
	mu.Lock()
	defer mu.Unlock()
	if shuttingDown == nil {
		shuttingDown = &outputShutdown{done: make(chan struct{}), voices: voices, fade: fade}
		// Keep voices active until their fade is rendered, so Player cleanup
		// cannot close an orphaned decoder in the middle of the fade.
		voices = nil
		warmupFrames = 0
		if outputState == outputClosed {
			completeShutdownLocked(nil)
		}
	}
	return shuttingDown
}

func completeShutdownLocked(err error) {
	if s := shuttingDown; s != nil {
		select {
		case <-s.done:
		default:
			for _, v := range s.voices {
				v.active = false
				v.close()
			}
			s.voices = nil
			s.err = err
			close(s.done)
		}
	}
}

// Fade continuous samples from the already-rendering voices, not a held last sample.
// Keep decoder sources alive until completion and bypass mix/OnEnd dispatch.
// Only voices which rendered on this session can contribute: a newly queued
// song must never be read on a previous song's output rate during shutdown.
func finishShutdown(dev device, rate, block int, hasAudio bool, last [channels]float32) error {
	if !hasAudio {
		return nil
	}
	mu.Lock()
	lat := latency
	fade := shuttingDown.fade
	var fading []*voice
	for _, v := range shuttingDown.voices {
		if !v.closed && v.playing && !v.finishing && v.mixedRate == rate {
			fading = append(fading, v)
		}
	}
	mu.Unlock()
	if fade <= 0 || len(fading) == 0 {
		// Paused audio may still be queued; retain the short anti-click tail.
		return finishRateTransition(dev, lat, rate, block, last)
	}
	total := max(int(fade.Seconds()*float64(rate)), 1)
	buf := make([]float32, min(block, total)*channels)
	for at := 0; at < total; {
		n := min(block, total-at)
		out := buf[:n*channels]
		clear(out)
		mu.Lock()
		for _, v := range fading {
			if !v.closed && v.playing && !v.finishing {
				v.play(out, volume, rate)
			}
		}
		mu.Unlock()
		for i := 0; i < n; i++ {
			t := float64(at+i+1) / float64(total)
			gain := float32(0.5 + 0.5*math.Cos(math.Pi*t))
			for ch := range channels {
				k := i*channels + ch
				out[k] = min(max(out[k], -1), 1) * gain
			}
		}
		if err := dev.write(out); err != nil {
			return err
		}
		at += n
	}
	if d, ok := dev.(drainingDevice); ok {
		_, err := d.drain()
		return err
	}
	return nil
}
