package lyrics

import (
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	qrcContent = regexp.MustCompile(`(?s)\bLyricContent="([^"]*)"`)
	qrcWord    = regexp.MustCompile(`\((\d+),(\d+)\)`)
)

// ParseQRC reads QQ's word-timed lyrics, either their XML wrapper or its
// LyricContent. Each text fragment precedes its (absolute start, duration)
// in milliseconds; literal parentheses and timed spaces remain in the text.
func ParseQRC(qrc string) []Line {
	if m := qrcContent.FindStringSubmatch(qrc); m != nil {
		// Extract the attribute before unescaping it: escaped quotes belong
		// to the lyrics, and the real newlines separate their timed lines.
		qrc = html.UnescapeString(m[1])
	}
	ms := func(s string) time.Duration {
		n, _ := strconv.ParseInt(s, 10, 64)
		return time.Duration(n) * time.Millisecond
	}
	var lines []Line
	for _, raw := range strings.Split(qrc, "\n") {
		raw = strings.TrimSpace(raw)
		header := yrcLine.FindStringSubmatch(raw)
		if header == nil {
			continue
		}
		line := Line{At: ms(header[1]), Timed: true}
		line.End = line.At + ms(header[2])
		body := raw[len(header[0]):]
		at := 0
		for _, m := range qrcWord.FindAllStringSubmatchIndex(body, -1) {
			text := body[at:m[0]]
			at = m[1]
			if text == "" {
				continue // An empty timed unit carries no text to highlight.
			}
			start := ms(body[m[2]:m[3]])
			word := Word{Text: text, Start: start, End: start + ms(body[m[4]:m[5]])}
			line.Words = append(line.Words, word)
			line.Text += text
			line.End = max(line.End, word.End)
		}
		line.Text = strings.TrimSpace(line.Text)
		if line.Text != "" && strings.TrimSpace(body[at:]) == "" {
			lines = append(lines, line)
		}
	}
	sortLines(lines)
	return lines
}
