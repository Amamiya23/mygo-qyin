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
	p, err := readTaskbarLyricsPlacement(taskbarLyricsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if p.parent == 0 || p.width <= 0 || p.height <= 0 {
		t.Fatalf("empty placement: %+v", p)
	}
	t.Logf("placement: %+v", p)
	right, err := readTaskbarLyricsPlacement(taskbarLyricsOptions{align: taskbarAlignRight})
	if err != nil {
		t.Fatalf("right alignment: %v", err)
	}
	if right.x+right.width <= p.x+p.width && right.x != p.x {
		t.Fatalf("right alignment did not move right: left=%+v right=%+v", p, right)
	}
	t.Logf("right placement: %+v", right)
	roots := taskbarLyricsRoots()
	for _, root := range roots {
		monitor, _, _ := taskbarUser32.NewProc("MonitorFromWindow").Call(root, 2)
		t.Logf("taskbar root 0x%X on monitor 0x%X", root, monitor)
	}
	if len(roots) > 1 {
		monitor, _, _ := taskbarUser32.NewProc("MonitorFromWindow").Call(roots[1], 2)
		secondary, err := readTaskbarLyricsPlacement(taskbarLyricsOptions{display: int64(monitor)})
		if err != nil {
			t.Logf("secondary taskbar has no placement: %v", err)
		} else {
			if secondary.taskbar != roots[1] {
				t.Fatalf("secondary choice landed on taskbar 0x%X, want 0x%X", secondary.taskbar, roots[1])
			}
			t.Logf("secondary placement: %+v", secondary)
		}
	}
}
