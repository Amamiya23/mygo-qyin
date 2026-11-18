package app

import (
	"runtime"
	"time"

	"github.com/egoist/mygo/ui"
)

type releaseInfoHover struct {
	entered, left             time.Time
	open, keyboard, dismissed bool
}

func (a *app) releaseInfoButton(c *ui.Context, notice string) {
	switch runtime.GOOS {
	case "windows", "darwin", "linux":
	default:
		return
	}
	if notice == "" {
		return
	}

	b := ui.ButtonBase(c.Key("release-info")).Height(36).PaddingX(8).Radius(8).Center().Label("发行信息")
	mediaFeedback(b, a.pal.muted)
	b.Children(func() { ui.Text(c, "发行信息").FontSize(12.5).SingleLine() })
	s := ui.Local(b, "hover", func() releaseInfoHover { return releaseInfoHover{} })
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

	panel := ui.PopoverBase(c.Key("release-info-card"), b, &s.open, func(panel ui.Element) {
		w, h := c.Size()
		width := min(float32(480), max(float32(64), w-32))
		const size, leading, gutter = 13, 1.6, 12
		font := ui.Font{Family: c.Theme().Font, Size: size, LineHeight: leading, Lang: c.Locale()}
		_, height := ui.ShapeText(notice, font, width-32).Size()
		a.styleReadingPopover(panel, width).Label("发行信息详情")
		ui.Scroll(c.Key("release-info-scroll")).Height(min(height, max(size*leading, min(float32(320), h*0.45)-32))).
			Padding(0, gutter, 0, 0).Focusable().Label("发行信息内容").Children(func() {
			ui.Text(c, notice).FontSize(size).LineHeight(leading).TextColor(a.pal.text).Selectable()
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
