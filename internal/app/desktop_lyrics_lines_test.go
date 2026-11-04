package app

import (
	"testing"
	"time"

	"lilt/internal/lyrics"
)

// Chinese double-line lyrics preview the successor, retain it through gaps, and follow seeks in both directions.
func TestDesktopLyricChineseLineModes(t *testing.T) {
	lines := []lyrics.Line{
		{At: time.Second, End: 3 * time.Second, Text: "我们一起听音乐", Translation: "Listen with me", Timed: true,
			Words: []lyrics.Word{{Text: "我们一起听音乐", Start: time.Second, End: 3 * time.Second}}},
		{At: 8 * time.Second, End: 10 * time.Second, Text: "让音乐陪伴远方"},
		{At: 12 * time.Second, End: 14 * time.Second, Text: "我们再见"},
	}
	for _, doubleLine := range []bool{false, true} {
		for _, tc := range []struct {
			position         time.Duration
			current, preview string
		}{
			{0, "", ""},
			{2 * time.Second, lines[0].Text, lines[1].Text},
			{7 * time.Second, lines[0].Text, lines[1].Text},
			{8 * time.Second, lines[1].Text, lines[2].Text},
			{12 * time.Second, lines[2].Text, ""},
			{30 * time.Second, lines[2].Text, ""},
			{9 * time.Second, lines[1].Text, lines[2].Text},
			{2 * time.Second, lines[0].Text, lines[1].Text},
		} {
			wantPreview := ""
			if doubleLine {
				wantPreview = tc.preview
			}
			got := desktopLyricAt(lines, true, tc.position, doubleLine)
			if got.text != tc.current || got.preview != wantPreview || got.translation != "" {
				t.Fatalf("double=%t at=%s: current=%q preview=%q translation=%q", doubleLine, tc.position, got.text, got.preview, got.translation)
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
	if state.current.preview != lines[2].Text || state.previous.preview != "" || !state.changed.IsZero() {
		t.Fatal("seeking kept the earlier double-line preview")
	}
	state.step(2, "", 2, desktopLyricContent{index: 0, text: "Stay with me", translation: "留在我身边"}, 0, now.Add(40*time.Millisecond), false, false)
	if state.current.preview != "" || state.previous.preview != "" || !state.changed.IsZero() {
		t.Fatal("changing songs kept a Chinese preview or transition")
	}
}
