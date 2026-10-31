package app

import (
	"context"
	"errors"
	"testing"

	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

type suggestionRequest struct {
	query string
	ctx   context.Context
}

func cancelableSuggestions(t *testing.T) (chan suggestionRequest, chan string) {
	t.Helper()
	started, stopped := make(chan suggestionRequest, 8), make(chan string, 8)
	original := searchSuggest
	searchSuggest = func(ctx context.Context, q string) (qqmusic.Suggestions, error) {
		started <- suggestionRequest{q, ctx}
		<-ctx.Done()
		stopped <- q
		return qqmusic.Suggestions{}, ctx.Err()
	}
	t.Cleanup(func() { searchSuggest = original })
	return started, stopped
}

// Editing, dismissing, leaving or submitting the search cancels an already running suggestion request.
func TestSuggestionRequestCancellation(t *testing.T) {
	for _, action := range []string{"edit", "escape", "blur", "submit", "clear"} {
		t.Run(action, func(t *testing.T) {
			queries := fakeSearch(t)
			started, stopped := cancelableSuggestions(t)
			a := newTestApp()
			a.settings = defaultSettings()
			t.Cleanup(a.dismissSuggestions)
			tt := ui.NewTester(a.view, 1200, 800)
			tt.Frame()
			if err := tt.Click("搜索"); err != nil {
				t.Fatal(err)
			}
			tt.Type("晴")
			tt.Frame()
			req := wait(t, started, "suggestions")
			if req.query != "晴" {
				t.Fatalf("suggestions for %q", req.query)
			}
			switch action {
			case "edit":
				tt.Type("天")
			case "escape":
				tt.Key(0, ui.KeyEscape)
			case "blur":
				tt.ClickAt(1100, 650)
			case "submit":
				tt.Key(0, ui.KeyEnter)
			case "clear":
				tt.Key(ui.Cmd, ui.KeyA)
				tt.Key(0, ui.KeyBackspace)
			}
			tt.Frame()
			if action == "submit" {
				if got := wait(t, queries, "submitted search"); got.query != "晴" {
					t.Fatalf("searched %q", got.query)
				}
			}
			if q := wait(t, stopped, "canceled suggestions"); q != "晴" {
				t.Fatalf("canceled %q", q)
			}
			select {
			case <-req.ctx.Done():
			default:
				t.Fatal("obsolete suggestion context remains active")
			}
		})
	}
}

// Returning to the same word must not let an older success or failure overwrite its new request.
func TestSuggestionReplyAfterQueryReentered(t *testing.T) {
	original := searchSuggest
	searchSuggest = func(context.Context, string) (qqmusic.Suggestions, error) { return qqmusic.Suggestions{}, nil }
	t.Cleanup(func() { searchSuggest = original })
	a := newTestApp()
	t.Cleanup(a.dismissSuggestions)
	a.suggest("晴天")
	old := a.suggestions.gen
	a.suggest("春日影")
	a.suggest("晴天")
	current := a.suggestions.gen
	if a.suggestions.timer != nil {
		a.suggestions.timer.Stop()
	}
	stale := qqmusic.Suggestions{Songs: []qqmusic.Track{{MID: "old", Name: "old"}}}
	a.finishSuggestions(old, "晴天", stale, nil)
	a.finishSuggestions(old, "晴天", qqmusic.Suggestions{}, errors.New("old failure"))
	if !a.suggestions.loading || a.suggestions.failed || len(a.suggestions.items) != 0 {
		t.Fatal("obsolete reply changed the reentered query")
	}
	fresh := qqmusic.Suggestions{Songs: []qqmusic.Track{{MID: "new", Name: "new"}}}
	a.finishSuggestions(current, "晴天", fresh, nil)
	if a.suggestions.loading || a.suggestions.failed || len(a.suggestions.items) != 1 || a.suggestions.items[0].name != "new" {
		t.Fatal("the current reply was not applied")
	}
}

// Sparse smartbox songs without numeric IDs still select the right candidate instead of sharing song:0.
func TestSuggestionMIDIdentity(t *testing.T) {
	queries := fakeSearch(t)
	original := searchSuggest
	searchSuggest = func(context.Context, string) (qqmusic.Suggestions, error) { return qqmusic.Suggestions{}, nil }
	t.Cleanup(func() { searchSuggest = original })
	a := newTestApp()
	a.settings = defaultSettings()
	t.Cleanup(a.dismissSuggestions)
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("搜索"); err != nil {
		t.Fatal(err)
	}
	tt.Type("晴")
	tt.Frame()
	a.suggestions.loading = false
	a.suggestions.items = suggestionList(qqmusic.Suggestions{Songs: []qqmusic.Track{{MID: "first", Name: "晴天"}, {MID: "second", Name: "春日影"}}})
	if len(a.suggestions.items) != 2 || a.suggestions.items[0].key == a.suggestions.items[1].key {
		t.Fatal("suggestions share an identity")
	}
	tt.Frame()
	if err := tt.Click("单曲：春日影"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := wait(t, queries, "chosen song search"); got.query != "春日影" {
		t.Fatalf("searched %q", got.query)
	}
}
