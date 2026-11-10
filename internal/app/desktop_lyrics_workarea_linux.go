package app

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/egoist/mygo"
)

// GTK's Wayland work area is the full output, including Plasma panels. Read
// panel geometry through Plasma's scripting API; the script only reads values.
// GIO is already a GTK dependency, so no qdbus executable is required.
const desktopLyricsPanelsScript = `print(JSON.stringify(panels().map(function(p) { var g = screenGeometry(p.screen); return {location:p.location,height:p.height,screen:{x:g.x,y:g.y,width:g.width,height:g.height}}; })))`

type desktopLyricsPanel struct {
	Location string
	Height   int
	Screen   mygo.Rectangle
}

var desktopLyricsDBus struct {
	once        sync.Once
	err         error
	bus         func(int32, uintptr, *uintptr) uintptr
	call        func(uintptr, *byte, *byte, *byte, *byte, uintptr, uintptr, int32, int32, uintptr, *uintptr) uintptr
	newString   func(*byte) uintptr
	newTuple    func(unsafe.Pointer, uintptr) uintptr
	child       func(uintptr, uintptr) uintptr
	getString   func(uintptr, *uintptr) *byte
	unref       func(uintptr)
	unrefObject func(uintptr)
	freeError   func(uintptr)
}

func desktopLyricsCString(s string) *byte { return &append([]byte(s), 0)[0] }

func queryDesktopLyricsPanels() ([]desktopLyricsPanel, error) {
	d := &desktopLyricsDBus
	d.once.Do(func() {
		lib, err := purego.Dlopen("libgio-2.0.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			d.err = err
			return
		}
		for _, binding := range []struct {
			name string
			fn   any
		}{
			{"g_bus_get_sync", &d.bus}, {"g_dbus_connection_call_sync", &d.call},
			{"g_variant_new_string", &d.newString}, {"g_variant_new_tuple", &d.newTuple},
			{"g_variant_get_child_value", &d.child}, {"g_variant_get_string", &d.getString},
			{"g_variant_unref", &d.unref}, {"g_object_unref", &d.unrefObject}, {"g_error_free", &d.freeError},
		} {
			if _, err := purego.Dlsym(lib, binding.name); err != nil {
				d.err = err
				return
			}
			purego.RegisterLibFunc(binding.fn, lib, binding.name)
		}
	})
	if d.err != nil {
		return nil, d.err
	}
	var failure uintptr
	conn := d.bus(2, 0, &failure) // G_BUS_TYPE_SESSION
	if conn == 0 {
		if failure != 0 {
			d.freeError(failure)
		}
		return nil, fmt.Errorf("session bus unavailable")
	}
	defer d.unrefObject(conn)
	arg := d.newString(desktopLyricsCString(desktopLyricsPanelsScript))
	params := d.newTuple(unsafe.Pointer(&arg), 1)
	reply := d.call(conn, desktopLyricsCString("org.kde.plasmashell"), desktopLyricsCString("/PlasmaShell"),
		desktopLyricsCString("org.kde.PlasmaShell"), desktopLyricsCString("evaluateScript"), params, 0, 1, 1000, 0, &failure)
	if reply == 0 {
		if failure != 0 {
			d.freeError(failure)
		}
		return nil, fmt.Errorf("Plasma panel geometry unavailable")
	}
	defer d.unref(reply)
	value := d.child(reply, 0)
	defer d.unref(value)
	var length uintptr
	bytes := d.getString(value, &length)
	var panels []desktopLyricsPanel
	err := json.Unmarshal(unsafe.Slice(bytes, length), &panels)
	return panels, err
}

func desktopLyricsPanelWorkArea(work mygo.Rectangle, panels []desktopLyricsPanel) mygo.Rectangle {
	left, top, right, bottom := work.X, work.Y, work.X+work.Width, work.Y+work.Height
	for _, p := range panels {
		s := p.Screen
		if p.Height <= 0 || s.Width <= 0 || s.Height <= 0 ||
			s.X >= work.X+work.Width || s.X+s.Width <= work.X || s.Y >= work.Y+work.Height || s.Y+s.Height <= work.Y {
			continue
		}
		switch p.Location {
		case "bottom":
			bottom = min(bottom, s.Y+s.Height-p.Height)
		case "top":
			top = max(top, s.Y+p.Height)
		case "left":
			left = max(left, s.X+p.Height)
		case "right":
			right = min(right, s.X+s.Width-p.Height)
		}
	}
	if right <= left || bottom <= top {
		return work
	}
	return mygo.Rectangle{X: left, Y: top, Width: right - left, Height: bottom - top}
}

func watchDesktopLyricsWorkArea(d *desktopLyrics) {
	if !strings.Contains(strings.ToUpper(os.Getenv("XDG_CURRENT_DESKTOP")), "KDE") || os.Getenv("WAYLAND_DISPLAY") == "" {
		return
	}
	w := d.win
	offMove := w.OnMove(d.fitWorkArea)
	stop := make(chan struct{})
	d.stopWorkArea = func() { close(stop); offMove() }
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		var previous []desktopLyricsPanel
		initialized := false
		for {
			panels, err := queryDesktopLyricsPanels()
			if err == nil && (!initialized || !slices.Equal(previous, panels)) {
				previous, initialized = panels, true
				w.Update(func() {
					if d.win != w {
						return
					}
					d.workAreaOverride = func(work mygo.Rectangle) mygo.Rectangle { return desktopLyricsPanelWorkArea(work, panels) }
					d.fitWorkArea()
				})
			}
			select {
			case <-stop:
				return
			case <-ticker.C:
			}
		}
	}()
}
