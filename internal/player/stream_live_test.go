package player

import (
	"os"
	"testing"
	"time"

	"qqmusic-mygo/internal/qqmusic"
)

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
	dl := startDownload(st.URL, st.Size, nil, nil)
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
