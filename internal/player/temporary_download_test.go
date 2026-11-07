package player

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/synctest"

	"lilt/internal/qqmusic"
)

type downloadRoundTrip func(*http.Request) (*http.Response, error)

func (f downloadRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func downloadBody(t *testing.T, body func(*http.Request) io.ReadCloser) {
	t.Helper()
	previous := streamHTTP
	streamHTTP = &http.Client{Transport: downloadRoundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: body(r), Header: make(http.Header)}, nil
	})}
	t.Cleanup(func() { streamHTTP = previous })
}

// Uncached audio used to retain the whole song in memory; temporary files keep seeks and shared decoding intact until the last reader closes.
func TestUncachedStreamUsesTemporaryFile(t *testing.T) {
	data, err := os.ReadFile("testdata/seek.flac")
	if err != nil {
		t.Fatal(err)
	}
	ref := decodeAll(t, data)
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		cache *Cache
	}{
		{"no cache", nil},
		{"disabled", NewCache(t.TempDir(), 0)},
		{"too large", NewCache(t.TempDir(), int64(len(data))*2)},
		{"cache unavailable", NewCache(blocked, 1<<30)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				downloadBody(t, func(*http.Request) io.ReadCloser { return io.NopCloser(bytes.NewReader(data)) })
				dl := tc.cache.download(1, &qqmusic.Stream{URL: "https://audio.test/song", Size: int64(len(data)), Type: "flac", Level: "lossless"})
				defer dl.Close()
				synctest.Wait()
				if dl.file == nil || dl.part == "" || cap(dl.data) != 0 {
					t.Fatal("uncached audio was not backed by a temporary file")
				}
				file, path := dl.file, dl.part
				playing, err := openStream(dl, 5000)
				if err != nil {
					t.Fatal(err)
				}
				defer playing.Close()
				shared, err := openShared(dl, 5000)
				if err != nil {
					t.Fatal(err)
				}
				defer shared.Close()
				buf := make([]float32, 4096)
				checkSeek := func(s stream, at int64) {
					t.Helper()
					if err := s.SetPosition(at); err != nil {
						t.Fatal(err)
					}
					n, err := s.Read(buf)
					if err != nil || n == 0 || maxDiff(buf[:n], ref[at*2:]) != 0 {
						t.Fatalf("seek to %d did not return the original audio: %d samples, %v", at, n, err)
					}
				}
				checkSeek(playing, 123457)
				playing.Close()
				if _, err := file.Stat(); err != nil {
					t.Fatalf("closing playback removed the shared reader's file: %v", err)
				}
				checkSeek(shared, 50000)
				shared.Close()
				if _, err := file.Stat(); err == nil {
					t.Fatalf("last reader left its temporary file open: %v", err)
				}
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("last reader left a temporary file: %v", err)
				}
				if kept, _, ok := tc.cache.lookup(1, "lossless"); ok {
					kept.Close()
					t.Fatal("temporary audio became a persistent cache entry")
				}
			})
		})
	}
}

type canceledDownloadReader struct{ ctx context.Context }

func (r canceledDownloadReader) Read([]byte) (int, error) {
	<-r.ctx.Done()
	return 0, r.ctx.Err()
}

// Canceling an unfinished temporary download preserves bytes for a shared reader and removes its file after that reader leaves.
func TestTemporaryDownloadCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		data := []byte("already downloaded audio")
		downloadBody(t, func(r *http.Request) io.ReadCloser {
			return io.NopCloser(io.MultiReader(bytes.NewReader(data), canceledDownloadReader{r.Context()}))
		})
		var cache *Cache
		dl := cache.download(1, &qqmusic.Stream{URL: "https://audio.test/song", Size: 1024})
		defer dl.Close()
		synctest.Wait()
		if dl.file == nil || !dl.share() {
			t.Fatal("temporary download could not be shared")
		}
		file, path := dl.file, dl.part
		dl.Close()
		synctest.Wait()
		if got := dl.peek(0, len(data)); !bytes.Equal(got, data) {
			t.Fatalf("canceling playback lost downloaded audio: %q", got)
		}
		if _, err := file.Stat(); err != nil {
			t.Fatalf("canceling playback removed the shared reader's file: %v", err)
		}
		dl.release()
		if _, err := file.Stat(); err == nil {
			t.Fatalf("canceled download left its temporary file open: %v", err)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("canceled download left a temporary file: %v", err)
		}
	})
}

// Process exit removes an unclosed temporary audio file without relying on player shutdown or Go cleanup callbacks.
func TestTemporaryDownloadExitCleanup(t *testing.T) {
	if os.Getenv("LILT_TEMP_AUDIO_CHILD") == "1" {
		file, err := temporaryDownloadFile()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		fmt.Fprintln(os.Stdout, file.Name())
		runtime.KeepAlive(file)
		os.Exit(0)
	}
	child := exec.Command(os.Args[0], "-test.run=^TestTemporaryDownloadExitCleanup$")
	child.Env = append(os.Environ(), "LILT_TEMP_AUDIO_CHILD=1")
	out, err := child.CombinedOutput()
	if err != nil {
		t.Fatalf("temporary audio child failed: %v\n%s", err, out)
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		t.Fatal("temporary audio child did not report its file")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("process exit left temporary audio at %q: %v", path, err)
	}
}

// A failed temporary-file creation still downloads into memory so an unavailable temp directory cannot stop playback.
func TestTemporaryDownloadFallsBackToMemory(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"TMP", "TEMP", "TMPDIR"} {
		t.Setenv(name, blocked)
	}
	synctest.Test(t, func(t *testing.T) {
		data := []byte("audio without temporary storage")
		downloadBody(t, func(*http.Request) io.ReadCloser { return io.NopCloser(bytes.NewReader(data)) })
		var cache *Cache
		dl := cache.download(1, &qqmusic.Stream{URL: "https://audio.test/song", Size: int64(len(data))})
		defer dl.Close()
		synctest.Wait()
		if dl.file != nil || !bytes.Equal(dl.data, data) || dl.err != nil {
			t.Fatalf("failed temporary file did not use memory: file=%v data=%q err=%v", dl.file, dl.data, dl.err)
		}
	})
}

// A write failure preserves the bytes already on disk and continues in memory without leaving a temporary file behind.
func TestTemporaryDownloadWriteFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audio.part")
	prefix, tail := []byte("downloaded "), []byte("audio")
	if err := os.WriteFile(path, prefix, 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	dl := &download{file: f, part: path, n: int64(len(prefix)), size: 1024, cancel: func() {}, users: 1}
	dl.cond.L = &dl.mu
	defer dl.Close()
	dl.write(tail)
	if dl.file != nil || !bytes.Equal(dl.peek(0, len(prefix)+len(tail)), append(prefix, tail...)) {
		t.Fatal("write failure lost audio instead of continuing in memory")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("write failure left a temporary file: %v", err)
	}
}
