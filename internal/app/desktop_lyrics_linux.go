package app

import (
	"github.com/ebitengine/purego"
	"github.com/egoist/mygo"
)

func styleDesktopLyricsWindow(w *mygo.Window) {
	lib, err := purego.Dlopen("libgtk-layer-shell.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return
	}
	defer purego.Dlclose(lib)
	var isLayer func(uintptr) bool
	var setLayer func(uintptr, int32)
	purego.RegisterLibFunc(&isLayer, lib, "gtk_layer_is_layer_window")
	purego.RegisterLibFunc(&setLayer, lib, "gtk_layer_set_layer")
	if widget := w.NativeHandle(); isLayer(widget) {
		// Lyrics float above applications, below popup menus. MyGo's default
		// overlay layer is intended for launchers, and covers menus in KWin.
		setLayer(widget, 2) // GTK_LAYER_SHELL_LAYER_TOP
	}
}

// MyGo keeps the pointer passthrough state and reapplies it when GTK realizes
// or maps the window, including when locked lyrics are closed and reopened.
func setDesktopLyricsIgnoreMouse(w *mygo.Window, ignore bool) {
	w.SetIgnoreMouseEvents(ignore)
}
