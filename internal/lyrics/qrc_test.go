package lyrics

import (
	"math"
	"testing"
	"time"
)

// QRC word times are absolute milliseconds; estimating or adding the line
// start again used to put the highlights at different times from the singing.
func TestBuildQRCWordTimes(t *testing.T) {
	const qrc = `<?xml version="1.0" encoding="utf-8"?>
<QrcInfos><LyricInfo><Lyric_1 LyricType="1" LyricContent="[ti:测试]
[offset:0]
[29264,3446]甲(29264,390)乙(29654,392)丙(30046,448)丁(30494,922)戊(31416,374)己(31790,504)
[32710,3537]下一句(32710,3537)
"/></LyricInfo></QrcInfos>`
	lines := Build("[00:30.00]旧逐行时间", "[00:29.26]第一句译文\n[00:32.71]第二句译文", qrc)
	if len(lines) != 2 {
		t.Fatalf("lines = %+v", lines)
	}
	l := lines[0]
	if !l.Timed || l.At != 29264*time.Millisecond || l.End != 32710*time.Millisecond || l.Text != "甲乙丙丁戊己" {
		t.Fatalf("QRC line = %+v", l)
	}
	if len(l.Words) != 6 || l.Words[3].Start != 30494*time.Millisecond || l.Words[3].End != 31416*time.Millisecond {
		t.Fatalf("QRC words = %+v", l.Words)
	}
	if l.Translation != "第一句译文" || lines[1].Translation != "第二句译文" {
		t.Fatalf("translations = %q, %q", l.Translation, lines[1].Translation)
	}
	if Index(lines, 29263*time.Millisecond) != -1 || Index(lines, 32710*time.Millisecond) != 1 {
		t.Fatal("line selection did not follow the QRC starts")
	}
}

// Timed spaces, literal parentheses and escaped XML text must survive QRC
// parsing without turning a silent empty token into a visible lyric line.
func TestParseQRCText(t *testing.T) {
	const content = `[5000,200]末(5000,200)
[0,1000](0,1000)
[1000,3500]I(1000,200) (1200,100)&amp;(1300,300) (1600,100)((1700,100)you(1800,1200))(3000,100)(3200,100)！(3300,200)`
	lines := ParseQRC(`<Lyric_1 LyricType="1" LyricContent="` + content + `"/>`)
	if len(lines) != 2 || lines[0].Text != "I & (you)！" || len(lines[0].Words) != 8 {
		t.Fatalf("lines = %+v", lines)
	}
	if word := lines[0].Words[1]; word.Text != " " || word.Start != 1200*time.Millisecond || word.End != 1300*time.Millisecond {
		t.Fatalf("timed space = %+v", word)
	}
	if word := lines[0].Words[5]; word.Text != "you" || word.Start != 1800*time.Millisecond || word.End != 3*time.Second {
		t.Fatalf("held word = %+v", word)
	}
	if lines[0].End != 4500*time.Millisecond {
		t.Fatal("a pause at the end of the line was shortened")
	}
	if plain := ParseQRC("[1000,400]原始(1000,400)"); len(plain) != 1 || plain[0].Text != "原始" {
		t.Fatalf("unwrapped QRC = %+v", plain)
	}
}

// Missing or malformed QRC falls back to line timing, with no claim that
// the estimated word times are real; instrumental placeholders stay hidden.
func TestBuildQRCFallback(t *testing.T) {
	for _, qrc := range []string{"", "not QRC", "[1000,500]没有字时间", "[1000,500]甲(1000,500)残缺尾部"} {
		lines := Build("[00:01.00]备用歌词\n[00:08.00]下一句", "", qrc)
		if len(lines) != 2 || lines[0].Timed || lines[0].At != time.Second || lines[0].Text != "备用歌词" {
			t.Fatalf("fallback for %q = %+v", qrc, lines)
		}
	}
	if lines := ParseQRC("[00:01.00]只有逐行歌词"); len(lines) != 0 {
		t.Fatalf("LRC was mistaken for QRC: %+v", lines)
	}
	if lines := Build("[00:00:00]此歌曲为没有填词的纯音乐，请您欣赏", "", ""); len(lines) != 0 {
		t.Fatalf("instrumental placeholder = %+v", lines)
	}
}

// Highlights use each word's actual duration, including short syllables,
// holds, pauses and repeated or backwards playback positions.
func TestWordProgress(t *testing.T) {
	for _, tc := range []struct {
		word Word
		pos  time.Duration
		want float32
	}{
		{Word{Start: time.Second, End: 3 * time.Second}, 500 * time.Millisecond, 0},
		{Word{Start: time.Second, End: 3 * time.Second}, time.Second, 0},
		{Word{Start: time.Second, End: 3 * time.Second}, 2 * time.Second, 0.5},
		{Word{Start: time.Second, End: 3 * time.Second}, 2 * time.Second, 0.5},
		{Word{Start: time.Second, End: 3 * time.Second}, 4 * time.Second, 1},
		{Word{Start: time.Second, End: 3 * time.Second}, 1500 * time.Millisecond, 0.25},
		{Word{Start: time.Second, End: 1020 * time.Millisecond}, 1010 * time.Millisecond, 0.5},
		{Word{Start: time.Second, End: time.Second}, 999 * time.Millisecond, 0},
		{Word{Start: time.Second, End: time.Second}, time.Second, 1},
	} {
		if got := tc.word.Progress(tc.pos); math.Abs(float64(got-tc.want)) > 1e-6 {
			t.Errorf("%+v at %v: got %v, want %v", tc.word, tc.pos, got, tc.want)
		}
	}
}
