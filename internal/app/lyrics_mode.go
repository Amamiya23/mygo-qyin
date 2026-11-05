package app

const (
	lyricsDesktop = "desktop"
	lyricsTaskbar = "taskbar"
)

// lyricsMode remembers the chosen location even while lyrics are switched off.
// Before the location preference existed, an enabled taskbar selected that mode.
func (s settings) lyricsMode() string {
	if supportsTaskbarLyrics() && (s.LyricsMode == lyricsTaskbar || s.LyricsMode == "" && s.TaskbarLyrics) {
		return lyricsTaskbar
	}
	return lyricsDesktop
}

func (s settings) lyricsEnabled() bool {
	return supportsDesktopLyrics() && s.DesktopLyrics || supportsTaskbarLyrics() && s.TaskbarLyrics
}

// normalizeLyrics migrates independent switches to a single supported location.
func (s *settings) normalizeLyrics() {
	enabled := s.lyricsEnabled()
	s.LyricsMode = s.lyricsMode()
	s.DesktopLyrics = enabled && s.LyricsMode == lyricsDesktop
	s.TaskbarLyrics = enabled && s.LyricsMode == lyricsTaskbar
}

func (a *app) setLyricsMode(mode string) {
	if mode == lyricsDesktop && supportsDesktopLyrics() || mode == lyricsTaskbar && supportsTaskbarLyrics() {
		a.applyLyricsMode(mode, a.settings.lyricsEnabled())
	}
}

func (a *app) setLyricsEnabled(enabled bool) {
	if !supportsDesktopLyrics() {
		return
	}
	a.applyLyricsMode(a.settings.lyricsMode(), enabled)
}

func (a *app) applyLyricsMode(mode string, enabled bool) {
	a.settings.LyricsMode = mode
	a.settings.DesktopLyrics = enabled && mode == lyricsDesktop
	a.settings.TaskbarLyrics = enabled && mode == lyricsTaskbar
	a.syncDesktopLyrics()
	a.syncTaskbarLyrics()
	a.settings.save()
	a.refreshLyricsTray()
	a.redraw()
}
