package app

import "github.com/egoist/mygo/ui"

// queueButton opens the queue above the player without leaving the current page.
func (a *app) queueButton(c *ui.Context) {
	p := a.pal
	surface := playerSurface
	if a.queueOpen {
		surface = playerSelectedSurface
	}
	b := iconButton(c.Key("queue"), iconQueue, 16, p.muted, "待播清单", surface).
		Disabled(a.pl.Current() == nil)
	if b.Clicked() {
		a.queueOpen = !a.queueOpen
		if a.queueOpen {
			a.queuePopupAt = -1
		}
	}
	pop := popoverColors(p.dark)
	ui.PopoverBase(c.Key("queue-popover"), b, &a.queueOpen, func(panel ui.Element) {
		w, h := c.Size()
		height := min(float32(len(a.pl.Queue())*60+64), 460, h-topBar(c)-playerBarHeight-24)
		panel.AttachTo(b, ui.AnchorTopEnd, ui.AnchorBottomEnd).
			Margin(0, 0, (playerBarHeight-32)/2+8, 0).Size(min(400, w-32), height).
			Padding(8).Radius(14).Background(pop.fill).TextColor(p.text).Border(0.5, pop.edge).
			Shadow(0, 8, 24, -4, radixInk.Alpha(0.2)).Label("待播清单")
		// Overlay content sits outside the page's Space shortcut scope.
		panel.OnShortcut(0, ui.KeySpace, a.pl.TogglePlay)
		ui.Row(c).Height(40).PaddingX(8).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "待播清单").FontSize(15).FontWeight(700)
			ui.Textf(c, "%d", len(a.pl.Queue())).FontSize(12).FontFeatures("tnum").TextColor(p.muted)
			ui.Spacer(c)
			if iconButton(c, iconX, 15, p.muted, "关闭待播清单").Clicked() {
				a.queueOpen = false
			}
		})
		a.queueTracks(c, &a.queuePopup, &a.queuePopupAt, p).Padding(4, 0, 8, 0)
	})
}
