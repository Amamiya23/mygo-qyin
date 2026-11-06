package player

import (
	"math"
	"os"
	"runtime"
	"testing"
	"testing/synctest"
	"unsafe"

	"github.com/mewkiz/flac/frame"
)

func flacPCMBlock(t testing.TB) (*flacStream, *frame.Frame) {
	t.Helper()
	data, err := os.ReadFile("testdata/seek.flac")
	if err != nil {
		t.Fatal(err)
	}
	s, err := openFLACStream(downloaded(data), false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	f, err := s.parse(s.r)
	if err != nil {
		t.Fatal(err)
	}
	return s, f
}

// Decoding another FLAC block reuses PCM storage after the previous slice was consumed.
func TestFLACPCMBufferReuse(t *testing.T) {
	s, f := flacPCMBlock(t)
	allocs := testing.AllocsPerRun(100, func() {
		s.decode(f)
		s.pend = s.pend[len(s.pend):]
	})
	if allocs != 0 {
		t.Fatalf("consumed FLAC blocks allocate %.1f PCM buffers per block", allocs)
	}
}

// BenchmarkFLACPCMBuffer measures the PCM storage needed for successive decoded blocks.
func BenchmarkFLACPCMBuffer(b *testing.B) {
	s, f := flacPCMBlock(b)
	b.ReportAllocs()
	b.SetBytes(int64(f.BlockSize) * 8)
	b.ResetTimer()
	for b.Loop() {
		s.decode(f)
		s.pend = s.pend[len(s.pend):]
	}
}

func spectrumTone(chunks int) []float32 {
	pcm := make([]float32, chunks*fftSize*2)
	for i := 0; i < len(pcm)/2; i++ {
		v := float32(0.5 * math.Sin(2*math.Pi*32*float64(i)/fftSize))
		pcm[2*i], pcm[2*i+1] = v, v
	}
	return pcm
}

func spectrumHasEnergy(bands [Bands]float32) bool {
	for _, v := range bands {
		if v > 0.1 {
			return true
		}
	}
	return false
}

// Buffered audio does no FFT work until a visualizer asks, and only its requested chunk is measured.
func TestSpectrumDemand(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newSpectrum(44100)
		s.feed(0, spectrumTone(8))
		synctest.Wait()
		if s.working || s.requested >= 0 {
			t.Fatal("spectrum started work without a visualizer")
		}
		for _, c := range s.chunks {
			if c.analyzed {
				t.Fatal("spectrum measured audio without demand")
			}
		}
		for range 8 {
			s.at(0)
		}
		synctest.Wait()
		bands, ready := s.at(0)
		if !ready || !spectrumHasEnergy(bands) {
			t.Fatalf("requested tone missing: ready=%v, bands=%v", ready, bands)
		}
		for _, c := range s.chunks {
			if c.at != 0 && c.analyzed {
				t.Fatalf("unrequested chunk %d was measured", c.at)
			}
		}
		s.feed(8*fftSize, spectrumTone(8))
		synctest.Wait()
		for _, c := range s.chunks {
			if c.at != 0 && c.analyzed {
				t.Fatalf("stopping queries still measured chunk %d", c.at)
			}
		}
	})
}

// Reopening a visualizer measures already buffered PCM without waiting for new decoded audio.
func TestSpectrumResumesFromBufferedAudio(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newSpectrum(192000)
		s.feed(0, spectrumTone(300))
		if _, ready := s.at(0); ready {
			t.Fatal("unrequested audio was already analyzed")
		}
		synctest.Wait()
		bands, ready := s.at(0)
		if !ready || !spectrumHasEnergy(bands) {
			t.Fatal("buffered PCM did not restore the visualizer")
		}
		// A 30 Hz consumer moves about six chunks per frame at this rate.
		next := int64(6 * fftSize)
		warm, _ := s.at(next)
		if warm != bands {
			t.Fatal("moving to a new chunk lost the recent spectrum while warming up")
		}
		synctest.Wait()
		if measured, ready := s.at(next); !ready || !spectrumHasEnergy(measured) {
			t.Fatal("visualizer did not advance to its next chunk")
		}
		// Jumping back must not show the later chunk's result while warming up.
		if old, _ := s.at(fftSize); spectrumHasEnergy(old) {
			t.Fatal("seek showed spectrum from a later audio position")
		}
		synctest.Wait()
	})
}

// Recent spectrum storage stays bounded as audio progresses and seeks replace ring slots safely.
func TestSpectrumRingAndSeek(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newSpectrum(44100)
		count := len(s.chunks)
		s.feed(0, spectrumTone(count+4))
		if _, ready := s.at(0); ready {
			t.Fatal("overwritten audio still has a spectrum")
		}
		latest := int64(count+3) * fftSize
		s.at(latest)
		synctest.Wait()
		if bands, ready := s.at(latest); !ready || !spectrumHasEnergy(bands) {
			t.Fatal("recent audio missing after the ring wrapped")
		}
		// A seek to silence replaces the old tone in the same modulo slot.
		s.feed(0, make([]float32, 4*fftSize*2))
		if bands, _ := s.at(0); spectrumHasEnergy(bands) {
			t.Fatal("seek reused spectrum from another ring position")
		}
		synctest.Wait()
		if bands, ready := s.at(0); !ready || spectrumHasEnergy(bands) {
			t.Fatal("seek did not measure the replacement silence")
		}
		if len(s.chunks) != count {
			t.Fatal("spectrum grew as audio played")
		}
	})
}

// Nearby seeks discard the previous display result instead of showing another audio chunk's bands.
func TestSpectrumNearbySeek(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newSpectrum(44100)
		s.feed(0, spectrumTone(2))
		s.feed(2*fftSize, make([]float32, 4*fftSize*2))
		s.at(fftSize)
		synctest.Wait()
		if bands, ready := s.at(fftSize); !ready || !spectrumHasEnergy(bands) {
			t.Fatal("initial tone was not analyzed")
		}
		for _, at := range []int64{2 * fftSize, 3 * fftSize, 0} {
			s.seek()
			if bands, _ := s.at(at); spectrumHasEnergy(bands) {
				t.Fatalf("seek to %d carried over another chunk's spectrum", at)
			}
			synctest.Wait()
		}
	})
}

// Frequency analysis has a fixed memory budget even for sample rates above normal music streams.
func TestSpectrumMemoryBudget(t *testing.T) {
	for _, rate := range []int{44100, 192000, 768000, 1048575} {
		s := newSpectrum(rate)
		bytes := uintptr(cap(s.chunks))*unsafe.Sizeof(spectrumChunk{}) + unsafe.Sizeof(*s) + fftSize*4
		if bytes > 9000000 {
			t.Fatalf("%d Hz spectrum needs %d bytes, above the 9 MB budget", rate, bytes)
		}
		// The oldest audible chunk must remain while MyGo has a second
		// decoded and another quarter second held by the audio output.
		if len(s.chunks)*fftSize < rate*5/4+fftSize {
			t.Fatalf("%d Hz spectrum discards audio before the listener hears it", rate)
		}
	}
}

// BenchmarkSpectrumWithoutDemand measures the ongoing cost while every visualizer is hidden.
func BenchmarkSpectrumWithoutDemand(b *testing.B) {
	s := newSpectrum(44100)
	pcm := spectrumTone(2)
	var at int64
	b.ReportAllocs()
	b.SetBytes(int64(len(pcm) * 4))
	b.ResetTimer()
	for b.Loop() {
		s.feed(at, pcm)
		at += int64(len(pcm) / 2)
	}
}

// BenchmarkSpectrumWithDemand includes a fresh chunk's asynchronous FFT and waits for its result.
func BenchmarkSpectrumWithDemand(b *testing.B) {
	s := newSpectrum(44100)
	pcm := spectrumTone(1)
	var at int64
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		s.feed(at, pcm)
		for {
			if _, ready := s.at(at); ready {
				break
			}
			runtime.Gosched()
		}
		// Real display frames are far enough apart for each worker to
		// finish; include starting a new one rather than reusing a busy one.
		for {
			s.mu.Lock()
			working := s.working
			s.mu.Unlock()
			if !working {
				break
			}
			runtime.Gosched()
		}
		at += fftSize
	}
}
