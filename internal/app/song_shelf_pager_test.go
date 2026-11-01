package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// The last page overlaps earlier songs so nine-song shelves never end with a mostly empty page.
func TestSongShelfPagerTailFill(t *testing.T) {
	for _, tt := range []struct {
		name        string
		count, size int
		starts      []int
	}{
		{"nine_wide", 9, 6, []int{0, 3}},
		{"fifteen_wide", 15, 6, []int{0, 6, 9}},
		{"nine_narrow", 9, 4, []int{0, 4, 5}},
		{"nine_one_column", 9, 2, []int{0, 2, 4, 6, 7}},
		{"exact_page", 6, 6, []int{0}},
		{"short_shelf", 3, 6, []int{0}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var pager songShelfPager
			pager.layout(tt.count, tt.size)
			if pager.pages() != len(tt.starts) || pager.move(-1) {
				t.Fatalf("pages=%d; previous at the beginning must not move", pager.pages())
			}
			for i, start := range tt.starts {
				if got := pager.start(); got != start {
					t.Fatalf("page %d starts at %d, want %d", i, got, start)
				}
				wantMove := i+1 < len(tt.starts)
				if pager.move(1) != wantMove {
					t.Fatalf("page %d: next moved incorrectly", i)
				}
			}
			for i := len(tt.starts) - 2; i >= 0; i-- {
				if !pager.move(-1) || pager.start() != tt.starts[i] {
					t.Fatalf("previous did not restore page %d at %d", i, tt.starts[i])
				}
			}
			if pager.move(-1) || pager.start() != 0 {
				t.Fatal("the first page wrapped around")
			}
		})
	}
}

// Resizing a shelf keeps its first visible song within the new page instead of jumping back to the beginning.
func TestSongShelfPagerResize(t *testing.T) {
	for _, count := range []int{9, 15, 30} {
		for _, from := range []int{2, 4, 6} {
			for _, to := range []int{2, 4, 6} {
				for page := 0; page < (count+from-1)/from; page++ {
					var pager songShelfPager
					pager.layout(count, from)
					for i := 0; i < page; i++ {
						pager.move(1)
					}
					anchor := pager.start()
					pager.layout(count, to)
					start := pager.start()
					if start > anchor || anchor >= min(start+to, count) {
						t.Fatalf("count=%d resize %d→%d page=%d: old first song %d left new range [%d,%d)", count, from, to, page, anchor, start, min(start+to, count))
					}
					pager.layout(count, to)
					if pager.start() != start {
						t.Fatal("a normal rebuild reset the selected page")
					}
				}
			}
		}
	}
}

// A shelf that shrinks or becomes empty clamps its page and never exposes an invalid song range.
func TestSongShelfPagerShrinking(t *testing.T) {
	var pager songShelfPager
	pager.layout(30, 6)
	for pager.move(1) {
	}
	pager.layout(9, 6)
	if pager.start() < 0 || pager.start()+6 > 9 {
		t.Fatal("the page still points past the shortened shelf")
	}
	pager.layout(0, 6)
	if pager.start() != 0 || pager.move(1) || pager.move(-1) {
		t.Fatal("an empty shelf retained a navigable page")
	}
}

// A horizontal touchpad gesture changes one page while vertical scrolling and momentum never skip songs.
func TestSongShelfPagerScroll(t *testing.T) {
	var pager songShelfPager
	pager.layout(30, 6)
	at := time.Unix(1700000000, 0)
	if pager.scroll(ui.InputEvent{Kind: ui.InputScroll, DX: 10, DY: 80, Precise: true, Time: at}) || pager.start() != 0 {
		t.Fatal("vertical scrolling was consumed or paged the shelf")
	}
	if pager.scroll(ui.InputEvent{Kind: ui.InputScroll, DX: 80, Mods: ui.Cmd, Precise: true, Time: at}) || pager.start() != 0 {
		t.Fatal("a modified scrolling gesture paged the shelf")
	}
	for _, dx := range []float32{24, 24, 100, 100} {
		if !pager.scroll(ui.InputEvent{Kind: ui.InputScroll, DX: dx, Precise: true, Time: at}) {
			t.Fatal("the horizontal gesture was not handled")
		}
	}
	if pager.start() != 6 {
		t.Fatal("one horizontal gesture skipped multiple pages")
	}
	pager.scroll(ui.InputEvent{Kind: ui.InputScroll, DX: 400, Precise: true, Momentum: true, Time: at.Add(time.Second)})
	if pager.start() != 6 {
		t.Fatal("touchpad momentum advanced another page")
	}
	pager.scroll(ui.InputEvent{Kind: ui.InputScroll, DX: 60, Precise: true, Time: at.Add(2 * time.Second)})
	if pager.start() != 12 {
		t.Fatal("a new horizontal gesture did not advance")
	}
	pager.scroll(ui.InputEvent{Kind: ui.InputScroll, DX: -60, Precise: true, Time: at.Add(2 * time.Second)})
	if pager.start() != 6 {
		t.Fatal("reversing the gesture did not return to the previous page")
	}
}
