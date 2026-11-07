//go:build linux && !android && (amd64 || arm64)

package audio

import (
	"fmt"
	"testing"
	"time"
	"unsafe"
)

func TestALSAUnsupportedNativeRateAllowsResampling(t *testing.T) {
	original := sndPcmSetParams
	defer func() { sndPcmSetParams = original }()
	for _, rate := range []int{44100, 48000, 88200, 96000, 192000} {
		hardwareRate := uint32(48000)
		sndPcmSetParams = func(_ uintptr, _, _ int32, ch, requested uint32, soft int32, _ uint32) int32 {
			if requested != uint32(rate) || ch != channels {
				t.Fatal("wrong stream format")
			}
			if requested != hardwareRate && soft != 1 {
				return -22
			}
			return 0 // emulate a 48k-only ALSA plugin with rate-conversion support
		}
		if got := configureALSA(1, 250*time.Millisecond, rate); got != 0 {
			t.Fatalf("rate %d: failed to enable ALSA fallback: %d", rate, got)
		}
	}
}

func TestALSARateFallbackOpensAndWritesAudio(t *testing.T) {
	for _, rate := range []int{44100, 48000, 88200, 96000} {
		t.Run(fmt.Sprint(rate), func(t *testing.T) { testALSARateFallback(t, rate) })
	}
}
func testALSARateFallback(t *testing.T, rate int) {
	oldError := sndStrerror
	defer func() { sndStrerror = oldError }()
	sndStrerror = func(int32) string { return "unsupported hardware rate" }
	oldOpen, oldParams, oldWrite, oldClose := sndPcmOpen, sndPcmSetParams, sndPcmWritei, sndPcmClose
	defer func() {
		sndPcmOpen, sndPcmSetParams, sndPcmWritei, sndPcmClose = oldOpen, oldParams, oldWrite, oldClose
	}()
	sndPcmOpen = func(pcm *uintptr, name *byte, _ int32, _ int32) int32 {
		if *name == 'p' {
			*pcm = 456
		} else {
			*pcm = 123
		}
		return 0
	}
	sndPcmSetParams = func(pcm uintptr, _ int32, _ int32, _ uint32, rate uint32, soft int32, _ uint32) int32 {
		if rate != 48000 && (pcm == 123 || soft == 0) {
			return -22
		}
		return 0
	}
	written := uintptr(0)
	sndPcmWritei = func(pcm uintptr, _ unsafe.Pointer, n uintptr) int {
		expectedPCM := uintptr(456)
		if rate == 48000 {
			expectedPCM = 123
		}
		if pcm != expectedPCM {
			t.Fatal("unsupported default PCM did not fall back to plug:default")
		}
		written += n
		return int(n)
	}
	closed := false
	sndPcmClose = func(uintptr) int32 { closed = true; return 0 }
	d, err := openALSALoaded(250*time.Millisecond, rate)
	if err != nil {
		t.Fatalf("48k-only plugin did not accept source stream with conversion: %v", err)
	}
	if err = d.write(make([]float32, rate/100*channels)); err != nil {
		t.Fatal(err)
	}
	d.close()
	if written != uintptr(rate/100) || !closed {
		t.Fatal("ALSA fallback did not write/close a real stream")
	}
}
