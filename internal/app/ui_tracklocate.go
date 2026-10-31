package app

import (
	"time"

	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

const trackRowHeight = 54

// listScrollState keeps the list's row anchor and its animated offset together
// when a page is hidden or the listener switches between collection tabs.
type listScrollState struct {
	ui.ListState
	scroll   ui.ScrollState
	locating int64
	target   float32
}

func (a *app) locateTrackButton(c *ui.Context, state *listScrollState, current *qqmusic.Track, currentIndex int, currentTop, scrollY float32, measured bool) {
	// Visible asks for another build when layout changes the visible rows.
	// Geometry below also excludes rows covered by the floating page bars.
	state.Visible()
	bounds := state.Bounds(c)
	if current == nil || !measured || bounds.H == 0 {
		state.locating = 0
		return
	}
	top, bottom := a.insets(c)
	viewTop, viewBottom := bounds.Y+top, bounds.Y+bounds.H-bottom
	target := scrollY + currentTop - (viewTop+viewBottom-trackRowHeight)/2
	if state.locating != 0 {
		if state.locating != current.ID || !state.scroll.Scrolling() {
			state.locating = 0
		} else if delta := target - state.target; delta < -1 || delta > 1 {
			// Unvisited rows initially use estimated heights. As the glide
			// measures them, keep its destination on the actual song.
			state.target = target
			state.scroll.ScrollTo(0, target, ui.Smooth)
		}
	}
	if currentTop+trackRowHeight > viewTop && currentTop < viewBottom {
		return
	}

	p := a.pal
	b := ui.ButtonBase(c.Key("locate-track")).Absolute().Right(20).Bottom(bottom+20).
		Size(32, 32).Radius(999).Center().Label("定位当前歌曲").Tooltip("定位当前歌曲").
		TextColor(radixGreen(p.dark, 11)).Shadow(0, 3, 12, 0, p.cardShade.Alpha(0.65))
	bg := radixSage(p.dark, 1).Alpha(0.88)
	border := radixGreen(p.dark, 6).Alpha(0.55)
	if b.Pressed() {
		bg = radixGreen(p.dark, 4)
		border = radixGreen(p.dark, 6)
	} else if b.Hovered() {
		bg = radixGreen(p.dark, 3).Alpha(0.96)
		border = radixGreen(p.dark, 6).Alpha(0.85)
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	b.Border(1, b.AnimateColor("border", border, ui.Animation{Duration: 120 * time.Millisecond}))
	pose := ui.Pose{}
	if b.Pressed() {
		pose.Scale = 0.94
	}
	b.AnimatePose(pose, ui.Snappy)
	b.Children(func() { ui.Icon(c, iconLocate).Size(16, 16) })
	if b.Clicked() {
		if c.Preferences().ReduceMotion {
			// No glide will measure the intervening rows: ask the virtual
			// list to measure and place the destination directly.
			state.ScrollTo(currentIndex+1, ui.Center)
			state.locating = 0
			return
		}
		state.locating, state.target = current.ID, target
		state.scroll.ScrollTo(0, target, ui.Smooth)
	}
}
