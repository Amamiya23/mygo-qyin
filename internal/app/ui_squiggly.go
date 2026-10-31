package app

import (
	"math"

	"github.com/egoist/mygo/ui"
)

const (
	squigglyWavelength = 30.0 // DIPs
	squigglySpeed      = 8.0  // DIPs per second
)

// drawSquiggly keeps the unplayed track straight and replaces only the
// played track with a flowing sine, easing its height to zero at the head.
func drawSquiggly(p *ui.Painter, r ui.Rect, head, thick, shown float32, track, fill ui.Color) {
	cy := r.Y + r.H/2
	p.Fill(ui.Rect{X: head, Y: cy - thick/2, W: r.X + r.W - head, H: thick}, track, thick/2)
	width := head - r.X
	if width <= 0 {
		return
	}
	if shown <= 0 {
		p.Fill(ui.Rect{X: r.X, Y: cy - thick/2, W: width, H: thick}, fill, thick/2)
		return
	}
	amplitude := min(float32(3), max(0, (r.H-thick)/2)) * shown
	phase := math.Mod(float64(p.Now().UnixMilli())/1000*squigglySpeed, squigglyWavelength)
	y := func(x float32) float32 {
		// A smooth envelope joins the sine to the playhead without a kink,
		// including while the listener drags through a very short played part.
		edge := min(float32(1), (width-x)/float32(squigglyWavelength/2))
		edge = edge * edge * (3 - 2*edge)
		return cy + amplitude*edge*float32(math.Sin((float64(x)+phase)*2*math.Pi/squigglyWavelength))
	}
	path := (&ui.Path{}).MoveTo(r.X, y(0))
	const step = float32(1.5)
	for x := step; x < width; x += step {
		path.LineTo(r.X+x, y(x))
	}
	path.LineTo(head, cy)
	// Round stroke caps must not spill the sine into the unplayed track.
	p.Clip(ui.Rect{X: r.X - thick/2, Y: r.Y, W: width + thick/2, H: r.H}, 0, func() {
		p.StrokePath(path, thick, fill)
	})
}
