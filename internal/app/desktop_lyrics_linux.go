package app

import (
	"sync"

	"github.com/ebitengine/purego"
	"github.com/egoist/mygo"
)

var desktopLyricsGTK struct {
	once       sync.Once
	realize    func(uintptr)
	window     func(uintptr) uintptr
	inputShape func(uintptr, uintptr, int32, int32)
	newRegion  func() uintptr
	freeRegion func(uintptr)
}

func styleDesktopLyricsWindow(*mygo.Window) {}

// MyGo's Linux SetIgnoreMouseEvents is a no-op. An empty GDK input region
// lets pointer events pass through on both X11 and Wayland.
func setDesktopLyricsIgnoreMouse(w *mygo.Window, ignore bool) {
	desktopLyricsGTK.once.Do(func() {
		gtk, err := purego.Dlopen("libgtk-3.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			return
		}
		gdk, err := purego.Dlopen("libgdk-3.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			return
		}
		cairo, err := purego.Dlopen("libcairo.so.2", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			return
		}
		purego.RegisterLibFunc(&desktopLyricsGTK.realize, gtk, "gtk_widget_realize")
		purego.RegisterLibFunc(&desktopLyricsGTK.window, gtk, "gtk_widget_get_window")
		purego.RegisterLibFunc(&desktopLyricsGTK.inputShape, gdk, "gdk_window_input_shape_combine_region")
		purego.RegisterLibFunc(&desktopLyricsGTK.newRegion, cairo, "cairo_region_create")
		purego.RegisterLibFunc(&desktopLyricsGTK.freeRegion, cairo, "cairo_region_destroy")
	})
	if desktopLyricsGTK.realize == nil {
		return
	}
	widget := w.NativeHandle()
	desktopLyricsGTK.realize(widget)
	window := desktopLyricsGTK.window(widget)
	if window == 0 {
		return
	}
	var region uintptr
	if ignore {
		region = desktopLyricsGTK.newRegion()
		defer desktopLyricsGTK.freeRegion(region)
	}
	desktopLyricsGTK.inputShape(window, region, 0, 0)
}
