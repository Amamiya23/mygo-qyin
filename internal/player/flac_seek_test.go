package player

import (
	"os"
	"testing"
)

// A probe inside the target FLAC frame must not fall back to decoding the
// whole prefix. Check the discarded sample count instead of wall-clock time,
// and compare PCM to ensure the faster lookup still lands on the exact sample.
func TestFLACSeekAvoidsDecodingPrefix(t *testing.T) {
	b, err := os.ReadFile("testdata/seek.flac")
	if err != nil {
		t.Fatal(err)
	}
	ref := decodeAll(t, b)
	for _, unknownSize := range []bool{false, true} {
		name := "known frame size"
		if unknownSize {
			name = "unknown frame size"
		}
		t.Run(name, func(t *testing.T) {
			s, err := openFLACStream(downloaded(b), false)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if unknownSize {
				s.info.FrameSizeMax = 0
			}
			buf := make([]float32, 4096)
			for _, percent := range []int64{70, 30, 85, 5, 50, 99, 0} {
				target := s.Len() * percent / 100
				if err := s.SetPosition(target); err != nil {
					t.Fatal(err)
				}
				if s.skip > int64(s.SampleRate()) {
					t.Errorf("seek to %d%% must start within one second of the target, discarding %d samples", percent, s.skip)
				}
				n, err := s.Read(buf)
				if err != nil || n == 0 {
					t.Fatalf("seek to %d%%: samples=%d err=%v", percent, n, err)
				}
				if d := maxDiff(buf[:n], ref[target*2:]); d != 0 {
					t.Errorf("seek to %d%% changed PCM by %.4f", percent, d)
				}
			}
		})
	}
}
