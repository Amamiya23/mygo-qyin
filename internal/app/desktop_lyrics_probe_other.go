//go:build !windows

package app

import "github.com/egoist/mygo"

func desktopLyricsNativeSnapshot(a *app) map[string]any {
	out := map[string]any{"enabled": a.settings.DesktopLyrics, "locked": a.settings.DesktopLyricsLocked, "mainVisible": a.win.IsVisible()}
	out["displays"] = mygo.Screen.Displays()
	if d := a.desktopLyrics; d != nil && d.win != nil {
		out["visible"] = d.win.IsVisible()
		out["bounds"] = d.win.Bounds()
		out["workArea"] = d.workArea(d.win.Bounds())
		out["desiredHeight"] = d.desiredHeight
	}
	return out
}
