// Package art owns the editable SVG layers shared by the macOS and
// cross-platform icon commands.
package art

import (
	"fmt"
	"os"
	"path/filepath"
)

var (
	centerX = 410
	centerY = 555
	radius  = 363.0
)

func wrap(body string) string {
	return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1024 1024">` + "\n" + body + "\n</svg>\n"
}

func Record() string {
	return wrap(fmt.Sprintf(`<defs>
<radialGradient id="d" cx="322" cy="414" r="511" gradientUnits="userSpaceOnUse">
<stop offset=".35" stop-color="#373B39"/><stop offset=".92" stop-color="#171918"/><stop offset="1" stop-color="#101211"/>
</radialGradient>
</defs>
<circle cx="%d" cy="%d" r="%.1f" fill="url(#d)"/>
<circle cx="%d" cy="%d" r="319" fill="none" stroke="#63706B" stroke-width="8"/>
<circle cx="%d" cy="%d" r="261" fill="none" stroke="#444947" stroke-width="8"/>
<circle cx="%d" cy="%d" r="197" fill="none" stroke="#5B625F" stroke-width="8"/>
<path d="M131 510A299 299 0 0 1 266 329M551 780A299 299 0 0 0 669 620" fill="none" stroke="#ADB5B2" stroke-opacity=".48" stroke-width="22" stroke-linecap="round"/>`,
		centerX, centerY, radius,
		centerX, centerY, centerX, centerY, centerX, centerY))
}

func Label() string {
	return wrap(fmt.Sprintf(`<defs>
<linearGradient id="l" x1="292" y1="450" x2="532" y2="672" gradientUnits="userSpaceOnUse">
<stop stop-color="#FFEE9C"/><stop offset="1" stop-color="#FFC53D"/>
</linearGradient>
</defs>
<circle cx="%d" cy="%d" r="136" fill="url(#l)"/>
<circle cx="%d" cy="%d" r="30" fill="#1A211E"/>`, centerX, centerY, centerX, centerY))
}

func Arm() string {
	return wrap(`<defs>
<linearGradient id="a" x1="625" y1="230" x2="842" y2="620" gradientUnits="userSpaceOnUse">
<stop stop-color="#FFFFFF"/><stop offset="1" stop-color="#E6E9E8"/>
</linearGradient>
</defs>
<path d="M832 263V438Q832 483 797 518L677 620" fill="none" stroke="#193B2D" stroke-opacity=".2" stroke-width="72" stroke-linecap="round" stroke-linejoin="round"/>
<path d="M820 245V420Q820 465 785 500L665 602" fill="none" stroke="url(#a)" stroke-width="72" stroke-linecap="round" stroke-linejoin="round"/>
<circle cx="820" cy="235" r="91" fill="url(#a)"/>
<circle cx="820" cy="235" r="36" fill="#FFC53D"/>
<path d="M660 524L730 594L655 676Q638 694 620 676L588 644Q570 626 588 608Z" fill="#FFC53D"/>`)
}

func Layers() map[string]string {
	return map[string]string{
		"record.svg": Record(),
		"label.svg":  Label(),
		"arm.svg":    Arm(),
	}
}

func WriteDocuments() error {
	for _, document := range []string{"icon/AppIcon.icon", "icon/AppIcon-dev.icon"} {
		for name, drawing := range Layers() {
			path := filepath.Join(document, "Assets", name)
			if err := os.WriteFile(path, []byte(drawing), 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}
