package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/audio"

	"lilt/internal/player"
	"lilt/internal/qqmusic"
)

func TestSessionWriterKeepsLatestSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	var w sessionWriter
	for i := range 100 {
		w.submit(path, player.Session{At: i, Position: int64(i)})
	}
	w.wait()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved player.Session
	if err := json.Unmarshal(b, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.At != 99 || saved.Position != 99 {
		t.Fatalf("an older write replaced the latest session: %+v", saved)
	}
	if _, err := os.Stat(path + ".part"); !os.IsNotExist(err) {
		t.Fatalf("temporary session file remains: %v", err)
	}
}

func TestSessionWriterRetriesFailedSave(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")
	path := filepath.Join(dir, "session.json")
	var w sessionWriter
	s := player.Session{At: 1, Position: 47000}
	<-w.submit(path, s)
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	<-w.submit(path, s)
	if _, err := os.ReadFile(path); err != nil {
		t.Fatalf("failed write was remembered as saved: %v", err)
	}
}

// The session is kept a while after playback changes and every while as
// it plays, and paused nothing more is due. It was kept by a ticker that
// woke the app, and redrew its window, every half minute even at rest.
func TestSessionRestsWhilePaused(t *testing.T) {
	var due []func()
	defer func(after func(time.Duration, func())) { sessionAfter = after }(sessionAfter)
	sessionAfter = func(_ time.Duration, fn func()) { due = append(due, fn) }

	a := newTestApp()
	a.pl.Changed = a.keepSession
	a.pl.Cue([]qqmusic.Track{{ID: 7, Name: "宝船", Duration: 200000}})
	out := audio.NewPlayer(&silence{})
	out.SetVolume(0)
	out.Play()
	a.pl.Attach(out)
	defer out.Close()

	a.pl.TogglePlay()
	a.pl.TogglePlay()
	if len(due) != 1 {
		t.Fatalf("%d saves due after pausing and playing", len(due))
	}
	due[0]()
	if len(due) != 2 {
		t.Fatalf("playing, %d saves due after one, want another", len(due)-1)
	}

	a.pl.TogglePlay()
	due[1]()
	if len(due) != 2 {
		t.Errorf("paused, %d more saves due", len(due)-2)
	}
}
