package player

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"syscall"
)

// temporaryDownloadFile is deleted when its handle closes, including at process exit.
func temporaryDownloadFile() (*os.File, error) {
	path := filepath.Join(os.TempDir(), "lilt-audio-"+rand.Text()+".part")
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	const (
		deleteAccess      = 0x00010000
		fileDeleteOnClose = 0x04000000
	)
	handle, err := syscall.CreateFile(name, syscall.GENERIC_READ|syscall.GENERIC_WRITE|deleteAccess,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE, nil,
		syscall.CREATE_NEW, syscall.FILE_ATTRIBUTE_NORMAL|fileDeleteOnClose, 0)
	if err != nil {
		return nil, &os.PathError{Op: "createtemp", Path: path, Err: err}
	}
	return os.NewFile(uintptr(handle), path), nil
}
