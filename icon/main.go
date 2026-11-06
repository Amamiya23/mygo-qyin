// Command icon generates macOS-only ICNS files from the shared PNG artwork.
// Run go run ./icon from the repository root on any platform. The shared
// Windows/Linux PNGs are inputs and keep their original full-canvas layout.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"

	xdraw "golang.org/x/image/draw"
)

var icons = []struct {
	source, output string
}{
	{"resources/icon.png", "icon/AppIcon.icns"},
	{"resources/icon-dev.png", "icon/AppIcon-dev.icns"},
}

// Keep PNG payloads out of the legacy icp4/icp5 slots: macOS expects ARGB
// there. Small icons are downsampled from the Retina ic11/ic12 entries.
var iconSizes = []struct {
	kind string
	size int
}{
	{"ic11", 32}, {"ic12", 64}, {"ic07", 128}, {"ic13", 256},
	{"ic08", 256}, {"ic14", 512}, {"ic09", 512}, {"ic10", 1024},
}

func main() {
	for _, ic := range icons {
		if err := generate(ic.source, ic.output); err != nil {
			log.Fatal(err)
		}
	}
}

func generate(source, output string) error {
	f, err := os.Open(source)
	if err != nil {
		return err
	}
	src, err := png.Decode(f)
	f.Close()
	if err != nil {
		return err
	}
	if src.Bounds().Dx() != 1024 || src.Bounds().Dy() != 1024 {
		return fmt.Errorf("%s: artwork must be 1024x1024", source)
	}
	data, err := encodeICNS(macOSIcon(src))
	if err != nil {
		return err
	}
	return os.WriteFile(output, data, 0o644)
}

// The macOS tile occupies 824 points of a 1024-point canvas. Padding is
// applied here once, before downsampling, rather than to the shared artwork.
func macOSIcon(src image.Image) *image.NRGBA {
	const canvas, body = 1024, 824
	tile := image.NewNRGBA(image.Rect(0, 0, body, body))
	xdraw.CatmullRom.Scale(tile, tile.Bounds(), src, src.Bounds(), draw.Src, nil)
	at := image.Pt((canvas-body)/2, (canvas-body)/2)
	dst := image.NewNRGBA(image.Rect(0, 0, canvas, canvas))
	shadow := image.NewAlpha(dst.Bounds())
	draw.Draw(shadow, tile.Bounds().Add(at).Add(image.Pt(0, 12)), tile, tile.Bounds().Min, draw.Src)
	blur(shadow, 10)
	draw.DrawMask(dst, dst.Bounds(), image.NewUniform(color.NRGBA{A: 72}), image.Point{}, shadow, image.Point{}, draw.Over)
	draw.Draw(dst, tile.Bounds().Add(at), tile, tile.Bounds().Min, draw.Over)
	return dst
}

func encodeICNS(src image.Image) ([]byte, error) {
	var body bytes.Buffer
	for _, entry := range iconSizes {
		dst := image.NewNRGBA(image.Rect(0, 0, entry.size, entry.size))
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
		var payload bytes.Buffer
		if err := png.Encode(&payload, dst); err != nil {
			return nil, err
		}
		body.WriteString(entry.kind)
		_ = binary.Write(&body, binary.BigEndian, uint32(payload.Len()+8))
		body.Write(payload.Bytes())
	}
	var out bytes.Buffer
	out.WriteString("icns")
	_ = binary.Write(&out, binary.BigEndian, uint32(body.Len()+8))
	out.Write(body.Bytes())
	return out.Bytes(), nil
}

func blur(m *image.Alpha, radius int) {
	w, h := m.Rect.Dx(), m.Rect.Dy()
	buf := make([]uint8, len(m.Pix))
	pass := func(src, dst []uint8, count, length, stride, step int) {
		for line := range count {
			base := line * stride
			sum := 0
			for i := -radius; i <= radius; i++ {
				sum += int(src[base+min(max(i, 0), length-1)*step])
			}
			for i := range length {
				dst[base+i*step] = uint8(sum / (2*radius + 1))
				sum += int(src[base+min(i+radius+1, length-1)*step]) - int(src[base+max(i-radius, 0)*step])
			}
		}
	}
	for range 3 {
		pass(m.Pix, buf, h, w, m.Stride, 1)
		pass(buf, m.Pix, w, h, 1, m.Stride)
	}
}
