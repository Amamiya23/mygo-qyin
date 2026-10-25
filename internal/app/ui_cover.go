package app

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// The cover of Now Playing is a thing to touch. It tilts toward the
// pointer in depth, a glare following the pointer over it as light over a
// card held up. Dragged or swiped across, it swings as it goes, and flung
// far enough it flies off: to the left for the next song, to the right
// for the one before, and the new cover comes in from the other side. A
// fling too weak springs back. Asked for less motion, it neither tilts nor
// swings.

// coverState is the cover as the pointer and the trackpad move it.
type coverState struct {
	off   ui.Point // where Draggable rests it, from its place
	gen   int      // counts the flings, each cover after one a new element
	flung int      // 1 flying off for the next song, -1 for the one before
	enter float32  // where the next cover comes in from, 0 for none
	// A trackpad's swipe: how far it has gone, when it last moved, and
	// whether it was decided, the momentum after it swallowed.
	swipe   float32
	swipeAt time.Time
	decided bool
}

type coverKey struct {
	id  int64
	gen int
}

// coverSwing is how the cover goes to rest, or off.
var coverSwing = ui.Animation{Spring: ui.SpringOptions{Duration: 480 * time.Millisecond, Bounce: 0.18}}

// swipeRest is how long a swipe is still before it is let go, for a
// trackpad that sends no momentum after it.
const swipeRest = 140 * time.Millisecond

// npCover shows the cover of tr, size across, which the pointer tilts,
// drags and flings.
func (a *app) npCover(c *ui.Context, tr *netease.Track, art *artwork, size float32) {
	cs, pl := &a.npCoverState, a.pl
	far := size * 1.5 // where a cover flies off to
	fling := func(dir int) {
		cs.flung = dir
		cs.off = ui.Point{X: -float32(dir) * far}
	}
	wrap := ui.Box(c.Key(coverKey{tr.ID, cs.gen})).Size(size, size).AlignSelf(ui.Center).Label("专辑封面")
	entering := cs.enter != 0
	if entering {
		// A new cover's first frame: it shows where it comes in from.
		cs.off, cs.enter = ui.Point{X: cs.enter}, 0
	}
	wrap.Draggable(&cs.off, ui.DraggableOptions{
		Axis: ui.Horizontal,
		Snap: func(p ui.Point) ui.Point {
			switch {
			case p.X < -size*0.4:
				fling(1)
			case p.X > size*0.4:
				fling(-1)
			default:
				return ui.Point{}
			}
			return cs.off
		},
		Animation: coverSwing,
	})
	if entering {
		cs.off = ui.Point{}
		c.AnimationFrame()
	}
	// Two fingers swiping across the trackpad move it as a drag does.
	wrap.HandleInput(func(ev ui.InputEvent) bool {
		if ev.Kind != ui.InputScroll || !ev.Precise || cs.flung != 0 {
			return false
		}
		if ev.Momentum {
			if cs.swipe != 0 {
				a.letSwipeGo(size, fling)
			}
			return cs.decided
		}
		if cs.swipe == 0 && math.Abs(float64(ev.DX)) <= math.Abs(float64(ev.DY)) {
			return false
		}
		cs.decided = false
		cs.swipe -= ev.DX
		cs.swipeAt = ev.Time
		cs.off = ui.Point{X: rubber(cs.swipe, size)}
		return true
	})
	if cs.swipe != 0 {
		if c.Now().Sub(cs.swipeAt) >= swipeRest {
			a.letSwipeGo(size, fling)
		} else {
			c.After(swipeRest)
		}
	}

	at := wrap.DragOffset()
	if cs.flung != 0 && math.Abs(float64(at.X)) >= float64(far)*0.97 {
		// Gone: the next cover comes in from the other side.
		dir := cs.flung
		cs.flung, cs.gen, cs.enter = 0, cs.gen+1, float32(dir)*far
		if dir > 0 {
			pl.Next()
		} else {
			pl.PrevTrack()
		}
	}

	// It tilts toward the pointer, and swings as it is dragged.
	px, py, over := wrap.PointerPosition()
	tx, ty := float32(0), float32(0)
	if over && !wrap.Dragging() {
		tx = clamp1(px/size*2 - 1)
		ty = clamp1(py/size*2 - 1)
	}
	swing := clamp1(at.X/size) * 16
	if c.Preferences().ReduceMotion {
		tx, ty, swing = 0, 0, 0
	}
	tilt := ui.Animation{Spring: ui.SpringOptions{Duration: 500 * time.Millisecond, Bounce: 0.2}}
	ry := wrap.AnimateTo("ry", tx*7+swing, tilt)
	rx := wrap.AnimateTo("rx", -ty*7, tilt)
	shine := float32(0)
	if over {
		shine = 1
	}
	shine = wrap.AnimateTo("shine", shine, ui.Animation{Duration: 300 * time.Millisecond})
	gx := wrap.AnimateTo("gx", px, tilt)
	gy := wrap.AnimateTo("gy", py, tilt)
	if over || wrap.Dragging() {
		wrap.Cursor(ui.CursorGrab)
	}
	// Flying off, it fades.
	if f := float32(math.Abs(float64(at.X))) / far; f > 0.01 {
		wrap.Opacity(1 - 0.85*min(f, 1))
	}

	wrap.Children(func() {
		// The cover breathes: smaller while paused, a little nearer under
		// the pointer.
		scale := float32(1)
		if !pl.Playing() {
			scale = 0.88
		}
		if over {
			scale *= 1.02
		}
		box := ui.Box(c).Fill().Radius(16).Shadow(0, 30, 60, -12, ui.RGBA(0, 0, 0, 0.55))
		if math.Abs(float64(rx))+math.Abs(float64(ry)) > 0.05 {
			// In depth only while tilted: flat, it draws as an image does.
			box.RotateX(rx).RotateY(ry).Perspective(900)
		}
		box.AnimatePose(ui.Pose{Scale: scale}, ui.Animation{Spring: ui.SpringOptions{Duration: 600 * time.Millisecond, Bounce: 0.25}})
		in := artIn(box, bitmap(art))
		if in < 1 {
			box.Background(white.Alpha(0.08))
		}
		box.Children(func() {
			artImage(c, bitmap(art), 16, in)
			if shine > 0.01 {
				ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().Draw(func(p *ui.Painter, r ui.Rect) {
					p.Clip(r, 16, func() {
						p.Shadow(ui.Rect{X: r.X + gx, Y: r.Y + gy}, 0, 0, 0, r.W*0.45, r.W*0.16, white.Alpha(0.26*shine))
					})
				})
			}
		})
	})
}

// letSwipeGo ends a trackpad's swipe: flung if it went far enough,
// springing back if not.
func (a *app) letSwipeGo(size float32, fling func(int)) {
	cs := &a.npCoverState
	switch {
	case cs.swipe < -size*0.3:
		fling(1)
	case cs.swipe > size*0.3:
		fling(-1)
	default:
		cs.off = ui.Point{}
	}
	cs.swipe, cs.decided = 0, true
}

// rubber holds a swipe's travel back past where it would fling, as a
// drag past an edge.
func rubber(d, size float32) float32 {
	return ui.Rubberband(d, -size*0.5, size*0.5, size)
}

func clamp1(v float32) float32 { return max(-1, min(v, 1)) }
