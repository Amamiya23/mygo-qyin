package main

import (
	"testing"
	"time"
)

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
	if i := lyricIndex(lines, 33*time.Second); i != 1 {
		t.Errorf("index at 33s = %d", i)
	}
	if i := lyricIndex(lines, 0); i != -1 {
		t.Errorf("index at 0 = %d", i)
	}
}

func TestFrameSize(t *testing.T) {
	// MPEG-1 layer III, 320 kbps, 44.1 kHz, no padding.
	if n := frameSize([]byte{0xff, 0xfb, 0xe0, 0x44}); n != 1044 {
		t.Errorf("frame size %d", n)
	}
	if n := frameSize([]byte{0xff, 0x00, 0xe0, 0x44}); n != 0 {
		t.Errorf("no frame: %d", n)
	}
}

func TestClockAndPlays(t *testing.T) {
	if s := clock(185 * time.Second); s != "3:05" {
		t.Error(s)
	}
	if s := plays(123456789); s != "1.2亿" {
		t.Error(s)
	}
	if s := plays(56789); s != "6万" {
		t.Error(s)
	}
}

func TestPlayableFollowsPrivilege(t *testing.T) {
	tracks := []Track{{ID: 1, Fee: 1}, {ID: 2, Fee: 8}, {ID: 3, Fee: 8}, {ID: 4, Fee: 1}}
	if tracks[0].Playable() || !tracks[1].Playable() {
		t.Fatal("without privileges, VIP songs do not play and free ones do")
	}
	withPrivileges(tracks, []Privilege{
		{ID: 1, Pl: 320000},           // VIP song, the listener subscribes
		{ID: 2, Pl: 0},                // nothing to play
		{ID: 3, St: -200, Pl: 128000}, // taken down
	})
	want := []bool{true, false, false, false}
	for i, tr := range tracks {
		if tr.Playable() != want[i] {
			t.Errorf("track %d: playable %v", tr.ID, tr.Playable())
		}
	}
}
