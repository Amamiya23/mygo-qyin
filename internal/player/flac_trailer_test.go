package player

import (
	"io"
	"os"
	"testing"
)

func TestFLACStopsBeforeTrailingTags(t *testing.T) {
	b, err := os.ReadFile("testdata/seek.flac")
	if err != nil {
		t.Fatal(err)
	}
	b = append(b, append([]byte("TAG"), make([]byte, 125)...)...)
	s, err := openStream(downloaded(b), 5000)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	buf := make([]float32, 4096)
	var samples int64
	for {
		n, err := s.Read(buf)
		samples += int64(n)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if samples != s.Len()*2 {
		t.Fatalf("decoded %d samples, want %d", samples, s.Len()*2)
	}
}
