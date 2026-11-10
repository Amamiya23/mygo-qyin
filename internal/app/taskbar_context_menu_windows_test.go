package app

import (
	"runtime"
	"syscall"
	"testing"
	"unsafe"
)

// Right-clicking an embedded child used to forward its context menu into Explorer.
func TestTaskbarContextMenuStaysInHost(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// Three hidden native windows reproduce the parent chain without involving
	// the real taskbar, moving the pointer, or depending on a particular shell.
	class, _ := syscall.UTF16PtrFromString("STATIC")
	create := func(parent uintptr) uintptr {
		style := uintptr(0x80000000) // WS_POPUP
		if parent != 0 {
			style = 0x40000000 // WS_CHILD
		}
		h, _, err := taskbarUser32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(class)), 0, style, 0, 0, 40, 40, parent, 0, 0, 0)
		if h == 0 {
			t.Fatalf("create hidden window: %v", err)
		}
		return h
	}
	parent := create(0)
	defer taskbarUser32.NewProc("DestroyWindow").Call(parent)
	host := create(parent)
	defer taskbarUser32.NewProc("DestroyWindow").Call(host)
	content := create(host)
	defer taskbarUser32.NewProc("DestroyWindow").Call(content)
	var menus, releases int
	const probe = 0x8001 // WM_APP + 1
	observer := syscall.NewCallback(func(hwnd, message, wp, lp, id, data uintptr) uintptr {
		switch {
		case hwnd == parent && message == 0x007B:
			menus++
			return 0
		case hwnd == host && message == probe:
			return 42
		case hwnd == content && message == 0x0205: // WM_RBUTTONUP
			releases++
			// As in MyGo, deliver PointerUp before default menu generation.
			r, _, _ := taskbarUser32.NewProc("DefWindowProcW").Call(hwnd, message, wp, lp)
			return r
		case hwnd == content && message == 0x007B:
			r, _, _ := taskbarUser32.NewProc("DefWindowProcW").Call(hwnd, message, wp, lp)
			return r
		}
		r, _, _ := taskbarComctl32.NewProc("DefSubclassProc").Call(hwnd, message, wp, lp)
		return r
	})
	for _, h := range []uintptr{parent, host, content} {
		if ok, _, _ := taskbarComctl32.NewProc("SetWindowSubclass").Call(h, observer, 2, 0); ok == 0 {
			t.Fatal("install observer")
		}
	}
	send := taskbarUser32.NewProc("SendMessageW")
	rightClick := func() { send.Call(content, 0x0205, 0, 0) }
	rightClick()
	if menus != 1 || releases != 1 {
		t.Fatalf("baseline: parent menus=%d, releases=%d; want 1, 1", menus, releases)
	}
	for i := 0; i < 2; i++ {
		if err := guardTaskbarContextMenu(host); err != nil {
			t.Fatal(err)
		}
	}
	rightClick()
	send.Call(content, 0x007B, content, ^uintptr(0)) // keyboard context menu
	if menus != 1 || releases != 2 {
		t.Fatalf("guarded: parent menus=%d, releases=%d; want 1, 2", menus, releases)
	}
	if r, _, _ := send.Call(host, probe, 0, 0); r != 42 {
		t.Fatalf("unrelated message returned %d, want 42", r)
	}
	taskbarComctl32.NewProc("RemoveWindowSubclass").Call(host, taskbarContextMenuProc, 1)
	rightClick()
	if menus != 2 || releases != 3 {
		t.Fatalf("removed: parent menus=%d, releases=%d; want 2, 3", menus, releases)
	}
	if err := guardTaskbarContextMenu(host); err != nil {
		t.Fatal(err)
	}
	rightClick()
	if menus != 2 || releases != 4 {
		t.Fatalf("reinstalled: parent menus=%d, releases=%d; want 2, 4", menus, releases)
	}
}
