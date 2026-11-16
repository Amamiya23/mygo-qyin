package ui

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"slices"
	"testing"
)

func TestDecodeBitmapWithImage(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	for y := range 2 {
		for x := range 3 {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(40 + 60*x), G: uint8(50 + 100*y), B: 90, A: 180})
		}
	}
	for _, format := range []string{"png", "jpeg-exif"} {
		t.Run(format, func(t *testing.T) {
			var encoded bytes.Buffer
			var err error
			if format == "png" {
				err = png.Encode(&encoded, img)
			} else {
				err = jpeg.Encode(&encoded, img, nil)
			}
			if err != nil {
				t.Fatal(err)
			}
			data := encoded.Bytes()
			if format == "jpeg-exif" {
				exif := []byte{
					'E', 'x', 'i', 'f', 0, 0, 'I', 'I', 42, 0, 8, 0, 0, 0,
					1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0,
				}
				header := append([]byte{0xff, 0xd8, 0xff, 0xe1, 0, byte(len(exif) + 2)}, exif...)
				data = append(header, data[2:]...)
			}
			bmp, source, err := DecodeBitmapWithImage(data)
			if err != nil {
				t.Fatal(err)
			}
			if source.Bounds().Dx() != 3 || source.Bounds().Dy() != 2 {
				t.Fatal("source orientation changed")
			}
			w, h := bmp.Size()
			if (format == "jpeg-exif" && (w != 2 || h != 3)) || (format == "png" && (w != 3 || h != 2)) {
				t.Fatalf("wrong display orientation: %dx%d", w, h)
			}
			for y := range h {
				for x := range w {
					sx, sy := x, y
					if format == "jpeg-exif" {
						sx, sy = y, 1-x
					}
					r, g, b, a := source.At(sx, sy).RGBA()
					want := []byte{byte(r >> 8), byte(g >> 8), byte(b >> 8), byte(a >> 8)}
					at := 4 * (y*w + x)
					if !slices.Equal(bmp.img.Pix[at:at+4], want) {
						t.Fatalf("display pixel at %d,%d changed", x, y)
					}
				}
			}
			before := slices.Clone(bmp.img.Pix)
			bmp.ReleasePixels()
			if bmp.img.Pix != nil || bmp.remake == nil || bmp.source != nil {
				t.Fatal("bitmap did not release pixels while retaining compressed restoration")
			}
			if !slices.Equal(bmp.img.Pixels(), before) {
				t.Fatal("restoring released pixels changed the bitmap")
			}
		})
	}
	bmp, imgSource, err := DecodeBitmapWithImage([]byte("invalid"))
	if err == nil || bmp != nil || imgSource != nil {
		t.Fatal("invalid encoded data was accepted")
	}
}
