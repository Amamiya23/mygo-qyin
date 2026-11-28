package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"lilt/internal/lyrics"
	"lilt/internal/qqmusic"
)

// quietApp is Now Playing a song sung from 20 s to 30 s, quiet before and
// after, with two hot comments.
func quietApp(pos time.Duration) (*app, qqmusic.Track, []lyrics.Line) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.HotComments = true
	tr := qqmusic.Track{ID: 9, Name: "晴天", Duration: 60000}
	lines := []lyrics.Line{
		{At: 0, End: 2 * time.Second, Text: "作词：周杰伦"},
		{At: 20 * time.Second, End: 25 * time.Second, Text: "故事的小黄花"},
		{At: 25 * time.Second, End: 30 * time.Second, Text: "从出生那年就飘着"},
	}
	a.pl.Cue([]qqmusic.Track{tr})
	a.pl.SetLyrics(lines)
	a.pl.Hold(pos)
	hot := []qqmusic.Comment{
		{ID: 1, Content: "刮风这天，我试过握着你手", LikedCount: 52000},
		{ID: 2, Content: "从前从前，有个人爱你很久", LikedCount: 31000},
	}
	hot[0].User.Nickname, hot[1].User.Nickname = "听众甲", "听众乙"
	a.comments = commentState{track: tr.ID, loaded: true, hot: hot}
	return a, tr, lines
}

func TestQuietComment(t *testing.T) {
	a, tr, lines := quietApp(0)
	s := time.Second
	// The intro, 1.2 s in to 1 s before the singing, shows two comments
	// for 8.9 s each; the outro, three for 9.27 s, going on from them.
	for _, c := range []struct {
		pos  time.Duration
		want int64
		next time.Duration
	}{
		{0, 0, 1200 * time.Millisecond},
		{5 * s, 1, 5100 * time.Millisecond},
		{12 * s, 2, 7 * s},
		{22 * s, 0, 9200 * time.Millisecond},
		{35 * s, 1, 5467 * time.Millisecond},
		{45 * s, 2, 4733 * time.Millisecond},
		{59500 * time.Millisecond, 0, 0},
	} {
		cm, next := a.quietComment(&tr, lines, true, c.pos)
		var id int64
		if cm != nil {
			id = cm.ID
		}
		if d := next - c.next; id != c.want || d > time.Millisecond || d < -time.Millisecond {
			t.Errorf("at %v: comment %d, next in %v; want %d, %v", c.pos, id, next, c.want, c.next)
		}
	}
	// Turned off, nothing shows.
	a.settings.HotComments = false
	if cm, _ := a.quietComment(&tr, lines, true, 5*s); cm != nil {
		t.Errorf("showed %q turned off", cm.Content)
	}
	a.settings.HotComments = true
	// No hot comments, nothing shows.
	a.comments.hot = nil
	if cm, _ := a.quietComment(&tr, lines, true, 5*s); cm != nil {
		t.Errorf("showed %q without hot comments", cm.Content)
	}
}

// In the quiet, the lyrics pane and the stage show the hot comment, which
// opens the comments.
func TestQuietCard(t *testing.T) {
	for _, pane := range []int{paneLyrics, paneStage} {
		a, _, _ := quietApp(5 * time.Second)
		a.nowPlaying, a.npPane = true, pane
		tt := ui.NewTester(a.view, 1000, 640)
		tt.SetPreferences(ui.Preferences{ReduceMotion: true})
		tt.Frame()
		tt.Advance(time.Second)
		tt.Frame()
		if !tt.HasText("刮风这天，我试过握着你手") || !tt.HasText("热评") {
			t.Fatalf("pane %d: no hot comment in %q", pane, tt.Texts())
		}
		if err := tt.Click("查看评论"); err != nil {
			t.Fatal(err)
		}
		if a.npPane != paneComments {
			t.Errorf("pane %d: the comment did not open the comments", pane)
		}
	}
}

func TestCommentTime(t *testing.T) {
	now := time.Date(2026, 10, 25, 22, 0, 0, 0, time.Local)
	for _, c := range []struct {
		t    time.Time
		want string
	}{
		{now.Add(-20 * time.Second), "刚刚"},
		{now.Add(-42 * time.Minute), "42分钟前"},
		{now.Add(-5 * time.Hour), "17:00"},
		{now.Add(-23 * time.Hour), "昨天 23:00"},
		{time.Date(2026, 3, 8, 9, 0, 0, 0, time.Local), "3月8日"},
		{time.Date(2021, 4, 10, 9, 0, 0, 0, time.Local), "2021年4月10日"},
	} {
		if got := commentTime(c.t, now); got != c.want {
			t.Errorf("%v: %q, want %q", c.t, got, c.want)
		}
	}
}

// Leaving QQ's personal radio from Now Playing keeps its current song.
func TestExitQQRadio(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &qqmusic.Profile{UserID: 1, Nickname: "听众"}
	a.pl.Cue([]qqmusic.Track{{ID: 1, Name: "晚风"}, {ID: 2, Name: "推荐的歌"}})
	a.pl.AttachRadio(fmRadio(qqmusic.FMDefault), func(got func([]qqmusic.Track)) {})
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Advance(time.Second)
	tt.Frame()
	if err := tt.Click("退出电台"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.pl.Radio() != "" || a.pl.Current().ID != 1 || len(a.pl.Queue()) != 1 {
		t.Fatalf("radio %q, current %v, queue %d", a.pl.Radio(), a.pl.Current(), len(a.pl.Queue()))
	}
}

// Instrumental tracks must recompute quiet intervals once their empty lyrics finish loading.
func TestQuietCommentInstrumentalAfterLyricsLoad(t *testing.T) {
	a, tr, _ := quietApp(5 * time.Second)
	if cm, _ := a.quietComment(&tr, nil, false, 5*time.Second); cm != nil {
		t.Fatalf("showed hot comment before lyrics loaded: %+v", cm)
	}
	cm, _ := a.quietComment(&tr, nil, true, 5*time.Second)
	if cm == nil || cm.ID != 1 {
		t.Fatalf("instrumental track missed hot comment after lyrics loaded: %+v", cm)
	}
}
