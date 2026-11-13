package app

import "github.com/egoist/mygo/ui"

const (
	lyricsFontSizeDefault = 30
	lyricsFontSizeMin     = 18
	lyricsFontSizeMax     = 60
)

func (s settings) lyricsFontSize() float64 {
	if s.LyricsFontSize <= 0 {
		return lyricsFontSizeDefault
	}
	return min(lyricsFontSizeMax, max(lyricsFontSizeMin, s.LyricsFontSize))
}

// lyricsSizeButton keeps reading preferences beside the lyrics, with the song
// itself as the preview and no separate confirmation step.
func (a *app) lyricsSizeButton(c *ui.Context) {
	b := ui.ButtonBase(c.Key("lyrics-size-button")).Bind(&a.lyricsSizeTrigger).
		Size(36, npTabsHeight).Shrink(0).Center().Radius(999).Label("歌词字号").Tooltip("歌词字号")
	mediaFeedback(b, radixWhite(8))
	if a.lyricsSizeOpen {
		b.Background(radixWhite(3)).TextColor(radixPaper)
	}
	b.Children(func() { ui.Text(c, "Aa").FontSize(15).FontWeight(600) })
	if b.Clicked() {
		a.lyricsSizeOpen = !a.lyricsSizeOpen
	}
	panel := ui.PopoverBase(c.Key("lyrics-size-popover"), b, &a.lyricsSizeOpen, func(panel ui.Element) {
		panel.AttachTo(b, ui.AnchorBottomEnd, ui.AnchorTopEnd).Margin(8, 0, 0, 0).
			Width(240).Padding(16).Gap(12).Radius(12).Background(radixSage(true, 2)).
			Border(0.5, radixSage(true, 6)).Shadow(0, 8, 24, -4, radixBlack(6)).TextColor(radixSage(true, 12))
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "歌词字号").FontSize(13).FontWeight(600).Grow(1)
			ui.Textf(c, "%.0f", a.settings.lyricsFontSize()).FontSize(13).FontFeatures("tnum").TextColor(radixSage(true, 11))
		})
		a.lyricsSizeSlider(c)
		reset := ui.ButtonBase(c.Key("reset")).Padding(5, 8).Margin(0, -8, -5, 0).
			AlignSelf(ui.Start).Radius(6).Label("恢复默认字号")
		mediaFeedback(reset, radixSage(true, 11))
		reset.Children(func() { ui.Text(c, "恢复默认").FontSize(12) })
		if reset.Clicked() {
			a.settings.LyricsFontSize = lyricsFontSizeDefault
			a.lyricsSizeChanged()
		}
	})
	if panel.Dismissed() && !panel.PressedOutside() {
		a.lyricsSizeTrigger.Focus(c)
	}
}

func (a *app) lyricsSizeSlider(c *ui.Context) {
	a.settings.LyricsFontSize = a.settings.lyricsFontSize()
	const knob, track = float32(16), float32(4)
	s := ui.SliderBase(c.Key("size"), &a.settings.LyricsFontSize, lyricsFontSizeMin, lyricsFontSizeMax).
		Height(28).PaddingX(knob / 2).Step(1).AutoFocus().Label("调整歌词字号").Cursor(ui.CursorPointer)
	if s.Changed() {
		a.lyricsSizeChanged()
	}
	value := float32((a.settings.LyricsFontSize - lyricsFontSizeMin) / (lyricsFontSizeMax - lyricsFontSizeMin))
	fill := radixGreen(true, 9)
	if s.Hovered() || s.Pressed() {
		fill = radixGreen(true, 10)
	}
	s.Draw(func(p *ui.Painter, r ui.Rect) {
		left, width := r.X+knob/2, r.W-knob
		x, y := left+width*value, r.Y+r.H/2
		p.Fill(ui.Rect{X: left, Y: y - track/2, W: width, H: track}, radixSage(true, 6), track/2)
		p.Fill(ui.Rect{X: left, Y: y - track/2, W: x - left, H: track}, fill, track/2)
		p.Fill(ui.Rect{X: x - knob/2, Y: y - knob/2, W: knob, H: knob}, radixPaper, knob/2)
	})
}

func (a *app) lyricsSizeChanged() {
	// A different size changes wrapping and line positions even while paused.
	a.lyricAt = -2
	// The offset stays where it is, not taken for the listener scrolling.
	a.lyricGlide = lyricGlide{set: a.lyricScroll.Y}
	a.settings.save()
}
