package app

import (
	"github.com/egoist/mygo"
	"syscall"
	"unsafe"
)

// ShowInactive affects showing only. Lyrics controls must also leave the
// foreground app active when clicked; Panel otherwise remains focusable.
func styleDesktopLyricsWindow(w *mygo.Window) {
	user := syscall.NewLazyDLL("user32.dll")
	index := int32(-20) // GWL_EXSTYLE
	hwnd := w.NativeHandle()
	style, _, _ := user.NewProc("GetWindowLongPtrW").Call(hwnd, uintptr(index))
	user.NewProc("SetWindowLongPtrW").Call(hwnd, uintptr(index), style|0x08000000) // WS_EX_NOACTIVATE
	// MyGo rounds every Panel on Windows 11, which also outlines the whole
	// transparent rectangle. A floating lyric has only its text and hover plate.
	dwm := syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")
	border, corner := uint32(0xFFFFFFFE), uint32(1) // COLOR_NONE / DONOTROUND
	dwm.Call(hwnd, 34, uintptr(unsafe.Pointer(&border)), 4)
	dwm.Call(hwnd, 33, uintptr(unsafe.Pointer(&corner)), 4)
}
