//go:build windows && (amd64 || arm64)

// Command check-window-state verifies hidden native window restoration on mixed-DPI displays.
package main

import (
	"flag"
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type pixelRect struct{ left, top, right, bottom int32 }

func (r pixelRect) String() string {
	return fmt.Sprintf("(%d,%d %dx%d)", r.left, r.top, r.right-r.left, r.bottom-r.top)
}

type monitorInfo struct {
	size         uint32
	bounds, work pixelRect
	flags        uint32
	device       [32]uint16
}

type display struct {
	info monitorInfo
	dpi  int
}

type target struct {
	index         int
	display       display
	key           string
	want          pixelRect
	width, height int // DIPs
}

type memoryStore struct{ states map[string]mygo.WindowState }

func copyStates(states map[string]mygo.WindowState) map[string]mygo.WindowState {
	out := make(map[string]mygo.WindowState, len(states))
	for key, state := range states {
		out[key] = state
	}
	return out
}

func (s *memoryStore) LoadWindowStates() (map[string]mygo.WindowState, error) {
	return copyStates(s.states), nil
}

func (s *memoryStore) SaveWindowStates(states map[string]mygo.WindowState) error {
	s.states = copyStates(states)
	return nil
}

var (
	user32        = syscall.NewLazyDLL("user32.dll")
	getWindowRect = user32.NewProc("GetWindowRect")
	getWindowDPI  = user32.NewProc("GetDpiForWindow")
	getMonitor    = user32.NewProc("GetMonitorInfoW")
	enumMonitors  = user32.NewProc("EnumDisplayMonitors")
	getMonitorDPI = syscall.NewLazyDLL("shcore.dll").NewProc("GetDpiForMonitor")
)

func enumerateDisplays() ([]display, error) {
	var displays []display
	var callbackErr error
	callback := syscall.NewCallback(func(handle, _, _, _ uintptr) uintptr {
		info := monitorInfo{size: uint32(unsafe.Sizeof(monitorInfo{}))}
		if ok, _, err := getMonitor.Call(handle, uintptr(unsafe.Pointer(&info))); ok == 0 {
			callbackErr = fmt.Errorf("GetMonitorInfoW: %v", err)
			return 0
		}
		var x, y uint32
		hr, _, _ := getMonitorDPI.Call(handle, 0, uintptr(unsafe.Pointer(&x)), uintptr(unsafe.Pointer(&y)))
		if hr != 0 || x == 0 || x != y {
			callbackErr = fmt.Errorf("GetDpiForMonitor: HRESULT=%#x dpi=%d,%d", hr, x, y)
			return 0
		}
		displays = append(displays, display{info: info, dpi: int(x)})
		return 1
	})
	ok, _, err := enumMonitors.Call(0, 0, callback, 0)
	if callbackErr != nil {
		return nil, callbackErr
	}
	if ok == 0 {
		return nil, fmt.Errorf("EnumDisplayMonitors: %v", err)
	}
	return displays, nil
}

func toPixels(dip, dpi int) int { return (dip*dpi + 48) / 96 }

func makeTargets(displays []display, store *memoryStore) []target {
	var targets []target
	for i, d := range displays {
		fmt.Printf("display=%d name=%q pixels=%s work=%s dpi=%d primary=%v\n",
			i, syscall.UTF16ToString(d.info.device[:]), d.info.bounds, d.info.work, d.dpi, d.info.flags&1 != 0)
		x, y := int(d.info.work.left)+101, int(d.info.work.top)+101
		availableW, availableH := int(d.info.work.right)-x, int(d.info.work.bottom)-y
		width, height := min(900, availableW*96/d.dpi), min(600, availableH*96/d.dpi)
		if width <= 0 || height <= 0 {
			fmt.Printf("SKIP display=%d: work area cannot fit a window at its origin +101px\n", i)
			continue
		}
		pixelW, pixelH := toPixels(width, d.dpi), toPixels(height, d.dpi)
		want := pixelRect{int32(x), int32(y), int32(x + pixelW), int32(y + pixelH)}
		key := fmt.Sprintf("window-state-regression-%d", i)
		store.states[key] = mygo.WindowState{
			Width: width, Height: height,
			PixelX: x, PixelY: y, PixelWidth: pixelW, PixelHeight: pixelH,
		}
		targets = append(targets, target{index: i, display: d, key: key, want: want, width: width, height: height})
	}
	return targets
}

func hasMixedDPI(targets []target) bool {
	for i := 1; i < len(targets); i++ {
		if targets[i].display.dpi != targets[0].display.dpi {
			return true
		}
	}
	return false
}

// Creating on the primary screen caused WM_DPICHANGED to scale already restored dimensions a second time.
func checkCycle(t target, store *memoryStore, cycle, cycles int) error {
	w := mygo.NewWindow(mygo.WindowOptions{
		Title: "Window state regression", StateKey: t.key,
		Width: t.width, Height: t.height, Hidden: true,
		TitleBarStyle: mygo.TitleBarHidden,
		Content:       ui.View(func(*ui.Context) {}),
	})
	var actual pixelRect
	ok, _, rectErr := getWindowRect.Call(w.NativeHandle(), uintptr(unsafe.Pointer(&actual)))
	actualDPI, _, _ := getWindowDPI.Call(w.NativeHandle())
	visible := w.IsVisible()
	w.Destroy()
	if ok == 0 {
		return fmt.Errorf("cycle=%d display=%d: GetWindowRect: %v", cycle, t.index, rectErr)
	}
	if actual != t.want || int(actualDPI) != t.display.dpi || visible {
		return fmt.Errorf("cycle=%d display=%d: actual=%s dpi=%d visible=%v; want=%s dpi=%d visible=false",
			cycle, t.index, actual, actualDPI, visible, t.want, t.display.dpi)
	}
	saved, exists := store.states[t.key]
	wantPixels := mygo.Rectangle{
		X: int(t.want.left), Y: int(t.want.top),
		Width: int(t.want.right - t.want.left), Height: int(t.want.bottom - t.want.top),
	}
	savedPixels := mygo.Rectangle{X: saved.PixelX, Y: saved.PixelY, Width: saved.PixelWidth, Height: saved.PixelHeight}
	if !exists || saved.Width != t.width || saved.Height != t.height || savedPixels != wantPixels {
		return fmt.Errorf("cycle=%d display=%d: savedDIP=%dx%d savedPixels=%+v exists=%v; wantDIP=%dx%d wantPixels=%+v",
			cycle, t.index, saved.Width, saved.Height, savedPixels, exists, t.width, t.height, wantPixels)
	}
	if cycle == 1 || cycle == cycles {
		fmt.Printf("PASS cycle=%d/%d display=%d pixels=%s dpi=%d savedDIP=%dx%d\n",
			cycle, cycles, t.index, actual, actualDPI, saved.Width, saved.Height)
	}
	return nil
}

func run(cycles int, requireMixed bool, store *memoryStore) (int, error) {
	displays, err := enumerateDisplays()
	if err != nil {
		return 1, err
	}
	targets := makeTargets(displays, store)
	if !hasMixedDPI(targets) {
		fmt.Println("SKIP: this regression requires at least two usable displays with different DPI values; display settings were not changed")
		if requireMixed {
			return 2, nil
		}
		return 0, nil
	}
	for cycle := 1; cycle <= cycles; cycle++ {
		for _, target := range targets {
			if err := checkCycle(target, store, cycle, cycles); err != nil {
				return 1, err
			}
		}
	}
	fmt.Printf("PASS: %d hidden-window restorations (%d cycles across %d displays), with unchanged physical placement and DIP dimensions\n",
		cycles*len(targets), cycles, len(targets))
	return 0, nil
}

func main() {
	cycles := flag.Int("cycles", 20, "number of save/restore cycles per display")
	requireMixed := flag.Bool("require-mixed", true, "exit 2 when mixed-DPI displays are unavailable instead of allowing SKIP")
	flag.Parse()
	if *cycles <= 0 {
		fmt.Fprintln(os.Stderr, "FAIL: -cycles must be positive")
		os.Exit(2)
	}
	store := &memoryStore{states: make(map[string]mygo.WindowState)}
	mygo.App.SetName("Window state regression")
	mygo.App.SetWindowStateStore(store)
	mygo.App.OnWindowAllClosed(func() {})
	exitCode := 0
	mygo.App.WhenReady(func() {
		defer mygo.App.Quit()
		var err error
		exitCode, err = run(*cycles, *requireMixed, store)
		if err != nil {
			fmt.Fprintln(os.Stderr, "FAIL:", err)
		}
	})
	if err := mygo.App.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "FAIL:", err)
		exitCode = 1
	}
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}
