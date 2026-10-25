package main

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// A LyricLine is a line of timed lyrics, with its translation if any.
type LyricLine struct {
	At          time.Duration
	Text        string
	Translation string
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

// mergeLyrics pairs lines with their translations by time, and drops
// empty lines between verses.
func mergeLyrics(lrc, translation string) []LyricLine {
	lines := parseLRC(lrc)
	trans := map[time.Duration]string{}
	for _, l := range parseLRC(translation) {
		if l.Text != "" {
			trans[l.At] = l.Text
		}
	}
	out := lines[:0]
	for _, l := range lines {
		if l.Text == "" {
			continue
		}
		l.Translation = trans[l.At]
		out = append(out, l)
	}
	return out
}

// lyricIndex returns the line sung at pos, or -1 before the first.
func lyricIndex(lines []LyricLine, pos time.Duration) int {
	i := sort.Search(len(lines), func(i int) bool { return lines[i].At > pos })
	return i - 1
}
