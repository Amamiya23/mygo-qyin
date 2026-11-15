package player

import (
	"os"
	"path/filepath"
	"testing"

	"lilt/internal/qqmusic"
)

func TestHiResCacheIsolation(t *testing.T) {
	c := NewCache(t.TempDir(), 1<<20)
	store := func(level string) {
		t.Helper()
		f, err := os.CreateTemp(c.Directory(), "*.part")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(level)); err != nil {
			t.Fatal(err)
		}
		f.Close()
		_, ok := c.keep(8991, &qqmusic.Stream{Level: level, Type: "flac"}, f.Name(), int64(len(level)))
		if !ok {
			t.Fatal("cache rejected level", level)
		}
	}
	store("lossless")
	if dl, _, ok := c.lookup(8991, "hires"); ok {
		dl.Close()
		t.Fatal("Hi-Res request used ordinary lossless cache")
	}
	store("hires")
	if _, err := os.Stat(filepath.Join(c.Directory(), "8991-hires.flac")); err != nil {
		t.Fatal(err)
	}
	dl, info, ok := c.lookup(8991, "hires")
	if !ok {
		t.Fatal("Hi-Res cache miss")
	}
	defer dl.Close()
	if info.Level != "hires" || info.Size != int64(len("hires")) {
		t.Fatal("incorrect cached format metadata")
	}
	// The existing policy permits a higher quality cache for lower requests.
	dl2, info2, ok := c.lookup(8991, "exhigh")
	if !ok {
		t.Fatal("lower quality request missed lossless cache")
	}
	defer dl2.Close()
	if info2.Level != "lossless" {
		t.Fatal("changed cache quality preference")
	}
}
