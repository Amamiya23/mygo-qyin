package main

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// A LyricLine is a line of timed lyrics, with its translation if any, and
// when each of its words is sung.
type LyricLine struct {
	At          time.Duration
	End         time.Duration
	Text        string
	Translation string
	Words       []LyricWord
	// Timed says the words carry their own times, as NetEase's word-timed
	// lyrics give them; else they share the line's time evenly.
	Timed bool
}

// A LyricWord is a word of a line, or a character of Chinese or Japanese,
// with when it is sung.
type LyricWord struct {
	Text       string
	Start, End time.Duration
}

var lrcTime = regexp.MustCompile(`\[(\d+):(\d+)(?:[.:](\d+))?\]`)

// parseLRC reads lyrics in the LRC format, as NetEase serves them: a line
// may carry several times, and credits come as lines of their own.
func parseLRC(lrc string) []LyricLine {
	var out []LyricLine
	for _, raw := range strings.Split(lrc, "\n") {
		times := lrcTime.FindAllStringSubmatchIndex(raw, -1)
		if len(times) == 0 {
			continue
		}
		text := strings.TrimSpace(raw[times[len(times)-1][1]:])
		for _, m := range times {
			min, _ := strconv.Atoi(raw[m[2]:m[3]])
			sec, _ := strconv.Atoi(raw[m[4]:m[5]])
			var frac time.Duration
			if m[6] >= 0 {
				f := raw[m[6]:m[7]]
				n, _ := strconv.Atoi(f)
				frac = time.Duration(n) * time.Second
				for range len(f) {
					frac /= 10
				}
			}
			at := time.Duration(min)*time.Minute + time.Duration(sec)*time.Second + frac
			out = append(out, LyricLine{At: at, Text: text})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At < out[j].At })
	return out
}

var (
	yrcLine = regexp.MustCompile(`^\[(\d+),(\d+)\]`)
	yrcWord = regexp.MustCompile(`\((\d+),(\d+),\d+\)([^(]*)`)
)

// parseYRC reads NetEase's word-timed lyrics: a line is
// "[start,length](start,length,0)word(start,length,0)word…", in
// milliseconds. Credits come as lines of JSON, which it leaves out.
func parseYRC(yrc string) []LyricLine {
	ms := func(s string) time.Duration {
		n, _ := strconv.Atoi(s)
		return time.Duration(n) * time.Millisecond
	}
	var out []LyricLine
	for _, raw := range strings.Split(yrc, "\n") {
		m := yrcLine.FindStringSubmatch(raw)
		if m == nil {
			continue
		}
		l := LyricLine{At: ms(m[1]), Timed: true}
		l.End = l.At + ms(m[2])
		for _, w := range yrcWord.FindAllStringSubmatch(raw[len(m[0]):], -1) {
			start := ms(w[1])
			word := LyricWord{Text: w[3], Start: start, End: start + ms(w[2])}
			if strings.TrimSpace(word.Text) == "" {
				// A space between words: it belongs to the word before.
				if n := len(l.Words); n > 0 {
					l.Words[n-1].Text += " "
				}
				continue
			}
			l.Words = append(l.Words, word)
		}
		for _, w := range l.Words {
			l.Text += w.Text
		}
		l.Text = strings.TrimSpace(l.Text)
		if l.Text != "" {
			out = append(out, l)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At < out[j].At })
	return out
}

// buildLyrics makes the lines to show from what NetEase serves: the
// word-timed lyrics where there are some, else the timed lines, with the
// translation of each.
func buildLyrics(lrc, translation, yrc string) []LyricLine {
	var lines []LyricLine
	if yrc != "" {
		lines = parseYRC(yrc)
	}
	if len(lines) == 0 {
		for _, l := range parseLRC(lrc) {
			if l.Text != "" {
				lines = append(lines, l)
			}
		}
		for i := range lines {
			end := lines[i].At + 6*time.Second
			if i+1 < len(lines) {
				end = min(end, lines[i+1].At)
			}
			lines[i].End = end
			lines[i].Words = spreadWords(lines[i].Text, lines[i].At, end)
		}
	}
	// Translations go to the line nearest their time: word-timed lines
	// start a little off the timed lines the translation follows.
	for _, t := range parseLRC(translation) {
		if t.Text == "" {
			continue
		}
		best, bestGap := -1, 1500*time.Millisecond
		for i := range lines {
			gap := lines[i].At - t.At
			if gap < 0 {
				gap = -gap
			}
			if gap < bestGap {
				best, bestGap = i, gap
			}
		}
		if best >= 0 && lines[best].Translation == "" {
			lines[best].Translation = t.Text
		}
	}
	return lines
}

// mergeLyrics pairs timed lines with their translations, without words'
// own times.
func mergeLyrics(lrc, translation string) []LyricLine {
	return buildLyrics(lrc, translation, "")
}

// spreadWords splits text into words, each Chinese, Japanese or Korean
// character one of them, and shares the time from start to end among them
// by their lengths, for lines whose words have no times of their own.
func spreadWords(text string, start, end time.Duration) []LyricWord {
	var words []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			words = append(words, cur.String())
			cur.Reset()
		}
	}
	for _, r := range text {
		switch {
		case unicode.IsSpace(r):
			if cur.Len() > 0 {
				cur.WriteRune(' ')
				flush()
			}
		case isCJK(r):
			flush()
			words = append(words, string(r))
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	if len(words) == 0 {
		return nil
	}
	total := 0
	for _, w := range words {
		total += utf8.RuneCountInString(strings.TrimSpace(w))
	}
	// Singing takes most of the line, not the pause before the next.
	span := time.Duration(float64(end-start) * 0.85)
	out := make([]LyricWord, len(words))
	at := start
	for i, w := range words {
		d := span * time.Duration(utf8.RuneCountInString(strings.TrimSpace(w))) / time.Duration(max(total, 1))
		out[i] = LyricWord{Text: w, Start: at, End: at + d}
		at += d
	}
	return out
}

func isCJK(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
}

// lyricIndex returns the line sung at pos, or -1 before the first.
func lyricIndex(lines []LyricLine, pos time.Duration) int {
	i := sort.Search(len(lines), func(i int) bool { return lines[i].At > pos })
	return i - 1
}
