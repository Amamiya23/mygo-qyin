package app

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"lilt/internal/lyrics"
	"lilt/internal/qqmusic"
)

func taskbarLyricsViewFixture() (*app, *taskbarLyrics) {
	a, _ := desktopLyricsViewFixture()
	a.settings.TaskbarLyrics = true
	d := &taskbarLyrics{a: a, visible: true, dark: true}
	return a, d
}

// Paused seeks move an overflowing timed sentence to the sung word and back without a stale scroll offset.
func TestTaskbarLyricsPausedSeek(t *testing.T) {
	a, d := taskbarLyricsViewFixture()
	a.pl.SetLyrics(lyrics.Build("", "", "[1000,10000]我们一起走过漫长道路(1000,5000)直到音乐带我们走向远方(6000,5000)"))
	var offset float32
	tt := ui.NewTester(func(c *ui.Context) {
		content := d.content(a.pl.Position())
		layout := makeTaskbarLyricLayout(c, content, a.pl.Position(), d.dark, 42)
		offset = taskbarLyricOffset(layout.width, 108, layout.progressX)
		d.view(c)
	}, 120, 42)
	a.pl.Seek(time.Second)
	tt.Frame()
	start := offset
	a.pl.Seek(10 * time.Second)
	tt.Frame()
	if offset >= start || offset >= 0 {
		t.Fatalf("forward seek did not reveal later words: start=%v after=%v", start, offset)
	}
	a.pl.Seek(time.Second)
	tt.Frame()
	if offset != start || a.pl.Playing() {
		t.Fatalf("paused backward seek retained scroll or resumed audio: offset=%v want=%v playing=%t", offset, start, a.pl.Playing())
	}
}

// The taskbar used to omit Chinese previews and shrink translations; retain equally sized rows even on a compact taskbar.
func TestTaskbarLyricsTwoLineContent(t *testing.T) {
	for _, tc := range []struct {
		name, lrc, translation, preview, translated string
	}{
		{"Chinese preview", "[00:01]我们一起听音乐\n[00:10]让歌声陪伴远方", "", "让歌声陪伴远方", ""},
		// 快乐的扑满 contains none of the old simplified-character whitelist.
		{"Chinese without whitelist characters", "[00:44.16]那个样子\n[00:48.39]友情 在积蓄", "", "友情 在积蓄", ""},
		{"traditional Chinese without translation", "[00:01]山河歲月\n[00:10]風雨同行", "", "風雨同行", ""},
		{"Han with duplicate translation", "[00:01]那个样子\n[00:10]友情 在积蓄", "[00:01]那个，样子。", "友情 在积蓄", ""},
		{"Han with distinct translation", "[00:01]学校 国体\n[00:10]Next line", "[00:01]学校与国体", "", "学校与国体"},
		{"Chinese with Japanese credit", "[00:00]作曲：たなか\n[00:01]那个样子\n[00:10]友情 在积蓄", "", "友情 在积蓄", ""},
		{"Japanese without translation", "[00:01]愛\n[00:10]君をのせて", "", "", ""},
		{"Korean without translation", "[00:01]사랑해\n[00:10]다시", "", "", ""},
		{"foreign translation", "[00:01]Stay with me\n[00:10]Through the night", "[00:01]留在我身边\n[00:10]陪伴漫漫长夜", "", "留在我身边"},
		{"foreign without translation", "[00:01]Stay with me\n[00:10]Through the night", "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, d := taskbarLyricsViewFixture()
			a.pl.SetLyrics(lyrics.Build(tc.lrc, tc.translation, ""))
			lines, _ := a.pl.Lyrics()
			for _, line := range lines {
				if !lyrics.IsCredit(line.Text) {
					a.pl.Seek(line.At + time.Second)
					break
				}
			}
			content := d.content(a.pl.Position())
			if content.preview != tc.preview || content.translation != tc.translated {
				t.Fatalf("wrong secondary sentence: preview=%q translation=%q", content.preview, content.translation)
			}
			for _, height := range []int{28, 42} {
				tt := ui.NewTester(func(c *ui.Context) {
					layout := makeTaskbarLyricLayout(c, content, a.pl.Position(), d.dark, float32(height))
					if tc.preview != "" || tc.translated != "" {
						if layout.secondary == nil {
							t.Fatal("compact layout discarded the second sentence")
						}
						_, secondaryHeight := layout.secondary.Size()
						if secondaryHeight != layout.originalHeight || layout.height > float32(height) {
							t.Fatalf("rows differ in size or exceed taskbar: first=%v second=%v total=%v available=%d", layout.originalHeight, secondaryHeight, layout.height, height)
						}
					} else if layout.secondary != nil {
						t.Fatal("foreign song without translation borrowed the next sentence")
					}
					d.view(c)
				}, 280, height)
				tt.Frame()
				if !taskbarFrameHasInk(tt.Image()) {
					t.Fatal("two-line layout stopped painting")
				}
			}
			a.pl.Seek(lines[len(lines)-1].At + time.Second)
			after := d.content(a.pl.Position())
			if tc.preview != "" {
				if after.activeRow != 1 || after.text != content.text || after.preview != content.preview {
					t.Fatalf("seek to second row of pair did not keep both rows in place: %+v", after)
				}
			} else if after.preview != "" || after.text == content.text {
				t.Fatalf("seek kept an old preview or current line: %+v", after)
			}
		})
	}
}

// Row 2 in paired taskbar lyrics highlights word-by-word and scrolls overflowing text without displacing Row 1.
func TestTaskbarLyricsDoubleLineAlternation(t *testing.T) {
	a, d := taskbarLyricsViewFixture()
	a.pl.SetLyrics(lyrics.Build("", "",
		"[1000,2000]短句(1000,2000)\n[4000,10000]我们一起走过漫长道路(4000,5000)直到音乐带我们走向远方(9000,5000)\n[16000,2000]下一段(16000,2000)",
	))
	var row1Layout, row2Early, row2Late taskbarLyricLayout
	tt := ui.NewTester(func(c *ui.Context) {
		content := d.content(a.pl.Position())
		layout := makeTaskbarLyricLayout(c, content, a.pl.Position(), d.dark, 42)
		switch a.pl.Position() {
		case 2 * time.Second:
			row1Layout = layout
		case 5 * time.Second:
			row2Early = layout
		case 12 * time.Second:
			row2Late = layout
		}
		d.view(c)
	}, 160, 42)
	a.pl.Seek(2 * time.Second)
	tt.Frame()
	if row1Layout.activeRow != 0 || row1Layout.secondaryProgressX != 0 || len(row1Layout.secondaryWordColors) != 2 {
		t.Fatalf("row 1 should keep row 2 waiting at start: %+v", row1Layout)
	}
	a.pl.Seek(5 * time.Second)
	tt.Frame()
	a.pl.Seek(12 * time.Second)
	tt.Frame()
	if row2Early.activeRow != 1 || row2Late.activeRow != 1 {
		t.Fatalf("row 2 did not become active in place: early=%d late=%d", row2Early.activeRow, row2Late.activeRow)
	}
	if row2Early.progress != 1 || row2Late.secondaryProgressX <= row2Early.secondaryProgressX {
		t.Fatalf("row 2 timed progress did not advance while row 1 stayed bright: row1Progress=%v earlyX=%v lateX=%v", row2Early.progress, row2Early.secondaryProgressX, row2Late.secondaryProgressX)
	}
	if taskbarLyricOffset(row2Late.secondaryWidth, 108, row2Late.secondaryProgressX) >= 0 {
		t.Fatalf("overflowing active row 2 did not scroll: width=%v progressX=%v", row2Late.secondaryWidth, row2Late.secondaryProgressX)
	}
	a.pl.Seek(17 * time.Second)
	if after := d.content(a.pl.Position()); after.text != "下一段" || after.preview != "" || after.activeRow != 0 {
		t.Fatalf("pair 2 did not replace pair 1 cleanly: %+v", after)
	}
}

// Right-aligned taskbars place the cover on the right with right-flushed short lines, and hiding the cover removes its button so lyrics span the full width.
func TestTaskbarLyricsRightAlignedAndCoverVisibility(t *testing.T) {
	a, d := taskbarLyricsViewFixture()
	a.pl.Cue([]qqmusic.Track{{ID: 7, Name: "测试歌曲", Duration: 60000, Album: qqmusic.Album{PicURL: "https://example.invalid/cover.jpg"}}})
	a.pl.SetLyrics(lyrics.Build("[00:01]我们一起听音乐", "", ""))
	a.pl.Seek(2 * time.Second)
	measure := func(xMin, xMax int) (hasCover bool, coverX, textMinX, textMaxX float32) {
		t.Helper()
		tt := ui.NewTester(d.view, 280, 42)
		tt.Frame()
		if r, ok := tt.Find("歌曲控制"); ok {
			hasCover, coverX = true, r.X
		}
		frame := tt.Image()
		textMinX = float32(xMax)
		for y := 0; y < 42; y++ {
			for x := xMin; x < xMax; x++ {
				if _, _, _, alpha := frame.At(x, y).RGBA(); alpha != 0 {
					if float32(x) < textMinX {
						textMinX = float32(x)
					}
					if float32(x) > textMaxX {
						textMaxX = float32(x)
					}
				}
			}
		}
		return hasCover, coverX, textMinX, textMaxX
	}

	d.placement.rightAligned = false
	hasCover, leftCoverX, leftMinX, leftMaxX := measure(48, 280)
	if !hasCover || leftCoverX > 10 || leftMinX < 48 || leftMaxX > 200 {
		t.Fatalf("unexpected left-aligned layout: cover=%t x=%v text=[%v,%v]", hasCover, leftCoverX, leftMinX, leftMaxX)
	}

	d.placement.rightAligned = true
	hasCover, rightCoverX, rightMinX, rightMaxX := measure(0, 232)
	if !hasCover || rightCoverX < 230 || rightMinX < 100 || rightMaxX < 200 {
		t.Fatalf("unexpected right-aligned layout: cover=%t x=%v text=[%v,%v]", hasCover, rightCoverX, rightMinX, rightMaxX)
	}

	a.settings.TaskbarLyricsHideCover = true
	hasCover, _, hiddenRightMinX, hiddenRightMaxX := measure(0, 280)
	if hasCover || hiddenRightMaxX <= rightMaxX+30 || hiddenRightMinX <= rightMinX {
		t.Fatalf("hiding cover on right-aligned taskbar did not expand lyrics to the right edge: cover=%t text=[%v,%v] prevMax=%v", hasCover, hiddenRightMinX, hiddenRightMaxX, rightMaxX)
	}

	d.placement.rightAligned = false
	hasCover, _, hiddenLeftMinX, _ := measure(0, 280)
	if hasCover || hiddenLeftMinX >= 20 {
		t.Fatalf("hiding cover on left-aligned taskbar did not reclaim left margin: cover=%t minX=%v", hasCover, hiddenLeftMinX)
	}
}

// Instrumentals used to be filtered into missing lyrics and never painted in the taskbar.
func TestTaskbarLyricsInstrumental(t *testing.T) {
	a, d := taskbarLyricsViewFixture()
	a.pl.SetInstrumentalLyrics()
	a.pl.Seek(0)
	content := d.content(0)
	if content.text != "纯音乐，请欣赏" || content.index != -1 || content.preview != "" || content.translation != "" || desktopLyricWake(content, 0) != 0 {
		t.Fatalf("instrumental lost its static presentation: %+v", content)
	}
	tt := ui.NewTester(d.view, 280, 42)
	tt.Frame()
	if !taskbarFrameHasInk(tt.Image()) {
		t.Fatal("instrumental status was not painted")
	}
	a.pl.SetLyrics(nil)
	tt.Frame()
	if a.pl.Instrumental() || taskbarFrameHasInk(tt.Image()) {
		t.Fatal("missing lyrics retained the previous instrumental status")
	}
}

// A new track without lyrics, the prelude, and a hidden/disabled host must erase the old song's pixels.
func TestTaskbarLyricsClearsUnavailableContent(t *testing.T) {
	for _, reason := range []string{"missing", "prelude", "hidden", "disabled", "no-track"} {
		t.Run(reason, func(t *testing.T) {
			a, d := taskbarLyricsViewFixture()
			tt := ui.NewTester(d.view, 280, 42)
			tt.Frame()
			if !taskbarFrameHasInk(tt.Image()) {
				t.Fatal("fixture did not paint lyrics")
			}
			switch reason {
			case "missing":
				a.pl.SetLyrics(nil)
			case "prelude":
				a.pl.Seek(0)
			case "hidden":
				d.visible = false
			case "disabled":
				a.settings.TaskbarLyrics = false
			case "no-track":
				d.a = newTestApp()
				d.a.settings.TaskbarLyrics = true
			}
			tt.Frame()
			if taskbarFrameHasInk(tt.Image()) {
				t.Fatalf("%s left old lyrics or an opaque background", reason)
			}
		})
	}
}

func taskbarFrameHasInk(frame image.Image) bool {
	for y := frame.Bounds().Min.Y; y < frame.Bounds().Max.Y; y++ {
		for x := frame.Bounds().Min.X; x < frame.Bounds().Max.X; x++ {
			if _, _, _, alpha := frame.At(x, y).RGBA(); alpha != 0 {
				return true
			}
		}
	}
	return false
}

// Long sentences remain inside their content range while both short rows begin beside the cover.
func TestTaskbarLyricScrollBounds(t *testing.T) {
	for _, tc := range []struct{ width, available, progress, want float32 }{
		{80, 100, 0, 0}, {80, 100, 80, 0}, {300, 100, 0, 0},
		{300, 100, 100, -45}, {300, 100, 300, -200}, {300, 100, 500, -200},
	} {
		if got := taskbarLyricOffset(tc.width, tc.available, tc.progress); got != tc.want {
			t.Errorf("offset(%v,%v,%v)=%v want=%v", tc.width, tc.available, tc.progress, got, tc.want)
		}
	}
}

// Export compact Chinese, translated, long and small-height taskbar frames for visual inspection.
func TestTaskbarLyricsFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip("set NM_FRAMES to inspect taskbar lyrics")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, scene := range []struct {
		name, lrc, translation, qrc string
		width, height               int
		dark                        bool
		position                    time.Duration
		rightAligned                bool
		hideCover                   bool
	}{
		{"chinese-dark", "", "", "[1000,9000]我们一起(1000,4000)听音乐(5000,5000)\n[12000,4000]让歌声陪伴远方(12000,4000)", 280, 42, true, 3 * time.Second, false, false},
		{"chinese-row2-dark", "", "", "[1000,4000]我们一起(1000,2000)听音乐(3000,2000)\n[6000,6000]让歌声(6000,3000)陪伴远方(9000,3000)", 280, 42, true, 7500 * time.Millisecond, false, false},
		{"chinese-row2-long-dark", "", "", "[1000,2000]我们(1000,2000)\n[4000,10000]我们一起走过漫长道路(4000,5000)直到音乐带我们走向远方(9000,5000)", 160, 42, true, 11 * time.Second, false, false},
		{"chinese-light", "", "", "[1000,9000]我们一起(1000,4000)听音乐(5000,5000)\n[12000,4000]让歌声陪伴远方(12000,4000)", 280, 42, false, 3 * time.Second, false, false},
		{"chinese-compact-light", "[00:01]我们一起听音乐\n[00:12]让歌声陪伴远方", "", "", 240, 28, false, 3 * time.Second, false, false},
		{"chinese-unrecognized-light", "[00:44.16]那个样子\n[00:48.39]友情 在积蓄", "", "", 280, 42, false, 45 * time.Second, false, false},
		{"instrumental-light", "[00:00:00]此歌曲为没有填词的纯音乐，请您欣赏", "", "", 280, 42, false, 0, false, false},
		{"translation-dark", "[00:01]君をのせて 空へ続く道", "[00:01]伴随着你，走向天空的路", "", 280, 42, true, 3 * time.Second, false, false},
		{"long-dark", "", "", "[1000,10000]我们一起走过漫长道路(1000,5000)直到音乐带我们走向远方(6000,5000)\n[20000,4000]让歌声陪伴远方(20000,4000)", 160, 42, true, 8 * time.Second, false, false},
		{"compact-light", "[00:01]Stay with me through the night", "[00:01]留在我身边", "", 240, 28, false, 3 * time.Second, false, false},
		{"right-dark", "", "", "[1000,9000]我们一起(1000,4000)听音乐(5000,5000)\n[12000,4000]让歌声陪伴远方(12000,4000)", 280, 42, true, 3 * time.Second, true, false},
		{"right-light", "", "", "[1000,9000]我们一起(1000,4000)听音乐(5000,5000)\n[12000,4000]让歌声陪伴远方(12000,4000)", 280, 42, false, 3 * time.Second, true, false},
		{"right-long-dark", "", "", "[1000,10000]我们一起走过漫长道路(1000,5000)直到音乐带我们走向远方(6000,5000)\n[20000,4000]让歌声陪伴远方(20000,4000)", 160, 42, true, 8 * time.Second, true, false},
		{"right-nocover-light", "", "", "[1000,9000]我们一起(1000,4000)听音乐(5000,5000)\n[12000,4000]让歌声陪伴远方(12000,4000)", 280, 42, false, 3 * time.Second, true, true},
		{"left-nocover-dark", "", "", "[1000,9000]我们一起(1000,4000)听音乐(5000,5000)\n[12000,4000]让歌声陪伴远方(12000,4000)", 280, 42, true, 3 * time.Second, false, true},
	} {
		t.Run(scene.name, func(t *testing.T) {
			a, d := taskbarLyricsViewFixture()
			d.dark = scene.dark
			d.placement.rightAligned = scene.rightAligned
			a.settings.TaskbarLyricsHideCover = scene.hideCover
			if scene.rightAligned && !scene.hideCover {
				a.pl.Cue([]qqmusic.Track{{ID: 7, Name: "测试歌曲", Duration: 60000, Album: qqmusic.Album{PicURL: "https://example.invalid/cover.jpg"}}})
			}
			a.pl.SetLyrics(lyrics.Build(scene.lrc, scene.translation, scene.qrc))
			if lyrics.IsInstrumental(scene.lrc) {
				a.pl.SetInstrumentalLyrics()
			}
			a.pl.Seek(scene.position)
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Box(c).Fill().Background(radixGray(scene.dark, 2)).Children(func() { d.view(c) })
			}, scene.width, scene.height)
			tt.Frame()
			f, err := os.Create(filepath.Join(dir, "taskbar-lyrics-"+scene.name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(f, tt.Image())
			closeErr := f.Close()
			if err != nil || closeErr != nil {
				t.Fatalf("save taskbar frame: %v / %v", err, closeErr)
			}
		})
	}
}
