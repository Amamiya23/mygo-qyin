package qqmusic

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var moveSessionFile = syscall.NewLazyDLL("kernel32.dll").NewProc("MoveFileExW")

func prepareSessionDirectory(dir string) error {
	info, err := os.Stat(dir)
	if err == nil && info.IsDir() {
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			err = errors.New("qqmusic: session directory is not a directory")
		}
		return err
	}
	parent := filepath.Dir(dir)
	if parent == dir {
		return err
	}
	if err := prepareSessionDirectory(parent); err != nil {
		return err
	}
	// A write-through rename also publishes the directory on its first use.
	tmp, err := os.MkdirTemp(parent, ".session-dir-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	if err := durableSessionRename(tmp, dir); err != nil {
		if info, statErr := os.Stat(dir); statErr != nil || !info.IsDir() {
			return err
		}
	}
	return nil
}

func durableSessionRename(from, to string) error {
	src, err := sessionRenamePath(from)
	if err != nil {
		return err
	}
	dst, err := sessionRenamePath(to)
	if err != nil {
		return err
	}
	const moveFileWriteThrough = 0x8
	if r, _, err := moveSessionFile.Call(uintptr(unsafe.Pointer(&src[0])), uintptr(unsafe.Pointer(&dst[0])), moveFileWriteThrough); r == 0 {
		return &os.LinkError{Op: "MoveFileExW", Old: from, New: to, Err: err}
	}
	return nil
}

func sessionRenamePath(path string) ([]uint16, error) {
	path = filepath.FromSlash(path)
	if strings.HasPrefix(path, `\\?\`) || strings.HasPrefix(path, `\\.\`) || strings.HasPrefix(path, `\??\`) {
		return syscall.UTF16FromString(path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if share, ok := strings.CutPrefix(abs, `\\`); ok {
		return syscall.UTF16FromString(`\\?\UNC\` + share)
	}
	return syscall.UTF16FromString(`\\?\` + abs)
}
