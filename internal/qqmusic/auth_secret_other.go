//go:build !windows

package qqmusic

import (
	"errors"
	"os"
	"path/filepath"
)

func prepareSessionDirectory(dir string) error {
	return os.MkdirAll(dir, 0700)
}

func durableSessionRename(from, to string) error {
	if err := os.Rename(from, to); err != nil {
		return err
	}
	// The session directory and the app's user data directory can both be new.
	dir := filepath.Dir(to)
	for range 3 {
		if err := syncSessionDirectory(dir); err != nil {
			os.Remove(to)
			return err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return nil
}

func syncSessionDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(dir.Sync(), dir.Close())
}
