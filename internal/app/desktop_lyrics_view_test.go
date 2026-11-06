package app

import (
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/ui"

	"lilt/internal/lyrics"
	"lilt/internal/qqmusic"
)

// Instrumental gaps used to clear the current line; retain it until its successor starts.
func TestDesktopLyricTimeline(t *testing.T) {
	lines := lyrics.Build("", "[00:01]译文", "[1000,2000]一(1000,1000)二(2000,1000)\n[8000,2000]三(8000,2000)")
	for _, tc := range []struct {
		pos   time.Duration
		text  string
		index int
		next  time.Duration
	}{
		{0, "", -1, time.Second}, {time.Second, "一二", 0, 7 * time.Second}, {3 * time.Second, "一二", 0, 5 * time.Second},
		{8 * time.Second, "三", 1, 0}, {10 * time.Second, "三", 1, 0}, {2 * time.Second, "一二", 0, 6 * time.Second},
	} {
		got := desktopLyricAt(lines, true, tc.pos, false)
		if got.text != tc.text || got.index != tc.index || got.next != tc.next {
			t.Errorf("position %s: text=%q index=%d next=%s", tc.pos, got.text, got.index, got.next)
		}
	}
	if got := desktopLyricAt(lines, false, time.Second, false); got.index >= 0 || got.text == lines[0].Text {
		t.Fatal("loading retained previous track lyrics")
	}
	if got := desktopLyricAt(nil, true, 0, false); got.index >= 0 {
		t.Fatal("missing lyrics retained a singing line")
	}
}

// A Han-only Japanese line keeps its translation when another line establishes kana.
func TestDesktopLyricTranslationPolicy(t *testing.T) {
	for _, tc := range []struct {
		name  string
		lines []lyrics.Line
		want  bool
	}{
		{"Chinese with English", []lyrics.Line{{Text: "我们一起 say goodbye"}}, false},
		{"Japanese kana later", []lyrics.Line{{Text: "愛"}, {Text: "君をのせて"}}, true},
		{"Japanese shinjitai", []lyrics.Line{{Text: "学校 国体 来世 会場 没頭"}}, true},
		{"unknown traditional Han", []lyrics.Line{{Text: "山河歲月"}}, true},
		{"Korean", []lyrics.Line{{Text: "사랑해"}}, true},
		{"English", []lyrics.Line{{Text: "Let it be"}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := desktopLyricsTranslation(tc.lines); got != tc.want {
				t.Fatalf("show translation=%t, want %t", got, tc.want)
			}
		})
	}
	line := lyrics.Line{At: time.Second, End: 3 * time.Second, Text: "山河岁月", Translation: "山河，岁月。"}
	if got := desktopLyricAt([]lyrics.Line{line}, true, 2*time.Second, false); got.translation != "" {
		t.Fatal("duplicate translation was displayed")
	}
}

// Real word gaps wait for the next word while active words request bounded highlight updates.
func TestDesktopLyricWordScheduling(t *testing.T) {
	line := lyrics.Line{At: time.Second, End: 8 * time.Second, Text: "慢唱", Timed: true, Words: []lyrics.Word{{Text: "慢", Start: time.Second, End: 3 * time.Second}, {Text: "唱", Start: 5 * time.Second, End: 8 * time.Second}}}
	for _, tc := range []struct{ position, want time.Duration }{{2 * time.Second, 40 * time.Millisecond}, {4 * time.Second, time.Second}, {8 * time.Second, 0}} {
		content := desktopLyricAt([]lyrics.Line{line}, true, tc.position, false)
		if got := desktopLyricWake(content, tc.position); got != tc.want {
			t.Errorf("position=%s wake=%s want=%s", tc.position, got, tc.want)
		}
	}
	line.Timed = false
	if got := desktopLyricWake(desktopLyricAt([]lyrics.Line{line}, true, 2*time.Second, false), 2*time.Second); got != 0 {
		t.Fatalf("LRC used estimated word timing: %s", got)
	}
}

// Line.End used to clear both texts during accompaniment; keep them bright until the next line.
func TestDesktopLyricInterludeRetainsTranslation(t *testing.T) {
	lines := lyrics.Build("", "[00:01]留在我身边\n[00:10]下一句译文", "[1000,2000]Stay with me(1000,2000)\n[10000,2000]Next line(10000,2000)")
	content := desktopLyricAt(lines, true, 9*time.Second, false)
	if content.index != 0 || content.text != "Stay with me" || content.translation != "留在我身边" {
		t.Fatalf("instrumental gap cleared the line or translation: %+v", content)
	}
	if content.line.Words[0].Progress(9*time.Second) != 1 || desktopLyricWake(content, 9*time.Second) != time.Second {
		t.Fatal("finished words did not stay bright while waiting for the next line")
	}
	if next := desktopLyricAt(lines, true, 10*time.Second, false); next.index != 1 || next.translation != "下一句译文" {
		t.Fatalf("successor did not replace the retained line: %+v", next)
	}
}

// Locking removes the hover adjustment plate so only the lyrics remain over the desktop.
func TestDesktopLyricsLockedPreservesTransparency(t *testing.T) {
	_, d := desktopLyricsViewFixture()
	tt := ui.NewTester(d.view, 760, 180)
	tt.Move(200, 80)
	tt.Frame()
	frame := tt.Image()
	if _, _, _, a := frame.At(8, 100).RGBA(); a == 0 {
		t.Fatal("unlocked hover did not reveal the adjustment plate")
	}
	if _, _, _, a := frame.At(380, 20).RGBA(); a == 0 {
		t.Fatal("hover did not reveal the toolbar")
	}
	d.setLocked(true)
	tt.Frame()
	frame = tt.Image()
	if _, _, _, a := frame.At(8, 100).RGBA(); a != 0 {
		t.Fatal("locked lyrics kept the full-window adjustment plate")
	}
	if _, _, _, a := frame.At(380, 20).RGBA(); a != 0 {
		t.Fatal("locked lyrics kept the playback toolbar")
	}
}

// A backward or paused seek discards a pending crossfade instead of showing the old line.
func TestDesktopLyricSeekCancelsTransition(t *testing.T) {
	start := time.Unix(0, 0)
	first := desktopLyricContent{index: 0, text: "first"}
	second := desktopLyricContent{index: 1, text: "second"}
	var state desktopLyricViewState
	state.step(1, "", 0, first, time.Second, start, true, false)
	state.step(1, "", 0, second, 2*time.Second, start.Add(time.Second), true, false)
	if state.previous.text != "first" || state.changed.IsZero() {
		t.Fatal("ordinary line change did not start its transition")
	}
	state.step(1, "", 0, first, time.Second, start.Add(1100*time.Millisecond), true, false)
	if state.previous.text != "" || !state.changed.IsZero() || state.current.text != "first" {
		t.Fatal("backward seek kept an obsolete transition")
	}
	state.step(1, "", 0, second, 2*time.Second, start.Add(2*time.Second), true, false)
	state.step(1, "", 0, first, time.Second, start.Add(2100*time.Millisecond), false, false)
	if state.previous.text != "" || !state.changed.IsZero() {
		t.Fatal("paused seek kept an obsolete transition")
	}
	state.step(2, "", 0, second, 0, start.Add(3*time.Second), true, false)
	if state.previous.text != "" || !state.changed.IsZero() {
		t.Fatal("track switch retained old content")
	}
}

// A short forward seek across a line cancels crossfade even below the clock-drift threshold.
func TestDesktopLyricSmallSeekCancelsTransition(t *testing.T) {
	start := time.Unix(0, 0)
	var state desktopLyricViewState
	state.step(1, "", 0, desktopLyricContent{index: 0, text: "first"}, 1900*time.Millisecond, start, true, false)
	state.step(1, "", 1, desktopLyricContent{index: 1, text: "second"}, 2100*time.Millisecond, start.Add(20*time.Millisecond), true, false)
	if state.previous.text != "" || !state.changed.IsZero() || state.current.text != "second" {
		t.Fatal("short forward seek retained an obsolete line")
	}
}

// Reduced motion cancels an in-flight sentence transition instead of leaving a fading old line.
func TestDesktopLyricReducedMotionCancelsTransition(t *testing.T) {
	start := time.Unix(0, 0)
	first := desktopLyricContent{index: 0, text: "first"}
	second := desktopLyricContent{index: 1, text: "second"}
	var state desktopLyricViewState
	state.step(1, "", 0, first, time.Second, start, true, false)
	state.step(1, "", 0, second, 2*time.Second, start.Add(time.Second), true, false)
	if state.previous.text == "" {
		t.Fatal("fixture did not begin a sentence transition")
	}
	state.step(1, "", 0, second, 2050*time.Millisecond, start.Add(1050*time.Millisecond), true, true)
	if state.previous.text != "" || !state.changed.IsZero() || state.current.text != "second" {
		t.Fatal("reduced motion kept the old sentence or its transition")
	}
}

// Export the actual lyric layers across a sentence change for visual inspection of overlap and motion.
func TestDesktopLyricsTransitionFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip("set NM_FRAMES to inspect lyric transitions")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, scene := range []struct{ name, previous, current, translation string }{
		{"chinese", "风吹过遥远的山海", "我们一起听音乐", ""},
		{"shorter", "让每个平凡瞬间化作旋律，陪伴我们走过漫长的夜晚", "直到远方", ""},
		{"translation", "Stay with me through the night", "Let the music carry us home", "让音乐陪伴我们回家"},
	} {
		t.Run(scene.name, func(t *testing.T) {
			var state desktopLyricViewState
			state.previous = desktopLyricContent{text: scene.previous}
			started := false
			tt := ui.NewTester(func(c *ui.Context) {
				if !started {
					state.changed, started = c.Now(), true
				}
				previous := desktopLyricsLayouts(c, desktopLyricContent{text: scene.previous}, 0, 32, 432, radixPaper)
				current := desktopLyricsLayouts(c, desktopLyricContent{text: scene.current, translation: scene.translation}, 0, 32, 432, radixPaper)
				blend := state.blend(c)
				ui.Box(c).Fill().Background(radixSage(false, 2)).PaddingX(24).Children(func() {
					drawDesktopLyricLayers(c, current, previous, 140, blend)
				})
			}, 480, 168)
			tt.Frame()
			elapsed := time.Duration(0)
			for _, ms := range []int{0, 40, 80, 120, 180, 240, 480} {
				at := time.Duration(ms) * time.Millisecond
				tt.Advance(at - elapsed)
				elapsed = at
				f, err := os.Create(filepath.Join(dir, "desktop-lyrics-transition-"+scene.name+"-"+strconv.Itoa(ms)+"ms.png"))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(f, tt.Image())
				closeErr := f.Close()
				if err != nil || closeErr != nil {
					t.Fatalf("save transition frame: %v / %v", err, closeErr)
				}
			}
		})
	}
}

func desktopLyricsViewFixture() (*app, *desktopLyrics) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.DesktopLyrics = true
	a.pl.Cue([]qqmusic.Track{{ID: 7, Name: "测试歌曲", Duration: 60000}})
	a.pl.SetLyrics(lyrics.Build("[00:01]我们一起听音乐", "", ""))
	a.pl.Seek(2 * time.Second)
	d := &desktopLyrics{a: a, width: desktopLyricsWidth}
	a.desktopLyrics = d
	return a, d
}

// Toolbar clicks adjust typography, lock through an external recovery path, and close only lyrics.
func TestDesktopLyricsViewActions(t *testing.T) {
	a, d := desktopLyricsViewFixture()
	tt := ui.NewTester(d.view, 760, 180)
	tt.Move(200, 80)
	tt.Frame()
	before := d.fontSize()
	if err := tt.Click("增大桌面歌词字号"); err != nil {
		t.Fatal(err)
	}
	if d.fontSize() != before+2 {
		t.Fatal("font increase did not reach settings")
	}
	if err := tt.Click("减小桌面歌词字号"); err != nil {
		t.Fatal(err)
	}
	if d.fontSize() != before {
		t.Fatal("font decrease did not reach settings")
	}
	if err := tt.Click("锁定桌面歌词"); err != nil {
		t.Fatal(err)
	}
	if !a.settings.DesktopLyricsLocked {
		t.Fatal("lock did not reach the window owner")
	}
	d.setLocked(false)
	tt.Frame()
	tt.Move(200, 80)
	tt.Frame()
	if err := tt.Click("关闭桌面歌词"); err != nil {
		t.Fatal(err)
	}
	if a.settings.DesktopLyrics || a.pl.Current() == nil {
		t.Fatal("close changed playback or left lyrics enabled")
	}
}

// The floating player's pause button acts on the same player without opening the main window.
func TestDesktopLyricsViewPause(t *testing.T) {
	a, d := desktopLyricsViewFixture()
	out := audio.NewPlayer(&silence{})
	out.SetVolume(0)
	defer out.Close()
	a.pl.Attach(out)
	tt := ui.NewTester(d.view, 760, 180)
	tt.Move(200, 80)
	tt.Frame()
	if err := tt.Click("播放/暂停"); err != nil {
		t.Fatal(err)
	}
	if a.pl.Playing() || a.nowPlaying {
		t.Fatal("toolbar did not pause independently")
	}
}

// Dragging the width grip changes width and does not activate a playback control.
func TestDesktopLyricsViewResize(t *testing.T) {
	a, d := desktopLyricsViewFixture()
	tt := ui.NewTester(d.view, 760, 180)
	tt.Move(750, 100)
	tt.Frame()
	tt.Press(750, 100)
	tt.Move(710, 100)
	tt.Release(710, 100)
	if d.width >= desktopLyricsWidth {
		t.Fatalf("drag left did not narrow the window: %v", d.width)
	}
	if a.pl.Playing() {
		t.Fatal("resize activated playback")
	}
}

// Exported lyric frames are inspected visually instead of asserting on typography pixels.
func TestDesktopLyricsFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip("set NM_FRAMES to inspect desktop lyrics")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, scene := range []struct {
		name, text, translation, qrc string
		color                        string
		width                        int
		dark                         bool
		position                     time.Duration
		hover                        bool
		locked                       bool
		doubleLine                   bool
	}{
		{name: "chinese-light", text: "[00:01]我们一起听音乐", width: 760, position: 2 * time.Second},
		{name: "mint", text: "[00:01]我们一起听音乐\n[00:12]让声音陪伴远方", color: radixGreenDark[11], width: 760, dark: true, doubleLine: true, position: 2 * time.Second},
		{name: "blue", text: "[00:01]我们一起听音乐\n[00:12]让声音陪伴远方", color: "#c2e6ff", width: 760, dark: true, doubleLine: true, position: 2 * time.Second},
		{name: "iris", text: "[00:01]我们一起听音乐\n[00:12]让声音陪伴远方", color: "#e0dffe", width: 760, dark: true, doubleLine: true, position: 2 * time.Second},
		{name: "amber", text: "[00:01]我们一起听音乐\n[00:12]让声音陪伴远方", color: radixAmberDark[11], width: 760, dark: true, doubleLine: true, position: 2 * time.Second},
		{name: "pink", text: "[00:01]我们一起听音乐\n[00:12]让声音陪伴远方", color: "#fdd1ea", width: 760, dark: true, doubleLine: true, position: 2 * time.Second},
		{name: "custom-word", text: "[00:01]慢唱", qrc: "[1000,7000]慢(1000,3000)唱(6000,2000)", color: "#b1f1cbcc", width: 760, dark: true, position: 2 * time.Second},
		{name: "blue-translation", text: "[00:01]Stay with me", translation: "[00:01]留在我身边", color: "#c2e6ff", width: 760, dark: true, position: 2 * time.Second},
		{name: "chinese-hover-dark", text: "[00:01]我们一起听音乐", width: 760, dark: true, hover: true, position: 2 * time.Second},
		{name: "chinese-locked-dark", text: "[00:01]我们一起听音乐", width: 760, dark: true, hover: true, locked: true, position: 2 * time.Second},
		{name: "japanese-dark", text: "[00:01]君をのせて 空へ続く道", translation: "[00:01]伴随着你，走向天空的路", width: 760, dark: true, position: 2 * time.Second},
		{name: "chinese-double-dark", text: "[00:01]我们一起听音乐\n[00:12]让声音陪伴远方", width: 760, dark: true, doubleLine: true, position: 2 * time.Second},
		{name: "chinese-double-long", text: "[00:01]我们一起听音乐\n[00:12]让音乐陪伴每一个普通的日子，走过漫长的道路直到远方", width: 480, doubleLine: true, position: 2 * time.Second},
		{name: "japanese-double-dark", text: "[00:01]君をのせて 空へ続く道\n[00:12]光る地平線", translation: "[00:01]伴随着你，走向天空的路\n[00:12]闪耀的地平线", width: 760, dark: true, doubleLine: true, position: 2 * time.Second},
		{name: "long-light", text: "[00:01]我们一起走过漫长的道路，让音乐陪伴每一个普通的日子直到远方", width: 480, position: 2 * time.Second},
		{name: "word-hold-dark", text: "[00:01]慢唱", qrc: "[1000,7000]慢(1000,3000)唱(6000,2000)", width: 760, dark: true, position: 2 * time.Second},
		{name: "interlude-dark", text: "[00:01]Stay with me\n[00:12]Next line", translation: "[00:01]留在我身边\n[00:12]下一句译文", qrc: "[1000,2000]Stay with me(1000,2000)\n[12000,2000]Next line(12000,2000)", width: 760, dark: true, position: 6 * time.Second},
	} {
		t.Run(scene.name, func(t *testing.T) {
			a, d := desktopLyricsViewFixture()
			a.settings.DesktopLyricsLocked = scene.locked
			a.settings.DesktopLyricsDoubleLine = scene.doubleLine
			a.settings.DesktopLyricsColor = scene.color
			a.pl.SetLyrics(lyrics.Build(scene.text, scene.translation, scene.qrc))
			a.pl.Seek(scene.position)
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Box(c).Fill().Background(radixGray(scene.dark, 2)).Children(func() { d.view(c) })
			}, scene.width, 240)
			tt.Move(-10, -10)
			if scene.hover {
				tt.Move(200, 80)
			}
			tt.Advance(time.Second)
			file, err := os.Create(filepath.Join(dir, "desktop-lyrics-"+scene.name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(file, tt.Image()); err != nil {
				file.Close()
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
