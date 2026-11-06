package app

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

// Radix dark step 12 provides soft, readable tints over the desktop.
var desktopLyricsColors = []struct {
	name string
	hex  string
}{
	{"月白", "#ffffff"}, // Radix white overlay endpoint.
	{"薄荷", radixGreenDark[11]},
	{"雾蓝", radixBlueDark12},
	{"淡紫", radixIrisDark12},
	{"暖杏", radixAmberDark[11]},
	{"浅粉", radixPinkDark12},
}

var desktopLyricsColorWheel = ui.ConicGradient{Stops: []ui.GradientStop{
	{Color: radixRed(false, 9), At: 0},
	{Color: radixOrange9, At: 0.125},
	{Color: radixAmber(false, 9), At: 0.25},
	{Color: radixGreen(false, 9), At: 0.375},
	{Color: radixCyan9, At: 0.5},
	{Color: radixBlue9, At: 0.625},
	{Color: radixPurple9, At: 0.75},
	{Color: radixPink9, At: 0.875},
	{Color: radixRed(false, 9), At: 1},
}}

func (s settings) desktopLyricsColor() ui.Color {
	value := strings.TrimPrefix(strings.TrimSpace(s.DesktopLyricsColor), "#")
	if len(value) == 6 || len(value) == 8 {
		if b, err := hex.DecodeString(value); err == nil {
			color := ui.RGB(b[0], b[1], b[2])
			if len(b) == 4 {
				color.A = b[3]
			}
			return color
		}
	}
	return radixPaper
}

func (a *app) setDesktopLyricsColor(color ui.Color) {
	value := fmt.Sprintf("#%02x%02x%02x", color.R, color.G, color.B)
	if color.A != 255 {
		value += fmt.Sprintf("%02x", color.A)
	}
	if color == radixPaper {
		value = ""
	}
	a.settings.DesktopLyricsColor = value
	a.settings.save()
	a.redraw()
}

// White keeps its original dim-to-bright treatment; other colors fade from
// white to the selected color as each word is sung. Opacity belongs to the whole layer.
func desktopLyricWordColor(color ui.Color, progress float32) ui.Color {
	if color == radixPaper {
		brightness := desktopLyricsRestAlpha + (desktopLyricsSungAlpha-desktopLyricsRestAlpha)*progress
		return color.Alpha(brightness)
	}
	return radixPaper.Mix(color, progress).Alpha(desktopLyricsSungAlpha)
}

func (a *app) desktopLyricsColorControl(c *ui.Context) {
	p := paletteFor(c.Theme())
	color := a.settings.desktopLyricsColor()
	ui.Row(c.Key("desktop-lyrics-colors")).AlignItems(ui.Center).Gap(4).Shrink(0).Children(func() {
		custom := true
		for _, preset := range desktopLyricsColors {
			value := ui.Hex(preset.hex)
			button := ui.ButtonBase(c.Key(preset.name)).Size(28, 28).Center().Radius(14).
				Label("桌面歌词颜色：" + preset.name).Tooltip(preset.name).Checked(color == value)
			if color == value {
				custom = false
				button.Border(1, radixGray(p.dark, 8))
			} else if button.Hovered() || button.FocusVisible() {
				button.Background(radixGray(p.dark, 3))
			}
			button.Children(func() {
				ui.Box(c).Size(18, 18).Radius(9).Background(value).Border(1, p.hairline)
			}).OnClick(func() { a.setDesktopLyricsColor(value) })
		}
		ui.Box(c).Width(1).Height(18).Background(p.hairline).MarginX(5)
		button := ui.ButtonBase(c.Key("custom")).Size(28, 28).Center().Radius(14).
			Label("自定义桌面歌词颜色").Tooltip("自定义颜色").Role(ui.RoleColorWell).Checked(custom).
			Value(fmt.Sprintf("#%02x%02x%02x%02x", color.R, color.G, color.B, color.A))
		open := ui.Local(button, "open", func() bool { return false })
		button.Expanded(*open).OnClick(func() { *open = !*open })
		if custom {
			button.Border(1, radixGray(p.dark, 8))
		} else if button.Hovered() || button.FocusVisible() {
			button.Background(radixGray(p.dark, 3))
		}
		button.Children(func() {
			ui.Box(c).Size(18, 18).Radius(9).ConicGradient(desktopLyricsColorWheel).Border(1, p.hairline)
		})
		ui.Popover(c.Key("custom-picker"), button, open, func() {
			ui.ColorPicker(c, &color).OnChange(func() { a.setDesktopLyricsColor(color) })
		}).Padding(12).Background(p.bg).Border(1, p.hairline).Shadow(0, 6, 20, 0, radixBlack(4))
	})
}
