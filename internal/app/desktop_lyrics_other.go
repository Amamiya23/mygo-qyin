//go:build !windows && !linux

package app

import "github.com/egoist/mygo"

func styleDesktopLyricsWindow(*mygo.Window) {}

func setDesktopLyricsIgnoreMouse(w *mygo.Window, ignore bool) { w.SetIgnoreMouseEvents(ignore) }
