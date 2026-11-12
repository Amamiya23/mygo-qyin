package app

import (
	"strings"
	"time"
	"unicode"

	"github.com/egoist/mygo/ui"
	"lilt/internal/lyrics"
	"lilt/internal/qqmusic"
)

// The taskbar has its own compact layout, but reads the same playback timeline
// and real word times as the floating lyrics. It never owns a playback clock.
func (d *taskbarLyrics) view(c *ui.Context) {
	c.Root().Background(ui.Transparent)
	if !d.a.settings.TaskbarLyrics || !d.visible || d.a.pl.Current() == nil {
		return
	}
	position := d.a.pl.Position()
	content := d.content(position)
	width, height := c.Size()
	left := float32(taskbarLyricsCoverInset)
	if url := d.a.pl.Current().Album.PicURL; url != "" {
		size := min(float32(34), max(1, height-4))
		if d.coverWin == nil {
			d.cover(c, url, size, height)
		}
		left += size + taskbarLyricsCoverGap
	}
	root := ui.Box(c.Key("taskbar-lyrics")).Fill().Clip()
	state := ui.Local(root, "presentation", func() desktopLyricViewState { return desktopLyricViewState{} })
	track := d.a.pl.Current()
	playing := d.a.pl.Playing()
	state.step(track.ID, track.MID, d.revision, content, position, c.Now(), playing, c.Preferences().ReduceMotion || !playing)
	var scrolling bool
	if content.text != "" && (content.index >= 0 || d.a.pl.Instrumental()) {
		layout := makeTaskbarLyricLayout(c, content, position, d.dark, height)
		available := max(1, width-left-6)
		scrolling = (layout.width > available || (!layout.preview && layout.secondaryWidth > available)) && !content.line.Timed && content.line.End > position && position >= content.line.At
		offset := float32(0)
		var previous taskbarLyricLayout
		if !state.changed.IsZero() {
			elapsed := max(0, c.Now().Sub(state.changed))
			progress := min(1, float32(elapsed)/float32(desktopLyricsFade))
			eased := 1 - (1-progress)*(1-progress)*(1-progress)
			offset = height * (1 - eased)
			previous = makeTaskbarLyricLayout(c, state.previous, position, d.dark, height)
			c.After(min(time.Second/60, desktopLyricsFade-elapsed))
		}
		root.Draw(func(p *ui.Painter, r ui.Rect) {
			viewport := ui.Rect{X: r.X + left, Y: r.Y, W: available, H: r.H}
			// Move whole blocks through a fixed viewport, keeping the two lines
			// together and preventing outgoing text from drawing over the cover.
			if offset > 0 {
				previous.draw(p, viewport, d.dark, offset-height)
			}
			layout.draw(p, viewport, d.dark, offset)
		})
	}
	if playing {
		next := desktopLyricWake(content, position)
		// LRC has no real word times. Scroll an overflowing sentence only while
		// its own interval plays, then stop; short lines stay entirely idle.
		if scrolling && (next <= 0 || next > 40*time.Millisecond) {
			next = min(40*time.Millisecond, content.line.End-position)
		}
		if next > 0 {
			c.After(next)
		}
	}
}

// Request the current album directly from the shared cache. A pending new cover
// uses its own placeholder rather than keeping the previous song's artwork.
func (d *taskbarLyrics) cover(c *ui.Context, url string, size, height float32) {
	cover := ui.ButtonBase(c.Key("taskbar-cover")).Absolute().Left(taskbarLyricsCoverInset).Top((height-size)/2).
		Size(size, size).Radius(min(6, size/2)).Background(radixGray(d.dark, 3)).Center().
		Label("歌曲控制").Cursor(ui.CursorPointer).OnClick(d.toggleControls)
	d.coverButton(c, cover, url, size)
}

func (d *taskbarLyrics) coverView(c *ui.Context) {
	c.Root().Background(ui.Transparent)
	tr := d.a.pl.Current()
	if !d.a.settings.TaskbarLyrics || !d.visible || tr == nil || tr.Album.PicURL == "" {
		return
	}
	w, h := c.Size()
	size := min(w, h)
	cover := ui.ButtonBase(c.Key("taskbar-cover")).Fill().Radius(min(6, size/2)).
		Background(radixGray(d.dark, 3)).Center().Label("歌曲控制").
		Cursor(ui.CursorPointer).OnClick(d.toggleControls)
	d.coverButton(c, cover, tr.Album.PicURL, size)
}

var taskbarControlsIcon = strokeIcon(`<rect x="3" y="4" width="18" height="16" rx="3"/><path d="M3 9h18M8 13h7M8 16h3"/><circle cx="16.5" cy="16" r="1.5"/>`)

func (d *taskbarLyrics) coverButton(c *ui.Context, cover ui.Element, url string, size float32) {
	active := float32(0)
	if cover.Hovered() || cover.FocusVisible() || d.controlsOpen {
		active = 1
	}
	active = cover.AnimateTo("interaction", active, ui.Animation{Duration: 120 * time.Millisecond})
	shade := radixBlack(7)
	if cover.Pressed() {
		shade = radixBlack(9)
	}
	cover.Children(func() {
		d.coverImage(c, url, size)
		if active > 0 {
			ui.Box(c.Key("control-overlay")).Absolute().Fill().Radius(min(6, size/2)).
				Background(shade).Opacity(active).Center().Children(func() {
				ui.Icon(c, taskbarControlsIcon).Size(size*0.55, size*0.55).TextColor(radixWhite(12))
			})
		}
	})
}

func (d *taskbarLyrics) coverImage(c *ui.Context, url string, size float32) {
	bmp := bitmap(d.a.art.get(qqmusic.Sized(url, 96), artCover))
	if bmp != nil {
		ui.Image(c, bmp).Fill().Fit(ui.Cover).Radius(min(6, size/2))
	} else {
		ui.Icon(c, iconNote).Size(size*0.4, size*0.4).TextColor(radixGray(d.dark, 9))
	}
}

func (d *taskbarLyrics) content(position time.Duration) desktopLyricContent {
	if d.a.pl.Current() == nil {
		return desktopLyricContent{index: -1}
	}
	lines, loaded := d.a.pl.Lyrics()
	if loaded && d.a.pl.Instrumental() {
		return desktopLyricContent{index: -1, text: "纯音乐，请欣赏"}
	}
	return lyricAt(lines, loaded, position, true, d.translationPolicy.get(lines, taskbarLyricsTranslation))
}

// Han lyrics without a translation use current/next even when they contain none
// of the simplified forms recognized by the desktop's conservative policy.
// Preserve distinct translations and Japanese/Korean originals; credits do not
// establish the language of the sung text.
func taskbarLyricsTranslation(lines []lyrics.Line) bool {
	if !desktopLyricsTranslation(lines) {
		return false
	}
	han := false
	for _, line := range lines {
		if lyrics.IsCredit(line.Text) {
			continue
		}
		if text := desktopLyricComparable(line.Translation); text != "" && text != desktopLyricComparable(line.Text) {
			return true
		}
		for _, r := range line.Text {
			if unicode.In(r, unicode.Hiragana, unicode.Katakana, unicode.Hangul) {
				return true
			}
			han = han || unicode.Is(unicode.Han, r)
		}
	}
	return !han
}

type taskbarLyricLayout struct {
	text, outline, secondary, secondaryOutline    *ui.TextLayout
	width, secondaryWidth, height, originalHeight float32
	progressX, progress                           float32
	preview                                       bool
	words                                         *lyricWordGeometry
	wordColors                                    []ui.Color
}

func makeTaskbarLyricLayout(c *ui.Context, content desktopLyricContent, position time.Duration, dark bool, height float32) taskbarLyricLayout {
	if content.text == "" {
		return taskbarLyricLayout{}
	}
	layout := retainedTaskbarLyricLayout(c, content, dark, height)
	if layout.words != nil {
		color := radixGray(dark, 12)
		layout.wordColors = make([]ui.Color, len(content.line.Words))
		for i, word := range content.line.Words {
			brightness := desktopLyricsRestAlpha + (desktopLyricsSungAlpha-desktopLyricsRestAlpha)*word.Progress(position)
			layout.wordColors[i] = color.Alpha(brightness)
		}
		for i, word := range content.line.Words {
			if position < word.Start {
				break
			}
			left, right := layout.words.left[i], layout.words.right[i]
			layout.progressX = left + (right-left)*word.Progress(position)
		}
		if layout.width > 0 {
			layout.progress = min(1, max(0, layout.progressX/layout.width))
		}
	} else if content.line.End > content.line.At {
		layout.progress = min(1, max(0, float32(position-content.line.At)/float32(content.line.End-content.line.At)))
		layout.progressX = layout.width * layout.progress
	}
	return layout
}

var taskbarLyricLineReplacer = strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ", "\t", " ")

func taskbarLyricSingleLine(text string) string { return taskbarLyricLineReplacer.Replace(text) }

// Keep the sung word visible and leave short sentences left aligned. The clamp is
// shared by the original and translation so seeking never leaves stale scroll.
func taskbarLyricOffset(width, available, progressX float32) float32 {
	if width <= available {
		return 0
	}
	return -min(width-available, max(0, progressX-available*0.55))
}

func (layout taskbarLyricLayout) draw(p *ui.Painter, r ui.Rect, dark bool, offsetY float32) {
	if layout.text == nil {
		return
	}
	y := r.Y + (r.H-layout.height)/2 + offsetY
	x := r.X + taskbarLyricOffset(layout.width, r.W, layout.progressX)
	outline := radixGray(dark, 1).Alpha(0.8)
	p.Clip(r, 0, func() {
		// A fine contrasting edge remains legible on translucent taskbars.
		for _, offset := range []ui.Point{{X: -0.5}, {X: 0.5}, {Y: -0.5}, {Y: 0.5}} {
			p.TextLayout(layout.outline, x+offset.X, y+offset.Y, outline)
		}
		p.TextLayoutColors(layout.text, x, y, radixGray(dark, 12), layout.wordColors)
		if layout.secondary != nil {
			w, progress := layout.secondaryWidth, layout.progress
			color := radixGray(dark, 12)
			if layout.preview {
				progress, color = 0, color.Alpha(desktopLyricsRestAlpha)
			}
			x = r.X + taskbarLyricOffset(w, r.W, w*progress)
			y += layout.originalHeight + 1
			for _, offset := range []ui.Point{{X: -0.5}, {X: 0.5}, {Y: -0.5}, {Y: 0.5}} {
				p.TextLayout(layout.secondaryOutline, x+offset.X, y+offset.Y, outline)
			}
			p.TextLayout(layout.secondary, x, y, color)
		}
	})
}
