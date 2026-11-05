package app

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"syscall"
	"unsafe"

	"github.com/egoist/mygo"
)

func taskbarLyricsNativeSnapshot(a *app) map[string]any {
	out := map[string]any{"enabled": a.settings.TaskbarLyrics, "mainVisible": a.win.IsVisible(), "windowExists": false}
	d := a.taskbarLyrics
	if d == nil {
		out["coverWindowExists"], out["controlsWindowExists"] = false, false
		return out
	}
	out["controlsOpen"] = d.controlsOpen
	out["coverWindowExists"] = d.coverWin != nil && !d.coverWin.IsDestroyed()
	out["controlsWindowExists"] = d.controls != nil && !d.controls.IsDestroyed()
	user := syscall.NewLazyDLL("user32.dll")
	if d.coverWin != nil && !d.coverWin.IsDestroyed() {
		h := d.coverWin.NativeHandle()
		var r struct{ Left, Top, Right, Bottom int32 }
		user.NewProc("GetWindowRect").Call(h, uintptr(unsafe.Pointer(&r)))
		exIndex := int32(-20)
		ex, _, _ := user.NewProc("GetWindowLongPtrW").Call(h, uintptr(exIndex))
		point := uintptr(uint32((r.Left+r.Right)/2)) | uintptr(uint32((r.Top+r.Bottom)/2))<<32
		hit, _, _ := user.NewProc("WindowFromPoint").Call(point)
		childHit, _, _ := user.NewProc("IsChild").Call(h, hit)
		parent, _, _ := user.NewProc("GetAncestor").Call(h, 1)
		previous, _, _ := user.NewProc("GetWindow").Call(h, 3)
		out["coverHit"], out["coverHandle"], out["coverParent"], out["coverPrevious"] = fmt.Sprintf("0x%X", hit), fmt.Sprintf("0x%X", h), fmt.Sprintf("0x%X", parent), fmt.Sprintf("0x%X", previous)
		out["coverBoundsPx"], out["coverVisible"], out["coverHitIsCover"] = r, d.coverWin.IsVisible(), hit == h || childHit != 0
		out["coverClickThrough"], out["coverNoActivate"] = ex&0x20 != 0, ex&0x08000000 != 0
	}
	if d.controls != nil && !d.controls.IsDestroyed() {
		h := d.controls.NativeHandle()
		var r struct{ Left, Top, Right, Bottom int32 }
		user.NewProc("GetWindowRect").Call(h, uintptr(unsafe.Pointer(&r)))
		fg, _, _ := user.NewProc("GetForegroundWindow").Call()
		out["controlsBoundsPx"], out["controlsVisible"], out["controlsForeground"] = r, d.controls.IsVisible(), fg == h
	}
	p := d.placement
	out["placement"] = map[string]any{"parent": fmt.Sprintf("0x%X", p.parent), "x": p.x, "y": p.y, "width": p.width, "height": p.height, "dpi": p.dpi, "dark": p.dark}
	out["error"] = d.lastError
	if d.win == nil || d.win.IsDestroyed() {
		return out
	}
	hwnd := d.win.NativeHandle()
	styleIndex, exIndex := int32(-16), int32(-20)
	style, _, _ := user.NewProc("GetWindowLongPtrW").Call(hwnd, uintptr(styleIndex))
	ex, _, _ := user.NewProc("GetWindowLongPtrW").Call(hwnd, uintptr(exIndex))
	parent, _, _ := user.NewProc("GetAncestor").Call(hwnd, 1)
	foreground, _, _ := user.NewProc("GetForegroundWindow").Call()
	dpi, _, _ := user.NewProc("GetDpiForWindow").Call(hwnd)
	awareness := func(handle uintptr) uintptr {
		context, _, _ := user.NewProc("GetWindowDpiAwarenessContext").Call(handle)
		value, _, _ := user.NewProc("GetAwarenessFromDpiAwarenessContext").Call(context)
		return value
	}
	var r, client struct{ Left, Top, Right, Bottom int32 }
	var origin struct{ X, Y int32 }
	user.NewProc("GetWindowRect").Call(hwnd, uintptr(unsafe.Pointer(&r)))
	user.NewProc("GetClientRect").Call(hwnd, uintptr(unsafe.Pointer(&client)))
	user.NewProc("ClientToScreen").Call(hwnd, uintptr(unsafe.Pointer(&origin)))
	out["windowExists"], out["visible"] = true, d.win.IsVisible()
	out["handle"], out["parent"] = fmt.Sprintf("0x%X", hwnd), fmt.Sprintf("0x%X", parent)
	out["parentMatches"], out["child"] = parent == p.parent, style&0x40000000 != 0
	out["noActivate"], out["clickThrough"], out["topmost"] = ex&0x08000000 != 0, ex&0x20 != 0, ex&8 != 0
	out["foregroundIsLyrics"] = foreground == hwnd
	out["boundsPx"], out["dpi"] = r, dpi
	out["mainDpiAwareness"], out["lyricsDpiAwareness"] = awareness(a.win.NativeHandle()), awareness(hwnd)
	out["clientFillsWindow"] = origin.X == r.Left && origin.Y == r.Top && client.Right == r.Right-r.Left && client.Bottom == r.Bottom-r.Top
	point := uintptr(uint32((r.Left+r.Right)/2)) | uintptr(uint32((r.Top+r.Bottom)/2))<<32
	hit, _, _ := user.NewProc("WindowFromPoint").Call(point)
	childHit, _, _ := user.NewProc("IsChild").Call(hwnd, hit)
	out["lyricsHitIsLyrics"] = hit == hwnd || childHit != 0
	return out
}

func raiseTaskbarLyricsHost(d *taskbarLyrics) error {
	if d.win == nil || d.win.IsDestroyed() {
		return fmt.Errorf("lyric window missing")
	}
	ok, _, err := taskbarUser32.NewProc("SetWindowPos").Call(d.win.NativeHandle(), 0, 0, 0, 0, 0, 0x1|0x2|0x10)
	if ok == 0 {
		return fmt.Errorf("raise test lyric child: %w", err)
	}
	return nil
}

// Send input only to the isolated test's own cover, without moving the user's pointer.
func clickTaskbarLyricsCover(d *taskbarLyrics) error {
	if d.coverWin == nil || d.coverWin.IsDestroyed() {
		return fmt.Errorf("interactive cover window missing")
	}
	user := syscall.NewLazyDLL("user32.dll")
	h := taskbarFindChild(d.coverWin.NativeHandle(), "MyGoSurface")
	if h == 0 {
		return fmt.Errorf("cover input surface missing")
	}
	var r struct{ Left, Top, Right, Bottom int32 }
	user.NewProc("GetClientRect").Call(h, uintptr(unsafe.Pointer(&r)))
	point := uintptr(uint16(r.Right/2)) | uintptr(uint16(r.Bottom/2))<<16
	post := user.NewProc("PostMessageW")
	post.Call(h, 0x200, 0, point)
	post.Call(h, 0x201, 1, point)
	ok, _, err := post.Call(h, 0x202, 0, point)
	if ok == 0 {
		return fmt.Errorf("click cover: %w", err)
	}
	return nil
}

func escapeTaskbarLyricsControls(d *taskbarLyrics) error {
	if d.controls == nil || d.controls.IsDestroyed() {
		return fmt.Errorf("controls window missing before Escape")
	}
	h := taskbarFindChild(d.controls.NativeHandle(), "MyGoSurface")
	if h == 0 {
		return fmt.Errorf("controls input surface missing")
	}
	post := syscall.NewLazyDLL("user32.dll").NewProc("PostMessageW")
	post.Call(h, 0x100, 0x1b, 0)
	post.Call(h, 0x101, 0x1b, 0)
	return nil
}

// Capture the composited taskbar region: an offscreen render alone cannot prove embedding.
func captureTaskbarLyricsScreen(w *mygo.Window, path string) error {
	return captureTaskbarLyricsBar(w.NativeHandle(), path)
}

func captureTaskbarLyricsBar(hwnd uintptr, path string) error {
	user, gdi := syscall.NewLazyDLL("user32.dll"), syscall.NewLazyDLL("gdi32.dll")
	var r struct{ Left, Top, Right, Bottom int32 }
	user.NewProc("GetWindowRect").Call(hwnd, uintptr(unsafe.Pointer(&r)))
	width, height := int(r.Right-r.Left), int(r.Bottom-r.Top)
	if width <= 0 || height <= 0 {
		return fmt.Errorf("empty capture bounds")
	}
	screen, _, _ := user.NewProc("GetDC").Call(0)
	if screen == 0 {
		return fmt.Errorf("GetDC failed")
	}
	defer user.NewProc("ReleaseDC").Call(0, screen)
	dc, _, _ := gdi.NewProc("CreateCompatibleDC").Call(screen)
	if dc == 0 {
		return fmt.Errorf("CreateCompatibleDC failed")
	}
	defer gdi.NewProc("DeleteDC").Call(dc)
	bmp, _, _ := gdi.NewProc("CreateCompatibleBitmap").Call(screen, uintptr(width), uintptr(height))
	if bmp == 0 {
		return fmt.Errorf("CreateCompatibleBitmap failed")
	}
	defer gdi.NewProc("DeleteObject").Call(bmp)
	old, _, _ := gdi.NewProc("SelectObject").Call(dc, bmp)
	ok, _, _ := gdi.NewProc("BitBlt").Call(dc, 0, 0, uintptr(width), uintptr(height), screen, uintptr(r.Left), uintptr(r.Top), 0x00CC0020|0x40000000)
	gdi.NewProc("SelectObject").Call(dc, old)
	if ok == 0 {
		return fmt.Errorf("BitBlt failed")
	}
	var info struct {
		Size                   uint32
		Width, Height          int32
		Planes, BitCount       uint16
		Compression, ImageSize uint32
		XPels, YPels           int32
		Used, Important        uint32
	}
	info.Size, info.Width, info.Height, info.Planes, info.BitCount = 40, int32(width), -int32(height), 1, 32
	data := make([]byte, width*height*4)
	lines, _, _ := gdi.NewProc("GetDIBits").Call(dc, bmp, 0, uintptr(height), uintptr(unsafe.Pointer(&data[0])), uintptr(unsafe.Pointer(&info)), 0)
	if lines != uintptr(height) {
		return fmt.Errorf("GetDIBits returned %d/%d rows", lines, height)
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for i := 0; i < len(data); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = data[i+2], data[i+1], data[i], 255
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
