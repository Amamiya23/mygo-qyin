//go:build windows

package app

import (
	"os"
	"runtime"
	"testing"
)

// A live Explorer query must produce an actual free region, not an empty success.
func TestTaskbarLyricsNativePlacement(t *testing.T) {
	if os.Getenv("TASKBAR_LYRICS_NATIVE") == "" {
		t.Skip("set TASKBAR_LYRICS_NATIVE for live Explorer diagnostics")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	dpiContext, _, _ := taskbarUser32.NewProc("SetThreadDpiAwarenessContext").Call(^uintptr(3))
	if dpiContext != 0 {
		defer taskbarUser32.NewProc("SetThreadDpiAwarenessContext").Call(dpiContext)
	}
	hr, _, _ := taskbarOle32.NewProc("CoInitializeEx").Call(0, 0)
	if int32(hr) < 0 {
		t.Fatalf("CoInitializeEx: 0x%X", uint32(hr))
	}
	defer taskbarOle32.NewProc("CoUninitialize").Call()
	p, err := readTaskbarLyricsPlacement()
	if err != nil {
		t.Fatal(err)
	}
	if p.parent == 0 || p.width <= 0 || p.height <= 0 {
		t.Fatalf("empty placement: %+v", p)
	}
	t.Logf("placement: %+v", p)
}
