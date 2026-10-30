package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// Choosing a directory stages a migration; the originals are removed only after the copied cache is committed.
func TestCacheDirectorySettings(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.CacheDir = t.TempDir()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	a.pl.Cache = newSongCache(a.settings)
	oldCache := a.pl.Cache
	writeCacheFixture(t, filepath.Join(oldCache.Directory(), "1-exhigh.mp3"), "old song")
	writeCacheFixture(t, filepath.Join(oldCache.Directory(), "1-exhigh.json"), `{"level":"exhigh","type":"mp3","size":8}`)

	selected := filepath.Join(t.TempDir(), "音乐缓存")
	writeCacheFixture(t, filepath.Join(selected, "Lilt", "songs", "2-exhigh.mp3"), "new song")
	userSong := filepath.Join(selected, "my-music.mp3")
	writeCacheFixture(t, userSong, "personal music")
	picker := pickCacheDirectory
	defer func() { pickCacheDirectory = picker }()
	pickCacheDirectory = func(opts mygo.OpenDialogOptions) ([]string, error) {
		if !opts.Directory || opts.Multiple || opts.DefaultPath != a.settings.CacheDir {
			t.Fatalf("picker options: %+v", opts)
		}
		return []string{selected}, nil
	}
	tt := ui.NewTester(a.settingsView, 560, 2000)
	tt.Frame()
	if err := tt.Click("更改缓存目录"); err != nil {
		t.Fatal(err)
	}
	if a.settings.PendingCacheDir == nil || *a.settings.PendingCacheDir != selected || a.pl.Cache != oldCache {
		t.Fatal("selection did not stage migration while keeping the current cache")
	}
	b, err := os.ReadFile(a.settings.path)
	if err != nil {
		t.Fatal(err)
	}
	restored := defaultSettings()
	if err := json.Unmarshal(b, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.PendingCacheDir == nil || *restored.PendingCacheDir != selected || restored.CacheDir != a.settings.CacheDir {
		t.Fatalf("restored cache choices: %+v", restored)
	}
	restored.CacheDir = *restored.PendingCacheDir
	target := newSongCache(restored)
	cleanup, err := oldCache.CopyTo(target, nil)
	if err != nil {
		t.Fatal(err)
	}
	if oldCache.Size() != int64(len("old song")) {
		t.Fatal("copying removed the source before committing settings")
	}
	if err := a.commitCacheDirectory(selected, target); err != nil {
		t.Fatal(err)
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	if a.settings.CacheDir != selected || a.settings.PendingCacheDir != nil || a.pl.Cache != target || oldCache.Size() != 0 {
		t.Fatal("completed migration did not select the destination and clean the originals")
	}
	b, err = os.ReadFile(a.settings.path)
	if err != nil {
		t.Fatal(err)
	}
	var saved settings
	if err := json.Unmarshal(b, &saved); err != nil || saved.CacheDir != selected || saved.PendingCacheDir != nil {
		t.Fatalf("committed settings: %+v, %v", saved, err)
	}
	if target.Size() != int64(len("new song")+len("old song")) {
		t.Fatalf("migrated cache size = %d", target.Size())
	}
	target.Clear()
	if b, err := os.ReadFile(userSong); err != nil || string(b) != "personal music" {
		t.Fatalf("user music changed: %q, %v", b, err)
	}
}

// Cancelled, failed, or unwritable selections leave the live cache and persisted path unchanged.
func TestCacheDirectorySelectionFailure(t *testing.T) {
	for _, scenario := range []string{"cancel", "picker error", "blocked directory", "save error"} {
		t.Run(scenario, func(t *testing.T) {
			a := newTestApp()
			a.settings = defaultSettings()
			a.settings.CacheDir = t.TempDir()
			a.settings.path = filepath.Join(t.TempDir(), "settings.json")
			if err := a.settings.save(); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(a.settings.path)
			if err != nil {
				t.Fatal(err)
			}
			a.pl.Cache = newSongCache(a.settings)
			previousCache, previousDir := a.pl.Cache, a.settings.CacheDir
			selected := t.TempDir()
			if scenario == "blocked directory" {
				writeCacheFixture(t, filepath.Join(selected, "Lilt"), "a file blocks the directory")
			}
			if scenario == "save error" {
				blocker := filepath.Join(t.TempDir(), "file")
				writeCacheFixture(t, blocker, "a file blocks settings writes")
				a.settings.path = filepath.Join(blocker, "settings.json")
			}
			picker := pickCacheDirectory
			defer func() { pickCacheDirectory = picker }()
			pickCacheDirectory = func(mygo.OpenDialogOptions) ([]string, error) {
				switch scenario {
				case "cancel":
					return nil, nil
				case "picker error":
					return nil, errors.New("picker unavailable")
				default:
					return []string{selected}, nil
				}
			}
			tt := ui.NewTester(a.settingsView, 560, 2000)
			tt.Frame()
			if err := tt.Click("更改缓存目录"); err != nil {
				t.Fatal(err)
			}
			if a.pl.Cache != previousCache || a.settings.CacheDir != previousDir {
				t.Fatal("unsuccessful selection changed the cache")
			}
			if scenario != "cancel" && a.cacheDirError == "" {
				t.Fatal("the failed selection was not reported")
			}
			if scenario != "save error" {
				after, err := os.ReadFile(a.settings.path)
				if err != nil || string(after) != string(before) {
					t.Fatalf("persisted settings changed: %v", err)
				}
			}
		})
	}
}

// Restoring the default stages migration to the platform cache without interrupting current playback.
func TestRestoreDefaultCacheDirectory(t *testing.T) {
	defaultRoot := t.TempDir()
	mygo.App.SetPath(mygo.PathCache, defaultRoot)
	defer mygo.App.SetPath(mygo.PathCache, "")
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.CacheDir = t.TempDir()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	a.pl.Cache = newSongCache(a.settings)
	tt := ui.NewTester(a.settingsView, 560, 2000)
	tt.Frame()
	if err := tt.Click("恢复默认缓存目录"); err != nil {
		t.Fatal(err)
	}
	if a.settings.PendingCacheDir == nil || *a.settings.PendingCacheDir != "" || a.pl.Cache.Directory() == filepath.Join(defaultRoot, "songs") {
		t.Fatal("restoring the default did not defer migration until restart")
	}
	b, err := os.ReadFile(a.settings.path)
	if err != nil {
		t.Fatal(err)
	}
	var saved settings
	if err := json.Unmarshal(b, &saved); err != nil || saved.PendingCacheDir == nil || *saved.PendingCacheDir != "" || saved.CacheDir != a.settings.CacheDir {
		t.Fatalf("saved custom directory = %q, %v", saved.CacheDir, err)
	}
}

// A measurement finishing after a directory switch must not replace the new directory's usage.
func TestCacheMeasurementAfterDirectoryChange(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.CacheDir = t.TempDir()
	oldCache := newSongCache(a.settings)
	a.settings.CacheDir = t.TempDir()
	a.pl.Cache = newSongCache(a.settings)
	a.cacheMeasured(a.pl.Cache, 15)
	a.cacheMeasured(oldCache, 99)
	if a.cacheUsed != 15 {
		t.Fatalf("usage = %d, want the current cache's 15", a.cacheUsed)
	}
}

// Cancelling a pending directory change leaves playback and saved settings on the original cache.
func TestCancelCacheDirectoryChange(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.CacheDir = t.TempDir()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	a.pl.Cache = newSongCache(a.settings)
	original := a.pl.Cache
	if err := a.setCacheDirectory(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(a.settingsView, 560, 2000)
	tt.Frame()
	if err := tt.Click("取消缓存目录更改"); err != nil {
		t.Fatal(err)
	}
	if a.settings.PendingCacheDir != nil || a.pl.Cache != original {
		t.Fatal("cancellation did not retain the original cache")
	}
	b, err := os.ReadFile(a.settings.path)
	if err != nil {
		t.Fatal(err)
	}
	var restored settings
	if err := json.Unmarshal(b, &restored); err != nil || restored.PendingCacheDir != nil {
		t.Fatalf("restored pending directory: %v, %v", restored.PendingCacheDir, err)
	}
}

// If the completed migration cannot be saved, the original cache and pending choice stay selected.
func TestCacheDirectoryCommitFailure(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.CacheDir = t.TempDir()
	destination := t.TempDir()
	a.settings.PendingCacheDir = &destination
	blocker := filepath.Join(t.TempDir(), "file")
	writeCacheFixture(t, blocker, "settings writes fail here")
	a.settings.path = filepath.Join(blocker, "settings.json")
	a.pl.Cache = newSongCache(a.settings)
	original := a.pl.Cache
	chosen := a.settings
	chosen.CacheDir = destination
	if err := a.commitCacheDirectory(destination, newSongCache(chosen)); err == nil {
		t.Fatal("the failed settings write was ignored")
	}
	if a.pl.Cache != original || a.settings.PendingCacheDir == nil || a.settings.CacheDir == destination {
		t.Fatal("failed commit changed the selected cache")
	}
}

// Clearing from settings during migration used to delete the source while it was being copied.
func TestCacheClearDuringMigration(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.CacheDir = t.TempDir()
	a.pl.Cache = newSongCache(a.settings)
	writeCacheFixture(t, filepath.Join(a.pl.Cache.Directory(), "1-exhigh.mp3"), "source")
	a.cacheUsed, a.cacheMoving = 6, true
	tt := ui.NewTester(a.settingsView, 560, 2000)
	tt.Frame()
	if err := tt.Click("清除"); err != nil {
		t.Fatal(err)
	}
	if a.cacheUsed != 6 || a.pl.Cache.Size() != 6 {
		t.Fatal("clearing during migration modified the source cache")
	}
}

func writeCacheFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
