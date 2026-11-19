package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
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

// Renaming the QQ fork to Lilt must keep its settings, queue, development login and cache in the matching build.
func TestLiltDataMigration(t *testing.T) {
	name := mygo.App.Name()
	t.Cleanup(func() {
		mygo.App.SetName(name)
		mygo.App.SetPath(mygo.PathUserData, "")
		mygo.App.SetPath(mygo.PathCache, "")
	})
	for _, current := range []string{"Lilt", "Lilt Dev"} {
		t.Run(current, func(t *testing.T) {
			mygo.App.SetName(current)
			root := t.TempDir()
			previous := oldName + strings.TrimPrefix(current, "Lilt")
			otherBuild := oldName
			if previous == oldName {
				otherBuild += " Dev"
			}
			for _, entry := range []struct {
				path  mygo.PathName
				base  string
				files map[string]string
			}{
				{mygo.PathUserData, "data", map[string]string{"settings.json": `{"volume":0.37}`, "session.json": "queue", "qqmusic-session": "login", "window-state.json": "window"}},
				{mygo.PathCache, "cache", map[string]string{"song.mp3": "audio"}},
			} {
				base := filepath.Join(root, entry.base)
				for _, source := range []string{previous, "网易云音乐", otherBuild} {
					dir := filepath.Join(base, source)
					if err := os.MkdirAll(dir, 0o700); err != nil {
						t.Fatal(err)
					}
					for file, content := range entry.files {
						if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o600); err != nil {
							t.Fatal(err)
						}
					}
				}
				dir := filepath.Join(base, current)
				if err := os.MkdirAll(dir, 0o700); err != nil {
					t.Fatal(err)
				}
				mygo.App.SetPath(entry.path, dir)
			}
			moveOldData()
			if got := loadSettings().Volume; got != 0.37 {
				t.Fatalf("restored volume = %v", got)
			}
			for _, file := range []string{"session.json", "qqmusic-session", "window-state.json"} {
				if _, err := os.ReadFile(filepath.Join(root, "data", current, file)); err != nil {
					t.Fatalf("missing migrated %s: %v", file, err)
				}
			}
			if b, err := os.ReadFile(filepath.Join(root, "cache", current, "song.mp3")); err != nil || string(b) != "audio" {
				t.Fatalf("migrated cache = %q, %v", b, err)
			}
			for _, other := range []string{"网易云音乐", otherBuild} {
				if _, err := os.Stat(filepath.Join(root, "data", other, "settings.json")); err != nil {
					t.Fatalf("changed unrelated %s data: %v", other, err)
				}
			}
			oldDir := filepath.Join(root, "data", previous)
			if err := os.MkdirAll(oldDir, 0o700); err != nil {
				t.Fatal(err)
			}
			oldSettings := filepath.Join(oldDir, "settings.json")
			if err := os.WriteFile(oldSettings, []byte(`{"volume":0.81}`), 0o600); err != nil {
				t.Fatal(err)
			}
			moveOldData()
			if got := loadSettings().Volume; got != 0.37 {
				t.Fatalf("migration overwrote existing settings: volume = %v", got)
			}
			if _, err := os.Stat(oldSettings); err != nil {
				t.Fatalf("migration removed the old settings: %v", err)
			}
		})
	}
}
