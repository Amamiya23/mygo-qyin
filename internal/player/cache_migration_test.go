package player

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"lilt/internal/qqmusic"
)

func migrationSong(t *testing.T, dir, name, text string) map[string][]byte {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	ext := filepath.Ext(name)
	meta, err := json.Marshal(qqmusic.Stream{Size: int64(len(text)), Level: "exhigh", Type: strings.TrimPrefix(ext, ".")})
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{name: []byte(text), strings.TrimSuffix(name, ext) + ".json": meta}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return files
}

// Copying keeps complete audio and metadata together, preserves LRU times, and only removes sources when committed.
func TestCacheCopyTo(t *testing.T) {
	root := t.TempDir()
	sourceDir, targetDir := filepath.Join(root, "old", "songs"), filepath.Join(root, "new", "songs")
	files := migrationSong(t, sourceDir, "1-exhigh.mp3", "first recording")
	for name, content := range migrationSong(t, sourceDir, "2-exhigh.flac", "second recording") {
		files[name] = content
	}
	// A completed source replaces an older copy at the destination.
	migrationSong(t, targetDir, "1-exhigh.mp3", "outdated recording")
	ignored := map[string]string{
		filepath.Join(root, "old", "personal.mp3"): "personal music beside the cache",
		filepath.Join(sourceDir, "notes.txt"):      "not cache audio",
		filepath.Join(sourceDir, "unfinished.mp3"): "no metadata",
	}
	for path, content := range ignored {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	modified := time.Date(2020, time.October, 30, 12, 0, 0, 0, time.UTC)
	var total int64
	for name, content := range files {
		path := filepath.Join(sourceDir, name)
		if err := os.Chtimes(path, modified, modified); err != nil {
			t.Fatal(err)
		}
		total += int64(len(content))
	}
	lastDone, reportedTotal := int64(-1), int64(-1)
	cleanup, err := NewCache(sourceDir, 1<<30).CopyTo(NewCache(targetDir, 1<<30), func(done, all int64) {
		if done < lastDone || done > all {
			t.Errorf("progress %d/%d after %d", done, all, lastDone)
		}
		lastDone, reportedTotal = done, all
	})
	if err != nil {
		t.Fatal(err)
	}
	if lastDone != total || reportedTotal != total {
		t.Fatalf("final progress %d/%d, want %d/%d", lastDone, reportedTotal, total, total)
	}
	for name, content := range files {
		for _, dir := range []string{sourceDir, targetDir} {
			path := filepath.Join(dir, name)
			if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, content) {
				t.Fatalf("%s: %q, %v", path, got, err)
			}
			info, err := os.Stat(path)
			if err != nil || !info.ModTime().Equal(modified) {
				t.Fatalf("mtime %s: %v, %v", path, info, err)
			}
		}
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	for name := range files {
		if _, err := os.Stat(filepath.Join(sourceDir, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("source %s remains: %v", name, err)
		}
	}
	for path, content := range ignored {
		if got, err := os.ReadFile(path); err != nil || string(got) != content {
			t.Fatalf("unrelated file %s changed: %q, %v", path, got, err)
		}
	}
	if err := cleanup(); err != nil {
		t.Fatalf("second cleanup: %v", err)
	}
}

// A failed copy leaves every source intact, including songs already copied before the failure.
func TestCacheCopyToFailureKeepsSources(t *testing.T) {
	root := t.TempDir()
	sourceDir, targetDir := filepath.Join(root, "old"), filepath.Join(root, "new")
	files := migrationSong(t, sourceDir, "1-exhigh.mp3", "first recording")
	for name, content := range migrationSong(t, sourceDir, "2-exhigh.flac", "second recording") {
		files[name] = content
	}
	if err := os.MkdirAll(filepath.Join(targetDir, "2-exhigh.flac"), 0o700); err != nil {
		t.Fatal(err)
	}
	cleanup, err := NewCache(sourceDir, 1<<30).CopyTo(NewCache(targetDir, 1<<30), nil)
	if err == nil || cleanup != nil {
		t.Fatalf("cleanup %v, error %v", cleanup != nil, err)
	}
	for name, content := range files {
		if got, err := os.ReadFile(filepath.Join(sourceDir, name)); err != nil || !bytes.Equal(got, content) {
			t.Fatalf("source %s changed: %q, %v", name, got, err)
		}
	}
	entries, err := os.ReadDir(targetDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".lilt-migrate-") {
			t.Fatalf("temporary copy remains: %s", entry.Name())
		}
	}
}

// A short source rejects the copy and removes its temporary file without changing the source.
func TestCacheMigrationShortSource(t *testing.T) {
	root := t.TempDir()
	source, target := filepath.Join(root, "source.mp3"), filepath.Join(root, "target")
	if err := os.WriteFile(source, []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	file := cacheMigrationFile{path: source, size: 100, modified: time.Now()}
	if err := file.copyTo(target, func(int64) {}); err == nil {
		t.Fatal("accepted a short source")
	}
	entries, err := os.ReadDir(target)
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed copy left files: %v, %v", entries, err)
	}
	if got, err := os.ReadFile(source); err != nil || string(got) != "short" {
		t.Fatalf("source changed: %q, %v", got, err)
	}
}

// Copying to the same directory or an alias must never delete the cache during cleanup.
func TestCacheCopyToSameDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "SameCache")
	files := migrationSong(t, dir, "1-exhigh.mp3", "keep this song")
	aliases := []string{dir, filepath.Join(dir, ".")}
	if runtime.GOOS == "windows" {
		aliases = append(aliases, filepath.Join(filepath.Dir(dir), "SAMECACHE"))
	} else {
		alias := filepath.Join(filepath.Dir(dir), "alias")
		if err := os.Symlink(dir, alias); err != nil {
			t.Fatal(err)
		}
		aliases = append(aliases, alias)
	}
	for _, target := range aliases {
		cleanup, err := NewCache(dir, 1<<30).CopyTo(NewCache(target, 1<<30), nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := cleanup(); err != nil {
			t.Fatal(err)
		}
		for name, content := range files {
			if got, err := os.ReadFile(filepath.Join(dir, name)); err != nil || !bytes.Equal(got, content) {
				t.Fatalf("same-directory copy changed %s: %q, %v", name, got, err)
			}
		}
	}
}

// An empty original cache can switch directories without requiring files to migrate.
func TestCacheCopyToMissingSource(t *testing.T) {
	root := t.TempDir()
	cleanup, err := NewCache(filepath.Join(root, "missing"), 1<<30).CopyTo(NewCache(filepath.Join(root, "new"), 1<<30), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
}

func TestCacheCopyToMigratesCoversAndLyrics(t *testing.T) {
	root := t.TempDir()
	sourceDir, targetDir := filepath.Join(root, "old"), filepath.Join(root, "new")
	source := NewCache(sourceDir, 1<<20)
	target := NewCache(targetDir, 1<<20)
	coverData := []byte("cover-image-bytes")
	if !source.KeepCover("https://example.com/a.jpg", coverData) {
		t.Fatal("failed to seed cover")
	}
	if !source.KeepLyrics(99, "[00:01.00]lyric", "[00:01.00]歌词", "") {
		t.Fatal("failed to seed lyrics")
	}
	if !source.KeepPlaylist(&qqmusic.Playlist{ID: 55, Name: "迁移歌单", Tracks: []qqmusic.Track{{ID: 1, Name: "曲1"}}}) {
		t.Fatal("failed to seed playlist")
	}
	if !source.KeepUserPlaylists(7, []qqmusic.Playlist{{ID: 55, Name: "迁移歌单"}}, nil) {
		t.Fatal("failed to seed user playlists")
	}
	if !source.KeepPurchasedSongs(7, []qqmusic.Track{{ID: 66, Name: "迁移单曲"}}, false) {
		t.Fatal("failed to seed purchased songs")
	}
	if !source.KeepPurchasedAlbums(7, []qqmusic.AlbumProfile{{Album: qqmusic.Album{ID: 77, Name: "迁移专辑"}}}, true) {
		t.Fatal("failed to seed purchased albums")
	}
	cleanup, err := source.CopyTo(target, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := target.LookupCover("https://example.com/a.jpg"); !ok || !bytes.Equal(got, coverData) {
		t.Fatalf("migrated cover mismatch: ok=%v", ok)
	}
	if got, ok := target.LookupLyrics(99); !ok || got.LRC != "[00:01.00]lyric" || got.Translation != "[00:01.00]歌词" {
		t.Fatalf("migrated lyrics mismatch: ok=%v got=%+v", ok, got)
	}
	if got, _, ok := target.LookupPlaylist(55); !ok || got.Name != "迁移歌单" || len(got.Tracks) != 1 {
		t.Fatalf("migrated playlist mismatch: ok=%v got=%+v", ok, got)
	}
	if made, _, ok := target.LookupUserPlaylists(7); !ok || len(made) != 1 || made[0].ID != 55 {
		t.Fatalf("migrated user playlists mismatch: ok=%v made=%+v", ok, made)
	}
	if songs, _, _, ok := target.LookupPurchasedSongs(7); !ok || len(songs) != 1 || songs[0].ID != 66 {
		t.Fatalf("migrated purchased songs mismatch: ok=%v songs=%+v", ok, songs)
	}
	if albums, more, _, ok := target.LookupPurchasedAlbums(7); !ok || !more || len(albums) != 1 || albums[0].ID != 77 {
		t.Fatalf("migrated purchased albums mismatch: ok=%v more=%v albums=%+v", ok, more, albums)
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	if source.Size() != 0 {
		t.Fatalf("expected source cache size 0 after cleanup, got %d", source.Size())
	}
}
