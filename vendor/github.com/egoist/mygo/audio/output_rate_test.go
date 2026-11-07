package audio

import (
	"fmt"
	"slices"
	"testing"
	"time"
)

// All tests drive frames explicitly; no device, decoder worker or clock sleeps.
func resetOutputTest(t *testing.T) {
	t.Helper()
	mu.Lock()
	testMode = true
	voices = nil
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
	return NewSound(rate, 2, samples).NewPlayer()
}

func TestNativeRatePreroll(t *testing.T) {
	for _, rate := range []int{44100, 48000, 88200, 96000, 192000} {
		t.Run(fmt.Sprint(rate), func(t *testing.T) {
			resetOutputTest(t)
			p := testSong(rate)
			p.Play()
			if outputRate != rate {
				t.Fatalf("rate = %d", outputRate)
			}
			src := p.v.src.(*memSource)
			buf := make([]float32, (rate/5-3)*2)
			mixTest(buf)
			if src.pos != 0 || p.Position() != 0 || slices.ContainsFunc(buf, func(x float32) bool { return x != 0 }) {
				t.Fatal("pre-roll consumed source, advanced position or produced sound")
			}
			// The boundary lies inside this buffer: exactly three silent frames.
			buf = make([]float32, 11*2)
			mixTest(buf)
			if !slices.Equal(buf[:6], make([]float32, 6)) || !slices.Equal(buf[6:], src.samples[:16]) {
				t.Fatalf("first music frames changed or skipped: %v", buf)
			}
			if got, want := p.Position(), frames(8, rate); got != want {
				t.Fatalf("position %v, want %v", got, want)
			}
			p.Pause()
			p.Play()
			if warmupFrames != 0 {
				t.Fatal("immediate resume added pre-roll")
			}
			p.Stop()
			p.Play()
			if warmupFrames != rate/5 {
				t.Fatal("restart missing pre-roll")
			}
			mixTest(make([]float32, rate/5*2))
			replacement := testSong(rate)
			replacement.Play()
			if warmupFrames != rate/5 {
				t.Fatal("same-rate replacement missing pre-roll")
			}
		})
	}
}

func TestPrerollPositionWithLatency(t *testing.T) {
	for _, rate := range []int{44100, 96000, 192000} {
		for _, lat := range []Latency{LatencyInteractive, LatencyPlayback} {
			t.Run(fmt.Sprintf("%d/%d", rate, lat), func(t *testing.T) {
				resetOutputTest(t)
				p := testSong(rate)
				p.Play()
				held = int64(lat.ahead()) * int64(rate) / int64(time.Second)
				// Mix silence plus exactly the device's look-ahead: no music heard yet.
				mixTest(make([]float32, (rate/5+int(held))*2))
				if p.Position() != 0 {
					t.Fatalf("music advanced during audible pre-roll: %v", p.Position())
				}
				mixTest(make([]float32, rate/100*2))
				if got, want := p.Position(), frames(int64(rate/100), rate); got != want {
					t.Fatalf("position %v, want %v", got, want)
				}
			})
		}
	}
}

type rateTestDevice struct {
	writeFn func([]float32) error
	closed  bool
}

func (d *rateTestDevice) write(b []float32) error { return d.writeFn(b) }
func (d *rateTestDevice) close()                  { d.closed = true }

func TestRateChangeClosesIdleDeviceAndColdStartWarms(t *testing.T) {
	resetOutputTest(t)
	old := testSong(48000)
	old.Play()
	// Prevent Play from launching the real output goroutine in this test.
	runOnce.Do(func() {})
	testMode = false
	next := testSong(96000).SetPrimary(true)
	var devices []*rateTestDevice
	openOutput = func(ahead time.Duration, rate int) (device, int, error) {
		if len(devices) > 0 && !devices[len(devices)-1].closed {
			t.Fatal("opened before closing previous device")
		}
		expected := 48000
		if len(devices) > 0 {
			expected = 96000
		}
		if rate != expected {
			t.Fatalf("opened at %d, want %d", rate, expected)
		}
		writes := 0
		d := &rateTestDevice{}
		d.writeFn = func(b []float32) error {
			writes++
			if slices.ContainsFunc(b, func(x float32) bool { return x != 0 }) {
				t.Fatal("cold device did not begin with silence")
			}
			if len(devices) == 1 {
				// The previous track stops while the device is still open.
				old.Pause()
				next.Play()
			} else {
				if next.Position() != 0 {
					t.Fatal("cold pre-roll advanced position")
				}
				mu.Lock()
				testMode = true // finish this session deterministically
				mu.Unlock()
			}
			return nil
		}
		devices = append(devices, d)
		return d, rate / 100, nil
	}
	playOutput()
	if !devices[0].closed {
		t.Fatal("rate change left old device open")
	}
	if next.v.src.(*memSource).pos != 0 {
		t.Fatal("mixed new track into stale device")
	}
	playOutput()
	if len(devices) != 2 || !devices[1].closed {
		t.Fatal("replacement device session failed")
	}
}

func TestPrimaryRateAndDynamicRamp(t *testing.T) {
	resetOutputTest(t)
	main := testSong(44100).SetPrimary(true)
	main.Play()
	effect := testSong(48000)
	effect.Play()
	if outputRate != 44100 {
		t.Fatal("effect changed music rate")
	}
	next := testSong(192000).SetPrimary(true)
	next.Play()
	if outputRate != 192000 {
		t.Fatal("primary did not take priority")
	}
	mixTest(make([]float32, warmupFrames*2))
	next.SetVolume(.5)
	mixTest(make([]float32, 2))
	if next.v.rampLeft != 192000/200-1 {
		t.Fatalf("ramp frames %d", next.v.rampLeft)
	}
}

// Resuming a player whose device has closed needs pre-roll even though
// Play itself correctly treats it as a mid-song resume.
func TestColdReopenAtSameRate(t *testing.T) {
	resetOutputTest(t)
	p := testSong(96000)
	p.Play()
	mixTest(make([]float32, (96000/5+100)*2))
	p.Pause()
	p.Play()
	if warmupFrames != 0 {
		t.Fatal("warm resume added silence")
	}
	sourceAt := p.v.at()
	runOnce.Do(func() {})
	testMode = false
	d := &rateTestDevice{}
	openOutput = func(ahead time.Duration, rate int) (device, int, error) {
		if rate != 96000 {
			t.Fatalf("reopened at %d", rate)
		}
		return d, 96000/5 + 7, nil
	}
	d.writeFn = func(b []float32) error {
		if slices.ContainsFunc(b[:96000/5*2], func(x float32) bool { return x != 0 }) {
			t.Fatal("missing cold pre-roll")
		}
		if p.v.at() != sourceAt+7 {
			t.Fatal("cold pre-roll moved source cursor")
		}
		if p.Position() != frames(int64(sourceAt), 96000) {
			t.Fatalf("cold pre-roll changed audible position: %v", p.Position())
		}
		mu.Lock()
		testMode = true
		mu.Unlock()
		return nil
	}
	playOutput()
	if !d.closed {
		t.Fatal("device was not closed")
	}
}

// Drive the actual five-second idle-close path by counting backend frames.
// The fake backend consumes frames immediately, so this needs no real sleep.
func TestLongPauseClosesAndResumesAtNativeRate(t *testing.T) {
	for _, rate := range []int{44100, 96000} {
		for _, lat := range []Latency{LatencyInteractive, LatencyPlayback} {
			t.Run(fmt.Sprintf("%d/%d", rate, lat), func(t *testing.T) {
				resetOutputTest(t)
				latency = lat
				p := testSong(rate).SetPrimary(true)
				p.Play()
				runOnce.Do(func() {})
				testMode = false
				session := 0
				writes := 0
				pausedAt := float64(0)
				var previous *rateTestDevice
				openOutput = func(ahead time.Duration, gotRate int) (device, int, error) {
					if gotRate != rate {
						t.Fatalf("open rate %d, want native %d", gotRate, rate)
					}
					if ahead != lat.ahead() {
						t.Fatal("wrong latency")
					}
					if outputState != outputOpening {
						t.Fatal("missing opening state")
					}
					if previous != nil && !previous.closed {
						t.Fatal("reopened before closing")
					}
					writes = 0
					d := &rateTestDevice{}
					d.writeFn = func(buf []float32) error {
						writes++
						if outputState != outputReady || deviceRate != rate {
							t.Fatal("device not ready at native rate")
						}
						if writes <= 20 {
							if slices.ContainsFunc(buf, func(x float32) bool { return x != 0 }) {
								t.Fatal("pre-roll was not silent")
							}
							if session == 1 && (p.v.at() != pausedAt || p.Position() != frames(int64(pausedAt), rate)) {
								t.Fatal("resume pre-roll advanced cursor or position")
							}
						} else if writes == 21 {
							if session == 0 {
								p.Pause()
							} else {
								if p.v.at() != pausedAt+float64(rate/100) {
									t.Fatal("resume skipped or consumed extra frames")
								}
								src := p.v.src.(*memSource)
								want := src.samples[int(pausedAt)*2] * (float32(1) / float32(rampFrames()))
								if buf[0] != want {
									t.Fatalf("resume first sample %v, want %v", buf[0], want)
								}
								mu.Lock()
								testMode = true
								mu.Unlock()
							}
						}
						return nil
					}
					previous = d
					return d, rate / 100, nil
				}
				playOutput()
				if !previous.closed || outputState != outputClosed || deviceRate != 0 || p.v.active || p.Playing() {
					t.Fatal("long pause did not close device and deactivate voice")
				}
				if writes < 500 {
					t.Fatal("idle close occurred before five seconds of frames")
				}
				pausedAt = p.v.at()
				// Even losing transient output/mixer rate state must not lose native metadata.
				outputRate = 48000
				p.v.rate = 48000
				p.Play()
				if outputRate != rate || p.v.rate != float64(rate) || warmupFrames != rate/5 {
					t.Fatal("Play did not recover source rate and arm pre-roll")
				}
				session = 1
				playOutput()
				if !previous.closed {
					t.Fatal("resume session failed to close")
				}
			})
		}
	}
}

func TestShortPauseResumesWithoutPrerollOrReopen(t *testing.T) {
	resetOutputTest(t)
	p := testSong(44100)
	p.Play()
	runOnce.Do(func() {})
	testMode = false
	writes := 0
	opens := 0
	d := &rateTestDevice{}
	openOutput = func(ahead time.Duration, rate int) (device, int, error) {
		opens++
		return d, rate / 100, nil
	}
	d.writeFn = func(buf []float32) error {
		writes++
		switch writes {
		case 21:
			p.Pause()
		case 24:
			if p.v.active {
				t.Fatal("pause ramp did not finish")
			}
			at := p.v.at()
			p.Play()
			if warmupFrames != 0 || p.v.at() != at || outputState != outputReady {
				t.Fatal("short resume added pre-roll or moved cursor")
			}
		case 25:
			if slices.ContainsFunc(buf, func(x float32) bool { return x != 0 }) == false {
				t.Fatal("short resume did not produce music immediately")
			}
			mu.Lock()
			testMode = true
			mu.Unlock()
		}
		return nil
	}
	playOutput()
	if opens != 1 {
		t.Fatalf("short resume opened %d devices", opens)
	}
}

func TestTrackChangesDuringDeviceOpen(t *testing.T) {
	resetOutputTest(t)
	first := testSong(44100)
	first.Play()
	runOnce.Do(func() {})
	testMode = false
	next := testSong(96000).SetPrimary(true)
	stale := &rateTestDevice{writeFn: func([]float32) error { t.Fatal("consumed music before reopening at new rate"); return nil }}
	openOutput = func(ahead time.Duration, rate int) (device, int, error) {
		if rate != 44100 || outputState != outputOpening {
			t.Fatal("unexpected opening state")
		}
		first.Pause()
		next.Play()
		return stale, 441, nil
	}
	playOutput()
	if !stale.closed || next.v.at() != 0 || outputState != outputClosed {
		t.Fatal("stale opening was not discarded safely")
	}
	fresh := &rateTestDevice{}
	openOutput = func(ahead time.Duration, rate int) (device, int, error) {
		if rate != 96000 {
			t.Fatal("replacement did not use new rate")
		}
		return fresh, 960, nil
	}
	fresh.writeFn = func(buf []float32) error {
		if slices.ContainsFunc(buf, func(x float32) bool { return x != 0 }) || next.v.at() != 0 {
			t.Fatal("replacement did not pre-roll")
		}
		mu.Lock()
		testMode = true
		mu.Unlock()
		return nil
	}
	playOutput()
}

type rateTestDecoder struct{ rate int }

func (d rateTestDecoder) SampleRate() int                 { return d.rate }
func (d rateTestDecoder) Channels() int                   { return 2 }
func (d rateTestDecoder) Len() int64                      { return int64(d.rate) }
func (d rateTestDecoder) SetPosition(int64) error         { return nil }
func (d rateTestDecoder) Read(buf []float32) (int, error) { clear(buf); return len(buf), nil }

func TestStreamPlayRestoresDecoderRate(t *testing.T) {
	resetOutputTest(t)
	p := NewPlayer(rateTestDecoder{96000})
	defer p.Close()
	p.v.rate = 48000
	p.Play()
	if outputRate != 96000 || p.v.rate != 96000 {
		t.Fatal("Play did not read stream's decoder rate")
	}
	p.Pause()
	outputRate = 48000
	p.v.rate = 48000
	p.Play()
	if outputRate != 96000 || p.v.rate != 96000 || warmupFrames != 96000/5 {
		t.Fatal("resume did not recover decoder rate")
	}
}
