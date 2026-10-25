package main

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestStreamLive(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip()
	}
	u, size, err := SongURL(2652820720, "exhigh")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	dl := startDownload(u, size)
	s, err := openMP3Stream(dl, 278961)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("opened in", time.Since(start), s.rate, s.frames, s.start)
	buf := make([]float32, 4096)
	n, err := s.Read(buf)
	t.Log("read", n, err)
	if err := s.SetPosition(int64(s.rate) * 120); err != nil {
		t.Fatal(err, len(dl.data), dl.done, dl.err)
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

func TestQRLoginLive(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip()
	}
	// A device of the test's own, out of the keychain.
	deviceOnce.Do(func() { deviceID = "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123" })
	key, err := QRKey()
	if err != nil {
		t.Fatal(err)
	}
	code, msg, _, _, err := QRCheck(key)
	if err != nil || code != qrWaiting {
		t.Fatalf("check: %d %q %v", code, msg, err)
	}
	t.Logf("key %s: %d %s", key, code, msg)
}

func TestSongURLEapiLive(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip()
	}
	deviceOnce.Do(func() { deviceID = "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123" })
	var res map[string]any
	err := eapiPost("/api/song/enhance/player/url/v1", map[string]any{"ids": "[2652820720]", "level": "exhigh", "encodeType": "mp3"}, &res)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(res)
	t.Log(string(b)[:min(400, len(b))])
}

func TestPrivilegesLive(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip()
	}
	deviceOnce.Do(func() { deviceID = "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123" })
	pl, err := PlaylistDetail(3778678)
	if err != nil {
		t.Fatal(err)
	}
	with := 0
	for _, tr := range pl.Tracks {
		if tr.Privilege != nil {
			with++
		}
	}
	daily, err := DailySongs()
	if err != nil {
		t.Fatal(err)
	}
	dailyWith := 0
	for _, tr := range daily {
		if tr.Privilege != nil {
			dailyWith++
		}
	}
	t.Logf("playlist: %d of %d tracks with privileges; daily: %d of %d", with, len(pl.Tracks), dailyWith, len(daily))
	if with == 0 || dailyWith == 0 {
		t.Fatal("no privileges")
	}
	u, size, err := SongURL(2652820720, "exhigh")
	if err != nil || u == "" || size == 0 {
		t.Fatalf("url %q %d %v", u, size, err)
	}
}
