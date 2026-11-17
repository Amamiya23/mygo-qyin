package app

import "github.com/egoist/mygo/ui"

// lyricsFamily resolves the default to the current window's theme font.
func (a *app) lyricsFamily(c *ui.Context) string {
	if a.settings.LyricsFont != "" {
		return a.settings.LyricsFont
	}
	return c.Theme().Font
}
