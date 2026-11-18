//go:build !windows

package app

import "github.com/egoist/mygo"

func placeTaskbarLyricsCover(w *mygo.Window, p taskbarLyricsPlacement, mirror bool) error { return nil }

func startTaskbarLyricsMonitor(options func() taskbarLyricsOptions, deliver func(taskbarLyricsPlacement)) (func(), func()) {
	return func() {}, func() {}
}
func placeTaskbarLyricsWindow(w *mygo.Window, p taskbarLyricsPlacement) error { return nil }
func detachTaskbarLyricsWindow(w *mygo.Window)                                {}
