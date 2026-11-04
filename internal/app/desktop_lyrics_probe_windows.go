package app

import (
	"fmt"
	"syscall"
	"unsafe"
)

func desktopLyricsNativeSnapshot(a *app) map[string]any {
	out := map[string]any{"enabled": a.settings.DesktopLyrics, "locked": a.settings.DesktopLyricsLocked, "mainVisible": a.win.IsVisible()}
	out["doubleLine"] = a.settings.DesktopLyricsDoubleLine
	d := a.desktopLyrics
	if d == nil || d.win == nil {
		out["windowExists"] = false
		return out
	}
	hwnd := d.win.NativeHandle()
	user := syscall.NewLazyDLL("user32.dll")
	get := user.NewProc("GetWindowLongPtrW")
	styleIndex, ownerIndex := int32(-20), int32(-8)
	ex, _, _ := get.Call(hwnd, uintptr(styleIndex))
	owner, _, _ := get.Call(hwnd, uintptr(ownerIndex))
	fg, _, _ := user.NewProc("GetForegroundWindow").Call()
	var r struct{ Left, Top, Right, Bottom int32 }
	user.NewProc("GetWindowRect").Call(hwnd, uintptr(unsafe.Pointer(&r)))
	// Restored bounds can hide a stale native caption until the first resize.
	// Capture the actual client geometry, not just the requested style flags.
	var client struct{ Left, Top, Right, Bottom int32 }
	var origin struct{ X, Y int32 }
	user.NewProc("GetClientRect").Call(hwnd, uintptr(unsafe.Pointer(&client)))
	user.NewProc("ClientToScreen").Call(hwnd, uintptr(unsafe.Pointer(&origin)))
	out["clientBoundsPx"], out["clientOriginPx"] = client, origin
	out["clientFillsWindow"] = origin.X == r.Left && origin.Y == r.Top && client.Right == r.Right-r.Left && client.Bottom == r.Bottom-r.Top
	x, y := (r.Left+r.Right)/2, (r.Top+r.Bottom)/2
	point := uintptr(uint32(x)) | uintptr(uint32(y))<<32
	hit, _, _ := user.NewProc("WindowFromPoint").Call(point)
	root, _, _ := user.NewProc("GetAncestor").Call(hit, 2)
	out["windowExists"], out["visible"] = true, d.win.IsVisible()
	out["handle"], out["owner"] = fmt.Sprintf("0x%X", hwnd), fmt.Sprintf("0x%X", owner)
	out["ownerIsMain"] = owner == a.win.NativeHandle()
	out["noActivate"], out["toolWindow"], out["topmost"] = ex&0x08000000 != 0, ex&0x80 != 0, ex&8 != 0
	out["clickThrough"], out["hitIsLyrics"], out["foregroundIsLyrics"] = ex&0x20 != 0, root == hwnd, fg == hwnd
	out["boundsPx"] = r
	return out
}
