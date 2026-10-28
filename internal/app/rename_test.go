package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo"
)

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
		})
	}
}
