//go:build windows && (amd64 || arm64)

package windows

import (
	"fmt"
	"testing"
)

// Menu measurement previously deducted more than Windows restored, clipping the label's last glyphs.
func TestMenuItemWidthContainsDrawnContent(t *testing.T) {
	b := &Backend{}
	t.Cleanup(b.dropMenuFonts)
	dc, _, _ := procGetDC.Call(0)
	if dc == 0 {
		t.Fatal("GetDC failed")
	}
	defer procReleaseDC.Call(0, dc)
	for _, dpi := range []int{96, 144, 192} {
		for _, tc := range []struct {
			name string
			look menuLook
		}{
			{"artist", menuLook{label: "查看歌手：房东的猫"}},
			{"album", menuLook{label: "查看专辑：云烟成雨"}},
			{"long mixed text", menuLook{label: "查看专辑：Memories of a Geisha（艺伎回忆录原声带）"}},
			{"shortcut", menuLook{label: "复制歌名和歌手", accel: "Ctrl+Shift+C"}},
			{"submenu", menuLook{label: "添加到歌单", submenu: true}},
			{"check and icon", menuLook{label: "查看歌手：房东的猫", column: true, icons: true}},
		} {
			t.Run(fmt.Sprintf("%s/%d", tc.name, dpi), func(t *testing.T) {
				width, _ := b.menuItemSize(dpi, &tc.look)
				font := b.menuFace(dpi).text
				// This is the interval drawItem gives the text after its own columns and padding.
				available := int32(width) - 2*toPx(menuInset+menuPadding, dpi)
				if tc.look.column {
					available -= toPx(menuMarkColumn, dpi)
				}
				if tc.look.icons {
					available -= toPx(menuIconColumn, dpi)
				}
				if tc.look.submenu {
					available -= toPx(menuAccelGap/2+menuChevron, dpi)
				}
				if tc.look.accel != "" {
					available -= toPx(menuAccelGap, dpi) + textWidth(dc, font, tc.look.accel)
				}
				if need := textWidth(dc, font, tc.look.label); available < need {
					t.Fatalf("label needs %d px, measured item leaves %d px", need, available)
				}
			})
		}
	}
}
