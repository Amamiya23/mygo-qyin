package player

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"
	"testing/synctest"
	"time"

	"lilt/internal/qqmusic"
)

func TestFrameSize(t *testing.T) {
	// MPEG-1 layer III, 320 kbps, 44.1 kHz, no padding.
	if n := frameSize([]byte{0xff, 0xfb, 0xe0, 0x44}); n != 1044 {
		t.Errorf("frame size %d", n)
	}
	if n := frameSize([]byte{0xff, 0x00, 0xe0, 0x44}); n != 0 {
		t.Errorf("no frame: %d", n)
	}
}

// decodeAll decodes the whole of a song from its start.
func decodeAll(t *testing.T, b []byte) []float32 {
	t.Helper()
	s, err := openStream(downloaded(b), 600000)
	if err != nil {
		t.Fatal(err)
	}
	var out []float32
	buf := make([]float32, 4096)
	for {
		n, err := s.Read(buf)
		out = append(out, buf[:n]...)
		if err != nil || n == 0 {
			return out
		}
	}
}

// maxDiff returns how far apart a and b are at most, over the length of
// the shorter.
func maxDiff(a, b []float32) float64 {
	d := 0.0
	for i := range min(len(a), len(b)) {
		d = math.Max(d, math.Abs(float64(a[i]-b[i])))
	}
	return d
}

// A seek into the middle of a 128 kbps MP3, whose frames borrow bits from
// two frames back, plays the music there, not a burst of noise.
func TestMP3SeekIsClean(t *testing.T) {
	b, err := os.ReadFile("testdata/seek.mp3")
	if err != nil {
		t.Fatal(err)
	}
	ref := decodeAll(t, b)
	dl := downloaded(b)
	s, err := openMP3Stream(dl, int64(len(ref)/2)*1000/44100)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]float32, 4096)
	for i := 1; i < 40; i++ {
		frame := s.frames * int64(i) / 41
		if err := s.SetPosition(frame); err != nil {
			t.Fatal(err)
		}
		n, _ := s.Read(buf)
		// Where the decoder started, counted in MP3 frames from the first,
		// and what it played from there on.
		var at int64
		if from := frame - preroll; from > 0 {
			off := s.start + int64(float64(dl.size-s.start)*float64(from)/float64(s.frames))
			off = findFrame(dl, off)
			k := int64(0)
			for p := s.start; p < off; p += int64(frameSize(dl.peek(p, 4))) {
				k++
			}
			at = (k*1152 + preroll) * 2
		} else {
			at = frame * 2
		}
		if d := maxDiff(buf[:n], ref[at:]); d > 1e-4 {
			t.Errorf("seek to frame %d: off by up to %.3f", frame, d)
		}
	}
}

func TestMP3DistinguishesDownloadFailureFromEOF(t *testing.T) {
	data, err := os.ReadFile("testdata/seek.mp3")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		tail io.Reader
		want error
	}{
		{"complete", bytes.NewReader(data[len(data)/2:]), io.EOF},
		{"truncated", iotest.ErrReader(io.ErrUnexpectedEOF), io.ErrUnexpectedEOF},
	} {
		t.Run(tc.name, func(t *testing.T) {
			downloadBody(t, func(*http.Request) io.ReadCloser {
				return io.NopCloser(io.MultiReader(bytes.NewReader(data[:len(data)/2]), tc.tail))
			})
			dl := startDownload("https://audio.test/song.mp3", int64(len(data)), nil, nil, nil)
			defer dl.Close()
			s, err := openStream(dl, 5000)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			buf := make([]float32, 4096)
			for err == nil {
				_, err = s.Read(buf)
			}
			if err != tc.want {
				t.Fatalf("MP3 ended with %v, want %v", err, tc.want)
			}
		})
	}
}

// A FLAC plays as its samples are, and a seek lands on the very sample.
func TestFLACSeek(t *testing.T) {
	b, err := os.ReadFile("testdata/seek.flac")
	if err != nil {
		t.Fatal(err)
	}
	ref := decodeAll(t, b)
	if len(ref) != 5*44100*2 {
		t.Fatalf("decoded %d samples", len(ref))
	}
	for _, tc := range []struct {
		name        string
		unknownSize bool
	}{
		{"known frame size", false},
		{"unknown frame size", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opened, err := openStream(downloaded(b), 5000)
			if err != nil {
				t.Fatal(err)
			}
			defer opened.Close()
			s, ok := opened.(*flacStream)
			if !ok {
				t.Fatalf("opened %T instead of a FLAC stream", opened)
			}
			if s.SampleRate() != 44100 || s.Len() != 5*44100 {
				t.Fatalf("rate %d, length %d", s.SampleRate(), s.Len())
			}
			if tc.unknownSize {
				s.info.FrameSizeMax = 0
			}
			buf := make([]float32, 4096)
			targets := []int64{0, 1, 4095, 4096, 50000, 123457, 200000}
			for _, percent := range []int64{70, 30, 85, 5, 50, 99, 0} {
				targets = append(targets, s.Len()*percent/100)
			}
			for _, target := range targets {
				if err := s.SetPosition(target); err != nil {
					t.Fatal(err)
				}
				// A seek should probe near the target instead of decoding all preceding frames.
				if s.skip > int64(s.SampleRate()) {
					t.Errorf("seek to %d discarded %d samples, more than one second", target, s.skip)
				}
				n, err := s.Read(buf)
				if err != nil || n == 0 {
					t.Fatalf("seek to %d: samples=%d err=%v", target, n, err)
				}
				if d := maxDiff(buf[:n], ref[target*2:]); d != 0 {
					t.Errorf("seek to %d: off by up to %.4f", target, d)
				}
			}
		})
	}
}

func TestFLACUnknownSizeSeekBeforeDownloadEnds(t *testing.T) {
	data, err := os.ReadFile("testdata/seek.flac")
	if err != nil {
		t.Fatal(err)
	}
	ref := decodeAll(t, data)
	synctest.Test(t, func(t *testing.T) {
		downloadBody(t, func(r *http.Request) io.ReadCloser {
			return io.NopCloser(io.MultiReader(bytes.NewReader(data[:len(data)*3/4]), canceledDownloadReader{r.Context()}))
		})
		dl := startDownload("https://audio.test/song.flac", 0, nil, nil, nil)
		defer dl.Close()
		s, err := openStream(dl, 5000)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		if _, complete := dl.completedSize(); complete {
			t.Fatal("test download finished before seeking")
		}
		buf := make([]float32, 4096)
		for _, target := range []int64{50000, 100000, 25000} {
			if err := s.SetPosition(target); err != nil {
				t.Fatal(err)
			}
			n, err := s.Read(buf)
			if err != nil || n == 0 || maxDiff(buf[:n], ref[target*2:]) != 0 {
				t.Fatalf("seek to %d did not return original audio: %d samples, %v", target, n, err)
			}
		}
	})
}

func TestFLACTruncatedBeforeDeclaredEnd(t *testing.T) {
	data, err := os.ReadFile("testdata/seek.flac")
	if err != nil {
		t.Fatal(err)
	}
	s, err := openStream(downloaded(data[:len(data)/2]), 5000)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	buf := make([]float32, 4096)
	for {
		_, err := s.Read(buf)
		if err != nil {
			if err == io.EOF {
				t.Fatal("truncated FLAC reported a clean end")
			}
			break
		}
	}
}

// A FLAC whose metadata block is of a length with bit 2 set plays from its
// first frame. The block's length was ORed into the 4 bytes of its header
// rather than added to them, so most of NetEase's lossless songs started 4
// bytes early, failed to decode and were skipped.
func TestFLACMetadataLength(t *testing.T) {
	b, err := os.ReadFile("testdata/seek.flac")
	if err != nil {
		t.Fatal(err)
	}
	ref := decodeAll(t, b)
	start, err := flacDataStart(downloaded(b))
	if err != nil {
		t.Fatal(err)
	}
	// The last block's header is no longer the last: a padding block of 6
	// bytes follows it.
	off := int64(4)
	for b[off]&0x80 == 0 {
		off += 4 + (int64(b[off+1])<<16 | int64(b[off+2])<<8 | int64(b[off+3]))
	}
	padded := append([]byte(nil), b[:start]...)
	padded[off] &^= 0x80
	padded = append(padded, 0x81, 0, 0, 6, 0, 0, 0, 0, 0, 0)
	padded = append(padded, b[start:]...)

	s, err := openStream(downloaded(padded), 5000)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]float32, 4096)
	n, err := s.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if d := maxDiff(buf[:n], ref); d != 0 {
		t.Errorf("off by up to %.4f", d)
	}
}

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

// A song plays as it streams into a file of the cache, rather than the
// app's memory, and the cache keeps it once it has all come in; one cut
// short leaves no file behind.
func TestStreamIntoCache(t *testing.T) {
	b, err := os.ReadFile("testdata/seek.mp3")
	if err != nil {
		t.Fatal(err)
	}
	half := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cut" {
			w.Header().Set("Content-Length", strconv.Itoa(len(b)))
			w.Write(b[:len(b)/2])
			w.(http.Flusher).Flush()
			<-half
			return
		}
		w.Write(b)
	}))
	defer srv.Close()
	defer close(half)
	dir := t.TempDir()
	c := NewCache(dir, 1<<30)
	ref := decodeAll(t, b)

	st := &qqmusic.Stream{URL: srv.URL + "/whole", Size: int64(len(b)), Level: "exhigh", Type: "mp3"}
	s, err := openStream(c.download(1, st), int64(len(ref)/2)*1000/44100)
	if err != nil {
		t.Fatal(err)
	}
	var got []float32
	buf := make([]float32, 4096)
	for {
		n, err := s.Read(buf)
		got = append(got, buf[:n]...)
		if err != nil || n == 0 {
			break
		}
	}
	s.Close()
	if d := maxDiff(got, ref); len(got) != len(ref) || d != 0 {
		t.Errorf("played %d of %d samples, off by up to %.4f", len(got), len(ref), d)
	}
	if dl, _, ok := c.lookup(1, "exhigh"); !ok {
		t.Error("not kept")
	} else {
		dl.Close()
	}

	cut := c.download(2, &qqmusic.Stream{URL: srv.URL + "/cut", Size: int64(len(b)), Level: "exhigh", Type: "mp3"})
	cut.waitFor(int64(len(b) / 2))
	cut.Close()
	for start := time.Now(); time.Since(start) < 2*time.Second; time.Sleep(10 * time.Millisecond) {
		if parts, _ := filepath.Glob(filepath.Join(dir, "*.part")); len(parts) == 0 {
			break
		}
	}
	if parts, _ := filepath.Glob(filepath.Join(dir, "*.part")); len(parts) != 0 {
		t.Errorf("left %v", parts)
	}
	if _, _, ok := c.lookup(2, "exhigh"); ok {
		t.Error("kept a song cut short")
	}
}

// A second stream of the song, as transcribing an episode opens, decodes
// it as the one that plays does. Closing it leaves the download to the
// one that plays, and the one that plays closing leaves it what is in.
func TestSharedStream(t *testing.T) {
	b, err := os.ReadFile("testdata/seek.mp3")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(b) }))
	defer srv.Close()
	c := NewCache(t.TempDir(), 1<<30)
	ref := decodeAll(t, b)
	ms := int64(len(ref)/2) * 1000 / 44100
	readAll := func(s stream) []float32 {
		var got []float32
		buf := make([]float32, 4096)
		for {
			n, err := s.Read(buf)
			got = append(got, buf[:n]...)
			if err != nil || n == 0 {
				return got
			}
		}
	}

	dl := c.download(1, &qqmusic.Stream{URL: srv.URL, Size: int64(len(b)), Level: "exhigh", Type: "mp3"})
	playing, err := openStream(dl, ms)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := openShared(dl, ms)
	if err != nil {
		t.Fatal(err)
	}
	shared.Close()
	if got := readAll(playing); len(got) != len(ref) || maxDiff(got, ref) != 0 {
		t.Errorf("played %d of %d samples after the second stream closed", len(got), len(ref))
	}
	playing.Close()

	kept, _, ok := c.lookup(1, "exhigh")
	if !ok {
		t.Fatal("not kept")
	}
	playing, _ = openStream(kept, ms)
	shared, err = openShared(kept, ms)
	if err != nil {
		t.Fatal(err)
	}
	playing.Close()
	if got := readAll(shared); len(got) != len(ref) || maxDiff(got, ref) != 0 {
		t.Errorf("decoded %d of %d samples after the stream playing closed", len(got), len(ref))
	}
	shared.Close()
	if _, err := openShared(kept, ms); err == nil {
		t.Error("shared a song let go")
	}
}

// A stream whose level would lead out of the cache's folder is played but
// not kept. The level named the file, taken as the API sent it, so a level
// of "/../../x" wrote the song and its JSON beside the cache instead.
func TestCacheKeepsOnlyKnownLevels(t *testing.T) {
	b, err := os.ReadFile("testdata/seek.mp3")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(b) }))
	defer srv.Close()
	root := t.TempDir()
	dir := filepath.Join(root, "cache")
	c := NewCache(dir, 1<<30)

	dl := c.download(1, &qqmusic.Stream{URL: srv.URL, Size: int64(len(b)), Level: "/../../pwned", Type: "mp3"})
	dl.waitFor(int64(len(b)))
	dl.Close()
	for start := time.Now(); time.Since(start) < 2*time.Second; time.Sleep(10 * time.Millisecond) {
		if parts, _ := filepath.Glob(filepath.Join(dir, "*.part")); len(parts) == 0 {
			break
		}
	}
	var outside []string
	filepath.WalkDir(root, func(path string, e os.DirEntry, err error) error {
		if err == nil && !e.IsDir() && filepath.Dir(path) != dir {
			outside = append(outside, path)
		}
		return nil
	})
	if len(outside) != 0 {
		t.Errorf("wrote %v outside the cache", outside)
	}
	if songs := c.songs(); len(songs) != 0 {
		t.Errorf("kept %v", songs)
	}
}

// A song the cache won't keep sets aside no more memory than maxPrealloc,
// whatever size the API says it is, which it set aside in full before.
func TestStreamSizeIsNotTrusted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("short")) }))
	defer srv.Close()
	dl := startDownload(srv.URL, 1<<40, nil, nil, nil)
	defer dl.Close()
	dl.mu.Lock()
	n := cap(dl.data)
	dl.mu.Unlock()
	if n > maxPrealloc {
		t.Errorf("set aside %d bytes", n)
	}
}

func TestCachePruneDoesNotBlockReads(t *testing.T) {
	data, err := os.ReadFile("testdata/seek.mp3")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
	defer srv.Close()
	c := NewCache(t.TempDir(), 1<<30)
	st := &qqmusic.Stream{Size: int64(len(data)), Level: "exhigh", Type: "mp3"}
	part := c.part(st)
	if part == nil {
		t.Fatal("no cache file")
	}
	pruning, resume, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	dl := startDownload(srv.URL, st.Size, part, func(path string, n int64) (string, bool) {
		return c.keep(1, st, path, n)
	}, func(path string) {
		defer close(finished)
		close(pruning)
		<-resume
		c.prune(path)
	})
	defer func() {
		close(resume)
		dl.Close()
		select {
		case <-finished:
		case <-time.After(2 * time.Second):
			t.Error("cache cleanup did not finish")
		}
	}()
	select {
	case <-pruning:
	case <-time.After(2 * time.Second):
		t.Fatal("download did not reach cache cleanup")
	}
	read := make(chan error, 1)
	go func() {
		var b [4]byte
		_, err := (&downloadReader{d: dl}).Read(b[:])
		read <- err
	}()
	select {
	case err := <-read:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cache cleanup holds the download read lock")
	}
}

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
