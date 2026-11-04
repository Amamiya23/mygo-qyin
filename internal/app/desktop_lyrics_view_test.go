package app

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/ui"

	"lilt/internal/lyrics"
	"lilt/internal/qqmusic"
)

// Lines disappear at their actual ends and resume immediately after a seek across a gap.
func TestDesktopLyricTimeline(t *testing.T) {
	lines := lyrics.Build("", "[00:01]译文", "[1000,2000]一(1000,1000)二(2000,1000)\n[8000,2000]三(8000,2000)")
	for _, tc := range []struct {
		pos   time.Duration
		text  string
		index int
		next  time.Duration
	}{
		{0, "", -1, time.Second}, {time.Second, "一二", 0, 2 * time.Second}, {3 * time.Second, "", -1, 5 * time.Second},
		{8 * time.Second, "三", 1, 2 * time.Second}, {10 * time.Second, "", -1, 0}, {2 * time.Second, "一二", 0, time.Second},
	} {
		got := desktopLyricAt(lines, true, tc.pos)
		if got.text != tc.text || got.index != tc.index || got.next != tc.next {
			t.Errorf("position %s: text=%q index=%d next=%s", tc.pos, got.text, got.index, got.next)
		}
	}
	if got := desktopLyricAt(lines, false, time.Second); got.index >= 0 || got.text == lines[0].Text {
		t.Fatal("loading retained previous track lyrics")
	}
	if got := desktopLyricAt(nil, true, 0); got.index >= 0 {
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
	if got := desktopLyricAt([]lyrics.Line{line}, true, 2*time.Second); got.translation != "" {
		t.Fatal("duplicate translation was displayed")
	}
}

// Real word gaps wait for the next word while active words request bounded highlight updates.
func TestDesktopLyricWordScheduling(t *testing.T) {
	line := lyrics.Line{At: time.Second, End: 8 * time.Second, Text: "慢唱", Timed: true, Words: []lyrics.Word{{Text: "慢", Start: time.Second, End: 3 * time.Second}, {Text: "唱", Start: 5 * time.Second, End: 8 * time.Second}}}
	for _, tc := range []struct{ position, want time.Duration }{{2 * time.Second, 40 * time.Millisecond}, {4 * time.Second, time.Second}, {8 * time.Second, 0}} {
		content := desktopLyricAt([]lyrics.Line{line}, true, tc.position)
		if got := desktopLyricWake(content, tc.position); got != tc.want {
			t.Errorf("position=%s wake=%s want=%s", tc.position, got, tc.want)
		}
	}
	line.Timed = false
	if got := desktopLyricWake(desktopLyricAt([]lyrics.Line{line}, true, 2*time.Second), 2*time.Second); got != 6*time.Second {
		t.Fatalf("LRC used estimated word timing: %s", got)
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
		width                        int
		dark                         bool
		position                     time.Duration
		hover                        bool
	}{
		{name: "chinese-light", text: "[00:01]我们一起听音乐", width: 760, position: 2 * time.Second},
		{name: "chinese-hover-dark", text: "[00:01]我们一起听音乐", width: 760, dark: true, hover: true, position: 2 * time.Second},
		{name: "japanese-dark", text: "[00:01]君をのせて 空へ続く道", translation: "[00:01]伴随着你，走向天空的路", width: 760, dark: true, position: 2 * time.Second},
		{name: "long-light", text: "[00:01]我们一起走过漫长的道路，让音乐陪伴每一个普通的日子直到远方", width: 480, position: 2 * time.Second},
		{name: "word-hold-dark", text: "[00:01]慢唱", qrc: "[1000,7000]慢(1000,3000)唱(6000,2000)", width: 760, dark: true, position: 2 * time.Second},
	} {
		t.Run(scene.name, func(t *testing.T) {
			a, d := desktopLyricsViewFixture()
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
