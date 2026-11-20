package app

import (
	"fmt"
	"math"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

type albumMetaFit struct {
	text  string
	font  ui.Font
	width float32
	fits  bool
}

func normalizeReleaseNotice(notice string) string {
	return strings.TrimSpace(strings.ReplaceAll(notice, "\r\n", "\n"))
}

func (a *app) albumMetaRow(c *ui.Context, al *qqmusic.AlbumProfile) {
	p := a.pal
	var meta string
	if al.PublishTime > 0 {
		meta = time.UnixMilli(al.PublishTime).Format("2006-01-02") + " · "
	}
	meta += fmt.Sprintf("%d 首", len(al.Songs))
	if len(al.Artists) > 0 {
		meta = " · " + meta
	}
	notice := normalizeReleaseNotice(al.Company)
	row := ui.Row(c.Key("meta-row")).Gap(4).AlignItems(ui.Center)
	if notice != "" {
		row.MinHeight(22)
	}
	inline, showButton := a.albumMetaNoticeMode(c, row, al.ArtistNames()+meta, notice)
	row.Children(func() {
		ui.RichText(c).Shrink(1).MinWidth(0).FontSize(13).TextColor(p.muted).Children(func() {
			for i, ar := range al.Artists {
				if i > 0 {
					ui.Text(c, ", ")
				}
				a.link(c.Key(i), ar.Name, p.muted, p.text, route{page: pageArtist, id: ar.ID})
			}
			if inline {
				ui.Text(c, meta+" · "+notice)
			} else {
				ui.Text(c, meta)
			}
		})
		if showButton {
			a.releaseInfoButton(c, notice)
		}
	})
}

func (a *app) albumMetaNoticeMode(c *ui.Context, row ui.Element, prefix, notice string) (inline, showButton bool) {
	if notice == "" {
		return false, false
	}
	if strings.Contains(notice, "\n") {
		return false, true
	}
	width := row.LayoutBounds().W
	if width <= 0 {
		c.Invalidate()
		return false, false
	}
	fullLine := prefix + " · " + notice
	font := ui.Font{Family: c.Theme().Font, Size: 13, Lang: c.Locale()}
	s := ui.Local(row, "meta-fit", func() albumMetaFit { return albumMetaFit{} })
	if s.text != fullLine || s.font != font || s.width != width {
		lineW, _ := ui.ShapeText(fullLine, font, 0).Size()
		*s = albumMetaFit{text: fullLine, font: font, width: width, fits: lineW <= width-2}
	}
	if s.fits {
		return true, false
	}
	return false, true
}

type releaseInfoHover struct {
	notice                    string
	font                      ui.Font
	maxWidth                  float32
	popupWidth                float32
	popupHeight               float32
	entered, left             time.Time
	open, keyboard, dismissed bool
}

func (a *app) releaseInfoButton(c *ui.Context, notice string) {
	switch runtime.GOOS {
	case "windows", "darwin", "linux":
	default:
		return
	}
	notice = normalizeReleaseNotice(notice)
	if notice == "" {
		return
	}

	b := ui.ButtonBase(c.Key("release-info")).Size(22, 22).Radius(999).Center().Label("发行信息")
	s := ui.Local(b, "hover", func() releaseInfoHover { return releaseInfoHover{} })
	if s.notice != notice {
		*s = releaseInfoHover{notice: notice}
	}
	hovered, now := b.Hovered(), c.Now()
	if !hovered {
		s.entered, s.dismissed = time.Time{}, false
	} else if !s.open && !s.dismissed {
		if s.entered.IsZero() {
			s.entered = now
		}
		if wait := readingHoverDelay - now.Sub(s.entered); wait > 0 {
			c.After(wait)
		} else {
			s.open = true
		}
	}
	if b.Clicked() {
		s.open, s.keyboard = true, b.FocusVisible()
		s.left = time.Time{}
	}

	p := a.pal
	bg, fg := ui.Transparent, p.faint.Alpha(0.75)
	if hovered || b.FocusVisible() || s.open {
		bg, fg = p.hover, p.text
	}
	pose := ui.Pose{}
	if b.Pressed() {
		bg, fg, pose.Scale = p.press, p.text, 0.92
	}
	motion := ui.Animation{Duration: 120 * time.Millisecond}
	b.Background(b.AnimateColor("bg", bg, motion)).
		TextColor(b.AnimateColor("fg", fg, motion)).
		AnimatePose(pose, ui.Snappy).
		Children(func() { ui.Icon(c, iconInfo).Size(14, 14) })

	panel := ui.PopoverBase(c.Key("release-info-card"), b, &s.open, func(panel ui.Element) {
		w, h := c.Size()
		maxWidth := min(float32(480), max(float32(64), w-32))
		const size, leading, gutter, chrome = 13, 1.6, 12, 34
		font := ui.Font{Family: c.Theme().Font, Size: size, LineHeight: leading, Lang: c.Locale()}
		if s.font != font || s.maxWidth != maxWidth || s.popupWidth == 0 {
			textW, textH := ui.ShapeText(notice, font, maxWidth-chrome).Size()
			s.font, s.maxWidth = font, maxWidth
			s.popupWidth = min(maxWidth, max(float32(64), float32(math.Ceil(float64(textW)))+chrome+2))
			s.popupHeight = float32(math.Ceil(float64(textH)))
		}
		a.styleReadingPopover(panel, s.popupWidth).Label("发行信息详情")
		ui.Scroll(c.Key("release-info-scroll")).Height(min(s.popupHeight, max(size*leading, min(float32(320), h*0.45)-32))).
			Padding(0, gutter, 0, 0).Focusable().Label("发行信息内容").Children(func() {
			ui.Text(c, notice).FontSize(size).LineHeight(leading).TextColor(p.text).Selectable()
		})
	})
	if panel.Dismissed() {
		s.keyboard, s.dismissed, s.left = false, hovered, time.Time{}
		if panel.FocusWithin() && !panel.PressedOutside() {
			b.Focus()
		}
		return
	}
	// Keep a keyboard-opened panel readable while focus moves into it.
	if s.keyboard && (panel.PressedInside() || !b.Focused() && !panel.FocusWithin()) {
		s.keyboard = false
	}
	if hovered || panel.Hovered() || s.keyboard {
		s.left = time.Time{}
		return
	}
	if !s.open {
		return
	}
	if s.left.IsZero() {
		s.left = now
	}
	if wait := readingHoverGrace - now.Sub(s.left); wait > 0 {
		c.After(wait)
	} else {
		s.open = false
		c.Invalidate()
	}
}
