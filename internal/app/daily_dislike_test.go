package app

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

func TestDailyDislikeMenu(t *testing.T) {
	old := addDislike
	defer func() { addDislike = old }()
	type request struct {
		kind qqmusic.DislikeType
		id   int64
	}
	requests := make(chan request, 2)
	addDislike = func(kind qqmusic.DislikeType, id int64) error {
		requests <- request{kind, id}
		return nil
	}
	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &qqmusic.Profile{UserID: 1}
	a.daily = []qqmusic.Track{{ID: 123, Name: "合唱", Artists: []qqmusic.Artist{{ID: 10, Name: "甲"}, {ID: 20, Name: "乙"}, {ID: 10, Name: "甲"}}}}
	a.route = route{page: pageDaily}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	for _, tc := range []struct {
		label string
		want  request
	}{
		{"不喜欢这首歌曲", request{qqmusic.DislikeSong, 123}},
		{"不喜欢歌手：乙", request{qqmusic.DislikeSinger, 20}},
	} {
		if err := tt.Click("更多操作：合唱"); err != nil {
			t.Fatal(err)
		}
		menu := tt.Menu()
		if !slices.Contains(menu, "不喜欢歌手：甲") || !slices.Contains(menu, "不喜欢歌手：乙") || !slices.Contains(menu, "不喜欢这首歌曲") {
			t.Fatalf("missing feedback options: %q", menu)
		}
		if err := tt.ChooseMenuItem(tc.label); err != nil {
			t.Fatal(err)
		}
		if got := wait(t, requests, "dislike request"); got != tc.want {
			t.Fatalf("request = %+v, want %+v", got, tc.want)
		}
		tt.Frame()
	}
}
