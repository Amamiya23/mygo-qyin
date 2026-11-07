//go:build !windows

package player

import "os"

// temporaryDownloadFile keeps an unlinked file alive only while its handle is open.
func temporaryDownloadFile() (*os.File, error) {
	f, err := os.CreateTemp("", "lilt-audio-*.part")
	if err != nil {
		return nil, err
	}
	if err := os.Remove(f.Name()); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}
