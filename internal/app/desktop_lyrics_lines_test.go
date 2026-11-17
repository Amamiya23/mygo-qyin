package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"lilt/internal/lyrics"
)

// Chinese double-line lyrics pair two sung lines, highlight rows in turn, and follow seeks in both directions.
func TestDesktopLyricChineseLineModes(t *testing.T) {
	lines := []lyrics.Line{
		{At: time.Second, End: 3 * time.Second, Text: "我们一起听音乐", Translation: "Listen with me", Timed: true,
			Words: []lyrics.Word{{Text: "我们一起听音乐", Start: time.Second, End: 3 * time.Second}}},
		{At: 8 * time.Second, End: 10 * time.Second, Text: "让音乐陪伴远方"},
		{At: 12 * time.Second, End: 14 * time.Second, Text: "我们再见"},
	}
	for _, doubleLine := range []bool{false, true} {
		for _, tc := range []struct {
			position                 time.Duration
			singleText               string
			pairText, pairPreview    string
			pairRow                  int
		}{
			{0, "", "", "", 0},
			{2 * time.Second, lines[0].Text, lines[0].Text, lines[1].Text, 0},
			{7 * time.Second, lines[0].Text, lines[0].Text, lines[1].Text, 0},
			{8 * time.Second, lines[1].Text, lines[0].Text, lines[1].Text, 1},
			{12 * time.Second, lines[2].Text, lines[2].Text, "", 0},
			{30 * time.Second, lines[2].Text, lines[2].Text, "", 0},
			{9 * time.Second, lines[1].Text, lines[0].Text, lines[1].Text, 1},
			{2 * time.Second, lines[0].Text, lines[0].Text, lines[1].Text, 0},
		} {
			wantText, wantPreview, wantRow := tc.singleText, "", 0
			if doubleLine {
				wantText, wantPreview, wantRow = tc.pairText, tc.pairPreview, tc.pairRow
			}
			got := desktopLyricAt(lines, true, tc.position, doubleLine)
			if got.text != wantText || got.preview != wantPreview || got.activeRow != wantRow || got.translation != "" {
				t.Fatalf("double=%t at=%s: current=%q preview=%q row=%d translation=%q", doubleLine, tc.position, got.text, got.preview, got.activeRow, got.translation)
			}
		}
		got := desktopLyricAt(lines, true, 2*time.Second, doubleLine)
		if !got.line.Timed || got.line.Words[0].Progress(2*time.Second) != 0.5 || desktopLyricWake(got, 2*time.Second) != 40*time.Millisecond {
			t.Fatalf("double=%t lost the current line's real word timing", doubleLine)
		}
		if got := desktopLyricAt(lines, false, 2*time.Second, doubleLine); got.preview != "" || got.index != -1 {
			t.Fatal("loading exposed a previous song's preview")
		}
		if got := desktopLyricAt(nil, true, 2*time.Second, doubleLine); got.preview != "" || got.index != -1 {
			t.Fatal("missing lyrics exposed a preview")
		}
	}
}

// Foreign and unknown-language songs keep the current original and translation, never a second original lyric.
func TestDesktopLyricForeignAndUnknownLineModes(t *testing.T) {
	for _, original := range []string{"Stay with me", "君をのせて", "사랑해", "山河歲月"} {
		for _, translation := range []string{"留在我身边", ""} {
			lines := []lyrics.Line{
				{At: time.Second, Text: original, Translation: translation},
				{At: 8 * time.Second, Text: "Next line", Translation: "下一句译文"},
			}
			for _, doubleLine := range []bool{false, true} {
				got := desktopLyricAt(lines, true, 2*time.Second, doubleLine)
				if got.text != original || got.translation != translation || got.preview != "" {
					t.Fatalf("original=%q double=%t: translation=%q preview=%q", original, doubleLine, got.translation, got.preview)
				}
			}
		}
	}
}

// Switching line modes retires the old preview, and seeking or changing songs clears it immediately.
func TestDesktopLyricLineModeChanges(t *testing.T) {
	lines := lyrics.Build("[00:01]我们一起听音乐\n[00:08]让声音陪伴远方\n[00:12]我们再见", "", "")
	start := time.Unix(0, 0)
	position := 2 * time.Second
	double := desktopLyricAt(lines, true, position, true)
	single := desktopLyricAt(lines, true, position, false)
	var state desktopLyricViewState
	state.step(1, "", 0, double, position, start, false, false)
	state.step(1, "", 0, single, position, start.Add(20*time.Millisecond), false, false)
	if state.current.preview != "" || state.previous.preview != lines[1].Text || state.changed.IsZero() {
		t.Fatal("switching to single-line did not replace and retire the preview")
	}
	now := start.Add(20*time.Millisecond + desktopLyricsFade)
	state.step(1, "", 0, single, position, now, false, false)
	if state.previous.text != "" || state.previous.preview != "" || !state.changed.IsZero() {
		t.Fatal("single-line mode retained the outgoing preview after its transition")
	}
	now = now.Add(20 * time.Millisecond)
	state.step(1, "", 0, double, position, now, false, false)
	position = 8 * time.Second
	state.step(1, "", 1, desktopLyricAt(lines, true, position, true), position, now.Add(20*time.Millisecond), false, false)
	if state.current.activeRow != 1 || state.current.preview != lines[1].Text || !state.changed.IsZero() {
		t.Fatal("seeking to second row of pair did not activate row 2 in place")
	}
	position = 12 * time.Second
	state.step(1, "", 2, desktopLyricAt(lines, true, position, true), position, now.Add(40*time.Millisecond), false, false)
	if state.current.text != lines[2].Text || state.current.preview != "" || state.previous.preview != "" || !state.changed.IsZero() {
		t.Fatal("seeking to next pair kept the earlier double-line preview")
	}
	state.step(2, "", 3, desktopLyricContent{index: 0, text: "Stay with me", translation: "留在我身边"}, 0, now.Add(60*time.Millisecond), false, false)
	if state.current.preview != "" || state.previous.preview != "" || !state.changed.IsZero() {
		t.Fatal("changing songs kept a Chinese preview or transition")
	}
}

// Double-line pairs skip leading credits, highlight row 2 words in place without a block jump, and fade only between pairs.
func TestDesktopLyricDoubleLineWordRowAlternation(t *testing.T) {
	lines := lyrics.Build(
		"",
		"",
		"[0000,500]作曲：测试(0,500)\n[1000,2000]我(1000,1000)们(2000,1000)\n[4000,2000]这(4000,1000)里(5000,1000)\n[7000,2000]他(7000,1000)们(8000,1000)",
	)
	if !lyrics.IsCredit(lines[0].Text) {
		t.Fatalf("expected leading line to be treated as credit: %q", lines[0].Text)
	}
	row1 := desktopLyricAt(lines, true, 1500*time.Millisecond, true)
	if row1.text != "我们" || row1.preview != "这里" || row1.activeRow != 0 {
		t.Fatalf("leading credit shifted pair alignment: %+v", row1)
	}
	if wake := desktopLyricWake(row1, 1500*time.Millisecond); wake != 40*time.Millisecond {
		t.Fatalf("row 1 wake=%s want=40ms", wake)
	}
	row2 := desktopLyricAt(lines, true, 4500*time.Millisecond, true)
	if row2.text != "我们" || row2.preview != "这里" || row2.activeRow != 1 {
		t.Fatalf("row 2 did not remain in pair 1: %+v", row2)
	}
	if wake := desktopLyricWake(row2, 4500*time.Millisecond); wake != 40*time.Millisecond {
		t.Fatalf("row 2 wake=%s want=40ms", wake)
	}
	var state desktopLyricViewState
	start := time.Unix(0, 0)
	state.step(1, "", 0, row1, 1500*time.Millisecond, start, true, false)
	state.step(1, "", 0, row2, 4500*time.Millisecond, start.Add(3*time.Second), true, false)
	if !state.changed.IsZero() || state.rowChanged.IsZero() {
		t.Fatalf("row 1 -> row 2 within same pair triggered block transition or missed row switch: changed=%v rowChanged=%v", state.changed, state.rowChanged)
	}
	var layout desktopLyricLayouts
	tt := ui.NewTester(func(c *ui.Context) {
		layout = desktopLyricsLayoutsAt(c, row2, 4500*time.Millisecond, 32, 400, radixPaper, 1)
	}, 440, 180)
	tt.Frame()
	if layout.wordColors[0] != desktopLyricWordColor(radixPaper, 1) || layout.wordColors[1] != desktopLyricWordColor(radixPaper, 1) {
		t.Fatalf("row 1 did not stay bright while row 2 sings: %+v", layout.wordColors)
	}
	if layout.previewWordColors[0] != desktopLyricWordColor(radixPaper, 0.5) || layout.previewWordColors[1] != desktopLyricWordColor(radixPaper, 0) {
		t.Fatalf("row 2 did not highlight word-by-word: %+v", layout.previewWordColors)
	}
	pair2 := desktopLyricAt(lines, true, 7500*time.Millisecond, true)
	if pair2.text != "他们" || pair2.preview != "" || pair2.activeRow != 0 {
		t.Fatalf("pair 2 mismatch: %+v", pair2)
	}
	state.step(1, "", 0, pair2, 7500*time.Millisecond, start.Add(6*time.Second), true, false)
	if state.changed.IsZero() || state.previous.activeRow != 1 || state.previous.text != "我们" || state.previous.preview != "这里" {
		t.Fatalf("pair 1 -> pair 2 did not fade outgoing finished pair: changed=%v prev=%+v", state.changed, state.previous)
	}
}

// Untimed LRC double-line lyrics smoothly reveal Row 2 over desktopLyricsRowFade and settle or cancel on ReduceMotion.
func TestDesktopLyricDoubleLineLrcRowReveal(t *testing.T) {
	lines := lyrics.Build("[00:01]我们一起听音乐\n[00:05]让声音陪伴远方", "", "")
	row1 := desktopLyricAt(lines, true, 2*time.Second, true)
	row2 := desktopLyricAt(lines, true, 6*time.Second, true)
	var state desktopLyricViewState
	started := false
	reveals := []float32{}
	tt := ui.NewTester(func(c *ui.Context) {
		if !started {
			state.step(1, "", 0, row1, 2*time.Second, c.Now().Add(-4*time.Second), true, false)
			state.step(1, "", 0, row2, 6*time.Second, c.Now(), true, false)
			started = true
		}
		reveals = append(reveals, state.rowReveal(c, row2.activeRow, row2.secondaryLine.Timed))
	}, 200, 100)
	tt.Frame()
	tt.Advance(desktopLyricsRowFade / 2)
	tt.Advance(desktopLyricsRowFade)
	if len(reveals) < 3 || reveals[0] >= 0.2 || reveals[len(reveals)-1] != 1 {
		t.Fatalf("LRC row 2 reveal did not ease from 0 to 1: %v", reveals)
	}
	state.step(1, "", 0, row2, 6500*time.Millisecond, time.Now(), true, true)
	if !state.rowChanged.IsZero() {
		t.Fatal("ReduceMotion did not cancel in-flight row reveal")
	}
}
