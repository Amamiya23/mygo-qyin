package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

// The count on the comments' tab holds the last song's while the next
// one's loads, for it to roll from one to the other, and goes once the
// next song turns out to have none.
func TestCommentCountHoldsWhileLoading(t *testing.T) {
	a, tt, _ := coverApp(t)
	a.comments = commentState{track: 2, loaded: true, total: 128}
	tt.Frame()
	a.pl.Next()
	a.comments = commentState{track: 3, loading: true}
	tt.Frame()
	if _, ok := tt.Find("评论 128"); !ok {
		t.Fatal("the count went as the next song's loads")
	}
	a.comments.loading, a.comments.loaded, a.comments.total = false, true, 96
	tt.Frame()
	if _, ok := tt.Find("评论 96"); !ok {
		t.Error("the count did not change once loaded")
	}
	a.pl.Next()
	a.comments = commentState{track: 1, loaded: true}
	tt.Advance(time.Second)
	tt.Frame()
	if _, ok := tt.Find("评论"); !ok {
		t.Error("a song without comments shows a count")
	}
}

// Now Playing shows an episode's comments, the episode's and not its
// audio's, under a tab of their own beside what it is about, which stays
// chosen as the frames go.
func TestEpisodeComments(t *testing.T) {
	asked := make(chan string, 4)
	threadComments = func(thread string, offset, limit int) (*qqmusic.Comments, error) {
		asked <- thread
		return &qqmusic.Comments{}, nil
	}
	defer func() { threadComments = qqmusic.ThreadComments }()
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]qqmusic.Track{{ID: 13, Name: "第一期", ProgramID: 3, PodcastID: 5, Duration: 600000}})
	a.pl.SetLyrics(nil)
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Advance(time.Second)
	tt.Frame()
	if thread := wait(t, asked, "the comments"); thread != "A_DJ_1_3" {
		t.Fatalf("asked for the comments of %s", thread)
	}
	if a.npPane != paneProgram {
		t.Fatalf("opened on pane %d", a.npPane)
	}
	if err := tt.Click("评论"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	tt.Frame()
	if a.npPane != paneComments {
		t.Errorf("on pane %d", a.npPane)
	}
}
