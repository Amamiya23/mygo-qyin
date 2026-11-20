package player

import (
	"io"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/egoist/mygo"

	"lilt/internal/qqmusic"
	"lilt/internal/testutil"
)

// Explicit account opt-in: download and decode a full track into a temporary
// cache, seek, and reopen the real player's stream from that cache. No audio is
// sent to an output device, and signed media URLs are never logged.
func TestLiveHiResPlayback(t *testing.T) {
	if os.Getenv("LIVE") != "1" || os.Getenv("LIVE_HIRES_PLAYBACK") != "1" {
		t.Skip("set LIVE=1 LIVE_HIRES_PLAYBACK=1 LIVE_HIRES_SONG_ID=<numeric ID>; uses the installed Lilt login")
	}
	id, err := strconv.ParseInt(os.Getenv("LIVE_HIRES_SONG_ID"), 10, 64)
	if err != nil || id <= 0 {
		t.Fatal("set LIVE_HIRES_SONG_ID to the numeric song ID printed by TestLiveHiRes")
	}
	secrets, dataDir, err := testutil.InstalledLilt()
	if err != nil {
		t.Fatal("cannot locate the installed Lilt user data directory")
	}
	previousSecrets := mygo.Secrets
	t.Setenv("MYGO_ENV", "production")
	mygo.Secrets = secrets
	mygo.App.SetPath(mygo.PathUserData, dataDir)
	t.Cleanup(func() {
		mygo.App.SetPath(mygo.PathUserData, "")
		mygo.Secrets = previousSecrets
	})
	if saved, err := qqmusic.RestoreSession(); err != nil || !saved {
		t.Fatal("sign in to installed Lilt before testing playback")
	}
	if mid := os.Getenv("LIVE_HIRES_SONG_MID"); mid != "" {
		qqmusic.RememberTracks([]qqmusic.Track{{ID: id, MID: mid, NeedsDetails: true}})
	}
	tracks, err := qqmusic.SongDetails([]int64{id})
	if err != nil || len(tracks) != 1 {
		t.Fatal("cannot load song detail")
	}
	p := &Player{Cache: NewCache(t.TempDir(), 1<<30)}
	info, s, _, err := p.open(tracks[0], "hires", false)
	if err != nil {
		// Errors from downloads may contain the signed URL.
		t.Fatalf("open failed (%T)", err)
	}
	defer s.Close()
	f, ok := s.(*flacStream)
	if !ok || info.Level != "hires" || f.info.BitsPerSample < 24 || f.SampleRate() < 44100 {
		t.Fatal("did not open Hi-Res FLAC")
	}
	t.Logf("opened Hi-Res: %d-bit %d Hz size=%d", f.info.BitsPerSample, f.SampleRate(), info.Size)
	buf := make([]float32, 32768)
	if n, err := s.Read(buf); n == 0 || err != nil {
		t.Fatal("initial audio decode failed")
	}
	if err := s.SetPosition(min(int64(s.SampleRate())*30, s.Len()/2)); err != nil {
		t.Fatal("seek failed")
	}
	if n, err := s.Read(buf); n == 0 || err != nil {
		t.Fatal("decoding after seek failed")
	}
	if err := s.SetPosition(0); err != nil {
		t.Fatal("rewind failed")
	}
	var samples int64
	for {
		n, err := s.Read(buf)
		samples += int64(n)
		if err == io.EOF {
			break
		}
		if err != nil || n == 0 {
			if _, signedURL := err.(*url.Error); signedURL {
				t.Fatalf("download failed after %d samples (%T)", samples, err)
			}
			t.Fatalf("decoding failed after %d samples (%T): %v", samples, err, err)
		}
	}
	if samples != s.Len()*2 {
		t.Fatalf("decoded %d samples, want %d", samples, s.Len()*2)
	}
	f.dl.waitFor(info.Size + 1) // Wait for EOF and completion of the cache write.
	if f.dl.Progress() != 1 {
		t.Fatal("download did not complete")
	}
	cached, metadata, ok := p.Cache.lookup(id, "hires")
	if !ok {
		t.Fatal("complete Hi-Res track was not cached")
	}
	cached.Close()
	if metadata.Level != "hires" || metadata.Size != info.Size {
		t.Fatal("cached metadata does not match stream")
	}
	_, replay, _, err := p.open(tracks[0], "hires", false)
	if err != nil {
		t.Fatal("cached playback failed")
	}
	defer replay.Close()
	if err := replay.SetPosition(replay.Len() / 2); err != nil {
		t.Fatal("cached seek failed")
	}
	if n, err := replay.Read(buf); n == 0 || err != nil {
		t.Fatal("cached audio decode failed")
	}
	t.Logf("decoded full track (%d samples), verified seeking and Hi-Res cache replay", samples)
}

// A real public QQ MP3 decodes incrementally and supports seeking while downloading.
func TestStreamLive(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip()
	}
	featured, err := qqmusic.FeaturedPlaylists(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(featured) == 0 {
		t.Fatal("QQ returned no public playlist")
	}
	playlist, err := qqmusic.PlaylistDetail(featured[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	var st *qqmusic.Stream
	var duration int64
	for _, track := range playlist.Tracks[:min(len(playlist.Tracks), 50)] {
		if track.Duration < 140000 {
			continue
		}
		candidate, e := qqmusic.SongURL(track.ID, "standard")
		if e == nil && candidate.Type == "mp3" {
			st, duration = candidate, track.Duration
			break
		}
	}
	if st == nil {
		t.Fatal("QQ returned no public MP3 long enough to seek")
	}
	start := time.Now()
	dl := startDownload(st.URL, st.Size, nil, nil, nil)
	s, err := openMP3Stream(dl, duration)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("opened in", time.Since(start), s.rate, s.frames, s.start)
	buf := make([]float32, 4096)
	n, err := s.Read(buf)
	t.Log("read", n, err)
	if err := s.SetPosition(int64(s.rate) * 120); err != nil {
		t.Fatal(err, dl.n, dl.done, dl.err)
	}
	total := 0
	for i := 0; i < 200; i++ {
		n, err := s.Read(buf)
		total += n
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Log("after seek read", total, "in", time.Since(start), "progress", dl.Progress())
	b, ok := s.spec.at(int64(s.rate)*120 + 20000)
	t.Log(ok, b)
	s.Close()
}
