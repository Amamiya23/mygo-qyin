package player

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"lilt/internal/qqmusic"
)

// CopyTo copies complete songs, covers and lyrics into target before playback
// starts. The caller commits the new cache setting before calling cleanup to
// remove the old files. A same-name target is replaced only after its source
// has been fully copied.
func (c *Cache) CopyTo(target *Cache, progress func(done, total int64)) (cleanup func() error, err error) {
	if c == nil || target == nil {
		return nil, errors.New("cache migration requires a source and target")
	}
	sourceDir, err := filepath.Abs(c.dir)
	if err != nil {
		return nil, err
	}
	targetDir, err := filepath.Abs(target.dir)
	if err != nil {
		return nil, err
	}
	noop := func() error { return nil }
	if sourceDir == targetDir {
		return noop, nil
	}
	if err := os.MkdirAll(targetDir, 0o700); err != nil {
		return nil, err
	}
	sourceInfo, err := os.Stat(sourceDir)
	if errors.Is(err, os.ErrNotExist) {
		return noop, nil
	}
	if err != nil {
		return nil, err
	}
	targetInfo, err := os.Stat(targetDir)
	if err != nil {
		return nil, err
	}
	if os.SameFile(sourceInfo, targetInfo) {
		return noop, nil
	}
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return nil, err
	}
	var songs []cacheMigrationSong
	var total int64
	for _, entry := range entries {
		ext := filepath.Ext(entry.Name())
		if ext != ".mp3" && ext != ".flac" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		audioPath := filepath.Join(sourceDir, entry.Name())
		metaPath := strings.TrimSuffix(audioPath, ext) + ".json"
		meta, err := os.ReadFile(metaPath)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var stream qqmusic.Stream
		if json.Unmarshal(meta, &stream) != nil || stream.Size != info.Size() ||
			stream.Type != strings.TrimPrefix(ext, ".") || !keepable(stream.Level, stream.Type) {
			continue
		}
		metaInfo, err := os.Stat(metaPath)
		if err != nil {
			return nil, err
		}
		song := cacheMigrationSong{
			audio: cacheMigrationFile{audioPath, info.Size(), info.ModTime()},
			meta:  cacheMigrationFile{metaPath, metaInfo.Size(), metaInfo.ModTime()},
		}
		songs = append(songs, song)
		total += song.audio.size + song.meta.size
	}
	covers, coverBytes, err := collectMigrationAssets(filepath.Join(sourceDir, coversSubdir), coversSubdir, coverExt, false)
	if err != nil {
		return nil, err
	}
	lyricsAssets, lyricBytes, err := collectMigrationAssets(filepath.Join(sourceDir, lyricsSubdir), lyricsSubdir, lyricExt, true)
	if err != nil {
		return nil, err
	}
	playlistAssets, playlistBytes, err := collectMigrationAssets(filepath.Join(sourceDir, playlistsSubdir), playlistsSubdir, playlistExt, false)
	if err != nil {
		return nil, err
	}
	purchasedAssets, purchasedBytes, err := collectMigrationAssets(filepath.Join(sourceDir, purchasedSubdir), purchasedSubdir, purchasedExt, false)
	if err != nil {
		return nil, err
	}
	assets := append(covers, lyricsAssets...)
	assets = append(assets, playlistAssets...)
	assets = append(assets, purchasedAssets...)
	total += coverBytes + lyricBytes + playlistBytes + purchasedBytes
	var done int64
	advance := func(n int64) {
		done += n
		if progress != nil {
			progress(done, total)
		}
	}
	advance(0)
	for _, song := range songs {
		for _, file := range []cacheMigrationFile{song.audio, song.meta} {
			if err := file.copyTo(targetDir, advance); err != nil {
				return nil, fmt.Errorf("copy cache %s: %w", filepath.Base(file.path), err)
			}
		}
	}
	for _, asset := range assets {
		destDir := filepath.Join(targetDir, asset.subDir)
		if err := os.MkdirAll(destDir, 0o700); err != nil {
			return nil, fmt.Errorf("create cache dir %s: %w", asset.subDir, err)
		}
		if err := asset.file.copyTo(destDir, advance); err != nil {
			return nil, fmt.Errorf("copy cache %s: %w", filepath.Base(asset.file.path), err)
		}
	}
	return func() error {
		var failures []error
		for _, song := range songs {
			if err := os.Remove(song.audio.path); err != nil && !errors.Is(err, os.ErrNotExist) {
				failures = append(failures, err)
				continue // Keep the matching metadata while the source audio remains.
			}
			if err := os.Remove(song.meta.path); err != nil && !errors.Is(err, os.ErrNotExist) {
				failures = append(failures, err)
			}
		}
		for _, asset := range assets {
			if err := os.Remove(asset.file.path); err != nil && !errors.Is(err, os.ErrNotExist) {
				failures = append(failures, err)
			}
		}
		return errors.Join(failures...)
	}, nil
}

type cacheMigrationSong struct {
	audio, meta cacheMigrationFile
}

type cacheMigrationAsset struct {
	subDir string
	file   cacheMigrationFile
}

type cacheMigrationFile struct {
	path     string
	size     int64
	modified time.Time
}

func collectMigrationAssets(dir, subDir, wantExt string, validateLyrics bool) ([]cacheMigrationAsset, int64, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	var out []cacheMigrationAsset
	var total int64
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != wantExt {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, 0, err
		}
		if !info.Mode().IsRegular() || info.Size() == 0 {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if validateLyrics {
			b, err := os.ReadFile(path)
			if err != nil {
				return nil, 0, err
			}
			var cached CachedLyrics
			if json.Unmarshal(b, &cached) != nil {
				continue
			}
		}
		out = append(out, cacheMigrationAsset{
			subDir: subDir,
			file:   cacheMigrationFile{path, info.Size(), info.ModTime()},
		})
		total += info.Size()
	}
	return out, total, nil
}

func (f cacheMigrationFile) copyTo(dir string, progress func(int64)) error {
	source, err := os.Open(f.path)
	if err != nil {
		return err
	}
	defer source.Close()
	temp, err := os.CreateTemp(dir, ".lilt-migrate-*")
	if err != nil {
		return err
	}
	defer func() {
		temp.Close()
		os.Remove(temp.Name())
	}()
	n, err := io.Copy(cacheMigrationWriter{temp, progress}, source)
	if err != nil {
		return err
	}
	if n != f.size {
		return fmt.Errorf("copied %d bytes, expected %d", n, f.size)
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Chtimes(temp.Name(), f.modified, f.modified); err != nil {
		return err
	}
	return os.Rename(temp.Name(), filepath.Join(dir, filepath.Base(f.path)))
}

type cacheMigrationWriter struct {
	io.Writer
	progress func(int64)
}

func (w cacheMigrationWriter) Write(b []byte) (int, error) {
	n, err := w.Writer.Write(b)
	w.progress(int64(n))
	return n, err
}
