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
	// transparent rectangle. Lyrics keep only their text and the small controls.
	dwm := syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")
	border, corner := uint32(0xFFFFFFFE), uint32(1) // COLOR_NONE / DONOTROUND
	dwm.Call(hwnd, 34, uintptr(unsafe.Pointer(&border)), 4)
	dwm.Call(hwnd, 33, uintptr(unsafe.Pointer(&corner)), 4)
	// Restoring the same bounds can leave the caption calculated during
	// CreateWindowEx, before MyGo can dispatch its frameless handler. Recalculate
	// that cached non-client area now, even when no resize is needed.
	const frameChanged = 0x0020
	const noMoveSizeZOrderActivate = 0x0002 | 0x0001 | 0x0004 | 0x0010
	user.NewProc("SetWindowPos").Call(hwnd, 0, 0, 0, 0, 0, frameChanged|noMoveSizeZOrderActivate)
}
