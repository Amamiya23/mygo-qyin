package gpu

import (
	"cmp"
	"image"
	"math"
	"slices"

	"github.com/egoist/mygo/internal/raster"
	"github.com/egoist/mygo/internal/scene"
)

// clipMaskSlot is a retained clip mask. Masks outlive the frames that
// don't use them, up to idleMaskBytes of them, so a transform that comes
// and goes finds its masks again instead of rasterizing them.
type clipMaskSlot struct {
	// shapes are the clips the mask is of, their stops in stops.
	shapes []scene.ClipShape
	stops  []scene.Stop
	area   image.Rectangle
	image  *scene.Image
	// frame is the last frame that used the mask, and slot the clip's
	// place among that frame's masked clips.
	frame uint64
	slot  int
}

// idleMaskBytes is how much memory the masks no clip of the frame uses
// keep, the least recently used dropped first.
const idleMaskBytes = 32 << 20

func (b *Builder) prepare(in *Instance, m scene.Affine, c *clip) bool {
	b.preparedMask = 0
	if c.scissor.Empty() || in.Rect[2] <= 0 || in.Rect[3] <= 0 {
		return false
	}
	if _, ok := m.Inverse(); !ok {
		return false
	}
	in.Transform0, in.Transform1, in.Transform2 = [4]float32{1, 0, 0, 0}, [4]float32{0, 1, 0, 0}, [4]float32{}
	if m.Set {
		in.Transform0 = [4]float32{m.A, m.C, m.X, 1}
		in.Transform1 = [4]float32{m.B, m.D, m.Y, 0}
		in.Transform2 = [4]float32{m.G, m.H, 0, 0}
	}
	in.Clip, in.ClipRadii = rect(c.rect), c.radii
	if !c.masked {
		return true
	}
	// The shaders evaluate a stationary rounded clip in frame coordinates,
	// even when the content inside it is scaled. Integer hard ancestors and
	// descendants are already exact in the batch scissor. Do not rasterize
	// a whole-window mask just because an opening card contains transforms.
	if !c.moved && !c.masks {
		if rounded, ok := b.analyticClip(); ok {
			in.Clip, in.ClipRadii = rect(rounded.rect), rounded.radii
			return true
		}
	}
	// A hard clip sharing the drawing's coordinates is cheap to evaluate in
	// the fragment shader. Keep the rounded ancestor's mask stationary even
	// when a text input or custom clipped drawing moves within it. Drawing
	// that a translation of the clip's moved shares them too, folded into
	// the frame's (scene.TranslationFolder): the clip's rectangle moves.
	shared, dx, dy := c.shape.Transform == m && m.Set, float32(0), float32(0)
	if !m.Set {
		dx, dy, shared = c.shape.Transform.Translation()
	}
	if c.shape.Radii == [4]float32{} && !c.shape.Masks() && shared && len(b.stack) > 1 {
		parent := &b.stack[len(b.stack)-2]
		if parent.masked {
			b.preparedMask = b.clipTexture(parent, b.clipShapes[:len(b.clipShapes)-1])
			if b.preparedMask == 0 {
				return false
			}
			in.Clip = rect(c.rect)
			in.Clip[0], in.Clip[1] = in.Clip[0]+dx, in.Clip[1]+dy
			in.ClipRadii = maskRect(parent.maskArea)
			in.Transform1[3] = 2
			return true
		}
	}
	b.preparedMask = b.clipTexture(c, b.clipShapes)
	if b.preparedMask == 0 {
		return false
	}
	in.Clip = maskRect(c.maskArea)
	in.ClipRadii = [4]float32{}
	in.Transform1[3] = 1
	return true
}

// analyticClip finds a single rounded clip surrounded only by integer
// hard clips. Fractional hard edges need shader antialiasing of their own;
// multiple rounded clips still need their combined coverage mask.
func (b *Builder) analyticClip() (clip, bool) {
	var rounded *clip
	for i := range b.stack {
		c := &b.stack[i]
		if c.radii != [4]float32{} {
			if rounded != nil {
				return clip{}, false
			}
			rounded = c
		} else {
			for _, v := range []float32{c.rect.X, c.rect.Y, c.rect.X + c.rect.W, c.rect.Y + c.rect.H} {
				if v != float32(math.Floor(float64(v))) {
					return clip{}, false
				}
			}
		}
	}
	if rounded == nil {
		return clip{}, false
	}
	return *rounded, true
}

func maskRect(area image.Rectangle) [4]float32 {
	return [4]float32{float32(area.Min.X), float32(area.Min.Y), float32(area.Dx()), float32(area.Dy())}
}

func (b *Builder) clipTexture(c *clip, shapes []scene.ClipShape) uintptr {
	if c.maskTexture != 0 {
		return c.maskTexture
	}
	// Animated clips used to resize their CPU buffer and GPU texture for
	// every pixel of growth. Keep the texture in tiles; the scissor and
	// shapes still cut at their exact edges, and prepare maps the full area.
	const tile = 64
	area := image.Rect(int(c.scissor.Left)&^(tile-1), int(c.scissor.Top)&^(tile-1),
		(int(c.scissor.Right)+tile-1)&^(tile-1), (int(c.scissor.Bottom)+tile-1)&^(tile-1))
	// The mask of the same clip, in this frame or an earlier one; else
	// the one the clip in its place had last frame, which it most likely
	// is, changed; else a new one.
	found, reuse, idle := -1, -1, -1
	idleBytes := 0
	for i := range b.maskSlots {
		m := &b.maskSlots[i]
		if area.In(m.area) && slices.EqualFunc(m.shapes, shapes, func(a, b scene.ClipShape) bool { return a.Same(&b) }) {
			found = i
			break
		}
		if reuse < 0 && m.frame == b.frame-1 && m.slot == c.maskSlot {
			reuse = i
		}
		if m.frame != b.frame {
			idleBytes += cap(m.image.Pix)
			if idle < 0 {
				idle = i
			}
		}
	}
	if found < 0 {
		found = reuse
		if found < 0 && idleBytes+4*area.Dx()*area.Dy() > idleMaskBytes {
			// Under budget pressure, reuse an idle slot rather than adding
			// another texture. Otherwise keep stationary clips cached while
			// other branches draw. Never overwrite this frame's used masks.
			found = idle
		}
		if found < 0 {
			found = len(b.maskSlots)
			b.maskSlots = append(b.maskSlots, clipMaskSlot{})
		}
		m := &b.maskSlots[found]
		if m.image != nil {
			// Retain the texture's size as the clip shrinks. On growth,
			// leave room for following frames, bounded by the viewport.
			// Moving the origin only repaints pixels; it needs no new texture.
			w, h := m.image.W, m.image.H
			if area.Dx() > w {
				w = (max(area.Dx(), w*3/2) + tile - 1) &^ (tile - 1)
			}
			if area.Dy() > h {
				h = (max(area.Dy(), h*3/2) + tile - 1) &^ (tile - 1)
			}
			if len(b.stack) > 0 {
				sc := b.stack[0].scissor
				w = max(area.Dx(), min(w, (int(sc.Right-sc.Left)+tile-1)&^(tile-1)))
				h = max(area.Dy(), min(h, (int(sc.Bottom-sc.Top)+tile-1)&^(tile-1)))
			}
			x := min(max(m.area.Min.X, area.Max.X-w), area.Min.X)
			y := min(max(m.area.Min.Y, area.Max.Y-h), area.Min.Y)
			area = image.Rect(x, y, x+w, y+h)
		}
		m.image = raster.ClipMask(m.image, shapes, area)
		m.area = area
		m.keep(shapes)
	}
	m := &b.maskSlots[found]
	m.frame, m.slot = b.frame, c.maskSlot
	c.maskArea = m.area
	c.maskTexture = b.imageTexture(m.image)
	return c.maskTexture
}

// keep keeps shapes as the mask's, with copies of their stops, which
// the scene's are not from frame to frame.
func (m *clipMaskSlot) keep(shapes []scene.ClipShape) {
	m.shapes = append(m.shapes[:0], shapes...)
	m.stops = m.stops[:0]
	for _, c := range shapes {
		m.stops = append(m.stops, c.Stops...)
	}
	at := 0
	for i := range m.shapes {
		if st := m.shapes[i].Stops; st != nil {
			m.shapes[i].Stops = m.stops[at : at+len(st) : at+len(st)]
			at += len(st)
		}
	}
}

// dropIdleMasks drops the least recently used masks this frame didn't use
// beyond idle bytes of them.
func (b *Builder) dropIdleMasks(idle int) {
	slices.SortFunc(b.maskSlots, func(x, y clipMaskSlot) int { return cmp.Compare(y.frame, x.frame) })
	for i := range b.maskSlots {
		m := &b.maskSlots[i]
		if m.frame == b.frame {
			continue
		}
		if idle -= cap(m.image.Pix); idle < 0 {
			clear(b.maskSlots[i:])
			b.maskSlots = b.maskSlots[:i]
			return
		}
	}
}
