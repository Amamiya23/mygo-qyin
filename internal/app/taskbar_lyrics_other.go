//go:build !windows

package app

import "github.com/egoist/mygo"

func placeTaskbarLyricsCover(w *mygo.Window, p taskbarLyricsPlacement) error { return nil }

func startTaskbarLyricsMonitor(deliver func(taskbarLyricsPlacement)) func()   { return func() {} }
func placeTaskbarLyricsWindow(w *mygo.Window, p taskbarLyricsPlacement) error { return nil }
func detachTaskbarLyricsWindow(w *mygo.Window)                                {}
