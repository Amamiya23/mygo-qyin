//go:build !windows

package app

import (
	"fmt"
	"github.com/egoist/mygo"
)

func taskbarLyricsNativeSnapshot(a *app) map[string]any            { return map[string]any{"supported": false} }
func captureTaskbarLyricsScreen(w *mygo.Window, path string) error { return nil }
func captureTaskbarLyricsBar(hwnd uintptr, path string) error      { return nil }

func clickTaskbarLyricsCover(d *taskbarLyrics) error     { return fmt.Errorf("Windows-only") }
func escapeTaskbarLyricsControls(d *taskbarLyrics) error { return fmt.Errorf("Windows-only") }
func raiseTaskbarLyricsHost(d *taskbarLyrics) error      { return fmt.Errorf("Windows-only") }
