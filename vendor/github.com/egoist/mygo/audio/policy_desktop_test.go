//go:build (darwin && !ios) || (((linux && !android) || windows) && (amd64 || arm64))

package audio

import (
	"testing"
	"time"
)

func TestEffectHasNoPrerollByDefault(t *testing.T) {
	resetOutputTest(t)
	p := testSong(44100).SetPreRoll(false)
	p.Play()
	if warmupFrames != 0 {
		t.Fatal("ordinary player requested silence")
	}
	src := p.v.src.(*memSource)
	out := make([]float32, 16)
	mixTest(out)
	if out[0] != src.samples[0] || p.v.at() != 8 {
		t.Fatal("ordinary effect did not start immediately")
	}
	p.Pause()
	p.Play()
	if warmupFrames != 0 {
		t.Fatal("ordinary resume requested silence")
	}
	p.Stop()
	p.Play()
	if warmupFrames != 0 {
		t.Fatal("ordinary restart requested silence")
	}
}

func TestEffectColdOpenHasNoPreroll(t *testing.T) {
	resetOutputTest(t)
	p := testSong(96000).SetPreRoll(false)
	p.Play()
	runOnce.Do(func() {})
	testMode = false
	d := &rateTestDevice{}
	openOutput = func(_ time.Duration, rate int) (device, int, error) {
		if rate != 96000 {
			t.Fatal("effect cold rate")
		}
		return d, 960, nil
	}
	d.writeFn = func(out []float32) error {
		if p.v.at() != 960 || warmupFrames != 0 || out[0] == 0 {
			t.Fatal("ordinary cold open added pre-roll")
		}
		return errRetry
	}
	playOutput()
}

func TestEffectDoesNotRestartMusicPreroll(t *testing.T) {
	resetOutputTest(t)
	music := testSong(44100)
	music.Play()
	mixTest(make([]float32, 200))
	left := warmupFrames
	effect := testSong(96000).SetPreRoll(false)
	effect.Play()
	if outputRate != 44100 || warmupFrames != left {
		t.Fatal("effect changed music output or restarted silence")
	}
	music.SetPreRoll(false)
	mixTest(make([]float32, 2))
	if warmupFrames != 0 || effect.v.at() == 0 {
		t.Fatal("disabling last request did not cancel pending silence")
	}
}

func TestUnconfiguredPlayerDefaultsToNoPreroll(t *testing.T) {
	resetOutputTest(t)
	p := NewSound(44100, 2, []float32{.25, .5}).NewPlayer()
	if p.v.preRoll {
		t.Fatal("NewPlayer defaulted to pre-roll")
	}
	p.Play()
	if warmupFrames != 0 {
		t.Fatal("unconfigured player armed silence")
	}
}

func TestCancelledPrerollDoesNotDelayEffect(t *testing.T) {
	for _, action := range []struct {
		name   string
		cancel func(*Player)
	}{
		{"disable", func(p *Player) { p.SetPreRoll(false) }},
		{"pause", (*Player).Pause},
		{"stop", (*Player).Stop},
		{"close", (*Player).Close},
	} {
		t.Run(action.name, func(t *testing.T) {
			resetOutputTest(t)
			music := testSong(44100)
			music.Play()
			action.cancel(music)
			effect := testSong(44100).SetPreRoll(false)
			effect.Play()
			out := make([]float32, 2)
			mixTest(out)
			if warmupFrames != 0 || out[0] == 0 {
				t.Fatal("cancelled music left pre-roll on ordinary effect")
			}
		})
	}
}
