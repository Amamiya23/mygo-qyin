package app

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

const (
	descriptionDelay = 500 * time.Millisecond
	descriptionGrace = 180 * time.Millisecond
)

type descriptionHover struct {
	text          string
	font          ui.Font
	width         float32
	overflow      bool
	open          bool
	dismissed     bool
	entered, left time.Time
	popupWidth    float32
	popupHeight   float32
}

// collectionDescription keeps the header compact while a hover card makes the full prose readable.
func (a *app) collectionDescription(c *ui.Context, description string) {
	description = strings.TrimSpace(strings.ReplaceAll(description, "\r\n", "\n"))
	if description == "" {
		return
	}
	p := a.pal
	preview := strings.ReplaceAll(description, "\n", " ")
	anchor := ui.Text(c.Key("description"), preview).
		FontSize(12.5).TextColor(p.muted).MaxLines(2).LineHeight(1.45).Margin(2, 0, 0, 0).Label("简介").Description(description)
	s := ui.Local(anchor, "hover", func() descriptionHover { return descriptionHover{} })
	hovered := anchor.Hovered()
	width := anchor.LayoutBounds().W
	if width <= 0 {
		c.Invalidate()
		return
	}
	font := ui.Font{Family: c.Theme().Font, Size: 12.5, LineHeight: 1.45, Lang: c.Locale()}
	if s.text != description || s.width != width || s.font != font {
		_, height := ui.ShapeText(preview, font, width).Size()
		*s = descriptionHover{text: description, font: font, width: width, overflow: height > 2*font.Size*font.LineHeight+0.5}
	}
	if !s.overflow {
		return
	}
	anchor.Focusable()
	focused := anchor.FocusVisible()
	now := c.Now()
	if hovered || focused {
		if s.entered.IsZero() {
			s.entered = now
		}
		if !s.dismissed {
			if wait := descriptionDelay - now.Sub(s.entered); wait > 0 && !focused {
				c.After(wait)
			} else {
				s.open = true
			}
		}
	} else {
		s.entered = time.Time{}
	}
	wasOpen := s.open
	panel := ui.PopoverBase(c.Key("description-card"), anchor, &s.open, func(panel ui.Element) {
		w, h := c.Size()
		cardWidth := min(float32(480), w-32)
		if s.popupWidth != cardWidth {
			s.popupWidth = cardWidth
			_, s.popupHeight = ui.ShapeText(description, ui.Font{Family: font.Family, Size: 13, LineHeight: 1.6, Lang: font.Lang}, cardWidth-32).Size()
		}
		panel.Width(cardWidth).Padding(16).Radius(12).Margin(8, 0, 0, 0).
			Background(radixSage(p.dark, 2)).Border(1, p.hairline).Shadow(0, 8, 28, -6, p.cardShade).Label("完整简介")
		ui.Scroll(c.Key("description-scroll")).Height(min(s.popupHeight, min(float32(320), h*0.45)-32)).Focusable().Label("简介内容").Children(func() {
			ui.Text(c, description).FontSize(13).TextColor(p.text).LineHeight(1.6).Selectable()
		})
	})
	if wasOpen && !s.open {
		s.dismissed = true
	}
	panelHovered, panelFocused := panel.Hovered(), panel.FocusWithin()
	if hovered || focused || panelHovered || panelFocused {
		s.left = time.Time{}
		return
	}
	s.dismissed = false
	if s.open {
		if s.left.IsZero() {
			s.left = now
		}
		if wait := descriptionGrace - now.Sub(s.left); wait > 0 {
			c.After(wait)
		} else {
			s.open = false
			c.Invalidate()
		}
	}
}
