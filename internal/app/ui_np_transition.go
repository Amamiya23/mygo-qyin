package app

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// npAtRest reports whether the last frame was all open without a pull,
// or all closed. The covered page can stop building only at rest.
func (a *app) npAtRest(open bool) bool {
	if open {
		return a.npT >= 1 && a.npShift == 0 && !a.npHeld
	}
	return a.npT <= 0
}

const npFromRadius = 8

// npFromRect is where the bar last laid out its cover, or where it would
// before the bar has been built, for a window opened on Now Playing.
func (a *app) npFromRect(w, h float32) ui.Rect {
	if a.npFrom.W > 0 {
		return a.npFrom
	}
	return ui.Rect{X: sidebarWidth + 21, Y: h - (playerBarHeight+48)/2, W: 48, H: 48}
}

// npMorph moves the cover from the bar to its place as the card opens,
// and back to the bar as it closes.
func (a *app) npMorph(c *ui.Context, e ui.Element) ui.Element {
	if a.npT >= 1 || a.npStill {
		return e
	}
	b := e.LayoutBounds()
	if b.W <= 0 {
		c.AnimationFrame()
		return e.Opacity(0)
	}
	w, h := c.Size()
	from := a.npFromRect(w, h)
	k := 1 - a.npT
	s := 1 + (from.W/b.W-1)*k
	return e.TransformOrigin(0, 0).Scale(s, s).Translate((from.X-b.X)*k, (from.Y-b.Y)*k)
}

// npBreathe blends the cover's resting scale into its flight, so even a
// paused cover meets the bar's cover at its full size.
func (a *app) npBreathe(e ui.Element, scale float32) {
	if a.npT < 1 && !a.npStill {
		s := lerp(1, scale, clamp01(a.npT))
		e.Scale(s, s)
		return
	}
	e.AnimatePose(ui.Pose{Scale: scale}, ui.Animation{Spring: ui.SpringOptions{Duration: 600 * time.Millisecond, Bounce: 0.25}})
}

// npRadius keeps the scaled cover's corners round as it meets the bar.
func (a *app) npRadius(c *ui.Context, r, size float32) float32 {
	if a.npT >= 1 || a.npStill {
		return r
	}
	w, h := c.Size()
	k := 1 - clamp01(a.npT)
	s := 1 + (a.npFromRect(w, h).W/size-1)*k
	return min(lerp(r, npFromRadius, k)/s, size/2)
}

// npRise brings controls in after the cover, in order, and reverses that
// order as the card closes.
func (a *app) npRise(e ui.Element, n int) ui.Element {
	if a.npT >= 1 || a.npStill {
		return e
	}
	from := 0.3 + 0.07*float32(n)
	k := smoothstep(from, from+0.4, a.npT)
	return e.Opacity(k).Translate(0, 24*(1-k))
}
