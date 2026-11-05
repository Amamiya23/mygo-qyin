package app

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// Restoring old or conflicting settings chooses one supported lyric owner and preserves the chosen mode across saves.
func TestLyricsModeSettingsMigration(t *testing.T) {
	for _, test := range []struct {
		name, encoded, mode string
		desktop, taskbar    bool
	}{
		{"old-desktop", "{\"desktopLyrics\":true}", "desktop", true, false},
		{"old-taskbar", "{\"taskbarLyrics\":true}", "taskbar", false, true},
		{"old-both", "{\"desktopLyrics\":true,\"taskbarLyrics\":true}", "taskbar", false, true},
		{"explicit-desktop", "{\"lyricsMode\":\"desktop\",\"taskbarLyrics\":true}", "desktop", true, false},
		{"explicit-taskbar", "{\"lyricsMode\":\"taskbar\",\"desktopLyrics\":true}", "taskbar", false, true},
		{"disabled-taskbar", "{\"lyricsMode\":\"taskbar\"}", "taskbar", false, false},
		{"invalid-mode", "{\"lyricsMode\":\"invalid\",\"taskbarLyrics\":true}", "desktop", true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var s settings
			if err := json.Unmarshal([]byte(test.encoded), &s); err != nil {
				t.Fatal(err)
			}
			mode, desktop, taskbar := test.mode, test.desktop, test.taskbar
			if !supportsTaskbarLyrics() {
				mode, desktop, taskbar = "desktop", s.DesktopLyrics && supportsDesktopLyrics(), false
			}
			s.normalizeLyrics()
			if s.LyricsMode != mode || s.lyricsMode() != mode || s.DesktopLyrics != desktop || s.TaskbarLyrics != taskbar || s.lyricsEnabled() != (desktop || taskbar) {
				t.Fatalf("migration = mode %q desktop %t taskbar %t enabled %t, want %q/%t/%t/%t", s.LyricsMode, s.DesktopLyrics, s.TaskbarLyrics, s.lyricsEnabled(), mode, desktop, taskbar, desktop || taskbar)
			}
			a := newTestApp()
			a.settings = s
			a.settings.path = filepath.Join(t.TempDir(), "settings.json")
			if err := a.settings.save(); err != nil {
				t.Fatal(err)
			}
			restored := readDesktopLyricsEntrySettings(t, a)
			restored.normalizeLyrics()
			if restored.LyricsMode != mode || restored.DesktopLyrics != desktop || restored.TaskbarLyrics != taskbar {
				t.Fatal("saving and restoring migrated settings changed the selected lyric owner")
			}
		})
	}
}

// Explicit owner controls remain mutually exclusive and an inactive owner's close cannot turn off the selected lyrics.
func TestLyricsModeOwnerControls(t *testing.T) {
	if !supportsTaskbarLyrics() {
		t.Skip("switching both lyric owners requires Windows")
	}
	a := newTestApp()
	a.settings = defaultSettings()
	a.setDesktopLyricsEnabled(true)
	a.setTaskbarLyricsEnabled(true)
	if a.settings.DesktopLyrics || !a.settings.TaskbarLyrics || a.settings.lyricsMode() != "taskbar" {
		t.Fatal("opening taskbar lyrics did not replace desktop lyrics")
	}
	a.setDesktopLyricsEnabled(false)
	if !a.settings.TaskbarLyrics {
		t.Fatal("closing inactive desktop lyrics turned off taskbar lyrics")
	}
	stopped := 0
	a.taskbarLyrics.stop = func() { stopped++ }
	a.setDesktopLyricsEnabled(true)
	if !a.settings.DesktopLyrics || a.settings.TaskbarLyrics || a.settings.lyricsMode() != "desktop" || a.taskbarLyrics != nil || stopped != 1 {
		t.Fatal("opening desktop lyrics did not replace and stop taskbar lyrics")
	}
	a.setTaskbarLyricsEnabled(false)
	if !a.settings.DesktopLyrics {
		t.Fatal("closing inactive taskbar lyrics turned off desktop lyrics")
	}
	a.desktopLyrics.close()
	if a.settings.lyricsEnabled() || a.settings.lyricsMode() != "desktop" {
		t.Fatal("closing the desktop window lost its selected mode")
	}
	a.setLyricsEnabled(true)
	if !a.settings.DesktopLyrics || a.settings.TaskbarLyrics {
		t.Fatal("reopening after a desktop close did not restore desktop lyrics")
	}
}

// Choosing an invalid or unavailable lyric mode leaves the current mode and enabled state untouched.
func TestLyricsModeRejectUnavailable(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	if supportsDesktopLyrics() {
		a.setDesktopLyricsEnabled(true)
	}
	mode, desktop, taskbar := a.settings.lyricsMode(), a.settings.DesktopLyrics, a.settings.TaskbarLyrics
	unavailable := []string{"invalid"}
	if !supportsTaskbarLyrics() {
		unavailable = append(unavailable, "taskbar")
	}
	if !supportsDesktopLyrics() {
		unavailable = append(unavailable, "desktop")
	}
	for _, choice := range unavailable {
		a.setLyricsMode(choice)
		if a.settings.lyricsMode() != mode || a.settings.DesktopLyrics != desktop || a.settings.TaskbarLyrics != taskbar {
			t.Fatalf("unavailable mode %q changed the active lyrics", choice)
		}
	}
}
