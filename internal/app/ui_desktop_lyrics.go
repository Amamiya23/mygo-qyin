package app

import (
	"math"
	"strings"
	"time"
	"unicode"

	"github.com/egoist/mygo/ui"

	"lilt/internal/lyrics"
)

const desktopLyricsToolbarHeight float32 = 44
const desktopLyricsFade = 240 * time.Millisecond
const desktopLyricsRestAlpha float32 = 0.478
const desktopLyricsSungAlpha float32 = 0.95

// desktopLyricContent is a snapshot of the player's timeline, never a second clock.
type desktopLyricContent struct {
	line              lyrics.Line
	preview           string
	index             int
	text, translation string
	next              time.Duration
}

// desktopLyricAt keeps the current line through instrumental gaps until its successor starts.
func desktopLyricAt(lines []lyrics.Line, loaded bool, position time.Duration, doubleLine bool) desktopLyricContent {
	return lyricAt(lines, loaded, position, doubleLine, desktopLyricsTranslation(lines))
}

func lyricAt(lines []lyrics.Line, loaded bool, position time.Duration, doubleLine, translation bool) desktopLyricContent {
	out := desktopLyricContent{index: -1}
	if !loaded {
		out.text = "歌词加载中…"
		return out
	}
	if len(lines) == 0 {
		out.text = "暂无歌词"
		return out
	}
	index := lyrics.Index(lines, position)
	if index+1 < len(lines) {
		out.next = lines[index+1].At - position
	}
	if index < 0 {
		return out
	}
	line := lines[index]
	out.line, out.index, out.text = line, index, line.Text
	if translation {
		out.translation = line.Translation
	} else if doubleLine && index+1 < len(lines) {
		out.preview = lines[index+1].Text
	}
	if desktopLyricComparable(out.translation) == desktopLyricComparable(out.text) {
		out.translation = ""
	}
	return out
}

func desktopLyricComparable(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, text)
}

// Track language is not available in QQ's current model. Kana or Hangul anywhere
// in the lyrics keeps translations, even for a Han-only line. The small set below
// contains common simplified Chinese forms absent from Japanese shinjitai; Han
// alone (including traditional Chinese) remains unknown and keeps distinct text.
func desktopLyricsTranslation(lines []lyrics.Line) bool {
	chinese := false
	for _, line := range lines {
		for _, r := range line.Text {
			if unicode.In(r, unicode.Hiragana, unicode.Katakana, unicode.Hangul) {
				return true
			}
			if strings.ContainsRune("们这说吗别该爱谢为对妈亲谁让", r) {
				chinese = true
			}
		}
	}
	return !chinese
}

// desktopLyricWake waits through genuine word gaps instead of keeping a 25 Hz loop.
func desktopLyricWake(content desktopLyricContent, position time.Duration) time.Duration {
	next := content.next
	if content.index < 0 || !content.line.Timed {
		return next
	}
	for _, word := range content.line.Words {
		delay := time.Duration(0)
		if position < word.Start {
			delay = word.Start - position
		} else if position < word.End {
			delay = min(40*time.Millisecond, word.End-position)
		}
		if delay > 0 && (next == 0 || delay < next) {
			next = delay
		}
	}
	return next
}

type desktopLyricViewState struct {
	track             int64
	mid               string
	revision          uint64
	current, previous desktopLyricContent
	changed, seen     time.Time
	position          time.Duration
	initialized       bool
}

type desktopLyricBlend struct {
	currentAlpha, previousAlpha float32
	currentY, previousY         float32
}

// Clear the outgoing sentence before revealing the new one, avoiding the
// illegible double image of two outlined sentences crossfading in place.
func (s *desktopLyricViewState) blend(c *ui.Context) desktopLyricBlend {
	if s.changed.IsZero() {
		return desktopLyricBlend{currentAlpha: 1}
	}
	elapsed := max(0, c.Now().Sub(s.changed))
	if elapsed >= desktopLyricsFade {
		return desktopLyricBlend{currentAlpha: 1}
	}
	c.After(min(time.Second/60, desktopLyricsFade-elapsed))
	exit := time.Duration(0)
	out := float32(0)
	if s.previous.text != "" {
		exit = 60 * time.Millisecond
		p := min(1, float32(elapsed)/float32(exit))
		out = 1 - p*p
	}
	p := max(0, float32(elapsed-exit)/float32(desktopLyricsFade-exit))
	in := 1 - (1-p)*(1-p)*(1-p)
	return desktopLyricBlend{currentAlpha: in, previousAlpha: out,
		currentY: 4 * (1 - in), previousY: -2 * (1 - out)}
}

func (s *desktopLyricViewState) step(track int64, mid string, revision uint64, content desktopLyricContent, position time.Duration, now time.Time, playing, reduced bool) {
	changed := !s.initialized || s.track != track || s.mid != mid || s.revision != revision
	jump := position < s.position
	if s.initialized && !changed && playing {
		drift := (position - s.position) - now.Sub(s.seen)
		jump = jump || drift > 500*time.Millisecond || drift < -500*time.Millisecond
	} else if s.initialized && position != s.position {
		jump = true
	}
	different := content.index != s.current.index || content.text != s.current.text || content.translation != s.current.translation || content.preview != s.current.preview
	if changed || jump || reduced {
		s.previous, s.changed = desktopLyricContent{}, time.Time{}
	} else if different {
		s.previous, s.changed = s.current, now
	}
	if !s.changed.IsZero() && now.Sub(s.changed) >= desktopLyricsFade {
		s.previous, s.changed = desktopLyricContent{}, time.Time{}
	}
	s.track, s.mid, s.current, s.position, s.seen, s.initialized = track, mid, content, position, now, true
	s.revision = revision
}

func (d *desktopLyrics) view(c *ui.Context) {
	c.Root().Background(ui.Transparent)
	if !d.a.settings.DesktopLyrics {
		return
	}
	root := ui.Box(c.Key("desktop-lyrics")).Fill().TextColor(radixWhite(12))
	state := ui.Local(root, "presentation", func() desktopLyricViewState { return desktopLyricViewState{} })
	unlocked := !d.a.settings.DesktopLyricsLocked
	if unlocked {
		root.DragWindow()
	}
	hovered := unlocked && root.Hovered()
	if hovered {
		root.Background(radixBlack(4)).Radius(16).Border(1, radixWhite(2))
	}
	width, windowHeight := c.Size()
	textWidth := max(width-48, 1)
	size := d.fontSize()
	content := desktopLyricContent{index: -1}
	var track int64
	var mid string
	if tr := d.a.pl.Current(); tr != nil {
		track, mid = tr.ID, tr.MID
		lines, loaded := d.a.pl.Lyrics()
		content = lyricAt(lines, loaded, d.a.pl.Position(), d.a.settings.DesktopLyricsDoubleLine, d.translationPolicy.get(lines, desktopLyricsTranslation))
	}
	position := d.a.pl.Position()
	playing := d.a.pl.Playing()
	state.step(track, mid, d.revision, content, position, c.Now(), playing, c.Preferences().ReduceMotion)
	blend := state.blend(c)
	color := d.a.settings.desktopLyricsColor()
	current := desktopLyricsLayouts(c, content, position, size, textWidth, color)
	previous := desktopLyricsLayouts(c, state.previous, position, size, textWidth, color)
	height := max(current.height, previous.height)
	d.fitHeight(float64(desktopLyricsToolbarHeight + height + 28))
	// Use the committed viewport until the deferred native resize completes.
	// A layout-height change must not move a line for that intervening frame.
	bodyHeight := max(0, windowHeight-desktopLyricsToolbarHeight-28)
	root.Children(func() {
		ui.Box(c.Key("toolbar-space")).Absolute().Top(4).Left(16).Right(16).Height(36).Children(func() {
			if hovered {
				d.toolbar(c)
			}
		})
		body := ui.Box(c.Key("lyric-body")).Absolute().Top(desktopLyricsToolbarHeight).Left(24).Right(24).Height(bodyHeight).Label(content.text)
		if unlocked {
			body.Cursor(ui.CursorGrab)
		}
		body.Children(func() {
			drawDesktopLyricLayers(c, current, previous, bodyHeight, blend)
		})
		if unlocked {
			d.widthGrips(c, hovered)
		}
	})
	if playing {
		if next := desktopLyricWake(content, position); next > 0 {
			c.After(next)
		}
	}
}

func drawDesktopLyricLayers(c *ui.Context, current, previous desktopLyricLayouts, height float32, blend desktopLyricBlend) {
	layer := func(key string, layout desktopLyricLayouts, alpha, offset float32) {
		alpha *= layout.opacity
		if layout.text == nil || alpha <= 0 {
			return
		}
		ui.Box(c.Key(key)).Absolute().Top(0).Left(0).Right(0).Height(height).Opacity(alpha).
			Draw(func(p *ui.Painter, r ui.Rect) {
				// The window keeps its bottom edge when it fits a shorter line.
				// Bottom-align both blocks so removing the old height cannot
				// make the new sentence jump after the transition completes.
				r.Y += r.H - layout.height + offset
				layout.draw(p, r, 1)
			})
	}
	layer("previous", previous, blend.previousAlpha, blend.previousY)
	layer("current", current, blend.currentAlpha, blend.currentY)
}

var desktopLyricsLockIcon = strokeIcon(`<rect x="5" y="10" width="14" height="11" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3"/>`)
var desktopLyricsMinusIcon = strokeIcon(`<path d="M5 12h14"/>`)

func (d *desktopLyrics) toolbar(c *ui.Context) {
	ui.Row(c.Key("toolbar")).Height(36).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Padding(1, 6).Gap(2).AlignItems(ui.Center).Radius(12).Background(radixBlack(10)).Border(1, radixWhite(3)).Children(func() {
			desktopLyricsButton(c.Key("previous"), iconPrev, "上一首").OnClick(func() { d.a.pl.Prev(); d.a.redraw() })
			icon := iconPlay
			if d.a.pl.Playing() {
				icon = iconPause
			}
			desktopLyricsButton(c.Key("play"), icon, "播放/暂停").OnClick(func() { d.a.pl.TogglePlay(); d.a.redraw() })
			desktopLyricsButton(c.Key("next"), iconNext, "下一首").OnClick(func() { d.a.pl.Next(); d.a.redraw() })
			ui.Box(c).Width(1).Height(16).Background(radixWhite(4)).MarginX(5)
			desktopLyricsButton(c.Key("smaller"), desktopLyricsMinusIcon, "减小桌面歌词字号").Disabled(d.fontSize() <= 18).OnClick(func() { d.setFontSize(d.fontSize() - 2) })
			desktopLyricsButton(c.Key("larger"), iconPlus, "增大桌面歌词字号").Disabled(d.fontSize() >= 64).OnClick(func() { d.setFontSize(d.fontSize() + 2) })
			desktopLyricsButton(c.Key("lock"), desktopLyricsLockIcon, "锁定桌面歌词").OnClick(func() { d.setLocked(true) })
			desktopLyricsButton(c.Key("close"), iconX, "关闭桌面歌词").OnClick(d.close)
		})
	})
}

func desktopLyricsButton(c *ui.Context, icon *ui.SVG, label string) ui.Element {
	button := ui.ButtonBase(c).Size(32, 32).Center().Radius(8).Label(label).Tooltip(label).TextColor(radixWhite(12))
	if button.Hovered() || button.FocusVisible() {
		button.Background(radixWhite(3))
	}
	if button.Pressed() {
		button.Background(radixWhite(4))
	}
	return button.Children(func() { ui.Icon(c, icon).Size(17, 17) })
}

func (d *desktopLyrics) widthGrips(c *ui.Context, shown bool) {
	grip := ui.ButtonBase(c.Key("width-grip")).Absolute().Right(0).Top(desktopLyricsToolbarHeight).Bottom(8).Width(16).Cursor(ui.CursorResizeEW).Label("调整桌面歌词宽度")
	if shown {
		grip.Draw(func(p *ui.Painter, r ui.Rect) {
			p.Fill(ui.Rect{X: r.X + 7, Y: r.Y + r.H/2 - 12, W: 2, H: 24}, radixWhite(5), 1)
		})
	}
	if dx, _, dragging := grip.Dragged(); dragging && dx != 0 {
		d.resizeWidth(float64(dx))
	}
}

type desktopLyricLayouts struct {
	outline, text, translationOutline, translation *ui.TextLayout
	previewOutline, preview                        *ui.TextLayout
	originalHeight, height                         float32
	opacity                                        float32
	wordColors                                     []ui.Color
}

func desktopLyricsLayouts(c *ui.Context, content desktopLyricContent, position time.Duration, size, width float32, color ui.Color) desktopLyricLayouts {
	if content.text == "" {
		return desktopLyricLayouts{}
	}
	font := ui.Font{Family: c.Theme().Font, Size: size, Weight: 700, LineHeight: 1.3}
	shape := ui.TextShape{Width: width, Align: ui.Center}
	text := content.text
	// Rich-text spans use zero alpha to mean inherited color. Apply the
	// user's opacity to the complete layer, including its outline, instead.
	opacity := float32(color.A) / 255
	color = ui.RGB(color.R, color.G, color.B)
	layout := desktopLyricLayouts{outline: retainedLyricText(c, text, font, shape, ui.Color{}), opacity: opacity}
	if content.line.Timed && len(content.line.Words) > 0 {
		layout.text = retainedLyricWords(c, content.line.Words, font, shape, false).layout
		layout.wordColors = make([]ui.Color, len(content.line.Words))
		for i, word := range content.line.Words {
			layout.wordColors[i] = desktopLyricWordColor(color, word.Progress(position))
		}
	} else {
		layout.text = retainedLyricText(c, text, font, shape, desktopLyricWordColor(color, 1))
	}
	_, layout.originalHeight = layout.text.Size()
	layout.height = layout.originalHeight
	if content.translation != "" {
		font.Size, font.Weight = size*0.625, 500
		layout.translationOutline = retainedLyricText(c, content.translation, font, shape, ui.Color{})
		layout.translation = layout.translationOutline
		_, h := layout.translation.Size()
		layout.height += h + 6
	} else if content.preview != "" {
		layout.previewOutline = retainedLyricText(c, content.preview, font, shape, ui.Color{})
		layout.preview = retainedLyricText(c, content.preview, font, shape, desktopLyricWordColor(color, 0))
		_, h := layout.preview.Size()
		layout.height += h + 6
	}
	return layout
}

func (layout desktopLyricLayouts) draw(p *ui.Painter, r ui.Rect, alpha float32) {
	if layout.text == nil || alpha <= 0 {
		return
	}
	draw := func(outline, text *ui.TextLayout, y float32, color ui.Color, colors []ui.Color) {
		if text == nil {
			return
		}
		p.TextLayout(outline, r.X, y+2, radixBlack(7).Alpha(0.5*alpha))
		for i := 0; i < 8; i++ {
			angle := float64(i) * math.Pi / 4
			p.TextLayout(outline, r.X+float32(math.Cos(angle))*0.8, y+float32(math.Sin(angle))*0.8, radixBlack(12).Alpha(0.95*alpha))
		}
		p.TextLayoutColors(text, r.X, y, color, colors)
	}
	draw(layout.outline, layout.text, r.Y, radixWhite(12), layout.wordColors)
	draw(layout.translationOutline, layout.translation, r.Y+layout.originalHeight+6, radixGray(true, 12), nil)
	draw(layout.previewOutline, layout.preview, r.Y+layout.originalHeight+6, radixWhite(12), nil)
}
