package qqmusic

import (
	"os"
	"testing"

	"lilt/internal/lyrics"
)

// Anonymous lyric requests must decode real word times all the way into the player's model.
func TestLiveLyricsWordTimes(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip("set LIVE to contact QQ Music")
	}
	previous := sessionSnapshot()
	t.Cleanup(func() { setSession(previous) })
	setSession(Session{})
	for _, song := range []Track{
		{ID: 97773, MID: "0039MnYb0qxYhV"},
		{ID: 1180947, MID: "002pXw073bnbSj"},
	} {
		t.Run(song.MID, func(t *testing.T) {
			RememberTracks([]Track{song})
			lrc, trans, qrc, err := Lyrics(song.ID)
			if err != nil {
				t.Fatal(err)
			}
			lines := lyrics.Build(lrc, trans, qrc)
			wordCount := 0
			for _, line := range lines {
				if line.Timed {
					wordCount += len(line.Words)
				}
			}
			if qrc == "" || wordCount == 0 {
				t.Fatalf("real word timing unavailable: lrc bytes=%d qrc bytes=%d lines=%d", len(lrc), len(qrc), len(lines))
			}
			if song.ID == 1180947 && trans == "" {
				t.Fatal("the Japanese sample lost its translation")
			}
			t.Logf("decoded %d timed words in %d lines; translation bytes=%d", wordCount, len(lines), len(trans))
		})
	}
}

// Line-only lyrics and instrumental placeholders must not be mistaken for real word timing.
func TestLiveLyricsLineFallback(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip("set LIVE to contact QQ Music")
	}
	previous := sessionSnapshot()
	t.Cleanup(func() { setSession(previous) })
	setSession(Session{})
	for _, mid := range []string{"003XGaNw3g8rjL", "001nnncU0VsH5O"} {
		t.Run(mid, func(t *testing.T) {
			lrc, trans, qrc, err := playLyrics(mid)
			if err != nil || lrc == "" || qrc != "" {
				t.Fatalf("unexpected line-only response: lrc bytes=%d qrc bytes=%d error=%v", len(lrc), len(qrc), err)
			}
			lines := lyrics.Build(lrc, trans, qrc)
			for _, line := range lines {
				if line.Timed {
					t.Fatalf("invented word timing: %+v", line)
				}
			}
			if mid == "001nnncU0VsH5O" && len(lines) != 0 {
				t.Fatalf("instrumental placeholder shown as lyrics: %d lines", len(lines))
			}
			t.Logf("decoded line-only payload, visible lines=%d", len(lines))
		})
	}
}
