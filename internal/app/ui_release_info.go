package app

import (
	"math"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

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
	notice = strings.TrimSpace(strings.ReplaceAll(notice, "\r\n", "\n"))
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
