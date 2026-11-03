package qqmusic

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// QQ cloud lyrics must decode with their altered S-boxes and word byte order.
func TestDecodeQRCCloud(t *testing.T) {
	encrypted, err := os.ReadFile("testdata/qrc_qingtian.hex")
	if err != nil {
		t.Fatal(err)
	}
	text, err := decodeQRC(strings.TrimSpace(string(encrypted)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "[29264,3446]故(29264,390)事(29654,392)") || !strings.Contains(text, "LyricContent=") {
		t.Fatalf("decoded lyric lost its absolute word times: %.120s", text)
	}
	t.Logf("decoded %d bytes", len(text))
}

// Empty optional fields stay empty; corrupt ciphertext cannot become valid lyrics.
func TestDecodeQRCInvalid(t *testing.T) {
	if text, err := decodeQRC(""); err != nil || text != "" {
		t.Fatalf("empty: %q %v", text, err)
	}
	for _, input := range []string{"not-hex", "00", "0000000000000000"} {
		if _, err := decodeQRC(input); err == nil {
			t.Errorf("accepted corrupt QRC %q", input)
		}
	}
}

// The same envelope carries LRC fallbacks and translations, without QRC timing.
func TestDecodeQRCLRCAndTranslation(t *testing.T) {
	for _, fixture := range []string{"qrc_lrc_only.hex", "qrc_kimi_trans.hex"} {
		t.Run(fixture, func(t *testing.T) {
			encrypted, err := os.ReadFile("testdata/" + fixture)
			if err != nil {
				t.Fatal(err)
			}
			text, err := decodeQRC(strings.TrimSpace(string(encrypted)))
			if err != nil {
				t.Fatal(err)
			}
			if !lyricTimeTag.MatchString(text) || strings.Contains(text, "LyricContent=") {
				t.Fatalf("not readable LRC: %.120s", text)
			}
			t.Logf("decoded %d bytes: %.100s", len(text), text)
		})
	}
}

// A Japanese cloud lyric must preserve the same absolute-time QRC format.
func TestDecodeQRCJapanese(t *testing.T) {
	encrypted, err := os.ReadFile("testdata/qrc_kimi.hex")
	if err != nil {
		t.Fatal(err)
	}
	text, err := decodeQRC(strings.TrimSpace(string(encrypted)))
	if err != nil {
		t.Fatal(err)
	}
	words := regexp.MustCompile(`\(\d+,\d+\)`).FindAllString(text, -1)
	if !strings.Contains(text, "LyricContent=") || !strings.Contains(text, "[ti:君をのせて]") || len(words) < 10 {
		t.Fatalf("decoded lyric lost its QRC envelope or word times: %.120s", text)
	}
	t.Logf("decoded %d bytes with %d words, offsets=%v", len(text), len(words), regexp.MustCompile(`\[offset:[^\]]*\]`).FindAllString(text, -1))
	// A changed block must fail the compressed stream's checksum rather than
	// passing a damaged lyric on to the parser.
	corrupt := []byte(strings.TrimSpace(string(encrypted)))
	if corrupt[len(corrupt)/2] == '0' {
		corrupt[len(corrupt)/2] = '1'
	} else {
		corrupt[len(corrupt)/2] = '0'
	}
	if _, err := decodeQRC(string(corrupt)); err == nil {
		t.Fatal("accepted a damaged cloud lyric")
	}
}
