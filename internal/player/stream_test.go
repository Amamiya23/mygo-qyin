package player

import "testing"

func TestFrameSize(t *testing.T) {
	// MPEG-1 layer III, 320 kbps, 44.1 kHz, no padding.
	if n := frameSize([]byte{0xff, 0xfb, 0xe0, 0x44}); n != 1044 {
		t.Errorf("frame size %d", n)
	}
	if n := frameSize([]byte{0xff, 0x00, 0xe0, 0x44}); n != 0 {
		t.Errorf("no frame: %d", n)
	}
}
