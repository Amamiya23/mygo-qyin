package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"lilt/internal/lyrics"
)

// Choosing a style from the stage's menu sets its lyrics in it, and
// choosing 流光 goes back to the classic.
func TestStageStyleMenu(t *testing.T) {
	a, tt, _ := playingApp()
	a.nowPlaying, a.npPane = true, paneStage
	a.pl.Seek(2 * time.Second)
	tt.Frame()
	tt.Advance(time.Second) // the tabs enter after the cover
	for _, c := range []struct{ item, want string }{{"云阶", "partita"}, {"倾诉", "tilt"}, {"流光", ""}} {
		if err := tt.Click("律动效果"); err != nil {
			t.Fatal(err)
		}
		if err := tt.Click(c.item); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if a.settings.StageStyle != c.want {
			t.Errorf("chose %s: style %q, want %q", c.item, a.settings.StageStyle, c.want)
		}
		if a.stageStyleOpen {
			t.Fatal("choosing a style left the menu open")
		}
		if err := tt.Click("律动效果"); err != nil {
			t.Fatal(err)
		}
		selected, ok := tt.AccessByRole(ui.RoleMenuItemRadio, c.item)
		if !ok || !selected.Checked {
			t.Fatal("reopening the menu lost its selected style")
		}
		tt.Key(0, ui.KeyEscape)
	}
}

// 火花 in the stage's menu turns the sparks on, and again off, whatever
// the style.
func TestStageSparksMenu(t *testing.T) {
	a, tt, _ := playingApp()
	a.nowPlaying, a.npPane = true, paneStage
	a.settings.StageStyle = "tilt"
	tt.Frame()
	tt.Advance(time.Second)
	for _, want := range []bool{true, false} {
		if err := tt.Click("律动效果"); err != nil {
			t.Fatal(err)
		}
		if err := tt.Click("火花"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if a.settings.StageSparks != want || a.settings.StageStyle != "tilt" {
			t.Errorf("sparks %v in %q, want %v in tilt", a.settings.StageSparks, a.settings.StageStyle, want)
		}
		if a.stageStyleOpen {
			t.Fatal("toggling sparks left the menu open")
		}
	}
}

// Choosing menu items with Space and Enter changes the effect without triggering the player's Space shortcut.
func TestStageStyleMenuKeyboard(t *testing.T) {
	a, tt, _ := playingApp()
	a.nowPlaying, a.npPane = true, paneStage
	tt.Frame()
	tt.Advance(time.Second)
	playing := a.pl.Playing()
	if err := tt.Click("律动效果"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyHome)
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeySpace)
	tt.Frame()
	if a.settings.StageStyle != "partita" || a.stageStyleOpen || a.pl.Playing() != playing || !a.nowPlaying {
		t.Fatalf("Space did not choose the style locally: style=%q open=%v playing=%v", a.settings.StageStyle, a.stageStyleOpen, a.pl.Playing())
	}
	if !tt.Focused("律动效果") {
		t.Fatal("choosing a style did not restore focus to its trigger")
	}
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyEnd)
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if !a.settings.StageSparks || a.settings.StageStyle != "partita" || a.stageStyleOpen || a.pl.Playing() != playing {
		t.Fatal("keyboard spark selection changed the style or playback")
	}
}

// Escape, a second trigger press and an outside press dismiss only the effect menu, preserving the stage and playback.
func TestStageStyleMenuDismissal(t *testing.T) {
	for _, dismiss := range []string{"escape", "trigger", "outside"} {
		t.Run(dismiss, func(t *testing.T) {
			a, tt, _ := playingApp()
			a.nowPlaying, a.npPane = true, paneStage
			a.settings.StageStyle, a.settings.StageSparks = "tilt", true
			tt.Frame()
			tt.Advance(time.Second)
			playing := a.pl.Playing()
			if err := tt.Click("律动效果"); err != nil {
				t.Fatal(err)
			}
			if !a.stageStyleOpen {
				t.Fatal("the effect menu did not open")
			}
			switch dismiss {
			case "escape":
				tt.Key(0, ui.KeyEscape)
			case "trigger":
				if err := tt.Click("律动效果"); err != nil {
					t.Fatal(err)
				}
			case "outside":
				tt.ClickAt(10, 400)
			}
			tt.Frame()
			if a.stageStyleOpen || !a.nowPlaying || a.npPane != paneStage || a.pl.Playing() != playing || a.settings.StageStyle != "tilt" || !a.settings.StageSparks {
				t.Fatal("dismissing the effect menu changed the stage, effects or playback")
			}
		})
	}
}

func words(texts ...string) []lyrics.Word {
	out := make([]lyrics.Word, len(texts))
	for i, s := range texts {
		out[i] = lyrics.Word{Text: s, Start: time.Duration(i) * time.Second, End: time.Duration(i+1) * time.Second}
	}
	return out
}

// A line breaks at its spaces and punctuation, punctuation staying with
// the word before it and a contraction whole, and a long run of
// characters in pieces.
func TestStageUnits(t *testing.T) {
	for _, c := range []struct {
		words []lyrics.Word
		want  [][2]int
	}{
		{words("从", "不 ", "主", "动 ", "示", "弱"), [][2]int{{0, 2}, {2, 4}, {4, 6}}},
		{words("Hello", ", ", "It", "’", "s ", "me"), [][2]int{{0, 2}, {2, 5}, {5, 6}}},
		{words("I ", "never ", "show"), [][2]int{{0, 1}, {1, 2}, {2, 3}}},
		{words("曾", "经", "的", "那", "些", "发", "生", "过"), [][2]int{{0, 4}, {4, 8}}},
	} {
		got := stageUnits(c.words)
		if len(got) != len(c.want) {
			t.Errorf("%v: units %v, want %v", c.words, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%v: units %v, want %v", c.words, got, c.want)
				break
			}
		}
	}
}

// 倾诉 splits a line into parts that follow one another, none empty, and
// rather after punctuation than nearest the middle.
func TestTiltSplitsAtPunctuation(t *testing.T) {
	ws := words("夜", "空", "中", "最", "亮", "的", "星，", "能", "否", "听", "清")
	units := stageUnits(ws)
	for n := 1; n <= min(4, len(units)); n++ {
		parts := splitParts(ws, units, n)
		if len(parts) != n || parts[0].from != 0 || parts[n-1].to != len(ws) {
			t.Fatalf("%d parts: %+v", n, parts)
		}
		for i := 1; i < n; i++ {
			if parts[i].from != parts[i-1].to || parts[i].from >= parts[i].to {
				t.Fatalf("%d parts: %+v", n, parts)
			}
		}
	}
	if parts := splitParts(ws, units, 2); parts[0].to != 7 {
		t.Errorf("split at %d, not after 星，", parts[0].to)
	}
}
