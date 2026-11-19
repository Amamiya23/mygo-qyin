package lyrics

import (
	"math"
	"testing"
	"time"
)

// Filtering QQ's instrumental marker must still let the player distinguish it from missing lyrics.
func TestIsInstrumental(t *testing.T) {
	for _, tc := range []struct {
		lrc  string
		want bool
	}{
		{"[00:00:00]此歌曲为没有填词的纯音乐，请您欣赏", true},
		{"", false},
		{"[00:01]那个样子", false},
	} {
		if got := IsInstrumental(tc.lrc); got != tc.want {
			t.Fatalf("instrumental(%q)=%t, want %t", tc.lrc, got, tc.want)
		}
		if tc.want && len(Build(tc.lrc, "", "")) != 0 {
			t.Fatal("instrumental marker became a sung lyric")
		}
	}
}

func TestParseLRC(t *testing.T) {
	lines := mergeLyrics("[00:01.000] 作词 : 某人\n[00:30.54]故事的小黄花\n[00:34.165][01:10.1]从出生那年就飘着\n[00:40.00]\n",
		"[00:30.54]The little yellow flower\n")
	if len(lines) != 4 {
		t.Fatalf("got %d lines: %+v", len(lines), lines)
	}
	if lines[1].At != 30540*time.Millisecond || lines[1].Translation != "The little yellow flower" {
		t.Errorf("line 1: %+v", lines[1])
	}
	if lines[3].At != 70100*time.Millisecond || lines[3].Text != "从出生那年就飘着" {
		t.Errorf("line 3: %+v", lines[3])
	}
	if i := Index(lines, 33*time.Second); i != 1 {
		t.Errorf("index at 33s = %d", i)
	}
	if i := Index(lines, 0); i != -1 {
		t.Errorf("index at 0 = %d", i)
	}
}

func TestParseYRC(t *testing.T) {
	yrc := `{"t":0,"c":[{"tx":"作词: "},{"tx":"某人"}]}
[13010,4780](13010,660,0)从(13670,670,0)不(14340,30,0) (14370,480,0)主
[17790,1770](17790,170,0)I(17960,450,0) (18410,290,0)love`
	lines := Build("", "[00:13.00]Never\n[00:17.80]我爱", yrc)
	if len(lines) != 2 || !lines[0].Timed {
		t.Fatalf("lines %+v", lines)
	}
	l := lines[0]
	if l.Text != "从不 主" || l.At != 13010*time.Millisecond || l.End != 17790*time.Millisecond {
		t.Errorf("line %q %v-%v", l.Text, l.At, l.End)
	}
	if len(l.Words) != 3 || l.Words[1].Text != "不 " || l.Words[2].Start != 14370*time.Millisecond {
		t.Errorf("words %+v", l.Words)
	}
	if l.Translation != "Never" || lines[1].Translation != "我爱" {
		t.Errorf("translations %q %q", l.Translation, lines[1].Translation)
	}
}

func TestSpreadWords(t *testing.T) {
	w := SpreadWords("我爱 you all", 0, 10*time.Second)
	got := []string{}
	for _, x := range w {
		got = append(got, x.Text)
	}
	if len(w) != 4 || got[0] != "我" || got[1] != "爱" || got[2] != "you " || got[3] != "all" {
		t.Fatalf("words %q", got)
	}
	if w[0].Start != 0 || w[3].End > 10*time.Second || w[1].Start != w[0].End {
		t.Errorf("times %+v", w)
	}
}

func TestQuiets(t *testing.T) {
	s := time.Second
	lines := []Line{
		{At: 0, End: 2 * s, Text: "作词 : 林夕"},
		{At: 2 * s, End: 4 * s, Text: "作曲：陈小霞"},
		{At: 20 * s, End: 25 * s, Text: "一"},
		{At: 26 * s, End: 30 * s, Text: "二"},
		{At: 45 * s, End: 50 * s, Text: "三"},
	}
	got := Quiets(lines, 60*s, 8*s)
	want := []Quiet{{0, 20 * s}, {30 * s, 45 * s}, {50 * s, 60 * s}}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("quiet %d: %v, want %v", i, got[i], want[i])
		}
	}
	if q := Quiets(nil, 60*s, 8*s); len(q) != 0 {
		t.Errorf("a song without lyrics: %v", q)
	}
}

func TestIsCredit(t *testing.T) {
	for text, want := range map[string]bool{
		"作词 : 林夕":            true,
		"作曲：陈小霞":             true,
		"编曲: 藤田淳平(Elements)": true,
		"Producer: 周杰伦":      true,
		"我说：你好":              false,
		"时间：是一把刀":            false,
		"悴んだ心 ふるえる眼差し":       false,
		"词：方文山":              true,
		"曲终人散：空余叹":           false,
	} {
		if IsCredit(text) != want {
			t.Errorf("%q: %v", text, !want)
		}
	}
}

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

// QQ can leave quotes in credit names unescaped inside LyricContent. Those
// quotes must not truncate the singing lines that follow the credits.
func TestBuildQRCEmbeddedQuotes(t *testing.T) {
	const content = `[0,100]测试歌曲(0,100)
[100,400]词：作者 "(100,100)alias"(200,100) 姓氏(300,200)
[1000,500]第一句(1000,200)歌词(1200,300)
[2000,600]保留 &quot;/&gt; 与 &amp;(2000,600)`
	for _, ending := range []string{`"/>`, "\" \r\n/>"} {
		t.Run(ending, func(t *testing.T) {
			qrc := `<QrcInfos><LyricInfo><Lyric_1 LyricType="1" LyricContent="` + content + ending + `
<Lyric_2 LyricType="2" LyricContent="[9000,500]其他歌词块(9000,500)"/>
</LyricInfo></QrcInfos>`
			lines := Build("[00:01.00]普通歌词备用", "[00:01.00]译文", qrc)
			if len(lines) != 4 {
				t.Fatalf("embedded quote truncated lyrics or swallowed another block: got %d lines", len(lines))
			}
			if lines[1].Text != `词：作者 "alias" 姓氏` || lines[2].Text != "第一句歌词" || lines[3].Text != `保留 "/> 与 &` {
				t.Fatalf("quotes or entities changed the lyric text: %+v", lines)
			}
			if !lines[2].Timed || lines[2].At != time.Second || lines[2].Translation != "译文" ||
				len(lines[2].Words) != 2 || lines[2].Words[1].Start != 1200*time.Millisecond || lines[2].Words[1].End != 1500*time.Millisecond {
				t.Fatalf("singing line lost word timing or translation: %+v", lines[2])
			}
		})
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
