package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// TestMacOSIcons checks the packaged and regenerated icons have macOS margins at every size.
func TestMacOSIcons(t *testing.T) {
	config, err := os.ReadFile("../mygo.json")
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct{ MacOS struct{ Icon string } }
	if err := json.Unmarshal(config, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.MacOS.Icon != "icon/AppIcon.icns" {
		t.Fatalf("macOS uses %q instead of its padded icon", cfg.MacOS.Icon)
	}
	for _, variant := range []struct{ source, output string }{
		{"../resources/icon.png", "AppIcon.icns"},
		{"../resources/icon-dev.png", "AppIcon-dev.icns"},
	} {
		t.Run(variant.output, func(t *testing.T) {
			// Shared full-canvas PNGs once shipped on macOS without its margins.
			before, err := os.ReadFile(variant.source)
			if err != nil {
				t.Fatal(err)
			}
			source := filepath.Join(t.TempDir(), "icon.png")
			if err := os.WriteFile(source, before, 0o644); err != nil {
				t.Fatal(err)
			}
			generated := filepath.Join(t.TempDir(), variant.output)
			if err := generate(source, generated); err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("generation changed the shared artwork")
			}
			for _, file := range []string{variant.output, generated} {
				data, err := os.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
				checkMacOSICNS(t, data)
			}
		})
	}
}

func checkMacOSICNS(t *testing.T, data []byte) {
	t.Helper()
	if len(data) < 8 || string(data[:4]) != "icns" || int(binary.BigEndian.Uint32(data[4:8])) != len(data) {
		t.Fatal("invalid ICNS header or file length")
	}
	want := map[string]int{"ic11": 32, "ic12": 64, "ic07": 128, "ic13": 256, "ic08": 256, "ic14": 512, "ic09": 512, "ic10": 1024}
	for offset := 8; offset < len(data); {
		if offset+8 > len(data) {
			t.Fatal("truncated ICNS entry")
		}
		kind := string(data[offset : offset+4])
		length := int(binary.BigEndian.Uint32(data[offset+4 : offset+8]))
		if length < 8 || length > len(data)-offset {
			t.Fatal("invalid ICNS entry length")
		}
		size, ok := want[kind]
		if !ok {
			t.Fatalf("unexpected or duplicate ICNS entry %q", kind)
		}
		delete(want, kind)
		img, err := png.Decode(bytes.NewReader(data[offset+8 : offset+length]))
		if err != nil {
			t.Fatal(err)
		}
		if img.Bounds() != image.Rect(0, 0, size, size) {
			t.Fatalf("%s: dimensions = %v", kind, img.Bounds())
		}
		// Inspect opaque coverage only, leaving colors and soft shadows free to change.
		for _, vertical := range []bool{false, true} {
			first, last := size, -1
			for i := range size {
				x, y := i, size/2
				if vertical {
					x, y = y, x
				}
				_, _, _, a := img.At(x, y).RGBA()
				if a >= 0x8000 {
					first = min(first, i)
					last = i
				}
			}
			margin := (100*size + 512) / 1024
			if first < margin-1 || first > margin+1 || last < size-margin-2 || last > size-margin {
				t.Errorf("%s: opaque body spans %d..%d, want margins of about %d pixels", kind, first, last, margin)
			}
		}
		offset += length
	}
	if len(want) != 0 {
		t.Errorf("missing ICNS renditions: %v", want)
	}
}
