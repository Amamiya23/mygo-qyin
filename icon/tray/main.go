// Command tray draws internal/app/tray/tray-template.png, the Lilt record in
// the macOS menu bar, as a black-on-transparent template at 2×.
//
// Run it from the repository root:
//
//	go run ./icon/tray
//
// With -preview dir, it also writes enlarged previews over light and dark bars.
package main

import (
	"flag"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
)

type point struct{ x, y float64 }

func distance(a, b point) float64 { return math.Hypot(a.x-b.x, a.y-b.y) }

// render draws the mark at size × size pixels with 4×4 antialiasing.
func render(size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	const samples = 4
	scale := float64(size) / 18
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			hits := 0
			for sy := 0; sy < samples; sy++ {
				for sx := 0; sx < samples; sx++ {
					p := point{
						x: (float64(x) + (float64(sx)+0.5)/samples) / scale,
						y: (float64(y) + (float64(sy)+0.5)/samples) / scale,
					}
					if inMark(p) {
						hits++
					}
				}
			}
			if hits != 0 {
				img.SetNRGBA(x, y, color.NRGBA{A: uint8(math.Round(float64(hits) * 255 / (samples * samples)))})
			}
		}
	}
	return img
}

// inMark is a record with an inner cutout and a lifted tonearm. It uses one
// silhouette so the design survives menu-bar tinting and 16 px rendering.
func inMark(p point) bool {
	c := point{7.1, 10.7}
	d := distance(p, c)
	disc := d < 7 && (d < 2.45 || d > 3.05)
	arm := segmentDistance(p, point{12.2, 4.8}, point{15.15, 2.75}) < .7
	pivot := distance(p, point{15.15, 2.75}) < 1.15
	knockout := segmentDistance(p, point{10.3, 6.15}, point{13.2, 4.15}) < 1.15 ||
		distance(p, point{15.15, 2.75}) < 1.95
	return disc && !knockout || arm || pivot
}

func segmentDistance(p, a, b point) float64 {
	dx, dy := b.x-a.x, b.y-a.y
	t := ((p.x-a.x)*dx + (p.y-a.y)*dy) / (dx*dx + dy*dy)
	t = max(0, min(1, t))
	return distance(p, point{a.x + t*dx, a.y + t*dy})
}

func save(path string, img image.Image) {
	f, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		log.Fatal(err)
	}
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
}

func preview(icon *image.NRGBA, ground color.NRGBA, ink uint8, zoom int) *image.NRGBA {
	w := icon.Bounds().Dx()
	pad := w / 3
	side := w + 2*pad
	out := image.NewNRGBA(image.Rect(0, 0, side*zoom, side*zoom))
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			c := ground
			if ix, iy := x-pad, y-pad; ix >= 0 && iy >= 0 && ix < w && iy < w {
				a := float64(icon.NRGBAAt(ix, iy).A) / 255
				mix := func(v uint8) uint8 { return uint8(float64(v)*(1-a) + float64(ink)*a) }
				c = color.NRGBA{mix(c.R), mix(c.G), mix(c.B), 255}
			}
			for dy := 0; dy < zoom; dy++ {
				for dx := 0; dx < zoom; dx++ {
					out.SetNRGBA(x*zoom+dx, y*zoom+dy, c)
				}
			}
		}
	}
	return out
}

func main() {
	out := flag.String("o", "internal/app/tray/tray-template.png", "where to write the icon")
	dir := flag.String("preview", "", "optional preview directory")
	flag.Parse()

	icon := render(36)
	save(*out, icon)
	if *dir != "" {
		save(filepath.Join(*dir, "menu-light.png"), preview(icon, color.NRGBA{236, 236, 238, 255}, 0, 8))
		save(filepath.Join(*dir, "menu-dark.png"), preview(icon, color.NRGBA{40, 40, 44, 255}, 255, 8))
		save(filepath.Join(*dir, "menu-16.png"), preview(render(16), color.NRGBA{246, 246, 246, 255}, 26, 12))
	}
}
