//go:build windows

package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var user32 = syscall.NewLazyDLL("user32.dll")
var ntdll = syscall.NewLazyDLL("ntdll.dll")
var dwmSetWindowAttribute = syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")
var gdi32 = syscall.NewLazyDLL("gdi32.dll")
var findWindow = user32.NewProc("FindWindowW")
var findWindowEx = user32.NewProc("FindWindowExW")
var getWindowRect = user32.NewProc("GetWindowRect")
var getClientRect = user32.NewProc("GetClientRect")
var getAncestor = user32.NewProc("GetAncestor")
var getClassName = user32.NewProc("GetClassNameW")
var getTopWindow = user32.NewProc("GetTopWindow")
var getWindow = user32.NewProc("GetWindow")
var getWindowLong = user32.NewProc("GetWindowLongPtrW")
var setWindowLong = user32.NewProc("SetWindowLongPtrW")
var setParent = user32.NewProc("SetParent")
var setWindowPos = user32.NewProc("SetWindowPos")
var getDPI = user32.NewProc("GetDpiForWindow")
var getDPIContext = user32.NewProc("GetWindowDpiAwarenessContext")
var getDPIAwareness = user32.NewProc("GetAwarenessFromDpiAwarenessContext")
var getForeground = user32.NewProc("GetForegroundWindow")
var getFocus = user32.NewProc("GetFocus")
var sendMessage = user32.NewProc("SendMessageW")
var isWindow = user32.NewProc("IsWindow")
var isVisible = user32.NewProc("IsWindowVisible")
var monitorFromWindow = user32.NewProc("MonitorFromWindow")
var getMonitorInfo = user32.NewProc("GetMonitorInfoW")
var getDC = user32.NewProc("GetDC")
var releaseDC = user32.NewProc("ReleaseDC")
var createCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
var createCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
var selectObject = gdi32.NewProc("SelectObject")
var bitBlt = gdi32.NewProc("BitBlt")
var getDIBits = gdi32.NewProc("GetDIBits")
var deleteObject = gdi32.NewProc("DeleteObject")
var deleteDC = gdi32.NewProc("DeleteDC")

const (
	gwlStyle        = -16
	gwlExStyle      = -20
	gwlpOwner       = -8
	wsChild         = 0x40000000
	wsPopup         = 0x80000000
	wsVisible       = 0x10000000
	wsExNoActivate  = 0x08000000
	wsExToolWindow  = 0x80
	wsExTopmost     = 0x08
	swpNoActivate   = 0x10
	swpFrameChanged = 0x20
	swpShowWindow   = 0x40
)

type nativeRect struct{ Left, Top, Right, Bottom int32 }
type monitorInfo struct {
	Size          uint32
	Monitor, Work nativeRect
	Flags         uint32
}

type windowSnapshot struct {
	Handle       string     `json:"handle"`
	Class        string     `json:"class"`
	Parent       string     `json:"parent"`
	Owner        string     `json:"parentOrOwner"`
	Exists       bool       `json:"exists"`
	Visible      bool       `json:"visible"`
	NoActivate   bool       `json:"noActivate"`
	ClickThrough bool       `json:"clickThrough"`
	Bounds       nativeRect `json:"boundsPx"`
	Style        string     `json:"style"`
	ExStyle      string     `json:"exStyle"`
	DPI          uint32     `json:"dpi"`
	DPIAwareness int32      `json:"dpiAwareness"`
}

func handleString(h uintptr) string { return fmt.Sprintf("0x%X", h) }

type osVersion struct {
	Major, Minor, Build uint32
	Error               string `json:"error,omitempty"`
}

func operatingSystem() osVersion {
	// RtlGetVersion is not affected by executable compatibility manifests or
	// the registry's legacy Windows 10 ProductName on newer Windows versions.
	var info struct {
		Size, Major, Minor, Build, Platform uint32
		CSD                                 [128]uint16
	}
	info.Size = uint32(unsafe.Sizeof(info))
	status, _, _ := ntdll.NewProc("RtlGetVersion").Call(uintptr(unsafe.Pointer(&info)))
	if status != 0 {
		return osVersion{Error: fmt.Sprintf("RtlGetVersion status 0x%X", status)}
	}
	return osVersion{Major: info.Major, Minor: info.Minor, Build: info.Build}
}

func windowClass(hwnd uintptr) string {
	var text [256]uint16
	n, _, _ := getClassName.Call(hwnd, uintptr(unsafe.Pointer(&text[0])), uintptr(len(text)))
	return syscall.UTF16ToString(text[:n])
}

func childSnapshots(parent uintptr) []windowSnapshot {
	var result []windowSnapshot
	child, _, _ := getTopWindow.Call(parent)
	for child != 0 {
		result = append(result, snapshot(child))
		child, _, _ = getWindow.Call(child, 2)
	}
	return result
}
func foreground() uintptr  { h, _, _ := getForeground.Call(); return h }
func threadFocus() uintptr { h, _, _ := getFocus.Call(); return h }

// Query the window procedure's activation decision without synthesizing a mouse
// click. MA_ACTIVATE=1, MA_ACTIVATEANDEAT=2, MA_NOACTIVATE=3, MA_NOACTIVATEANDEAT=4.
// This is diagnostic only: actual pointer input still needs a native UI check.
func mouseActivate(hwnd uintptr) uint32 {
	root, _, _ := getAncestor.Call(hwnd, 2)
	result, _, _ := sendMessage.Call(hwnd, 0x21, root, (0x0201<<16)|1)
	return uint32(result)
}
func getLong(hwnd uintptr, index int) uintptr {
	r, _, _ := getWindowLong.Call(hwnd, uintptr(index))
	return r
}
func putLong(hwnd uintptr, index int, value uintptr) { setWindowLong.Call(hwnd, uintptr(index), value) }
func parentOf(hwnd uintptr) uintptr                  { r, _, _ := getAncestor.Call(hwnd, 1); return r }
func bounds(hwnd uintptr) nativeRect {
	var r nativeRect
	getWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	return r
}
func dpi(hwnd uintptr) uint32 { r, _, _ := getDPI.Call(hwnd); return uint32(r) }

func snapshot(hwnd uintptr) windowSnapshot {
	s := windowSnapshot{Handle: handleString(hwnd), Class: windowClass(hwnd), Parent: handleString(parentOf(hwnd)), Owner: handleString(getLong(hwnd, gwlpOwner)), Bounds: bounds(hwnd), Style: handleString(getLong(hwnd, gwlStyle)), ExStyle: handleString(getLong(hwnd, gwlExStyle)), DPI: dpi(hwnd)}
	r, _, _ := isWindow.Call(hwnd)
	s.Exists = r != 0
	r, _, _ = isVisible.Call(hwnd)
	s.Visible = r != 0
	s.NoActivate = getLong(hwnd, gwlExStyle)&wsExNoActivate != 0
	s.ClickThrough = getLong(hwnd, gwlExStyle)&0x20 != 0
	context, _, _ := getDPIContext.Call(hwnd)
	r, _, _ = getDPIAwareness.Call(context)
	s.DPIAwareness = int32(r)
	return s
}

func primaryBottomTaskbar() (uintptr, error) {
	class, _ := syscall.UTF16PtrFromString("Shell_TrayWnd")
	hwnd, _, _ := findWindow.Call(uintptr(unsafe.Pointer(class)), 0)
	if hwnd == 0 {
		return 0, fmt.Errorf("primary Explorer taskbar not found")
	}
	monitor, _, _ := monitorFromWindow.Call(hwnd, 2)
	var info monitorInfo
	info.Size = uint32(unsafe.Sizeof(info))
	getMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info)))
	r := bounds(hwnd)
	if r.Right-r.Left <= r.Bottom-r.Top || r.Bottom < info.Monitor.Bottom-4 || r.Bottom > info.Monitor.Bottom+4 || r.Top >= info.Monitor.Bottom-4 {
		return 0, fmt.Errorf("T0 only supports a visible primary bottom taskbar")
	}
	return hwnd, nil
}

func taskbarContainer(taskbar uintptr, mode string) (uintptr, error) {
	if mode == "taskbar" {
		return taskbar, nil
	}
	if mode == "auto" {
		version := operatingSystem()
		if version.Error != "" {
			return 0, fmt.Errorf("%s", version.Error)
		}
		if version.Build >= 22000 {
			return taskbar, nil
		}
	}
	// A classic taskbar has a rebar/WorkerW container. Its presence alone is
	// insufficient to choose it on Windows 11: it may be the task-button host.
	for _, name := range []string{"ReBarWindow32", "WorkerW"} {
		class, _ := syscall.UTF16PtrFromString(name)
		h, _, _ := findWindowEx.Call(taskbar, 0, uintptr(unsafe.Pointer(class)), 0)
		if h != 0 {
			return h, nil
		}
	}
	return 0, fmt.Errorf("classic taskbar container not found")
}

type attachment struct {
	hwnd, parent, style, exStyle, owner uintptr
	attached                            bool
}

func (a *attachment) attach(hwnd, taskbar uintptr, mode string) error {
	a.hwnd = hwnd
	a.style = getLong(hwnd, gwlStyle)
	a.exStyle = getLong(hwnd, gwlExStyle)
	a.owner = getLong(hwnd, gwlpOwner)
	var err error
	a.parent, err = taskbarContainer(taskbar, mode)
	if err != nil {
		return err
	}
	// Clear topmost before crossing into Explorer; never raise its own host.
	setWindowPos.Call(hwnd, ^uintptr(1), 0, 0, 0, 0, 0x1|0x2|swpNoActivate)
	setParent.Call(hwnd, a.parent)
	if parentOf(hwnd) != a.parent {
		return fmt.Errorf("Explorer reparent rejected")
	}
	a.attached = true
	putLong(hwnd, gwlStyle, (a.style&^wsPopup)|wsChild|wsVisible)
	putLong(hwnd, gwlExStyle, (a.exStyle&^(wsExToolWindow|wsExTopmost))|wsExNoActivate)
	var client nativeRect
	getClientRect.Call(a.parent, uintptr(unsafe.Pointer(&client)))
	scale := dpi(a.parent)
	if scale == 0 {
		scale = 96
	}
	w := int32(280 * scale / 96)
	h := int32(42 * scale / 96)
	if w > client.Right {
		w = client.Right
	}
	if h > client.Bottom {
		h = client.Bottom
	}
	y := (client.Bottom - h) / 2
	// HWND_TOP is numerically zero: keep the child at the top of its own
	// parent's children, without making Explorer itself topmost or active.
	setWindowPos.Call(hwnd, 0, 12, uintptr(y), uintptr(w), uintptr(h), swpNoActivate|swpFrameChanged|swpShowWindow)
	return nil
}

func (a *attachment) detach() error {
	if !a.attached {
		return nil
	}
	hwnd := a.hwnd
	r, _, _ := isWindow.Call(hwnd)
	if r == 0 {
		a.attached = false
		return nil
	}
	setParent.Call(hwnd, 0)
	putLong(hwnd, gwlStyle, a.style)
	putLong(hwnd, gwlExStyle, a.exStyle)
	putLong(hwnd, gwlpOwner, a.owner)
	a.attached = false
	if parentOf(hwnd) == a.parent || getLong(hwnd, gwlStyle)&wsChild != 0 {
		return fmt.Errorf("lyrics window remained an Explorer child after detach")
	}
	return nil
}

func placeFloating(hwnd uintptr) {
	monitor, _, _ := monitorFromWindow.Call(hwnd, 2)
	var info monitorInfo
	info.Size = uint32(unsafe.Sizeof(info))
	getMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info)))
	scale := dpi(hwnd)
	if scale == 0 {
		scale = 96
	}
	w, h := int32(280*scale/96), int32(42*scale/96)
	x := info.Work.Left + (info.Work.Right-info.Work.Left-w)/2
	y := info.Work.Bottom - h - 60
	setWindowPos.Call(hwnd, ^uintptr(0), uintptr(x), uintptr(y), uintptr(w), uintptr(h), swpNoActivate|swpFrameChanged|swpShowWindow)
}

func clearPanelFrame(hwnd uintptr) {
	// MyGo rounds Panel windows via DWM, which outlines the entire HWND even
	// when its content is transparent. A lyrics overlay needs neither outline
	// nor clipping to the full rectangular window.
	noBorder := uint32(0xFFFFFFFE) // DWMWA_COLOR_NONE
	noCorners := uint32(1)         // DWMWCP_DONOTROUND
	dwmSetWindowAttribute.Call(hwnd, 34, uintptr(unsafe.Pointer(&noBorder)), 4)
	dwmSetWindowAttribute.Call(hwnd, 33, uintptr(unsafe.Pointer(&noCorners)), 4)
}

// captureWindowRect captures only this test window's rectangle from the desktop.
// A screenshot is needed because a valid HWND and successful view build do not
// prove that DWM actually composited the surface after a cross-process detach.
func captureWindowRect(hwnd uintptr, path string) error {
	r := bounds(hwnd)
	w, h := int(r.Right-r.Left), int(r.Bottom-r.Top)
	if w <= 0 || h <= 0 {
		return fmt.Errorf("cannot capture an empty window rectangle")
	}
	screen, _, _ := getDC.Call(0)
	if screen == 0 {
		return fmt.Errorf("GetDC failed")
	}
	defer releaseDC.Call(0, screen)
	dc, _, _ := createCompatibleDC.Call(screen)
	if dc == 0 {
		return fmt.Errorf("CreateCompatibleDC failed")
	}
	defer deleteDC.Call(dc)
	bmp, _, _ := createCompatibleBitmap.Call(screen, uintptr(w), uintptr(h))
	if bmp == 0 {
		return fmt.Errorf("CreateCompatibleBitmap failed")
	}
	defer deleteObject.Call(bmp)
	old, _, _ := selectObject.Call(dc, bmp)
	ok, _, _ := bitBlt.Call(dc, 0, 0, uintptr(w), uintptr(h), screen, uintptr(r.Left), uintptr(r.Top), 0x00CC0020|0x40000000)
	selectObject.Call(dc, old)
	if ok == 0 {
		return fmt.Errorf("BitBlt failed")
	}
	type bitmapInfo struct {
		Size                   uint32
		Width, Height          int32
		Planes, BitCount       uint16
		Compression, ImageSize uint32
		XPels, YPels           int32
		Used, Important        uint32
	}
	info := bitmapInfo{Size: 40, Width: int32(w), Height: -int32(h), Planes: 1, BitCount: 32}
	data := make([]byte, w*h*4)
	lines, _, _ := getDIBits.Call(dc, bmp, 0, uintptr(h), uintptr(unsafe.Pointer(&data[0])), uintptr(unsafe.Pointer(&info)), 0)
	if lines != uintptr(h) {
		return fmt.Errorf("GetDIBits returned %d/%d rows", lines, h)
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(data); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = data[i+2], data[i+1], data[i], 255
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
